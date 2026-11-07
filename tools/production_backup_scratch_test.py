"""Offline dedicated plaintext scratch tests; never connects to SSH."""
import ast
import importlib.util
import io
import json
import pathlib
import tempfile
import unittest
from unittest import mock

ROOT = pathlib.Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location('scratch_backup', ROOT / 'tools/production_backup.py')
backup = importlib.util.module_from_spec(spec)
spec.loader.exec_module(backup)


class ScratchTests(unittest.TestCase):
    def context(self):
        temporary = tempfile.TemporaryDirectory(prefix='puty-backup-scratch-test-')
        self.addCleanup(temporary.cleanup)
        return pathlib.Path(temporary.name).resolve()

    def archive(self, root):
        payload = b'SYNTHETIC PRIVATE BACKUP PAYLOAD' * 1000
        output = io.BytesIO()
        backup.encrypt_stream(io.BytesIO(payload), output, b'k' * 32, chunk_bytes=1024)
        path = root / 'synthetic.putybak'
        path.write_bytes(output.getvalue())
        return path, payload

    def test_empty_explicit_directory_is_protected_marked_and_reusable(self):
        root = self.context()
        scratch = root / 'explicit-scratch'
        self.assertEqual(backup.owned_scratch_directory(scratch), scratch)
        marker = scratch / backup.SCRATCH_MARKER
        self.assertEqual(json.loads(marker.read_bytes())['purpose'], 'temporary-authenticated-backup-verification-only')
        backup.assert_scratch_protected(scratch)
        backup.assert_scratch_protected(marker)
        self.assertEqual(backup.owned_scratch_directory(scratch), scratch)

    def test_real_authenticated_decryption_uses_explicit_directory_then_cleans_plaintext(self):
        root = self.context()
        archive, expected = self.archive(root)
        scratch = root / 'explicit-scratch'
        with mock.patch.object(backup, 'verify_archive', return_value={'synthetic': True}):
            with backup.decrypted_archive(archive, b'k' * 32, scratch_directory=scratch) as (payload, metadata, manifest):
                self.assertTrue(payload.is_relative_to(scratch))
                self.assertEqual(payload.read_bytes(), expected)
                self.assertEqual(metadata['payloadBytes'], len(expected))
                self.assertTrue(manifest['synthetic'])
                backup.assert_scratch_protected(payload)
            self.assertEqual([path.name for path in scratch.iterdir()], [backup.SCRATCH_MARKER])

    def test_corrupt_archive_or_caller_failure_always_removes_plaintext_folder(self):
        root = self.context()
        archive, _ = self.archive(root)
        scratch = root / 'explicit-scratch'
        with self.assertRaises(Exception):
            with backup.decrypted_archive(archive, b'x' * 32, scratch_directory=scratch):
                self.fail('Wrong key must not yield plaintext')
        self.assertEqual([path.name for path in scratch.iterdir()], [backup.SCRATCH_MARKER])
        with mock.patch.object(backup, 'verify_archive', return_value={}):
            with self.assertRaisesRegex(RuntimeError, 'synthetic consumer'):
                with backup.decrypted_archive(archive, b'k' * 32, scratch_directory=scratch):
                    raise RuntimeError('synthetic consumer failure')
        self.assertEqual([path.name for path in scratch.iterdir()], [backup.SCRATCH_MARKER])

    def test_existing_business_files_or_foreign_marker_are_rejected_without_changing_them(self):
        root = self.context()
        scratch = root / 'nonempty'
        scratch.mkdir()
        (scratch / 'preserve.txt').write_bytes(b'SYNTHETIC BUSINESS FILE')
        with mock.patch.object(backup, 'protect') as protect:
            with self.assertRaisesRegex(RuntimeError, 'initially empty'):
                backup.owned_scratch_directory(scratch)
            protect.assert_not_called()
        (scratch / 'preserve.txt').unlink()
        (scratch / backup.SCRATCH_MARKER).write_text('{"format":"foreign"}')
        with self.assertRaisesRegex(RuntimeError, 'Foreign'):
            backup.owned_scratch_directory(scratch)

    def test_root_archive_key_directories_and_links_are_rejected(self):
        root = self.context()
        for path in (pathlib.Path(root.anchor), backup.PRIVATE, backup.BACKUPS, backup.DEFAULT_KEY.parent):
            with self.assertRaisesRegex(RuntimeError, 'dedicated'):
                backup.owned_scratch_directory(path)
        target = root / 'target'
        target.mkdir()
        alias = root / 'linked'
        try:
            alias.symlink_to(target, target_is_directory=True)
        except (OSError, NotImplementedError):
            self.skipTest('Operator token does not permit local synthetic symlinks')
        with self.assertRaisesRegex(RuntimeError, 'Linked'):
            backup.owned_scratch_directory(alias)
        self.assertEqual(list(target.iterdir()), [])

    def test_failed_acl_check_happens_before_any_decryption(self):
        root = self.context()
        archive, _ = self.archive(root)
        with mock.patch.object(backup, 'assert_scratch_protected', side_effect=RuntimeError('synthetic ACL rejected')), \
             mock.patch.object(backup, 'decrypt_stream') as decrypt:
            with self.assertRaisesRegex(RuntimeError, 'ACL'):
                with backup.decrypted_archive(archive, b'k' * 32, root / 'scratch'):
                    self.fail('ACL failure cannot yield plaintext')
            decrypt.assert_not_called()

    def test_owned_directory_with_abandoned_plaintext_is_not_silently_adopted_or_deleted(self):
        root = self.context()
        scratch = backup.owned_scratch_directory(root / 'scratch')
        abandoned = scratch / 'verify-abandoned'
        abandoned.mkdir()
        (abandoned / 'payload.tar.gz').write_bytes(b'SYNTHETIC RECOVERY EVIDENCE')
        with self.assertRaisesRegex(RuntimeError, 'initially empty'):
            backup.owned_scratch_directory(scratch)
        self.assertEqual((abandoned / 'payload.tar.gz').read_bytes(), b'SYNTHETIC RECOVERY EVIDENCE')

    def test_default_scratch_location_and_private_archive_key_boundary_are_unchanged(self):
        root = self.context()
        archive, expected = self.archive(root)
        private = root / 'private'
        with mock.patch.object(backup, 'PRIVATE', private), mock.patch.object(backup, 'verify_archive', return_value={}):
            with backup.decrypted_archive(archive, b'k' * 32) as (payload, _, _):
                self.assertTrue(payload.is_relative_to(private / 'production-backup-scratch'))
                self.assertEqual(payload.read_bytes(), expected)
            self.assertEqual(list((private / 'production-backup-scratch').iterdir()), [])
            with self.assertRaisesRegex(RuntimeError, '.private'):
                backup.private_path(root / 'external-final.putybak')

    def test_all_three_decryption_callers_pass_selected_scratch(self):
        source = (ROOT / 'tools/production_backup.py').read_text(encoding='utf-8')
        tree = ast.parse(source)
        calls = [node for node in ast.walk(tree) if isinstance(node, ast.Call)
                 and isinstance(node.func, ast.Name) and node.func.id == 'decrypted_archive']
        self.assertEqual(len(calls), 3)
        self.assertTrue(all(any(key.arg == 'scratch_directory' for key in call.keywords) for call in calls))
        self.assertIn("parser.add_argument('--scratch-directory'", source)


if __name__ == '__main__':
    unittest.main()
