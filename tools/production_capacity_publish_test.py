import copy
import contextlib
import importlib.util
import io
import json
import os
import pathlib
import sys
import tempfile
import types
import unittest
from unittest import mock
import yaml

ROOT = pathlib.Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location('capacity', ROOT / 'deploy/production/publish_capacity_verification.py')
capacity = importlib.util.module_from_spec(spec)
spec.loader.exec_module(capacity)


class PlanOutputTests(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.base = pathlib.Path(temporary.name).resolve()
        (self.base / 'staging/security').mkdir(parents=True)
        (self.base / '.backup-stage').mkdir()
        (self.base / '.backup-stage/backup.lock').write_bytes(b'SYNTHETIC LOCK')
        source = ROOT / 'deploy/production/publish_security_policy.py'
        spec = importlib.util.spec_from_file_location('capacity_actual_stage_worker', source)
        self.publisher = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(self.publisher)
        self.publisher.BASE = self.base
        self.worker = types.SimpleNamespace(
            BASE=self.base, owner_guard=lambda: None, regular=lambda p: p,
            # These are the actual publisher functions, not stage-file mocks.
            stage_file=self.publisher.stage_file, json_file=self.publisher.json_file,
            private_write=self.publisher.private_write, digest=self.publisher.digest)

    def cli(self, *args):
        output = io.StringIO()
        flock = types.SimpleNamespace(LOCK_EX=2, LOCK_NB=4, flock=lambda *args: None)
        signals = types.SimpleNamespace(SIGHUP=1, SIGINT=2, SIGTERM=15, signal=lambda *args: None)
        with mock.patch.object(capacity, 'load_helpers', return_value=(self.worker, object())), \
             mock.patch.object(capacity, 'basis', return_value={'format': 'SYNTHETIC CAPACITY PLAN'}), \
             mock.patch.object(sys, 'argv', [str(capacity.__file__), *args]), \
             mock.patch.dict(sys.modules, {'fcntl': flock}), \
             mock.patch.object(capacity, 'signal', signals), contextlib.redirect_stdout(output):
            capacity.main()
        return json.loads(output.getvalue())

    def test_plan_cli_writes_new_output_with_actual_existing_input_guard_and_atomic_writer(self):
        relative = 'security/capacity-plan.json'
        output = self.base / 'staging' / relative
        # This is the exact real guard that made the former plan CLI fail.
        with self.assertRaises(FileNotFoundError):
            self.worker.stage_file(relative)
        result = self.cli('plan', '--out', relative)
        self.assertTrue(result['planned'])
        self.assertEqual(json.loads(output.read_bytes()), {'format': 'SYNTHETIC CAPACITY PLAN'})
        self.assertEqual(result['planSHA256'], self.publisher.digest(output))
        if os.name == 'posix':
            self.assertEqual(output.stat().st_mode & 0o777, 0o600)
        previous = output.read_bytes()
        with self.assertRaisesRegex(RuntimeError, 'capacity_plan_already_exists'):
            self.cli('plan', '--out', relative)
        self.assertEqual(output.read_bytes(), previous)

    def test_output_rejects_unsafe_paths_missing_parent_and_existing_directory_without_creation(self):
        for relative in ('', '../escape.json', '/escape.json', 'security//new.json',
                         'security/./new.json', 'security/../new.json', 'C:/escape.json',
                         'security\\new.json', 'security/new.json\n'):
            with self.subTest(relative=relative), self.assertRaisesRegex(RuntimeError, 'capacity_output_path_invalid'):
                capacity.stage_output(self.worker, relative)
        with self.assertRaisesRegex(RuntimeError, 'capacity_output_parent_invalid'):
            self.cli('plan', '--out', 'missing/new.json')
        self.assertFalse((self.base / 'staging/missing').exists())
        with self.assertRaisesRegex(RuntimeError, 'capacity_plan_already_exists'):
            capacity.stage_output(self.worker, 'security')
        self.assertEqual(list((self.base / 'staging/security').iterdir()), [])

    def test_output_rejects_parent_and_dangling_leaf_symlinks(self):
        outside = self.base / 'outside'
        outside.mkdir()
        linked_parent = self.base / 'staging/linked'
        leaf = self.base / 'staging/security/linked.json'
        try:
            linked_parent.symlink_to(outside, target_is_directory=True)
            leaf.symlink_to(outside / 'absent.json')
        except OSError as error:
            self.skipTest('Creating synthetic symlinks is unavailable: ' + type(error).__name__)
        with self.assertRaisesRegex(RuntimeError, 'capacity_output_parent_invalid'):
            self.cli('plan', '--out', 'linked/new.json')
        with self.assertRaisesRegex(RuntimeError, 'capacity_output_parent_invalid|capacity_plan_already_exists'):
            self.cli('plan', '--out', 'security/linked.json')
        self.assertEqual(list(outside.iterdir()), [])

    def test_apply_cli_still_requires_an_existing_regular_reviewed_input(self):
        with mock.patch.object(capacity, 'apply') as apply:
            with self.assertRaises(FileNotFoundError):
                self.cli('apply', '--plan', 'security/not-created.json', '--plan-sha256', '0' * 64)
        apply.assert_not_called()
        self.assertFalse((self.base / 'staging/security/not-created.json').exists())


class CapacityTests(unittest.TestCase):
    def compose(self, value="'false'"):
        return ("name: puty-suite-prod\nservices:\n  enterprise-service:\n    environment:\n"
                "      KEEP_SECRET: keep-exactly\n      STORAGE_CAPACITY_DATA_VERIFIED: " + value +
                "\n    volumes:\n    - mongo-data:/capacity/mongo:ro\n").encode()

    def row(self, letter, flag):
        return {'Id': letter * 64, 'Image': 'sha256:fixed-image', 'State': {'Pid': 123},
                'Config': {'Hostname': letter * 12, 'Labels': {'com.docker.compose.config-hash': letter},
                           'Env': ['KEEP_SECRET=keep-exactly', capacity.FLAG + '=' + flag], 'Entrypoint': ['/app/bin/enterprise-service']},
                'HostConfig': {'PortBindings': {'8090/tcp': [{'HostIp': '127.0.0.1', 'HostPort': '18090'}]}},
                'Mounts': [{'Destination': '/capacity/mongo', 'Source': '/owned/mongo', 'RW': False}]}

    def test_exact_flag_bytes_only_and_preserve_all_other_content(self):
        for quote in ("'", '"'):
            raw = self.compose(quote + 'false' + quote)
            changed = capacity.proposed(raw)
            self.assertEqual(changed, raw.replace((quote + 'false' + quote).encode(), (quote + 'true' + quote).encode()))

    def test_reject_missing_boolean_true_duplicate_and_different_field(self):
        inputs = [self.compose('false'), self.compose("'true'"), self.compose().replace(capacity.FLAG.encode(), b'OTHER_FLAG'),
                  self.compose() + b"      STORAGE_CAPACITY_DATA_VERIFIED: 'false'\n"]
        for raw in inputs:
            with self.subTest(raw=raw), self.assertRaises((RuntimeError, yaml.YAMLError)):
                capacity.proposed(raw)

    def test_automatic_identity_and_flag_change_preserves_effective_configuration(self):
        old, new = self.row('a', 'false'), self.row('b', 'true')
        new['Config']['Labels']['com.docker.compose.replace'] = old['Id']
        self.assertEqual(capacity.immutable_container(old), capacity.immutable_container(new))
        for field in ('port', 'secret', 'image', 'explicit-hostname'):
            changed = copy.deepcopy(new)
            if field == 'port': changed['HostConfig']['PortBindings']['8090/tcp'][0]['HostPort'] = '18091'
            if field == 'secret': changed['Config']['Env'][0] = 'KEEP_SECRET=changed'
            if field == 'image': changed['Image'] = 'sha256:changed'
            if field == 'explicit-hostname': changed['Config']['Hostname'] = 'explicit-other'
            self.assertNotEqual(capacity.immutable_container(old), capacity.immutable_container(changed))

    def fixture(self, directory):
        root = pathlib.Path(directory)
        (root / 'openim').mkdir()
        (root / 'openim/bin').mkdir()
        binary = root / 'openim/bin/enterprise-service'
        binary.write_bytes(b'SYNTHETIC FIXED BINARY')
        (root / 'backups').mkdir()
        compose = root / 'openim/compose.yaml'
        compose.write_bytes(self.compose())
        state = {'row': self.row('a', 'false'), 'commands': [], 'restarts': 0}
        digest = lambda p: capacity.sha(pathlib.Path(p).read_bytes())
        canonical = lambda x: (json.dumps(x, sort_keys=True, separators=(',', ':')) + '\n').encode()
        value = {'compose': {'sha256': digest(compose), 'mode': 0o600, 'uid': 0, 'gid': 0},
                 'candidateSHA256': capacity.sha(capacity.proposed(compose.read_bytes())), 'volumes': {'owned': True}, 'plugin': {'fixed': True},
                 'enterpriseSHA256': digest(binary), 'containers': {'enterprise-service': {'id': 'a' * 64, 'image': 'sha256:fixed-image'},
                 'mongo': {'id': 'mongo-fixed', 'image': 'mongo-image'}},
                 'enterpriseConfigurationSHA256': capacity.sha(canonical(capacity.immutable_container(state['row'])))}
        def write(path, body, exclusive=False):
            if exclusive and path.exists(): raise RuntimeError('already_exists')
            path.write_bytes(canonical(body))
        worker = types.SimpleNamespace(BASE=root, COMPOSE='openim/compose.yaml', INSTANCE='synthetic-instance',
            canonical=canonical, digest=digest, regular=lambda p:p, owner_guard=lambda:None, sync_directory=lambda p:None,
            private_write=write, compose_command=lambda:['docker','compose'], execute=lambda argv:state['commands'].append(argv),
            volume_guard=lambda c:value['volumes'], plugin_proof=lambda:value['plugin'],
            process_proof=lambda pid, expected:self.assertEqual(expected,value['enterpriseSHA256']),
            capacity_mount_proof=lambda rows,volumes:self.assertEqual(rows['enterprise-service']['Config']['Env'][-1],capacity.FLAG+'=false'),
            metadata=lambda p:{**value['compose'],'sha256':digest(p)},
            owned=lambda *args:{**({'enterprise-service':state['row']} if state['row'] is not None else {}),
                                'mongo':{'Id':'mongo-fixed','Image':'mongo-image'}})
        auditor = types.SimpleNamespace(capacity_summary=lambda c:[{'kind':name,'ownedVolume':True,'capacityAliasReadonly':True,
                    'operatorVerified':state['row']['Config']['Env'][-1].endswith('=true')} for name in ('mongo','minio')])
        def install(w, raw, metadata): compose.write_bytes(raw)
        def restart(w):
            state['restarts'] += 1
            flag = 'true' if b"'true'" in compose.read_bytes() else 'false'
            state['row'] = self.row('b', flag)
        return worker, auditor, value, state, compose, install, restart

    def test_apply_changes_only_enterprise_and_commits_actual_verified_mounts(self):
        with tempfile.TemporaryDirectory() as directory:
            w,a,v,s,p,install,restart = self.fixture(directory)
            with mock.patch.object(capacity,'basis',return_value=v), mock.patch.object(capacity,'install',side_effect=install), mock.patch.object(capacity,'restart',side_effect=restart):
                result = capacity.apply(w,a,v)
            self.assertTrue(result['passed'])
            self.assertEqual(s['commands'], [['docker','compose','stop','enterprise-service']])
            self.assertEqual(json.loads((pathlib.Path(result['transaction'])/'journal.json').read_bytes())['status'],'committed')
            self.assertIn(b"'true'",p.read_bytes())

    def test_readiness_failure_rolls_back_exact_bytes_before_restarting_old_service(self):
        with tempfile.TemporaryDirectory() as directory:
            w,a,v,s,p,install,restart = self.fixture(directory)
            original = p.read_bytes()
            def fail_once(worker):
                if not s['restarts']:
                    s['restarts'] += 1
                    raise RuntimeError('synthetic_readiness_failed')
                restart(worker)
            with mock.patch.object(capacity,'basis',return_value=v), mock.patch.object(capacity,'install',side_effect=install), mock.patch.object(capacity,'restart',side_effect=fail_once):
                with self.assertRaisesRegex(RuntimeError,'capacity_failed_verified_rollback_complete'):
                    capacity.apply(w,a,v)
            self.assertEqual(p.read_bytes(), original)
            self.assertEqual(s['commands'], [['docker','compose','stop','enterprise-service']]*2)
            journal = next((pathlib.Path(directory)/'backups').glob('*/journal.json'))
            self.assertEqual(json.loads(journal.read_bytes())['status'],'rolledBack')

    def test_changed_plan_rejects_before_backup_or_stop(self):
        with tempfile.TemporaryDirectory() as directory:
            w,a,v,s,p,install,restart = self.fixture(directory)
            with mock.patch.object(capacity,'basis',return_value={**v,'unexpected':True}):
                with self.assertRaisesRegex(RuntimeError,'capacity_reviewed_plan_changed'):
                    capacity.apply(w,a,v)
            self.assertEqual(s['commands'],[])
            self.assertEqual(list((pathlib.Path(directory)/'backups').iterdir()),[])

    def test_recreate_lost_enterprise_recovers_without_touching_any_other_container(self):
        with tempfile.TemporaryDirectory() as directory:
            w,a,v,s,p,install,restart = self.fixture(directory)
            original = p.read_bytes()
            def lose_once(worker):
                if not s['restarts']:
                    s['restarts'] += 1
                    s['row'] = None
                    raise RuntimeError('synthetic_container_create_failed')
                restart(worker)
            with mock.patch.object(capacity,'basis',return_value=v), mock.patch.object(capacity,'install',side_effect=install), mock.patch.object(capacity,'restart',side_effect=lose_once):
                with self.assertRaisesRegex(RuntimeError,'capacity_failed_verified_rollback_complete'):
                    capacity.apply(w,a,v)
            self.assertEqual(p.read_bytes(),original)
            self.assertIsNotNone(s['row'])
            self.assertEqual(s['commands'],[['docker','compose','stop','enterprise-service']]*2)

    def test_recovery_rejects_missing_unrelated_service_or_changed_binary_before_writes(self):
        with tempfile.TemporaryDirectory() as directory:
            w,a,v,s,p,install,restart = self.fixture(directory)
            for rows in ({'enterprise-service':s['row']}, {}):
                with self.assertRaisesRegex(RuntimeError,'capacity_service_set_changed'):
                    capacity.unchanged(w,v,rows,allow_missing=True)
            transaction=pathlib.Path(directory)/'backups/synthetic'
            transaction.mkdir()
            (transaction/'compose.previous').write_bytes(p.read_bytes())
            (pathlib.Path(directory)/'openim/bin/enterprise-service').write_bytes(b'SYNTHETIC CHANGED BINARY')
            with mock.patch.object(capacity,'install',side_effect=install):
                with self.assertRaisesRegex(RuntimeError,'rollback_enterprise_binary_changed'):
                    capacity.rollback(w,a,transaction,v)
            self.assertEqual(s['commands'],[])


if __name__ == '__main__':
    unittest.main()
