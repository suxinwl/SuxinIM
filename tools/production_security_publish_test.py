"""Offline isolated release/compensation tests. No real Docker, supervisor or credentials."""
import copy
import importlib.util
import json
import os
import pathlib
import shutil
import tempfile
import time
import types
import unittest
from unittest import mock

import yaml

ROOT = pathlib.Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location('security_publish', ROOT / 'deploy/production/publish_security_policy.py')
pub = importlib.util.module_from_spec(spec)
spec.loader.exec_module(pub)
ACTUAL_OWNED = pub.owned
ACTUAL_VOLUME_GUARD = pub.volume_guard
ELF = b'\x7fELF\x02\x01' + bytes(12) + b'\x3e\x00' + b'SYNTHETIC ONLY'


def compose():
    return {'name': pub.PROJECT, 'services': {name: {'environment': {'UNCHANGED': 'SYNTHETIC_SECRET'},
        'volumes': [name + '-data:' + pub.DATA_DESTINATIONS[name]] if name in pub.STORAGE - {'livekit'} else [],
        'networks': ['unchanged']} for name in set(pub.BINARIES.values()) | pub.STORAGE},
        'volumes': {name + '-data': {} for name in pub.STORAGE - {'livekit'}},
        'networks': {'unchanged': {'name': 'unchanged'}}}


class Fixture:
    def __init__(self, case):
        self.temporary = tempfile.TemporaryDirectory(prefix='puty-publish-unit-')
        case.addCleanup(self.temporary.cleanup)
        self.base = pathlib.Path(self.temporary.name).resolve()
        self.events, self.fail, self.running = [], None, True
        self.starts = 0
        self.files = {}
        for name in ('staging', 'backups', 'gosuxin/resource/suxinweb'):
            (self.base / name).mkdir(parents=True, exist_ok=True)
        self.write(pub.CRON, yaml.safe_dump({'retainChatRecords': 30, 'fileExpireTime': 30,
                   'deleteObjectType': ['file'], 'other': 'UNCHANGED'}).encode())
        self.write(pub.COMPOSE, yaml.safe_dump(compose()).encode())
        self.write(pub.MAIN, ELF + b'OLD_MAIN')
        self.write(pub.WEB + '/index.html', b'OLD_INDEX')
        self.write(pub.MANIFEST, pub.canonical({name: {'sha256': pub.sha(ELF + name.encode()),
            'bytes': len(ELF + name.encode())} for name in pub.BINARIES}))
        for name in pub.BINARIES:
            self.write('openim/bin/' + name, ELF + name.encode())
        package = 'a' * 64
        self.write('gosuxin/storage/plugins/biz-suite/active.json', pub.canonical({'package': package}))
        self.write('gosuxin/storage/plugins/biz-suite/packages/' + package + '/plugin.json', b'{"name":"biz-suite"}')
        self.write('gosuxin/storage/plugins/biz-suite/packages/' + package + '/bin/biz-suite', b'SYNTHETIC_PLUGIN')
        self.values = {name: {'Id': name + '-id', 'Image': name + '-image', 'Config': {'Env': []},
                             'State': {'Running': True}} for name in set(pub.BINARIES.values()) | pub.STORAGE}
        for binary, service in pub.BINARIES.items():
            self.values[service]['Config']['Entrypoint'] = ['/app/bin/' + binary]
            self.values[service]['State']['Pid'] = 1234
        self.volumes = {name: {'name': pub.PROJECT + '_' + name + '-data', 'source': '/owned/' + name,
                              'destination': pub.DATA_DESTINATIONS[name]}
                        for name in pub.STORAGE - {'livekit'}}
        self.request = {'format': 'puty-security-release-1', 'backend': {}, 'main': {},
                        'web': {'path': 'web.zip', 'sha256': pub.sha(b'SYNTHETIC_ARCHIVE'),
                                'indexSHA256': pub.sha(b'NEW_INDEX')},
                        'backupReceipt': {'path': 'backup-proof.json', 'sha256': ''}}
        for name in [*pub.BINARIES, 'main']:
            raw = ELF + b'NEW_' + name.encode()
            path = name + '.next'
            self.write('staging/' + path, raw)
            entry = {'path': path, 'sha256': pub.sha(raw), 'bytes': len(raw)}
            if name == 'main': self.request['main'] = entry
            else: self.request['backend'][name] = entry
        self.write('staging/web.zip', b'SYNTHETIC_ARCHIVE')
        proof = {'format': 'puty-security-backup-proof-1', 'instanceID': pub.INSTANCE,
            'archiveID': 'synthetic-archive', 'archiveSHA256': 'a' * 64, 'archiveBytes': 100,
            'manifestSHA256': 'b' * 64, 'restoreCheckSHA256': 'c' * 64,
            'verifiedAt': int(time.time()), 'consistency': 'all-suite-writers-and-storage-stopped',
            'restorePassed': True, 'cleanupVerified': True, 'archivePresentAndHashedLocally': True, 'keyIncluded': False}
        self.write('staging/backup-proof.json', pub.canonical(proof))
        self.request['backupReceipt']['sha256'] = pub.sha(pub.canonical(proof))
        self.web = types.SimpleNamespace(inspect_archive=self.inspect_archive, publish=self.publish_web,
                                         atomic_write=self.atomic_write)
        self.backend = types.SimpleNamespace(atomic_copy=self.atomic_copy)
        patches = [mock.patch.object(pub, 'BASE', self.base), mock.patch.object(pub, 'owner_guard'),
            mock.patch.object(pub, 'regular', side_effect=self.regular),
            mock.patch.object(pub, 'owned', side_effect=lambda *args: copy.deepcopy(self.values)),
            mock.patch.object(pub, 'volume_guard', side_effect=lambda *_: copy.deepcopy(self.volumes)),
            mock.patch.object(pub, 'supervisor_status', return_value='RUNNING'),
            mock.patch.object(pub, 'process_proof', side_effect=lambda pid, expected:
                {'pid': pid, 'startTime': 'synthetic', 'sha256': expected}),
            mock.patch.object(pub, 'hrm_process_proof', side_effect=lambda expected:
                {'pid': 1234, 'startTime': 'synthetic', 'sha256': expected}),
            mock.patch.object(pub, 'stop_writers', side_effect=self.stop),
            mock.patch.object(pub, 'start_writers', side_effect=self.start)]
        for patch in patches:
            patch.start()
            case.addCleanup(patch.stop)
        self.plan = pub.build_plan(self.request, pub.sha(pub.canonical(self.request)), self.web)
        self.old = {name: (self.base / name).read_bytes() for name in self.plan['current']}

    def write(self, name, raw):
        target = self.base / name
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_bytes(raw)

    def regular(self, path, root_owned=True):
        if path.resolve() != path or not path.is_file() or path.is_symlink():
            raise RuntimeError('synthetic_regular_rejected')
        return path

    def atomic_write(self, target, raw, mode, uid=None, gid=None):
        self.events.append(('write', str(target.relative_to(self.base))))
        if callable(self.fail) and self.fail(target):
            raise RuntimeError('synthetic_write_failure')
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_bytes(raw)
        os.chmod(target, mode)

    def atomic_copy(self, source, target, expected, mode, uid=None, gid=None):
        if pub.digest(source) != expected:
            raise RuntimeError('synthetic_copy_hash_failure')
        self.atomic_write(target, source.read_bytes(), mode, uid, gid)

    def inspect_archive(self, path, expected, index, application):
        if pub.digest(path) != expected or index != pub.sha(b'NEW_INDEX') or application != 'hrm':
            raise RuntimeError('synthetic_archive_invalid')
        return {'assets/new.js': b'NEW_JS', 'index.html': b'NEW_INDEX'}

    def publish_web(self, target, payload, backup, uid, gid):
        backup.mkdir()
        previous, metadata, created = {}, {}, []
        for name in payload:
            path = target / name
            if path.exists():
                previous[name] = pub.sha(path.read_bytes())
                value = path.stat()
                metadata[name] = [value.st_mode & 0o777, value.st_uid, value.st_gid]
                saved = backup / name
                saved.parent.mkdir(parents=True, exist_ok=True)
                shutil.copyfile(path, saved)
            else: created.append(name)
        (backup / 'release.json').write_bytes(pub.canonical({'newFiles': {n: pub.sha(v) for n, v in payload.items()},
            'previousFiles': previous, 'previousMetadata': metadata, 'createdFiles': created}))
        for name, raw in payload.items(): self.atomic_write(target / name, raw, 0o666)
        if self.fail == 'web': raise RuntimeError('synthetic_after_web_index_failure')

    def stop(self):
        self.events.append(('stop', None))
        self.running = False

    def start(self, backend):
        self.starts += 1
        self.events.append(('start', None))
        if self.fail == 'start' and self.starts == 1:
            raise RuntimeError('synthetic_start_failure')
        spec = yaml.safe_load((self.base / pub.COMPOSE).read_bytes())['services']['enterprise-service']
        self.values['enterprise-service']['Mounts'] = [
            {'Type': 'volume', 'Name': self.volumes[name]['name'], 'Source': self.volumes[name]['source'],
             'Destination': '/capacity/' + name, 'RW': False}
            for name in ('mongo', 'minio') if name + '-data:/capacity/' + name + ':ro' in spec['volumes']]
        self.values['enterprise-service']['Config']['Env'] = [key + '=' + value for key, value in spec['environment'].items()]
        self.running = True

    def assert_restored(self, case):
        for name, raw in self.old.items(): case.assertEqual((self.base / name).read_bytes(), raw, name)
        case.assertFalse((self.base / pub.WEB / 'assets/new.js').exists())
        case.assertTrue(self.running)
        case.assertEqual(pub.plugin_proof(), self.plan['plugin'])


class PublisherTests(unittest.TestCase):
    def test_candidate_requires_exact_seventeen_files(self):
        f = Fixture(self)
        bad = copy.deepcopy(f.request)
        bad['backend'].pop('openim-push')
        with self.assertRaisesRegex(RuntimeError, '17'):
            pub.request_sources(bad, f.web)
        self.assertEqual(f.events, [])

    def test_configuration_delta_preserves_all_unrelated_scope(self):
        source = compose()
        source['services']['enterprise-service']['environment']['STORAGE_CAPACITY_DATA_VERIFIED'] = 'true'
        cron, raw = pub.proposed_configs(b'retainChatRecords: 30\nfileExpireTime: 30\ndeleteObjectType: [file]\nother: keep\n', yaml.safe_dump(source).encode())
        after = yaml.safe_load(raw)
        self.assertEqual(yaml.safe_load(cron), {'retainChatRecords': 0, 'fileExpireTime': 0, 'deleteObjectType': [], 'other': 'keep'})
        self.assertEqual(after['volumes'], source['volumes'])
        self.assertEqual(after['networks'], source['networks'])
        for name in pub.STORAGE: self.assertEqual(after['services'][name], source['services'][name])
        self.assertEqual(after['services']['enterprise-service']['environment'],
            {'UNCHANGED': 'SYNTHETIC_SECRET', 'STORAGE_CAPACITY_DATA_VERIFIED': 'false'})
        self.assertEqual(len(after['services']['enterprise-service']['volumes']), 2)

    def test_existing_conflicting_capacity_target_is_rejected(self):
        value = compose()
        value['services']['enterprise-service']['volumes'] = ['foreign:/capacity/mongo:rw']
        with self.assertRaisesRegex(RuntimeError, 'mismatch'):
            pub.proposed_configs(b'retainChatRecords: 0\nfileExpireTime: 0\ndeleteObjectType: []', yaml.safe_dump(value).encode())

    def test_plan_contains_hashes_not_raw_configuration(self):
        f = Fixture(self)
        self.assertNotIn('SYNTHETIC_SECRET', pub.canonical(f.plan).decode())
        self.assertEqual(len(f.plan['containers']), 24)
        self.assertEqual(set(f.plan['request']['backend']), set(pub.BINARIES))

    def test_success_retains_plugin_and_never_targets_storage_or_livekit(self):
        f = Fixture(self)
        result = pub.apply(f.plan, f.web, f.backend)
        self.assertTrue(result['committed'])
        self.assertEqual(pub.plugin_proof(), f.plan['plugin'])
        self.assertEqual((f.base / pub.WEB / 'index.html').read_bytes(), b'NEW_INDEX')
        self.assertEqual(f.events.count(('stop', None)), 1)
        self.assertTrue(f.running)

    def test_partial_binary_failure_restores_every_original_and_stops_before_rollback(self):
        f = Fixture(self)
        failed = False
        def fail(path):
            nonlocal failed
            if not failed and path == f.base / 'openim/bin/openim-rpc-msg':
                failed = True
                return True
            return False
        f.fail = fail
        with self.assertRaisesRegex(RuntimeError, 'verified_rollback'):
            pub.apply(f.plan, f.web, f.backend)
        f.assert_restored(self)
        self.assertEqual(f.events.count(('stop', None)), 2)

    def test_web_failure_after_index_replacement_removes_only_created_files(self):
        f = Fixture(self)
        f.fail = 'web'
        with self.assertRaisesRegex(RuntimeError, 'verified_rollback'):
            pub.apply(f.plan, f.web, f.backend)
        f.assert_restored(self)

    def test_new_readiness_failure_restarts_verified_previous_release(self):
        f = Fixture(self)
        f.fail = 'start'
        with self.assertRaisesRegex(RuntimeError, 'verified_rollback'):
            pub.apply(f.plan, f.web, f.backend)
        f.assert_restored(self)
        self.assertEqual(f.starts, 2)

    def test_rollback_write_failure_leaves_writers_stopped_and_recovery_journal(self):
        f = Fixture(self)
        f.fail = lambda path: path == f.base / 'openim/bin/openim-api'
        with self.assertRaisesRegex(RuntimeError, 'rollback_incomplete'):
            pub.apply(f.plan, f.web, f.backend)
        self.assertFalse(f.running)
        journal = next((f.base / 'backups').glob('security-policy-*/journal.json'))
        self.assertEqual(json.loads(journal.read_bytes())['status'], 'recoveryRequired')

    def test_source_or_plugin_changes_after_plan_reject_before_stopping(self):
        f = Fixture(self)
        (f.base / 'staging/main.next').write_bytes(ELF + b'TAMPER')
        with self.assertRaisesRegex(RuntimeError, 'hash'):
            pub.apply(f.plan, f.web, f.backend)
        self.assertEqual(f.events, [])

    def test_changed_worker_plan_is_rejected_before_apply_or_recovery_stops(self):
        f = Fixture(self)
        f.plan['publisherSHA256'] = '0' * 64
        with self.assertRaisesRegex(RuntimeError, 'plan_state_changed'):
            pub.apply(f.plan, f.web, f.backend)
        with self.assertRaisesRegex(RuntimeError, 'publisher_source_changed'):
            pub.recover_transaction(f.base / 'backups/unused', {}, f.plan, f.web, f.backend)
        self.assertEqual(f.events, [])

    def test_exact_reviewed_legacy_worker_can_only_roll_back_not_apply(self):
        f = Fixture(self)
        f.plan['publisherSHA256'] = pub.LEGACY_RECOVERY_PUBLISHER_SHA256
        with self.assertRaisesRegex(RuntimeError, 'plan_state_changed'):
            pub.apply(f.plan, f.web, f.backend)
        self.assertEqual(f.events, [])
        transaction = f.base / 'backups/security-policy-' / 'unused'
        journal = {'status': 'recoveryRequired'}
        with mock.patch.object(pub, 'rollback') as rollback, \
             mock.patch.object(pub, 'private_write') as private_write:
            pub.recover_transaction(transaction, journal, f.plan, f.web, f.backend)
        rollback.assert_called_once_with(transaction, f.plan, f.web, f.backend)
        private_write.assert_called_once_with(transaction / 'journal.json', {'status': 'rolledBack'})
        self.assertEqual(journal['status'], 'rolledBack')
        self.assertEqual(f.events, [])

    def test_recovery_legacy_exception_rejects_missing_random_and_nearby_pins_before_action(self):
        f = Fixture(self)
        for pin in (None, '0' * 64, 'f' * 64, pub.LEGACY_RECOVERY_PUBLISHER_SHA256[:-1] + '0'):
            with self.subTest(pin=pin):
                f.plan['publisherSHA256'] = pin
                with mock.patch.object(pub, 'rollback') as rollback, \
                     mock.patch.object(pub, 'private_write') as private_write:
                    with self.assertRaisesRegex(RuntimeError, 'publisher_source_changed'):
                        pub.recover_transaction(f.base / 'backups/unused', {}, f.plan, f.web, f.backend)
                rollback.assert_not_called()
                private_write.assert_not_called()
        self.assertEqual(f.events, [])

    def test_backup_receipt_matches_real_archive_and_restore_report_and_expires(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = pathlib.Path(temporary).resolve()
            archive, manifest, restored = [root / name for name in ('archive.putybak', 'manifest.json', 'restore.json')]
            archive.write_bytes(b'SYNTHETIC ENCRYPTED ARCHIVE')
            value = {'sourceInstanceID': pub.INSTANCE, 'sourceArchiveID': 'synthetic-archive',
                'ciphertextSha256': pub.digest(archive), 'ciphertextBytes': archive.stat().st_size,
                'consistency': 'all-suite-writers-and-storage-stopped',
                'recoveryKeyStoredOnServer': False, 'recoveryKeyIncludedInArchive': False}
            manifest.write_bytes(pub.canonical(value))
            restored.write_bytes(pub.canonical({'sourceArchiveID': value['sourceArchiveID'],
                'ciphertextSha256': value['ciphertextSha256'], 'passed': True, 'cleanupVerified': True}))
            receipt = pub.backup_receipt(archive, manifest, restored)
            pub.verify_receipt(receipt)
            receipt['verifiedAt'] -= 86401
            with self.assertRaisesRegex(RuntimeError, 'expired'): pub.verify_receipt(receipt)
            archive.write_bytes(b'CHANGED')
            with self.assertRaisesRegex(RuntimeError, 'mismatch'):
                pub.backup_receipt(archive, manifest, restored)

    def test_hrm_starting_waits_without_duplicate_start(self):
        with mock.patch.object(pub, 'supervisor_status', side_effect=['STARTING', 'RUNNING']), \
             mock.patch.object(pub, 'ready', return_value=True), mock.patch.object(pub, 'execute') as execute:
            pub.hrm_start()
            execute.assert_not_called()

    def test_actual_owned_guard_allows_missing_app_only_for_recovery(self):
        f = Fixture(self)
        values = list(copy.deepcopy(f.values).values())
        reverse = {service: binary for binary, service in pub.BINARIES.items()}
        for row in values:
            service = row['Id'].removesuffix('-id')
            row['Config']['Labels'] = {'com.docker.compose.service': service,
                'com.docker.compose.project': pub.PROJECT, 'com.puty.suite': pub.INSTANCE,
                'com.puty.environment': 'production'}
            row['Mounts'] = [] if service not in reverse else [{'Type': 'bind',
                'Source': str(f.base / 'openim/bin' / reverse[service]),
                'Destination': '/app/bin/' + reverse[service], 'RW': False}]
        def execute(argv, capture=False, timeout=180):
            return ('\n'.join(row['Id'] for row in values).encode() if argv[1] == 'ps'
                    else json.dumps(values).encode())
        with mock.patch.object(pub, 'execute', side_effect=execute):
            self.assertEqual(len(ACTUAL_OWNED()), 24)
            values[:] = [row for row in values if row['Id'] != 'openim-api-id']
            with self.assertRaisesRegex(RuntimeError, '24'): ACTUAL_OWNED(False)
            self.assertEqual(len(ACTUAL_OWNED(False, True)), 23)
            values[:] = [row for row in values if row['Id'] != 'mongo-id']
            with self.assertRaisesRegex(RuntimeError, 'service_set'): ACTUAL_OWNED(False, True)

    def test_volume_guard_accepts_only_logical_owned_data_not_anonymous_configdb(self):
        f = Fixture(self)
        containers, volumes = {}, {}
        for name in pub.STORAGE - {'livekit'}:
            containers[name] = {'Mounts': [{'Type': 'volume', 'Name': name, 'RW': True,
                'Source': '/owned/' + name, 'Destination': pub.DATA_DESTINATIONS[name]}]}
            volumes[name] = [{'Driver': 'local', 'Scope': 'local', 'Mountpoint': '/owned/' + name,
                'Labels': {'com.puty.suite': pub.INSTANCE, 'com.docker.compose.project': pub.PROJECT,
                           'com.docker.compose.volume': name + '-data'}}]
        containers['mongo']['Mounts'].append({'Type': 'volume', 'Name': 'anonymous-configdb'})
        volumes['anonymous-configdb'] = [{'Labels': {}}]
        with mock.patch.object(pub, 'execute', side_effect=lambda argv, capture=False:
                               json.dumps(volumes[argv[-1]]).encode()):
            self.assertEqual(len(ACTUAL_VOLUME_GUARD(containers)), 6)
            containers['mongo']['Mounts'][0]['Destination'] = '/wrong-data'
            with self.assertRaisesRegex(RuntimeError, 'foreign'): ACTUAL_VOLUME_GUARD(containers)
            containers['mongo']['Mounts'][0]['Destination'] = pub.DATA_DESTINATIONS['mongo']
            volumes['mongo'][0]['Labels']['com.puty.suite'] = 'foreign'
            with self.assertRaisesRegex(RuntimeError, 'foreign'): ACTUAL_VOLUME_GUARD(containers)

    def test_recovery_checks_plugin_volumes_and_storage_identity_before_any_write(self):
        for kind in ('plugin', 'volume', 'storage'):
            with self.subTest(kind=kind):
                f = Fixture(self)
                if kind == 'plugin':
                    active = f.base / 'gosuxin/storage/plugins/biz-suite/active.json'
                    active.write_bytes(pub.canonical({'package': 'b' * 64}))
                elif kind == 'volume':
                    f.volumes['mongo'] = {**f.volumes['mongo'], 'source': '/foreign'}
                else:
                    f.values['mysql']['Id'] = 'recreated-storage'
                with self.assertRaises(RuntimeError):
                    pub.rollback(f.base / 'backups/unused', f.plan, f.web, f.backend)
                self.assertEqual(f.events, [])

    def test_recover_metadata_failure_after_start_stops_writers_again(self):
        f = Fixture(self)
        f.fail = lambda path: path == f.base / 'openim/bin/openim-api'
        with self.assertRaisesRegex(RuntimeError, 'rollback_incomplete'):
            pub.apply(f.plan, f.web, f.backend)
        transaction = next((f.base / 'backups').glob('security-policy-*'))
        journal = json.loads((transaction / 'journal.json').read_bytes())
        f.fail = None
        original = pub.metadata
        def metadata(path, root_owned=True):
            value = original(path, root_owned)
            if f.running and path == f.base / pub.MAIN:
                value['sha256'] = '0' * 64
            return value
        with mock.patch.object(pub, 'metadata', side_effect=metadata):
            with self.assertRaisesRegex(RuntimeError, 'recovery_incomplete'):
                pub.recover_transaction(transaction, journal, f.plan, f.web, f.backend)
        self.assertFalse(f.running)
        self.assertEqual(json.loads((transaction / 'journal.json').read_bytes())['status'], 'recoveryRequired')

    def test_recover_journal_write_failure_still_stops_writers(self):
        f = Fixture(self)
        journal = {'status': 'starting'}
        def failed_rollback(*args):
            f.start(f.backend)
            raise RuntimeError('synthetic_after_start')
        with mock.patch.object(pub, 'rollback', side_effect=failed_rollback), \
             mock.patch.object(pub, 'private_write', side_effect=OSError('synthetic_disk_full')):
            with self.assertRaisesRegex(RuntimeError, 'recovery_incomplete'):
                pub.recover_transaction(f.base / 'backups/unused', journal, f.plan, f.web, f.backend)
        self.assertFalse(f.running)

    def test_capacity_proof_requires_exact_owned_readonly_mounts_and_false_flag(self):
        f = Fixture(self)
        pub.apply(f.plan, f.web, f.backend)
        pub.capacity_mount_proof(f.values, f.volumes)
        f.values['enterprise-service']['Mounts'][0]['RW'] = True
        with self.assertRaisesRegex(RuntimeError, 'capacity_mount'):
            pub.capacity_mount_proof(f.values, f.volumes)
        f.values['enterprise-service']['Mounts'][0]['RW'] = False
        f.values['enterprise-service']['Config']['Env'].append('STORAGE_CAPACITY_DATA_VERIFIED=true')
        with self.assertRaisesRegex(RuntimeError, 'flag'):
            pub.capacity_mount_proof(f.values, f.volumes)


class ReadinessContractTests(unittest.TestCase):
    def probe(self, value, status=200, url='http://127.0.0.1:18090/enterprise/v1/readyz', raw=None):
        response = mock.MagicMock()
        response.status = status
        response.read.return_value = json.dumps(value).encode() if raw is None else raw
        response.__enter__.return_value = response
        opener = mock.Mock()
        opener.open.return_value = response
        with mock.patch.object(pub.urllib.request, 'build_opener', return_value=opener):
            result = pub.ready(url)
        opener.open.assert_called_once_with(url, timeout=3)
        response.read.assert_called_once_with(1 << 20)
        return result

    def test_readyz_accepts_only_the_actual_success_envelope(self):
        self.assertTrue(self.probe({'data': {'status': 'ready'}, 'errCode': 0,
                                   'errDlt': '', 'errMsg': ''}))

    def test_readyz_rejects_failed_partial_and_malformed_envelopes(self):
        values = [None, [], 'ready', {'status': 'ready'}, {'data': {'status': 'ready'}},
                  {'errCode': 1, 'data': {'status': 'ready'}},
                  {'errCode': '0', 'data': {'status': 'ready'}},
                  {'errCode': False, 'data': {'status': 'ready'}},
                  {'errCode': 0, 'data': None}, {'errCode': 0, 'data': []},
                  {'errCode': 0, 'data': {}}, {'errCode': 0, 'data': {'status': 'starting'}},
                  {'errCode': 0, 'data': {'status': True}}]
        for value in values:
            with self.subTest(value=value):
                self.assertFalse(self.probe(value))
        self.assertFalse(self.probe({'errCode': 0, 'data': {'status': 'ready'}}, status=503))
        self.assertFalse(self.probe(None, raw=b'{malformed'))

    def test_readyz_transport_failure_stays_unready(self):
        opener = mock.Mock()
        opener.open.side_effect = OSError('synthetic dependency unavailable')
        with mock.patch.object(pub.urllib.request, 'build_opener', return_value=opener):
            self.assertFalse(pub.ready('http://127.0.0.1:18090/enterprise/v1/readyz'))

    def test_hrm_captcha_readiness_contract_is_unchanged(self):
        url = 'http://127.0.0.1:18000/common/im-sso/captcha'
        for value in ({'errCode': 0, 'data': {'show': False}},
                      {'code': 0, 'data': {'enabled': True}}):
            self.assertTrue(self.probe(value, url=url))
        for value in ({'errCode': 0, 'data': {'status': 'ready'}},
                      {'errCode': 1, 'data': {'show': True}},
                      {'errCode': 0, 'data': {'show': 'true'}}):
            self.assertFalse(self.probe(value, url=url))


if __name__ == '__main__':
    unittest.main()
