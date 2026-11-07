"""Synthetic vhost/NGINX fault tests; no private production input or network."""
import importlib.util
import json
import os
import pathlib
import tempfile
import unittest
from unittest import mock

ROOT = pathlib.Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location('cors_release', ROOT / 'deploy/production/publish_resource_proof_cors.py')
cors = importlib.util.module_from_spec(spec)
spec.loader.exec_module(cors)


class Fixture:
    def __init__(self, test):
        temporary = tempfile.TemporaryDirectory(prefix='puty-cors-unit-')
        test.addCleanup(temporary.cleanup)
        self.base = pathlib.Path(temporary.name).resolve()
        for name in ('staging', 'backups'):
            (self.base / name).mkdir()
        self.vhost = self.base / 'original.conf'
        self.raw = b'# SYNTHETIC ONLY\n' + cors.OLD + b'\nkeep-all-original-bytes\n' + cors.OLD + b'\n'
        self.vhost.write_bytes(self.raw)
        os.chmod(self.vhost, 0o640)
        self.commands = []
        for patch in (mock.patch.object(cors, 'BASE', self.base), mock.patch.object(cors, 'VHOST', self.vhost),
                      mock.patch.object(cors, 'OLD_SHA', cors.sha(self.raw)), mock.patch.object(cors, 'guard'),
                      mock.patch.object(cors, 'regular', side_effect=lambda path: path.lstat()),
                      mock.patch.object(cors.os, 'chown', create=True), mock.patch.object(cors, 'nginx', side_effect=self.nginx)):
            patch.start()
            test.addCleanup(patch.stop)
        self.value = cors.plan()

    def nginx(self, *args):
        self.commands.append(args)


class CorsTests(unittest.TestCase):
    def test_only_two_exact_appends_and_no_unrelated_bytes_change(self):
        f = Fixture(self)
        changed = cors.candidate(f.raw)
        self.assertEqual(changed.replace(cors.NEW, cors.OLD), f.raw)
        self.assertEqual(changed.count(b'X-Puty-Resource-Proof'), 2)
        with self.assertRaisesRegex(RuntimeError, 'original'):
            cors.candidate(f.raw + b'unreviewed')

    def test_changed_old_plan_rejects_before_backup_or_write(self):
        f = Fixture(self)
        f.value['publisherSHA256'] = '0' * 64
        with self.assertRaisesRegex(RuntimeError, 'plan_changed'):
            cors.apply(f.value)
        self.assertEqual(f.vhost.read_bytes(), f.raw)
        self.assertEqual(list((f.base / 'backups').iterdir()), [])
        self.assertEqual(f.commands, [])

    def test_success_preserves_owner_mode_and_original_backup(self):
        f = Fixture(self)
        result = cors.apply(f.value)
        self.assertTrue(result['passed'])
        self.assertEqual(f.commands, [('-t',), ('-s', 'reload')])
        self.assertEqual(f.vhost.stat().st_mode & 0o777, f.value['current']['mode'])
        transaction = pathlib.Path(result['transaction'])
        self.assertEqual((transaction / 'previous.conf').read_bytes(), f.raw)
        self.assertEqual(json.loads((transaction / 'journal.json').read_bytes())['status'], 'committed')

    def test_nginx_test_failure_restores_before_validating_and_reloading_old(self):
        f = Fixture(self)
        def nginx(*args):
            f.commands.append(args)
            if len(f.commands) == 1:
                raise RuntimeError('synthetic_nginx_test')
            self.assertEqual(f.vhost.read_bytes(), f.raw)
        with mock.patch.object(cors, 'nginx', side_effect=nginx):
            with self.assertRaisesRegex(RuntimeError, 'verified_rollback_complete'):
                cors.apply(f.value)
        self.assertEqual(f.commands, [('-t',), ('-t',), ('-s', 'reload')])
        self.assertEqual(cors.metadata(f.vhost), f.value['current'])

    def test_reload_failure_also_restores_exact_old_bytes_and_metadata(self):
        f = Fixture(self)
        def nginx(*args):
            f.commands.append(args)
            if len(f.commands) == 2:
                raise RuntimeError('synthetic_reload_failure')
        with mock.patch.object(cors, 'nginx', side_effect=nginx):
            with self.assertRaisesRegex(RuntimeError, 'verified_rollback_complete'):
                cors.apply(f.value)
        self.assertEqual(cors.metadata(f.vhost), f.value['current'])
        self.assertEqual(f.commands, [('-t',), ('-s', 'reload'), ('-t',), ('-s', 'reload')])

    def test_failed_compensation_is_not_reported_as_success(self):
        f = Fixture(self)
        with mock.patch.object(cors, 'nginx', side_effect=RuntimeError('synthetic_nginx_failure')):
            with self.assertRaisesRegex(RuntimeError, 'requires_operator'):
                cors.apply(f.value)
        self.assertEqual(f.vhost.read_bytes(), f.raw)

    def test_staging_escape_is_rejected(self):
        f = Fixture(self)
        for name in ('../outside.json', '/outside.json', 'a//b', 'a/./b', 'a\\b'):
            with self.assertRaises(RuntimeError):
                cors.staged(name)

    def test_nonroot_guard_fails_before_reading_production_files(self):
        with mock.patch.object(cors.os, 'geteuid', return_value=999, create=True):
            with self.assertRaisesRegex(RuntimeError, 'root_owned'):
                cors.guard()


if __name__ == '__main__':
    unittest.main()
