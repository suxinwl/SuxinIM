"""Synthetic guarded rollback: no Docker, Nginx, credentials or production paths."""
import importlib.util
import json
import pathlib
import subprocess
import sys
import tempfile
import types
import unittest
from unittest import mock

ROOT = pathlib.Path(__file__).resolve().parents[1]
def module(name, filename):
    spec = importlib.util.spec_from_file_location(name, ROOT / 'deploy/production' / filename)
    value = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(value)
    return value

rollback = module('rollback_subject', 'rollback_security_release.py')
publisher = module('rollback_frozen_publisher', 'publish_security_policy.py')


class Fixture:
    def __init__(self, test):
        temp = tempfile.TemporaryDirectory(prefix='puty-committed-rollback-')
        test.addCleanup(temp.cleanup)
        self.base = pathlib.Path(temp.name).resolve()
        self.transaction = self.base / 'backups' / ('security-policy-' + 'a' * 32)
        self.gate_transaction = self.base / 'backups' / ('security-maintenance-' + 'b' * 32)
        self.gate_transaction.mkdir(parents=True)
        self.transaction.mkdir(parents=True)
        self.mode = {}
        def regular(path, root_owned=True):
            assert path.is_file() and path.resolve() == path and not path.is_symlink()
            return path
        def metadata(path, root_owned=True):
            regular(path)
            return {'sha256':rollback.sha(path.read_bytes()), 'bytes':path.stat().st_size,
                    'mode':self.mode.get(path, 0o640), 'uid':0, 'gid':0}
        self.metadata = metadata
        self.containers = {name:{'Id':name + '-instance', 'Image':name + '-image'} for name in ('a','b','storage')}
        self.plugin = {'synthetic':'unchanged'}
        for patch in (mock.patch.object(publisher, 'BASE', self.base),
                      mock.patch.object(publisher, 'BINARIES', {'a':'a', 'b':'b'}),
                      mock.patch.object(publisher, 'regular', side_effect=regular),
                      mock.patch.object(publisher, 'metadata', side_effect=metadata),
                      mock.patch.object(publisher, 'owner_guard'),
                      mock.patch.object(publisher, 'recovery_guard', return_value=self.containers),
                      mock.patch.object(publisher, 'running_process_proof'),
                      mock.patch.object(publisher, 'capacity_mount_proof'),
                      mock.patch.object(publisher, 'plugin_proof', return_value=self.plugin),
                      mock.patch.object(publisher, 'stage_file', side_effect=lambda name:self.base / 'staging' / name)):
            patch.start()
            test.addCleanup(patch.stop)
        targets = [publisher.MAIN, publisher.CRON, publisher.COMPOSE, publisher.MANIFEST,
                   'gosuxin/resource/suxinweb/index.html', 'openim/bin/a', 'openim/bin/b']
        self.old = {}
        for name in targets:
            raw = b'old-' + name.encode()
            if name == publisher.MANIFEST:
                raw = publisher.canonical({'a':{'old':True}, 'b':{'old':True}})
            self.write(self.base / name, raw)
            self.old[name] = metadata(self.base / name)
            if not name.endswith('/suxinweb/index.html'):
                self.write(self.transaction / 'previous' / name, raw)
        backend = {key:{'path':'security/' + key, 'bytes':len(b'new-' + key.encode()),
                        'sha256':rollback.sha(b'new-' + key.encode())} for key in ('a','b')}
        self.payload = {'index.html':b'new synthetic index', 'new.js':b'new synthetic script'}
        self.plan = {'format':'puty-security-plan-1', 'instanceID':publisher.INSTANCE,
            'publisherSHA256':rollback.PINS['publish_security_policy'], 'current':self.old,
            'request':{'backend':backend, 'main':{'bytes':8,'sha256':rollback.sha(b'new-main')},
                       'web':{'path':'security/web.zip','sha256':'1'*64,'indexSHA256':rollback.sha(self.payload['index.html'])}},
            'proposedCronSHA256':rollback.sha(b'new-cron'), 'proposedComposeSHA256':rollback.sha(b'new-compose'),
            'volumes':{'storage':'owned-original'}}
        for name in ('a','b'):
            self.write(self.base / ('openim/bin/' + name), b'new-' + name.encode())
        self.write(self.base / publisher.MAIN, b'new-main')
        self.write(self.base / publisher.CRON, b'new-cron')
        self.write(self.base / publisher.COMPOSE, b'new-compose')
        manifest = {'a':{'bytes':5, 'sha256':rollback.sha(b'new-a')}, 'b':{'bytes':5, 'sha256':rollback.sha(b'new-b')}}
        self.write(self.base / publisher.MANIFEST, publisher.canonical(manifest))
        receipt = {'format':'puty-static-release-v2', 'newFiles':{k:rollback.sha(v) for k,v in self.payload.items()},
                   'previousFiles':{'index.html':self.old['gosuxin/resource/suxinweb/index.html']['sha256']},
                   'previousMetadata':{'index.html':[0o640,0,0]}, 'createdFiles':['new.js']}
        self.write(self.transaction / 'web/index.html', b'old-gosuxin/resource/suxinweb/index.html')
        self.write(self.transaction / 'web/release.json', publisher.canonical(receipt))
        for name, raw in self.payload.items():
            path = self.base / publisher.WEB / name
            self.write(path, raw)
            self.mode[path] = 0o644
        self.write(self.transaction / 'plan.json', publisher.canonical(self.plan))
        self.journal = {'instanceID':publisher.INSTANCE, 'status':'committed', 'planSHA256':publisher.digest(self.transaction / 'plan.json')}
        self.write(self.transaction / 'journal.json', publisher.canonical(self.journal))
        self.gate_state = {'status':'closed'}
        self.write(self.gate_transaction / 'journal.json', publisher.canonical(self.gate_state))
        self.web = types.SimpleNamespace(inspect_archive=mock.Mock(return_value=self.payload))
        self.gate_meta = {'sha256':'2'*64}
        self.gate = types.SimpleNamespace(load_transaction=mock.Mock(return_value=(self.gate_transaction, {}, b'old', b'gated')),
            VHOST=self.base / 'vhost', metadata=mock.Mock(return_value=self.gate_meta),
            gated_metadata=mock.Mock(return_value=self.gate_meta))
        self.reviewed = self.basis()

    def write(self, path, raw):
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(raw)

    def basis(self):
        return rollback.basis(publisher, self.gate, str(self.transaction), str(self.gate_transaction), '3'*64, self.web)

    def recover(self, transaction, journal, plan, web, backend):
        journal['status'] = 'rolledBack'
        publisher.private_write(transaction / 'journal.json', journal)


class CommittedRollbackTests(unittest.TestCase):
    def test_cli_help_is_offline_and_exits_successfully(self):
        result = subprocess.run([sys.executable, str(ROOT / 'deploy/production/rollback_security_release.py'), '--help'],
                                capture_output=True, timeout=20)
        self.assertEqual(result.returncode, 0)
        self.assertNotIn(b'credentialsPrinted', result.stdout)

    def test_preflight_binds_all_installed_and_original_web_payloads(self):
        f = Fixture(self)
        self.assertEqual(len(f.reviewed['installed']), 8)
        self.assertEqual(f.reviewed['containers']['storage']['id'], 'storage-instance')
        self.assertEqual(f.reviewed['plugin'], f.plugin)
        publisher.running_process_proof.assert_called_with(f.containers,
            {key:value['sha256'] for key,value in f.plan['request']['backend'].items()}, f.plan['request']['main']['sha256'])

    def test_later_binary_config_manifest_or_web_change_rejected_before_stop(self):
        for name in ('openim/bin/a', publisher.MAIN, publisher.CRON, publisher.COMPOSE, publisher.MANIFEST,
                     'gosuxin/resource/suxinweb/index.html', 'gosuxin/resource/suxinweb/new.js'):
            with self.subTest(name=name):
                f = Fixture(self)
                f.write(f.base / name, b'foreign change')
                with mock.patch.object(publisher, 'recover_transaction') as recover:
                    with self.assertRaises(RuntimeError):
                        rollback.apply(publisher, f.gate, f.web, None, f.reviewed)
                    recover.assert_not_called()
                self.assertEqual(publisher.json_file(f.transaction / 'journal.json')['status'], 'committed')

    def test_gate_open_or_changed_current_vhost_blocks_rollback(self):
        f = Fixture(self)
        f.write(f.gate_transaction / 'journal.json', publisher.canonical({'status':'released'}))
        with self.assertRaisesRegex(RuntimeError, 'closed_maintenance'):
            f.basis()
        f.write(f.gate_transaction / 'journal.json', publisher.canonical({'status':'closed'}))
        f.gate.metadata.return_value = {'sha256':'changed'}
        with self.assertRaisesRegex(RuntimeError, 'closed_maintenance'):
            f.basis()

    def test_original_backup_or_receipt_corruption_blocks_compensation(self):
        for suffix in ('previous/' + publisher.MAIN, 'web/index.html', 'web/release.json'):
            with self.subTest(suffix=suffix):
                f = Fixture(self)
                f.write(f.transaction / suffix, b'corrupt')
                with self.assertRaises((RuntimeError, ValueError)):
                    f.basis()

    def test_plan_changed_after_review_rejected_without_journal_mutation(self):
        f = Fixture(self)
        f.reviewed['publisherSHA256'] = '0'*64
        with mock.patch.object(publisher, 'recover_transaction') as recover:
            with self.assertRaisesRegex(RuntimeError, 'reviewed_rollback_plan_changed'):
                rollback.apply(publisher, f.gate, f.web, None, f.reviewed)
            recover.assert_not_called()

    def test_explicit_apply_records_intent_then_uses_original_compensation(self):
        f = Fixture(self)
        def recover(*args):
            self.assertEqual(publisher.json_file(f.transaction / 'journal.json')['status'], 'rollbackIntent')
            self.assertEqual(publisher.json_file(f.transaction / 'committed-rollback.json'), f.reviewed)
            f.recover(*args)
        with mock.patch.object(publisher, 'recover_transaction', side_effect=recover) as call:
            self.assertTrue(rollback.apply(publisher, f.gate, f.web, None, f.reviewed)['rolledBack'])
            call.assert_called_once()

    def test_interrupted_preintent_record_is_reusable_only_if_identical(self):
        f = Fixture(self)
        publisher.private_write(f.transaction / 'committed-rollback.json', f.reviewed, True)
        with mock.patch.object(publisher, 'recover_transaction', side_effect=f.recover):
            self.assertTrue(rollback.apply(publisher, f.gate, f.web, None, f.reviewed)['rolledBack'])

    def test_failed_compensation_does_not_report_success_or_open_ingress(self):
        f = Fixture(self)
        def fail(*args):
            self.assertEqual(publisher.json_file(f.transaction / 'journal.json')['status'], 'rollbackIntent')
            raise RuntimeError('synthetic_recovery_failure')
        with mock.patch.object(publisher, 'recover_transaction', side_effect=fail):
            with self.assertRaisesRegex(RuntimeError, 'synthetic_recovery_failure'):
                rollback.apply(publisher, f.gate, f.web, None, f.reviewed)
        self.assertEqual(publisher.json_file(f.gate_transaction / 'journal.json')['status'], 'closed')

    def test_success_flag_requires_gate_still_closed_after_compensation(self):
        f = Fixture(self)
        def recover(*args):
            f.recover(*args)
            f.write(f.gate_transaction / 'journal.json', publisher.canonical({'status':'released'}))
        with mock.patch.object(publisher, 'recover_transaction', side_effect=recover):
            with self.assertRaisesRegex(RuntimeError, 'closed_maintenance'):
                rollback.apply(publisher, f.gate, f.web, None, f.reviewed)
        self.assertEqual(publisher.json_file(f.transaction / 'journal.json')['status'], 'rolledBack')

    def test_unrelated_transaction_or_terminal_release_rejected(self):
        f = Fixture(self)
        with self.assertRaisesRegex(RuntimeError, 'transaction_invalid'):
            rollback.transaction_path(publisher, str(f.base / 'other'))
        f.write(f.transaction / 'journal.json', publisher.canonical({**f.journal,'status':'rolledBack'}))
        with self.assertRaisesRegex(RuntimeError, 'exact_committed'):
            f.basis()


if __name__ == '__main__':
    unittest.main()
