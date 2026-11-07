"""Offline synthetic maintenance transitions; never read production/private files or connect."""
import importlib.util
import json
import os
import pathlib
import tempfile
import unittest
from unittest import mock

ROOT = pathlib.Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location('maintenance_test_subject', ROOT / 'deploy/production/security_maintenance.py')
gate = importlib.util.module_from_spec(spec)
spec.loader.exec_module(gate)


def original(newline=b'\n'):
    lines = [b'# SYNTHETIC MAINTENANCE FIXTURE']
    for index, (listen, name) in enumerate(gate.PUBLIC):
        lines.extend((b'server {', listen, name, b' # unchanged public body ' + str(index).encode(),
                      b' location / { proxy_pass http://fixture; }', b'}', b''))
    for listen in gate.PRIVATE:
        lines.extend((b'server {', listen, b' server_name _;', b' allow 172.30.38.10;',
                      b' deny all;', b'}', b''))
    return newline.join(lines)


class Fixture:
    def __init__(self, test):
        temporary = tempfile.TemporaryDirectory(prefix='puty-maintenance-unit-')
        test.addCleanup(temporary.cleanup)
        self.base = pathlib.Path(temporary.name).resolve()
        for name in ('staging', 'backups'):
            (self.base / name).mkdir()
        self.vhost = self.base / 'original.conf'
        self.raw = original(b'\r\n')
        self.vhost.write_bytes(self.raw)
        os.chmod(self.vhost, 0o640)
        self.commands = []
        for patch in (mock.patch.object(gate, 'BASE', self.base), mock.patch.object(gate, 'VHOST', self.vhost),
                      mock.patch.object(gate, 'OLD_SHA', gate.sha(self.raw)), mock.patch.object(gate, 'guard'),
                      mock.patch.object(gate, 'regular', side_effect=lambda path: path.lstat()),
                      mock.patch.object(gate.common, 'regular', side_effect=lambda path: path.lstat()),
                      mock.patch.object(gate.common.os, 'chown', create=True),
                      mock.patch.object(gate, 'private_directory'),
                      mock.patch.object(gate, 'nginx', side_effect=self.nginx),
                      mock.patch.dict(os.environ, {'SSH_CONNECTION': '192.168.10.10 12345 192.168.10.11 2223'})):
            patch.start()
            test.addCleanup(patch.stop)
        self.value = gate.plan('192.168.10.10')

    def nginx(self, *args):
        self.commands.append(args)

    def close(self):
        result = gate.close(self.value)
        return pathlib.Path(result['transaction']), result['planSHA256']


class MaintenanceTests(unittest.TestCase):
    def test_candidate_only_changes_four_public_servers_and_preserves_each_newline(self):
        for nl in (b'\n', b'\r\n'):
            raw = original(nl)
            with mock.patch.object(gate, 'OLD_SHA', gate.sha(raw)):
                changed = gate.candidate(raw, '192.168.10.10')
            addition = b' if ($puty_security_maintenance) { return 503; }' + nl
            self.assertEqual(changed.count(addition), 4)
            self.assertIn(b'map $remote_addr', changed)
            self.assertNotIn(b'$http_x_forwarded_for', changed)
            # Both private bridge suffixes, including ACLs, are byte-identical.
            before_private = raw.index(nl.join((b'server {', gate.PRIVATE[0])))
            self.assertTrue(changed.endswith(raw[before_private:]))
            self.assertEqual(changed.count(nl), raw.count(nl) + 11)

    def test_wrong_hash_shape_or_already_gated_source_fails_closed(self):
        raw = original()
        with mock.patch.object(gate, 'OLD_SHA', gate.sha(raw)):
            with self.assertRaisesRegex(RuntimeError, 'original'):
                gate.candidate(raw + b'x', '192.168.10.10')
        changed = raw.replace(gate.PRIVATE[0], b' listen 0.0.0.0:18001;')
        with mock.patch.object(gate, 'OLD_SHA', gate.sha(changed)):
            with self.assertRaisesRegex(RuntimeError, 'private_vhost_shape'):
                gate.candidate(changed, '192.168.10.10')
        changed = raw + b'# puty_security_maintenance'
        with mock.patch.object(gate, 'OLD_SHA', gate.sha(changed)):
            with self.assertRaisesRegex(RuntimeError, 'original'):
                gate.candidate(changed, '192.168.10.10')

    def test_operator_is_actual_ssh_peer_not_caller_supplied_forwarded_ip(self):
        f = Fixture(self)
        for invalid in ('127.0.0.1', '0.0.0.0', '192.168.010.10', '192.168.10.10;include x;', '::', 'fe80::1'):
            with self.assertRaisesRegex(RuntimeError, 'ip_invalid'):
                gate.plan(invalid)
        with self.assertRaisesRegex(RuntimeError, 'ssh_operator'):
            gate.plan('192.168.10.20')
        with mock.patch.dict(os.environ, {'SSH_CONNECTION': ''}):
            with self.assertRaisesRegex(RuntimeError, 'ssh_operator'):
                gate.plan('192.168.10.10')
        self.assertEqual(f.vhost.read_bytes(), f.raw)

    def test_stale_plan_fails_before_backup_or_nginx(self):
        f = Fixture(self)
        f.value['publisherSHA256'] = '0' * 64
        with self.assertRaisesRegex(RuntimeError, 'plan_changed'):
            gate.close(f.value)
        self.assertEqual(list((f.base / 'backups').iterdir()), [])
        self.assertEqual(f.commands, [])

    def test_close_then_open_and_repeat_open_preserves_exact_old_metadata(self):
        f = Fixture(self)
        transaction, digest = f.close()
        self.assertEqual((transaction / 'previous.conf').read_bytes(), f.raw)
        self.assertEqual(json.loads((transaction / 'journal.json').read_bytes())['status'], 'closed')
        self.assertEqual(gate.metadata(f.vhost), gate.gated_metadata(f.value, f.vhost.read_bytes()))
        for _ in range(2):
            self.assertTrue(gate.release(str(transaction), digest)['passed'])
            self.assertEqual(gate.metadata(f.vhost), f.value['current'])
        self.assertEqual(json.loads((transaction / 'journal.json').read_bytes())['status'], 'released')
        self.assertEqual(f.commands, [('-t',), ('-s', 'reload')] * 3)

    def test_close_test_or_reload_failure_restores_old_before_reporting_failure(self):
        for fail_at in (1, 2):
            with self.subTest(fail_at=fail_at):
                f = Fixture(self)
                def nginx(*args):
                    f.commands.append(args)
                    if len(f.commands) == fail_at:
                        raise RuntimeError('synthetic_nginx_failure')
                with mock.patch.object(gate, 'nginx', side_effect=nginx):
                    with self.assertRaisesRegex(RuntimeError, 'close_failed_verified_rollback_complete'):
                        gate.close(f.value)
                self.assertEqual(gate.metadata(f.vhost), f.value['current'])

    def test_release_reload_failure_restores_gate_and_keeps_recovery_receipt(self):
        f = Fixture(self)
        transaction, digest = f.close()
        gated = f.vhost.read_bytes()
        f.commands.clear()
        def nginx(*args):
            f.commands.append(args)
            if len(f.commands) == 2:
                raise RuntimeError('synthetic_release_reload_failure')
        with mock.patch.object(gate, 'nginx', side_effect=nginx):
            with self.assertRaisesRegex(RuntimeError, 'release_failed_gate_preserved'):
                gate.release(str(transaction), digest)
        self.assertEqual(f.vhost.read_bytes(), gated)
        self.assertEqual(json.loads((transaction / 'journal.json').read_bytes())['status'], 'releaseFailedClosed')
        self.assertTrue(gate.release(str(transaction), digest)['passed'])

    def test_foreign_edit_is_never_clobbered_or_reloaded(self):
        f = Fixture(self)
        transaction, digest = f.close()
        foreign = f.vhost.read_bytes() + b'# external edit\r\n'
        f.vhost.write_bytes(foreign)
        f.commands.clear()
        with self.assertRaisesRegex(RuntimeError, 'another_writer'):
            gate.release(str(transaction), digest)
        self.assertEqual(f.vhost.read_bytes(), foreign)
        self.assertEqual(f.commands, [])

    def test_interrupted_prepared_transaction_recovers_old_or_gated_only(self):
        f = Fixture(self)
        transaction, digest = f.close()
        gate.journal(transaction, f.value, 'prepared')
        gate.atomic(f.vhost, f.raw, f.value['current']['mode'])
        self.assertTrue(gate.release(str(transaction), digest)['passed'])
        self.assertEqual(gate.metadata(f.vhost), f.value['current'])

    def test_plan_backup_or_journal_mismatch_prevents_any_reload(self):
        f = Fixture(self)
        transaction, digest = f.close()
        f.commands.clear()
        with self.assertRaisesRegex(RuntimeError, 'plan_mismatch'):
            gate.release(str(transaction), '0' * 64)
        gate.atomic(transaction / 'previous.conf', f.raw + b'not-original')
        with self.assertRaisesRegex(RuntimeError, 'backup_mismatch'):
            gate.release(str(transaction), digest)
        self.assertEqual(f.commands, [])

    def test_rollback_failure_reports_operator_needed(self):
        f = Fixture(self)
        with mock.patch.object(gate, 'nginx', side_effect=RuntimeError('synthetic_nginx_failure')):
            with self.assertRaisesRegex(RuntimeError, 'rollback_requires_operator'):
                gate.close(f.value)
        self.assertEqual(f.vhost.read_bytes(), f.raw)

    def test_transaction_escape_rejected_before_read(self):
        f = Fixture(self)
        for value in (str(f.base / 'unrelated'), str(f.base / 'backups' / 'security-maintenance-a'),
                      str(f.base / 'backups' / '..' / ('security-maintenance-' + 'a' * 32))):
            with self.assertRaisesRegex(RuntimeError, 'transaction_invalid'):
                gate.release(value, '0' * 64)


if __name__ == '__main__':
    unittest.main()
