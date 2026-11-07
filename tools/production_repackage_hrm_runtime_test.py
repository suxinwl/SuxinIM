"""Offline runtime repackaging: synthetic ELF, immutable admin, no server I/O."""
import importlib.util
import json
import os
import pathlib
import struct
import tempfile
import unittest
import zipfile
from unittest import mock

ROOT = pathlib.Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location('repackage_hrm_runtime', ROOT / 'deploy/production/repackage_hrm_runtime.py')
worker = importlib.util.module_from_spec(spec)
spec.loader.exec_module(worker)


def synthetic_elf():
    raw = bytearray(192)
    raw[:7] = b'\x7fELF\x02\x01\x01'
    struct.pack_into('<HHIQQ', raw, 16, 2, 62, 1, 0x400080, 64)
    struct.pack_into('<HHH', raw, 52, 64, 56, 1)
    struct.pack_into('<IIQQQQQQ', raw, 64, 1, 5, 0, 0x400000, 0x400000, len(raw), len(raw), 4096)
    raw[128:151] = b'SYNTHETIC-NOT-RUNNABLE!!'
    return bytes(raw)


class RuntimeRepackage(unittest.TestCase):
    def fixture(self, folder):
        base = pathlib.Path(folder).resolve()
        files = {'bin/biz-suite': b'ORIGINAL-SYNTHETIC-BINARY', 'README.md': b'Original readme.',
                 'LICENSE.txt': b'Original license.', 'public/keep.txt': b'Keep this resource.',
                 'admin/index.html': b'<img width="40" height="40" src="/resource/static/brand/hr/logo.png">',
                 'admin/assets/app.js': b'fixed logo navigation',
                 'admin/assets/style.css': b'.brand-mark{width:40px;height:40px}',
                 'admin/assets/brand.png': bytes(range(256))}
        manifest = {'format': 'suxin-runtime-v1', 'name': 'biz-suite', 'version': '0.4.0-openim3.8',
                    'protocol': 1, 'os': 'linux', 'arch': 'amd64', 'title': 'Synthetic HRM',
                    'unknownMetadata': {'preserveEveryValue': True},
                    'files': {name: worker.digest(value) for name, value in files.items()}}
        raw = worker.canonical(manifest) + b'\n'
        identity = worker.digest(raw)
        package = base / 'gosuxin/storage/plugins/biz-suite/packages' / identity
        for name, value in {**files, 'plugin.json': raw}.items():
            path = package / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(value)
        (package.parent.parent / 'active.json').write_text(json.dumps({'package': identity}))
        (base / 'openim').mkdir()
        owner = 'a' * 32
        (base / 'openim/instance.json').write_text(json.dumps({'instanceID': owner, 'production': True,
             'testOnly': False, 'root': str(base / 'openim'), 'stackName': worker.helpers.PROJECT}))
        (base / 'activation.json').write_text(json.dumps({'instanceID': owner}))
        (base / 'staging').mkdir()
        binary = base / 'staging/biz-suite-new'
        binary.write_bytes(synthetic_elf())
        return base, package, identity, binary, worker.file_hash(binary), manifest, files

    def patched(self, base, identity):
        return mock.patch.multiple(worker, EXPECTED_ACTIVE=identity), mock.patch.object(worker.helpers, 'BASE', base)

    def snapshot(self, base):
        return {p.relative_to(base).as_posix(): p.read_bytes() for p in base.rglob('*') if p.is_file()}

    def test_plan_is_read_only_and_only_binary_hash_and_version_change(self):
        with tempfile.TemporaryDirectory() as folder:
            base, _, identity, binary, sha, manifest, files = self.fixture(folder)
            before = self.snapshot(base)
            one, two = self.patched(base, identity)
            with one, two:
                plan = worker.create_plan(identity, binary, sha)
            self.assertEqual(before, self.snapshot(base))
            self.assertEqual(plan['version'], '0.4.1-openim3.8')
            expected = {**manifest, 'version': worker.VERSION, 'files': {**manifest['files'], worker.BINARY: sha}}
            self.assertEqual(plan['proposedManifest'], expected)
            self.assertEqual(plan['preservedAdminFileCount'], 4)
            self.assertTrue(plan['adminFilesUnchanged'])

    def test_build_preserves_exact_admin_and_other_files_and_does_not_install(self):
        with tempfile.TemporaryDirectory() as folder:
            base, package, identity, binary, sha, manifest, files = self.fixture(folder)
            before = self.snapshot(base)
            one, two = self.patched(base, identity)
            with one, two:
                plan = worker.create_plan(identity, binary, sha)
                plan_sha = worker.digest(worker.canonical(plan))
                result = worker.build(plan, binary, plan_sha)
            self.assertTrue(result['builtStagingOnly'])
            self.assertFalse(result['productionModified'])
            self.assertFalse(result['installationExecuted'])
            self.assertEqual(result['reviewedPlanSHA256'], plan_sha)
            with zipfile.ZipFile(result['packageZIP']) as archive:
                proposed = json.loads(archive.read('plugin.json'))
                self.assertEqual(worker.digest(archive.read('plugin.json')), result['newPackageIdentity'])
                self.assertEqual(archive.read(worker.BINARY), binary.read_bytes())
                self.assertEqual(proposed['version'], worker.VERSION)
                self.assertEqual(len(archive.infolist()), len(files) + 1)
                for name, body in files.items():
                    if name != worker.BINARY:
                        self.assertEqual(archive.read(name), body, name)
                for name, expected in proposed['files'].items():
                    self.assertEqual(worker.digest(archive.read(name)), expected)
            for name, body in before.items():
                self.assertEqual((base / name).read_bytes(), body, name)

    def test_wrong_plan_or_changed_candidate_refused_before_creating_stage(self):
        for kind in ('plan-sha', 'mutated-plan', 'candidate', 'active-marker', 'admin'):
            with self.subTest(kind=kind), tempfile.TemporaryDirectory() as folder:
                base, package, identity, binary, sha, *_ = self.fixture(folder)
                one, two = self.patched(base, identity)
                with one, two:
                    plan = worker.create_plan(identity, binary, sha)
                    plan_sha = worker.digest(worker.canonical(plan))
                    if kind == 'plan-sha':
                        plan_sha = 'f' * 64
                    elif kind == 'mutated-plan':
                        plan['proposedManifest']['title'] = 'Unreviewed metadata'
                    elif kind == 'candidate':
                        binary.write_bytes(binary.read_bytes() + b'changed')
                    elif kind == 'active-marker':
                        (package.parent.parent / 'active.json').write_text(json.dumps({'package': 'f' * 64}))
                    else:
                        (package / 'admin/assets/app.js').write_bytes(b'changed')
                    with self.assertRaises(RuntimeError):
                        worker.build(plan, binary, plan_sha)
                self.assertFalse((base / 'staging/plugin-runtime-repackage').exists())

    def test_owner_other_active_hash_unlisted_file_wrong_binary_hash_rejected(self):
        for kind in ('owner', 'active-hash', 'extra', 'binary-sha'):
            with self.subTest(kind=kind), tempfile.TemporaryDirectory() as folder:
                base, package, identity, binary, sha, *_ = self.fixture(folder)
                one, two = self.patched(base, identity)
                with one, two:
                    if kind == 'owner':
                        (base / 'activation.json').write_text(json.dumps({'instanceID': 'b' * 32}))
                    elif kind == 'extra':
                        (package / 'admin/unlisted.js').write_bytes(b'x')
                    elif kind == 'binary-sha':
                        sha = 'f' * 64
                    with self.assertRaises(RuntimeError):
                        worker.create_plan('f' * 64 if kind == 'active-hash' else identity, binary, sha)

    def test_wrong_platform_and_malformed_or_nonexecutable_ELF_rejected(self):
        for kind in ('PE', 'arm64', 'no-entry', 'bad-segment', 'truncated-table'):
            with self.subTest(kind=kind), tempfile.TemporaryDirectory() as folder:
                base, _, identity, binary, *_ = self.fixture(folder)
                raw = bytearray(synthetic_elf())
                if kind == 'PE':
                    raw[:4] = b'MZ\x00\x00'
                elif kind == 'arm64':
                    struct.pack_into('<H', raw, 18, 183)
                elif kind == 'no-entry':
                    struct.pack_into('<Q', raw, 24, 0)
                elif kind == 'bad-segment':
                    struct.pack_into('<Q', raw, 96, len(raw) + 1)
                else:
                    raw = raw[:90]
                binary.write_bytes(raw)
                one, two = self.patched(base, identity)
                with one, two, self.assertRaises(RuntimeError):
                    worker.create_plan(identity, binary, worker.file_hash(binary))

    def test_binary_outside_staging_and_noncanonical_path_rejected(self):
        with tempfile.TemporaryDirectory() as folder:
            base, _, identity, binary, sha, *_ = self.fixture(folder)
            outside = base / 'candidate'
            outside.write_bytes(binary.read_bytes())
            one, two = self.patched(base, identity)
            with one, two:
                for path in (outside, binary.parent / '..' / 'staging' / binary.name):
                    with self.subTest(path=path), self.assertRaises(RuntimeError):
                        worker.create_plan(identity, path, sha)

    def test_duplicate_build_does_not_replace_existing_output(self):
        with tempfile.TemporaryDirectory() as folder:
            base, _, identity, binary, sha, *_ = self.fixture(folder)
            one, two = self.patched(base, identity)
            with one, two:
                plan = worker.create_plan(identity, binary, sha)
                plan_sha = worker.digest(worker.canonical(plan))
                result = worker.build(plan, binary, plan_sha)
                before = pathlib.Path(result['packageZIP']).read_bytes()
                with self.assertRaises(FileExistsError):
                    worker.build(plan, binary, plan_sha)
                self.assertEqual(pathlib.Path(result['packageZIP']).read_bytes(), before)

    def test_hardlinked_candidate_is_refused(self):
        with tempfile.TemporaryDirectory() as folder:
            base, _, identity, binary, sha, *_ = self.fixture(folder)
            try:
                os.link(binary, base / 'staging/linked')
            except OSError:
                self.skipTest('hard links unavailable')
            one, two = self.patched(base, identity)
            with one, two, self.assertRaisesRegex(RuntimeError, 'regular_unlinked'):
                worker.create_plan(identity, binary, sha)


if __name__ == '__main__':
    unittest.main()
