"""Offline publication boundaries; no production files, credentials or network."""
import hashlib
import importlib.util
import io
import json
import os
import pathlib
import sys
import tempfile
import types
import unittest
import zipfile
from unittest import mock

ROOT = pathlib.Path(__file__).resolve().parents[1]


def load(name, relative):
    with mock.patch.dict(sys.modules, {'fcntl': types.SimpleNamespace(LOCK_EX=1, LOCK_NB=2, flock=lambda *a: None),
                                       'pwd': types.SimpleNamespace(getpwnam=lambda *a: None)}):
        spec = importlib.util.spec_from_file_location(name, ROOT / relative)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        return module


web = load('publish_web_tested', 'deploy/production/publish_hrm_web.py')
host = load('publish_host_tested', 'deploy/production/publish_hrm_host.py')
backend = load('publish_backend_tested', 'deploy/production/update_backend.py')
plugin = load('publish_plugin_tested', 'tools/production_hrm_plugin_release.py')
ELF = b'\x7fELF\x02\x01' + b'\0' * 12 + b'\x3e\x00' + b'fixture'
digest = lambda value: hashlib.sha256(value).hexdigest()


class PublishTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.root = pathlib.Path(self.temp.name).resolve()
        # Unix ownership calls are exercised as mocked contracts on Windows.
        self.chmod = mock.patch.object(os, 'fchmod', create=True).start()
        self.chown = mock.patch.object(os, 'fchown', create=True).start()
        self.addCleanup(mock.patch.stopall)
        self.addCleanup(self.temp.cleanup)

    def zip(self, rows):
        out = io.BytesIO()
        with zipfile.ZipFile(out, 'w') as package:
            for name, raw in rows:
                package.writestr(name, raw)
        return out.getvalue()

    def test_frontend_complete_archive_and_dependency(self):
        html = b'<script src="/suxinweb/assets/app.js"></script>'
        raw = self.zip([('index.html', html), ('assets/app.js', b'fixture')])
        path = self.root / 'web.zip'
        path.write_bytes(raw)
        self.assertEqual(set(web.inspect_archive(path, digest(raw), digest(html), 'hrm')), {'index.html', 'assets/app.js'})
        bad = self.zip([('index.html', html), ('assets/other.js', b'fixture')])
        path.write_bytes(bad)
        with self.assertRaises(RuntimeError):
            web.inspect_archive(path, digest(bad), digest(html), 'hrm')

    def test_frontend_rejects_alias_reserved_and_special_members(self):
        for extra in ('assets//a.js', '../escape', 'downloads/a', 'admin/a'):
            raw = self.zip([('index.html', b'fixture'), (extra, b'x')])
            path = self.root / 'web.zip'
            path.write_bytes(raw)
            with self.assertRaises(RuntimeError):
                web.inspect_archive(path, digest(raw), digest(b'fixture'), 'im')
        out = io.BytesIO()
        with zipfile.ZipFile(out, 'w') as archive:
            archive.writestr('index.html', b'x')
            info = zipfile.ZipInfo('assets/link')
            info.external_attr = 0o120777 << 16
            archive.writestr(info, 'target')
        path.write_bytes(out.getvalue())
        with self.assertRaises(RuntimeError):
            web.inspect_archive(path, digest(out.getvalue()), digest(b'x'), 'hrm')

    def frontend(self):
        target = self.root / 'web'
        (target / 'assets').mkdir(parents=True)
        (target / 'index.html').write_bytes(b'old index')
        (target / 'assets/app.js').write_bytes(b'old asset')
        payload = {'index.html': b'new index', 'assets/app.js': b'new asset', 'assets/new.js': b'created'}
        return target, payload, self.root / 'backup'

    def test_frontend_html_last_and_persistent_backup(self):
        target, payload, backup = self.frontend()
        original, order = web.atomic_write, []
        def writer(path, raw, *args):
            if path.is_relative_to(target):
                order.append(path.relative_to(target).as_posix())
            return original(path, raw, *args)
        with mock.patch.object(web, 'atomic_write', writer):
            web.publish(target, payload, backup, 0, 0)
        self.assertEqual(order[-1], 'index.html')
        self.assertEqual((backup / 'assets/app.js').read_bytes(), b'old asset')
        self.assertEqual((backup / 'index.html').read_bytes(), b'old index')
        self.assertTrue((backup / 'release.json').is_file())

    def test_frontend_partial_publish_rolls_back_and_removes_only_created_files(self):
        target, payload, backup = self.frontend()
        original = web.atomic_write
        failed = False
        def writer(path, raw, *args):
            nonlocal failed
            if path == target / 'index.html' and raw == b'new index' and not failed:
                failed = True
                original(path, raw, *args)  # Failure after rename must also restore.
                raise OSError('injected directory fsync failure')
            return original(path, raw, *args)
        with mock.patch.object(web, 'atomic_write', writer), self.assertRaises(OSError):
            web.publish(target, payload, backup, 0, 0)
        self.assertEqual((target / 'index.html').read_bytes(), b'old index')
        self.assertEqual((target / 'assets/app.js').read_bytes(), b'old asset')
        self.assertFalse((target / 'assets/new.js').exists())

    def test_frontend_rejects_internal_symlink_before_backup_or_publish(self):
        target, payload, backup = self.frontend()
        linked = target / 'linked'
        try:
            linked.symlink_to(target / 'assets', target_is_directory=True)
        except OSError:
            self.skipTest('OS does not permit symlink creation')
        with self.assertRaises(RuntimeError):
            web.publish(target, {'index.html': b'new', 'linked/app.js': b'new'}, backup, 0, 0)
        self.assertFalse(backup.exists())
        self.assertEqual((target / 'assets/app.js').read_bytes(), b'old asset')

    def test_host_failed_restart_restores_original_executable_and_metadata(self):
        target, source, backup = self.root / 'main', self.root / 'candidate', self.root / 'backup'
        target.write_bytes(b'original')
        source.write_bytes(ELF)
        backup.mkdir()
        old = target.stat()
        with mock.patch.object(host, 'restart_ready', side_effect=[RuntimeError('injected readiness'), None]) as restart:
            with self.assertRaises(RuntimeError):
                host.replace_host(source, target, backup, digest(b'original'), digest(ELF), 1234, 1234)
        self.assertEqual(target.read_bytes(), b'original')
        self.assertEqual(restart.call_count, 2)
        self.assertEqual(self.chown.call_args.args[1:], (old.st_uid, old.st_gid))

    def backend_fixture(self):
        base, stage = self.root / 'openim', self.root / 'staging'
        (base / 'bin').mkdir(parents=True)
        stage.mkdir()
        updates, state = {}, {}
        for name in ('enterprise-service', 'openim-msggateway', 'openim-rpc-group'):
            old, new = ELF + b'old-' + name.encode(), ELF + b'new-' + name.encode()
            (base / 'bin' / name).write_bytes(old)
            (stage / (name + '.next')).write_bytes(new)
            state[name] = {'bytes': len(old), 'sha256': digest(old)}
            updates[name] = {'bytes': len(new), 'sha256': digest(new)}
        raw = (json.dumps(state) + '\n').encode()
        (base / 'bin/backend-binaries.json').write_bytes(raw)
        return base, stage, updates, state, raw

    def test_backend_partial_stop_and_recreate_failures_restore_complete_transaction(self):
        for fail in ('stop', 'up'):
            with self.subTest(fail=fail), tempfile.TemporaryDirectory(dir=self.root) as nested:
                saved_root, self.root = self.root, pathlib.Path(nested).resolve()
                try:
                    base, stage, updates, state, raw = self.backend_fixture()
                    failed, calls = False, []
                    def runner(args):
                        nonlocal failed
                        calls.append(args)
                        if fail in args and not failed:
                            failed = True
                            raise RuntimeError('injected docker failure')
                    with mock.patch.object(backend, 'BASE', base), mock.patch.object(backend, 'STAGE', stage), \
                         mock.patch.object(backend, 'inspect_service', return_value=True), \
                         mock.patch.object(backend, 'wait_running'), mock.patch.object(backend, 'run', runner):
                        with self.assertRaises(RuntimeError):
                            backend.apply(updates, 'fixture-owner')
                    for name in updates:
                        self.assertEqual(backend.digest(base / 'bin' / name), state[name]['sha256'])
                    self.assertEqual((base / 'bin/backend-binaries.json').read_bytes(), raw)
                    self.assertTrue(any('--no-deps' in args for args in calls))
                finally:
                    self.root = saved_root

    def test_backend_current_manifest_mismatch_fails_before_stop(self):
        base, stage, updates, state, raw = self.backend_fixture()
        (base / 'bin/enterprise-service').write_bytes(b'tampered current')
        with mock.patch.object(backend, 'BASE', base), mock.patch.object(backend, 'STAGE', stage), \
             mock.patch.object(backend, 'inspect_service'), mock.patch.object(backend, 'run') as runner:
            with self.assertRaises(RuntimeError):
                backend.apply(updates, 'fixture-owner')
            runner.assert_not_called()

    def test_backend_failure_after_manifest_rename_restores_original_manifest_and_all_binaries(self):
        base, stage, updates, state, raw = self.backend_fixture()
        original, failed = backend.atomic_copy, False
        def copier(source, target, *args):
            nonlocal failed
            result = original(source, target, *args)
            if source.name == 'new-backend-binaries.json' and not failed:
                failed = True
                raise OSError('injected manifest directory fsync failure')
            return result
        with mock.patch.object(backend, 'BASE', base), mock.patch.object(backend, 'STAGE', stage), \
             mock.patch.object(backend, 'inspect_service', return_value=True), \
             mock.patch.object(backend, 'wait_running'), mock.patch.object(backend, 'run'), \
             mock.patch.object(backend, 'atomic_copy', copier):
            with self.assertRaises(OSError):
                backend.apply(updates, 'fixture-owner')
        self.assertTrue(failed)
        self.assertEqual((base / 'bin/backend-binaries.json').read_bytes(), raw)
        self.assertTrue(all(backend.digest(base / 'bin' / name) == state[name]['sha256'] for name in updates))

    def test_backend_container_owner_and_exact_readonly_binary_mount(self):
        owner, service = 'fixture-owner', 'openim-msggateway'
        value = {'Config': {'Labels': {'com.puty.suite': owner, 'com.docker.compose.project': 'puty-suite-prod',
                                      'com.docker.compose.service': service}}, 'State': {'Running': True},
                 'Mounts': [{'Type': 'bind', 'Source': str(backend.BASE / 'bin' / service),
                             'Destination': '/app/bin/' + service, 'RW': False}]}
        for wrong in ('owner', 'mount', None):
            row = json.loads(json.dumps(value))
            if wrong == 'owner':
                row['Config']['Labels']['com.puty.suite'] = 'foreign'
            if wrong == 'mount':
                row['Mounts'][0]['RW'] = True
            with mock.patch.object(backend.subprocess, 'check_output', side_effect=[b'fixture-id', json.dumps([row]).encode()]):
                if wrong:
                    with self.assertRaises(RuntimeError):
                        backend.inspect_service(service, service, owner, True)
                else:
                    self.assertTrue(backend.inspect_service(service, service, owner, True))

    def package(self, extras=(), missing=False):
        files = {'bin/biz-suite': digest(ELF), 'admin/index.html': digest(b'html')}
        manifest = json.dumps({'name': 'biz-suite', 'format': 'suxin-runtime-v1', 'protocol': 1,
                               'os': 'linux', 'arch': 'amd64', 'version': '0.4.1', 'files': files}).encode()
        rows = [('plugin.json', manifest), ('bin/biz-suite', ELF)]
        if not missing:
            rows.append(('admin/index.html', b'html'))
        return self.zip(rows + list(extras)), digest(manifest)

    def test_plugin_requires_exact_full_file_set_before_loading_credentials(self):
        for extras, missing in (((), False), ((('public/unlisted.txt', b'x'),), False), ((), True),
                                ((('admin//alias', b'x'),), False), ((('../escape', b'x'),), False)):
            raw, identity = self.package(extras, missing)
            with mock.patch.object(plugin, 'hrm_client') as credentials:
                if extras or missing:
                    with self.assertRaises(RuntimeError):
                        plugin.validate_package(raw, digest(raw), identity)
                else:
                    self.assertEqual(plugin.validate_package(raw, digest(raw), identity)['name'], 'biz-suite')
                credentials.assert_not_called()

    def test_plugin_candidate_digest_or_manifest_identity_mismatch(self):
        raw, identity = self.package()
        for expected, ident in (('0' * 64, identity), (digest(raw), '0' * 64)):
            with self.assertRaises(RuntimeError):
                plugin.validate_package(raw, expected, ident)

    def test_source_has_shared_lock_and_no_shell_commands(self):
        for relative in ('deploy/production/publish_hrm_web.py', 'deploy/production/publish_hrm_host.py',
                         'deploy/production/update_backend.py'):
            source = (ROOT / relative).read_text(encoding='utf-8-sig')
            self.assertIn('LOCK_EX | fcntl.LOCK_NB', source)
            self.assertIn('backup.lock', source)
            self.assertIn('sync_directory', source)
            self.assertNotIn('shell=True', source)


if __name__ == '__main__':
    unittest.main()
