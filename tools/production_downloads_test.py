"""Lightweight, offline download preparation/publication and rollback checks."""
import importlib.util
import json
import os
import pathlib
import shutil
import sys
import tempfile
import types
import unittest
from unittest import mock

ROOT = pathlib.Path(__file__).resolve().parents[1]


def load(name, path):
    with mock.patch.dict(sys.modules, {'fcntl': types.SimpleNamespace(LOCK_EX=1, LOCK_NB=2, flock=lambda *args: None),
                                       'pwd': types.SimpleNamespace(getpwnam=lambda *args: None)}):
        spec = importlib.util.spec_from_file_location(name, ROOT / path)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        return module


prep = load('download_preparation_tested', 'tools/prepare_production_downloads.py')
pub = load('download_publication_tested', 'deploy/production/publish_downloads.py')


class DownloadsTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.root = pathlib.Path(self.temporary.name).resolve()
        self.addCleanup(self.temporary.cleanup)
        self.addCleanup(mock.patch.stopall)
        mock.patch.object(os, 'fchmod', create=True).start()
        mock.patch.object(os, 'fchown', create=True).start()
        (self.root / 'clients').mkdir()
        (self.root / 'clients/brand.json').write_text(json.dumps({'displayName': 'PUTY <内部通讯>'}))
        artifacts = []
        self.native = {'PUTY_IM-3.8.9-win-x64.exe': b'MZ-synthetic-windows',
                       'PUTY_IM-3.8.16-android-arm64.apk': b'PK-synthetic-android'}
        for platform, filename in zip(('windows', 'android'), self.native):
            folder = 'clients/web-desktop/release' if platform == 'windows' else 'clients/flutter/build/delivery'
            path = self.root / folder / filename
            path.parent.mkdir(parents=True)
            path.write_bytes(self.native[filename])
            row = {'platform': platform, 'fileName': filename, 'sha256': pub.sha(self.native[filename]),
                   'size': len(self.native[filename]), 'version': '3.8.9' if platform == 'windows' else '3.8.16',
                   'build': 380901 if platform == 'windows' else 116}
            if platform == 'android':
                row.update(applicationID='com.szputy.im', developmentCAIncluded=False,
                           signingCertificateSHA256=prep.ANDROID_SIGNER)
            artifacts.append(row)
        self.metadata = {'format': 'synthetic', 'enterpriseRoot': {'derSHA256': prep.ROOT_SHA}, 'artifacts': artifacts}
        self.manifest = self.root / '.private/candidate.json'
        self.manifest.parent.mkdir()
        self.manifest.write_text(json.dumps(self.metadata))
        self.source = self.manifest.read_bytes()
        # The public generator input must not be overwritten by a private candidate.
        (self.root / 'clients/artifacts-manifest.json').write_bytes(b'PUBLIC-UNCHANGED')
        self.out = self.root / '.private/downloads-candidate'
        self.stage = self.root / 'staging'
        self.stage.mkdir()

    def staged(self):
        result = prep.prepare(self.root, self.manifest, self.out, pub.sha(self.source))
        names = ('index.html', 'artifacts-manifest.json', 'sha256sums.txt')
        for name in names:
            shutil.copyfile(self.out / name, self.stage / name)
        for name, raw in self.native.items():
            (self.stage / name).write_bytes(raw)
        publish = self.stage / 'downloads-publish.json'
        shutil.copyfile(self.out / 'publish.json', publish)
        return publish, result['publishSHA256']

    def target(self):
        target, backup = self.root / 'downloads', self.root / 'backups/unique'
        target.mkdir()
        backup.parent.mkdir()
        (target / 'index.html').write_bytes(b'OLD HTML')
        (target / 'artifacts-manifest.json').write_bytes(b'OLD MANIFEST')
        (target / 'sha256sums.txt').write_bytes(b'OLD SUMS')
        (target / 'old-version.exe').write_bytes(b'KEEP OLDER PACKAGE')
        return target, backup, pub.sha(b'OLD HTML'), pub.sha(b'OLD MANIFEST')

    def test_private_prepare_custom_name_png_release_notes_and_no_public_writes(self):
        result = prep.prepare(self.root, self.manifest, self.out, pub.sha(self.source))
        html = (self.out / 'index.html').read_text(encoding='utf-8')
        self.assertIn('PUTY &lt;内部通讯&gt; 客户端下载', html)
        self.assertIn('favicon-32x32.png?v=puty3', html)
        self.assertIn('logo.png?v=puty3', html)
        self.assertNotIn('favicon.ico', html)
        for value in ('按月加密', '原文件名', '本机记录清理', '重置与解锁', '安卓 7.x', '安卓 8.0'):
            self.assertIn(value, html)
        self.assertEqual(self.manifest.read_bytes(), self.source)
        self.assertEqual((self.root / 'clients/artifacts-manifest.json').read_bytes(), b'PUBLIC-UNCHANGED')
        self.assertEqual(len(json.loads((self.out / 'publish.json').read_text())['files']), 5)
        self.assertFalse(result['published'])

    def test_mandatory_upgrade_records_are_prepared_privately_without_publishing(self):
        result = prep.prepare(self.root, self.manifest, self.out, pub.sha(self.source), force_upgrade=True)
        self.assertTrue(result['forceUpgrade'])
        for platform in ('windows', 'android'):
            update = json.loads((self.out / (platform + '-version.json')).read_bytes())
            self.assertIs(update['force'], True)
            self.assertIn(update['sha256'], (self.out / 'sha256sums.txt').read_text())
        self.assertFalse(result['published'])
        self.assertEqual((self.root / 'clients/artifacts-manifest.json').read_bytes(), b'PUBLIC-UNCHANGED')

    def test_prepare_rejects_bad_candidate_hash_platform_identity_or_public_output(self):
        with self.assertRaises(RuntimeError):
            prep.prepare(self.root, self.manifest, self.out, '0' * 64)
        self.assertFalse(self.out.exists())
        bad = json.loads(self.source)
        bad['artifacts'][1]['developmentCAIncluded'] = True
        self.manifest.write_text(json.dumps(bad))
        with self.assertRaises(RuntimeError):
            prep.prepare(self.root, self.manifest, self.out)
        self.assertFalse(self.out.exists())
        self.manifest.write_bytes(self.source)
        with self.assertRaises(RuntimeError):
            prep.prepare(self.root, self.manifest, self.root / 'clients/public-output')

    def test_exact_five_staged_files_and_hash_mismatch(self):
        path, digest = self.staged()
        payload = pub.inspect_staged(self.stage, path, digest)
        self.assertEqual(set(payload), set(self.native) | pub.STATIC)
        with self.assertRaises(RuntimeError):
            pub.inspect_staged(self.stage, path, '0' * 64)
        (self.stage / next(iter(self.native))).write_bytes(b'CHANGED')
        with self.assertRaises(RuntimeError):
            pub.inspect_staged(self.stage, path, digest)

    def test_output_manifest_cannot_overwrite_its_source(self):
        self.out.mkdir()
        source = self.out / 'artifacts-manifest.json'
        source.write_bytes(self.source)
        with self.assertRaisesRegex(RuntimeError, 'overwrite the source'):
            prep.prepare(self.root, source, self.out)
        self.assertEqual(source.read_bytes(), self.source)
        self.assertEqual({p.name for p in self.out.iterdir()}, {'artifacts-manifest.json'})

    def test_duplicate_traversal_extra_entry_and_checksum_metadata_mismatch_rejected(self):
        path, digest = self.staged()
        original = path.read_bytes()
        for kind in ('duplicate', 'traversal', 'extra', 'sums', 'public-manifest'):
            with self.subTest(kind=kind):
                data = json.loads(original)
                if kind == 'duplicate':
                    data['files'][0] = data['files'][1]
                elif kind == 'traversal':
                    data['files'][0]['name'] = '../outside.exe'
                elif kind == 'extra':
                    data['files'].append(data['files'][0])
                else:
                    name = 'sha256sums.txt' if kind == 'sums' else 'artifacts-manifest.json'
                    raw = b'WRONG SUMS' if kind == 'sums' else b'{"enterpriseRoot":{},"artifacts":[]}'
                    (self.stage / name).write_bytes(raw)
                    for row in data['files']:
                        if row['name'] == name:
                            row.update(bytes=len(raw), sha256=pub.sha(raw))
                path.write_text(json.dumps(data))
                with self.assertRaises(RuntimeError):
                    pub.inspect_staged(self.stage, path, pub.digest(path))
                for name in ('sha256sums.txt', 'artifacts-manifest.json'):
                    shutil.copyfile(self.out / name, self.stage / name)

    def test_old_index_and_public_manifest_guards_reject_before_backup(self):
        path, digest = self.staged()
        payload = pub.inspect_staged(self.stage, path, digest)
        target, backup, old_index, old_manifest = self.target()
        for index, manifest in (('0' * 64, old_manifest), (old_index, '0' * 64)):
            with self.assertRaises(RuntimeError):
                pub.publish(target, payload, backup, index, manifest, digest)
            self.assertFalse(backup.exists())

    def test_publish_root_owner_html_last_and_retains_older_packages(self):
        path, digest = self.staged()
        payload = pub.inspect_staged(self.stage, path, digest)
        target, backup, old_index, old_manifest = self.target()
        original, calls = pub.helpers.atomic_write, []
        def writer(path, raw, mode, uid=None, gid=None):
            if path.is_relative_to(target):
                calls.append((path.name, mode, uid, gid))
            return original(path, raw, mode, uid, gid)
        with mock.patch.object(pub.helpers, 'atomic_write', writer):
            receipt = pub.publish(target, payload, backup, old_index, old_manifest, digest)
        self.assertEqual(calls[-1][0], 'index.html')
        self.assertEqual(len(calls), 5)
        self.assertTrue(all(call[1:] == (0o644, 0, 0) for call in calls))
        self.assertEqual((target / 'old-version.exe').read_bytes(), b'KEEP OLDER PACKAGE')
        self.assertEqual((backup / 'index.html').read_bytes(), b'OLD HTML')
        self.assertEqual(receipt['newFilesOwner'], 'root:root')

    def test_partial_after_rename_failure_rolls_back_and_removes_only_new_files(self):
        path, digest = self.staged()
        payload = pub.inspect_staged(self.stage, path, digest)
        target, backup, old_index, old_manifest = self.target()
        before = {p.name: p.read_bytes() for p in target.iterdir()}
        original, failed = pub.helpers.atomic_write, False
        def writer(path, raw, *args):
            nonlocal failed
            original(path, raw, *args)
            if path == target / 'index.html' and raw == payload['index.html'] and not failed:
                failed = True
                raise OSError('Injected failure after rename')
        with mock.patch.object(pub.helpers, 'atomic_write', writer), self.assertRaises(OSError):
            pub.publish(target, payload, backup, old_index, old_manifest, digest)
        self.assertEqual({p.name: p.read_bytes() for p in target.iterdir()}, before)

    def test_published_native_filename_cannot_be_overwritten_with_new_binary(self):
        path, digest = self.staged()
        payload = pub.inspect_staged(self.stage, path, digest)
        target, backup, old_index, old_manifest = self.target()
        (target / next(iter(self.native))).write_bytes(b'EXISTING DIFFERENT BINARY')
        with self.assertRaisesRegex(RuntimeError, 'immutable'):
            pub.publish(target, payload, backup, old_index, old_manifest, digest)
        self.assertFalse(backup.exists())

    def test_staging_symlink_is_refused(self):
        path, digest = self.staged()
        source = self.stage / next(iter(self.native))
        peer = self.root / 'outside-binary'
        source.replace(peer)
        try:
            source.symlink_to(peer)
        except OSError:
            self.skipTest('Symlink creation unavailable')
        with self.assertRaises(RuntimeError):
            pub.inspect_staged(self.stage, path, digest)


if __name__ == '__main__':
    unittest.main()
