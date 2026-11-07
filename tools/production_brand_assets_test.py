"""Offline brand publication, exact paths and protected-avatar regressions."""
import hashlib
import importlib.util
import io
import json
import os
import pathlib
import stat
import sys
import tempfile
import types
import unittest
import warnings
import zipfile
from unittest import mock

ROOT = pathlib.Path(__file__).resolve().parents[1]
with mock.patch.dict(sys.modules, {'fcntl': types.SimpleNamespace(LOCK_EX=1, LOCK_NB=2, flock=lambda *a: None),
                                   'pwd': types.SimpleNamespace(getpwnam=lambda *a: None)}):
    spec = importlib.util.spec_from_file_location('brand_publisher_tested', ROOT / 'deploy/production/publish_brand_assets.py')
    brand = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(brand)


class BrandTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.root = pathlib.Path(self.temp.name).resolve()
        self.addCleanup(self.temp.cleanup)
        self.addCleanup(mock.patch.stopall)
        mock.patch.object(os, 'fchmod', create=True).start()
        mock.patch.object(os, 'fchown', create=True).start()
        self.payload = {name: (b'\x00\x00\x01\x00' if name.endswith('.ico') else b'\x89PNG\r\n\x1a\n') + name.encode()
                        for name in brand.ALLOWED}

    def bundle(self, names=None, special=None, metadata_change=None):
        out = io.BytesIO()
        with warnings.catch_warnings():
            warnings.simplefilter('ignore', UserWarning)
            with zipfile.ZipFile(out, 'w') as archive:
                for name in (names if names is not None else sorted(self.payload)):
                    item = zipfile.ZipInfo(name)
                    if name == special:
                        item.external_attr = (stat.S_IFLNK | 0o777) << 16
                    archive.writestr(item, self.payload.get(name, b'unknown'))
        archive_raw = out.getvalue()
        metadata = {'format': 'puty-public-brand-assets-v1', 'targetRoot': 'resource/static/brand', 'fileCount': 28,
                    'avatarExcluded': True, 'zipSHA256': brand.sha(archive_raw),
                    'files': {name: {'bytes': len(raw), 'sha256': brand.sha(raw)} for name, raw in self.payload.items()}}
        if metadata_change:
            metadata_change(metadata)
        manifest_raw = json.dumps(metadata).encode()
        archive, manifest = self.root / 'brand.zip', self.root / 'brand.manifest.json'
        archive.write_bytes(archive_raw)
        manifest.write_bytes(manifest_raw)
        return archive, manifest, brand.sha(archive_raw), brand.sha(manifest_raw)

    def target(self, all_previous=True):
        target = self.root / 'brand'
        target.mkdir()
        (target / 'default-avatar.png').write_bytes(b'existing-protected-avatar')
        (target / 'outside.txt').write_bytes(b'not a brand release target')
        old = {}
        for name in brand.ALLOWED:
            if all_previous or '/' not in name:
                path = target / name
                path.parent.mkdir(exist_ok=True)
                old[name] = b'old-' + name.encode()
                path.write_bytes(old[name])
        return target, old, self.root / 'backup'

    def publish(self, target, backup):
        return brand.publish(target, self.payload, backup, 0, 0, 'a' * 64, 'b' * 64)

    def test_approved_public_bundle_matches_pinned_archive_manifest_and_all_28_files(self):
        archive = pathlib.Path('E:/PUTY_AI/kaoqinstyem/gosuxinwl/.cache/public-brand-puty3-20261103.zip')
        manifest = archive.with_suffix('.manifest.json')
        rows = brand.inspect_archive(archive, manifest, brand.APPROVED_ZIP_SHA256, brand.APPROVED_MANIFEST_SHA256)
        self.assertEqual(set(rows), brand.ALLOWED)
        self.assertEqual(len(rows), 28)

    def test_manifest_digest_bytes_path_or_avatar_optout_mismatch_rejected(self):
        changes = [lambda m: m.update(avatarExcluded=False),
                   lambda m: m['files'].update({'default-avatar.png': {'bytes': 1, 'sha256': '0' * 64}}),
                   lambda m: m['files']['logo.png'].update(bytes=1),
                   lambda m: m['files']['logo.png'].update(sha256='0' * 64)]
        for change in changes:
            with self.subTest(change=changes.index(change)):
                with self.assertRaises(RuntimeError):
                    brand.inspect_archive(*self.bundle(metadata_change=change))
        args = self.bundle()
        with self.assertRaises(RuntimeError):
            brand.inspect_archive(args[0], args[1], '0' * 64, args[3])

    def test_exact_zip_paths_reject_extra_alias_duplicate_avatar_and_symlink(self):
        original = sorted(self.payload)
        variants = [original + ['default-avatar.png'], original[:-1],
                    original[:-1] + ['hr//logo.png'], original + [original[0]],
                    original[:-1] + ['../logo.png']]
        for rows in variants:
            with self.assertRaises(RuntimeError):
                brand.inspect_archive(*self.bundle(names=rows))
        with self.assertRaises(RuntimeError):
            brand.inspect_archive(*self.bundle(special='logo.png'))

    def test_success_writes_only_28_whitelisted_images_and_keeps_avatars_and_other_files(self):
        target, old, backup = self.target()
        original, written = brand.atomic_write, []
        def writer(path, raw, *args):
            if path.is_relative_to(target):
                written.append(path.relative_to(target).as_posix())
            return original(path, raw, *args)
        with mock.patch.object(brand, 'atomic_write', writer):
            receipt = self.publish(target, backup)
        self.assertEqual(set(written), brand.ALLOWED)
        self.assertEqual((target / 'default-avatar.png').read_bytes(), b'existing-protected-avatar')
        self.assertEqual((target / 'outside.txt').read_bytes(), b'not a brand release target')
        self.assertTrue(receipt['defaultAvatarUnchanged'])
        self.assertEqual(receipt['protectedAvatarsBefore'], receipt['protectedAvatarsAfter'])
        self.assertEqual(json.loads((backup / 'release.json').read_text())['status'], 'committed')
        self.assertTrue(all((target / n).read_bytes() == raw for n, raw in self.payload.items()))
        self.assertTrue(all((backup / n).read_bytes() == raw for n, raw in old.items()))

    def test_failure_after_brand_rename_restores_old_files_and_removes_exact_new_files_and_empty_dirs(self):
        target, old, backup = self.target(False)
        original, count = brand.atomic_write, 0
        def writer(path, raw, *args):
            nonlocal count
            if path.is_relative_to(target) and raw == self.payload[path.relative_to(target).as_posix()]:
                count += 1
                if count == 15:
                    original(path, raw, *args)
                    raise OSError('injected fsync failure after brand rename')
            return original(path, raw, *args)
        with mock.patch.object(brand, 'atomic_write', writer), self.assertRaises(OSError):
            self.publish(target, backup)
        self.assertEqual((target / 'default-avatar.png').read_bytes(), b'existing-protected-avatar')
        self.assertTrue(all((target / n).read_bytes() == raw for n, raw in old.items()))
        self.assertTrue(all(not (target / n).exists() for n in brand.ALLOWED - set(old)))
        self.assertFalse((target / 'hr').exists())
        self.assertFalse((target / 'im').exists())
        self.assertFalse((target / 'suxin').exists())
        self.assertEqual(json.loads((backup / 'release.json').read_text())['status'], 'rolledBack')

    def test_missing_default_avatar_and_internal_symlink_rejected_before_backup(self):
        target, _, backup = self.target()
        (target / 'default-avatar.png').unlink()
        with self.assertRaises(RuntimeError):
            self.publish(target, backup)
        self.assertFalse(backup.exists())
        (target / 'default-avatar.png').write_bytes(b'avatar')
        (target / 'hr/logo.png').unlink()
        try:
            (target / 'hr/logo.png').symlink_to(target / 'im/logo.png')
        except OSError:
            self.skipTest('OS does not permit symlink creation')
        with self.assertRaises(RuntimeError):
            self.publish(target, backup)
        self.assertFalse(backup.exists())

    def test_unexpected_avatar_change_rolls_back_own_files_without_writing_avatar(self):
        target, old, backup = self.target()
        original, written = brand.atomic_write, []
        def writer(path, raw, *args):
            if path.is_relative_to(target):
                name = path.relative_to(target).as_posix()
                written.append(name)
                if name == 'logo.png' and raw == self.payload[name]:
                    (target / 'default-avatar.png').write_bytes(b'external-avatar-change')
            return original(path, raw, *args)
        with mock.patch.object(brand, 'atomic_write', writer), self.assertRaises(RuntimeError):
            self.publish(target, backup)
        self.assertNotIn('default-avatar.png', written)
        self.assertTrue(all((target / n).read_bytes() == raw for n, raw in old.items()))
        self.assertEqual((target / 'default-avatar.png').read_bytes(), b'external-avatar-change')

    def test_existing_nested_default_avatar_also_protected_and_source_uses_lock_and_frozen_helper(self):
        target, _, backup = self.target()
        (target / 'im/default-avatar.png').write_bytes(b'nested-protected-avatar')
        receipt = self.publish(target, backup)
        self.assertEqual(receipt['protectedAvatarsBefore']['im/default-avatar.png'], brand.sha(b'nested-protected-avatar'))
        self.assertEqual(receipt['protectedAvatarsAfter'], receipt['protectedAvatarsBefore'])
        source = (ROOT / 'deploy/production/publish_brand_assets.py').read_text(encoding='utf-8-sig')
        self.assertIn('LOCK_EX | fcntl.LOCK_NB', source)
        self.assertIn('backup.lock', source)
        self.assertEqual(brand.sha((ROOT / 'deploy/production/publish_hrm_web.py').read_bytes()), brand.HELPER_SHA256)

    def test_non_directory_prefix_rejected_before_any_backup_or_file_change(self):
        target, old, backup = self.target(False)
        (target / 'hr').write_bytes(b'not a directory')
        with self.assertRaises((RuntimeError, OSError)):
            self.publish(target, backup)
        self.assertFalse(backup.exists())
        self.assertTrue(all((target / name).read_bytes() == raw for name, raw in old.items()))


if __name__ == '__main__':
    unittest.main()
