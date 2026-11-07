"""Offline immutable-package checks with synthetic binaries and metadata only."""
import hashlib
import importlib.util
import json
import pathlib
import tempfile
import unittest
import zipfile
from unittest import mock

ROOT = pathlib.Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location('repackage_hrm_admin', ROOT / 'deploy/production/repackage_hrm_admin.py')
worker = importlib.util.module_from_spec(spec)
spec.loader.exec_module(worker)


class Repackage(unittest.TestCase):
    def fixture(self, folder):
        base = pathlib.Path(folder).resolve()
        files = {'bin/biz-suite': b'EXACT-SYNTHETIC-BINARY', 'README.md': b'Keep this readme.',
                 'public/keep.txt': b'Keep this resource.', 'admin/index.html': b'old frontend'}
        manifest = {'format': 'suxin-runtime-v1', 'name': 'biz-suite', 'version': '0.3.2', 'protocol': 1,
                    'os': 'linux', 'arch': 'amd64', 'title': 'Synthetic HRM',
                    'unknownMetadata': {'preserved': True},
                    'files': {name: worker.digest(raw) for name, raw in files.items()}}
        raw = worker.canonical(manifest) + b'\n'
        identity = worker.digest(raw)
        package = base / 'gosuxin/storage/plugins/biz-suite/packages' / identity
        for name, content in {**files, 'plugin.json': raw}.items():
            path = package / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(content)
        (package.parent.parent / 'active.json').write_text(json.dumps({'package': identity}))
        (base / 'openim').mkdir()
        owner = 'a' * 32
        (base / 'openim/instance.json').write_text(json.dumps({'instanceID': owner, 'production': True,
             'testOnly': False, 'root': str(base / 'openim'), 'stackName': worker.PROJECT}))
        (base / 'activation.json').write_text(json.dumps({'instanceID': owner}))
        (base / 'staging').mkdir()
        frontend = base / 'staging/admin.zip'
        with zipfile.ZipFile(frontend, 'w') as archive:
            archive.writestr('index.html', '<link href="/plugins/biz-suite/admin/assets/new.css"><script src="/plugins/biz-suite/admin/assets/new.js"></script>')
            archive.writestr('assets/new.css', 'img{width:40px;height:40px}')
            archive.writestr('assets/new.js', 'console.log("synthetic")')
        return base, package, identity, frontend, worker.file_hash(frontend), manifest, files

    def test_plan_is_read_only_and_preserves_all_nonfile_metadata(self):
        with tempfile.TemporaryDirectory() as folder:
            base, package, identity, frontend, sha, manifest, files = self.fixture(folder)
            before = {p.relative_to(base).as_posix(): worker.file_hash(p) for p in base.rglob('*') if p.is_file()}
            with mock.patch.object(worker, 'BASE', base):
                plan = worker.create_plan(identity, frontend, sha)
            after = {p.relative_to(base).as_posix(): worker.file_hash(p) for p in base.rglob('*') if p.is_file()}
            self.assertEqual(before, after)
            self.assertEqual({k: v for k, v in manifest.items() if k != 'files'},
                             {k: v for k, v in plan['proposedManifest'].items() if k != 'files'})
            self.assertTrue(plan['sameVersionNewDigestSupported'])
            self.assertEqual(plan['binarySHA256'], worker.digest(files['bin/biz-suite']))

    def test_build_only_staging_replaces_admin_tree_and_preserves_original_binary(self):
        with tempfile.TemporaryDirectory() as folder:
            base, package, identity, frontend, sha, manifest, files = self.fixture(folder)
            active = (package.parent.parent / 'active.json').read_bytes()
            original = {p.relative_to(package).as_posix(): p.read_bytes() for p in package.rglob('*') if p.is_file()}
            with mock.patch.object(worker, 'BASE', base):
                plan = worker.create_plan(identity, frontend, sha)
                result = worker.build(plan, frontend)
            with zipfile.ZipFile(result['packageZIP']) as archive:
                self.assertEqual(archive.read('bin/biz-suite'), files['bin/biz-suite'])
                self.assertEqual(archive.read('README.md'), files['README.md'])
                self.assertEqual(archive.read('public/keep.txt'), files['public/keep.txt'])
                fresh = json.loads(archive.read('plugin.json'))
                self.assertEqual(fresh['version'], manifest['version'])
                for name, expected in fresh['files'].items():
                    self.assertEqual(worker.digest(archive.read(name)), expected)
                self.assertEqual(worker.digest(archive.read('plugin.json')), result['newPackageIdentity'])
                self.assertEqual(set(archive.namelist()), set(fresh['files']) | {'plugin.json'})
            self.assertEqual(active, (package.parent.parent / 'active.json').read_bytes())
            self.assertEqual(original, {p.relative_to(package).as_posix(): p.read_bytes() for p in package.rglob('*') if p.is_file()})
            self.assertFalse(result['productionModified'])
            self.assertFalse(result['installationExecuted'])

    def test_foreign_owner_digest_extra_file_and_tampered_binary_refused(self):
        for kind in ('owner', 'digest', 'extra', 'binary'):
            with self.subTest(kind=kind), tempfile.TemporaryDirectory() as folder:
                base, package, identity, frontend, sha, *_ = self.fixture(folder)
                if kind == 'owner':
                    (base / 'activation.json').write_text(json.dumps({'instanceID': 'b' * 32}))
                elif kind == 'digest':
                    (package / 'plugin.json').write_bytes(b'{}')
                elif kind == 'extra':
                    (package / 'unlisted.txt').write_bytes(b'not allowed')
                else:
                    (package / 'bin/biz-suite').write_bytes(b'changed')
                with mock.patch.object(worker, 'BASE', base), self.assertRaises(RuntimeError):
                    worker.create_plan(identity, frontend, sha)

    def test_frontend_traversal_unlisted_root_and_missing_asset_are_refused(self):
        for name, body in (('../escape.js', 'x'), ('plugin.json', '{}'),
                           ('index.html', '<script src="/plugins/biz-suite/admin/assets/missing.js"></script>')):
            with self.subTest(name=name), tempfile.TemporaryDirectory() as folder:
                base, package, identity, frontend, sha, *_ = self.fixture(folder)
                mode = 'w' if name == 'index.html' else 'a'
                with zipfile.ZipFile(frontend, mode) as archive:
                    archive.writestr(name, body)
                with mock.patch.object(worker, 'BASE', base), self.assertRaises(RuntimeError):
                    worker.create_plan(identity, frontend, worker.file_hash(frontend))

    def test_active_change_before_build_is_refused(self):
        with tempfile.TemporaryDirectory() as folder:
            base, package, identity, frontend, sha, *_ = self.fixture(folder)
            with mock.patch.object(worker, 'BASE', base):
                plan = worker.create_plan(identity, frontend, sha)
                (package.parent.parent / 'active.json').write_text(json.dumps({'package': 'f' * 64}))
                with self.assertRaisesRegex(RuntimeError, 'active_package_changed'):
                    worker.build(plan, frontend)
            self.assertFalse((base / 'staging/plugin-admin-repackage').exists())

    def test_runtime_path_rules_match_immutable_package_format(self):
        for name in ('../x', 'admin/../x', 'admin/.hidden', 'admin/a\\b', 'admin/C:/x', 'admin/AUX.txt', 'config.yaml'):
            self.assertFalse(worker.safe_path(name), name)
        for name in ('bin/biz-suite', 'admin/index.html', 'admin/assets/app-A12.js', 'LICENSE.txt'):
            self.assertTrue(worker.safe_path(name), name)


if __name__ == '__main__':
    unittest.main()
