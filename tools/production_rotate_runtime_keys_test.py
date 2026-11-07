"""Offline rotation boundaries and crash compensation; never contacts production."""
import base64
import hashlib
import importlib.util
import json
import pathlib
import tempfile
import types
import unittest
import copy
import yaml
from unittest import mock
from cryptography.hazmat.primitives.ciphers.aead import AESGCM
from cryptography.exceptions import InvalidTag

ROOT = pathlib.Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location('rotation', ROOT/'deploy/production/rotate_runtime_keys.py')
r = importlib.util.module_from_spec(spec)
spec.loader.exec_module(r)


def bundle():
    return {'format': 'puty-runtime-key-rotation-v1', 'rotationID': 'rotate-'+'a'*24,
            'instanceID': 'b'*32, 'startAt': '2026-11-02T16:00:00Z',
            'old': {name: hashlib.sha256(('old:'+name).encode()).hexdigest() for name in r.NAMES},
            'new': {name: hashlib.sha256(('new:'+name).encode()).hexdigest() for name in r.NAMES}}


def document(key, identity='fixture', kind='hrm_generation', completed=False):
    clear = b'{"UserID":"synthetic","IMToken":"synthetic-only","unknown":12345678901234567890}'
    nonce = bytes(range(12))
    encrypted = nonce+AESGCM(bytes.fromhex(key)).encrypt(nonce, clear, r.AAD)
    content = (b'{ "Ciphertext" : "'+base64.b64encode(encrypted)
               +b'", "Complete":'+(b'true' if completed else b'false')
               +b', "Epoch":9223372036854775807, "UnknownNumber":1.2300e+12 }')
    doc = {'_id': identity, 'kind': kind, 'owner': 'synthetic',
           'version': {'$numberLong': '9223372036854775807'},
           'updated': {'$date': {'$numberLong': '1790000000000'}},
           'data': {'$binary': {'base64': base64.b64encode(content).decode(), 'subType': '00'}},
           'unknownMetadata': {'$numberDecimal': '1.200000000000000001'}}
    return json.dumps(doc), clear, content


class RotationBoundaries(unittest.TestCase):
    def test_private_atomic_write_syncs_parent_after_replace(self):
        events = []
        with tempfile.TemporaryDirectory() as folder:
            path = pathlib.Path(folder).resolve()/'journal.json'; replace = r.os.replace
            def renamed(source, target):
                replace(source, target); events.append('replace')
            with mock.patch.object(r.os, 'replace', side_effect=renamed), \
                 mock.patch.object(r, 'sync_parent_directory', side_effect=lambda _: events.append('parent-fsync')):
                r.save(path, {'status': 'committed'})
            self.assertEqual(events, ['replace', 'parent-fsync'])
            with mock.patch.object(r, 'sync_parent_directory', side_effect=OSError('synthetic fsync failure')):
                with self.assertRaises(OSError):
                    r.save(path, {'status': 'committed'})

    def test_POSIX_parent_sync_always_closes_descriptor(self):
        path = pathlib.Path('fixture')
        with mock.patch.object(r.os, 'name', 'posix'), mock.patch.object(r.os, 'O_DIRECTORY', 65536, create=True), \
             mock.patch.object(r.os, 'open', return_value=52) as opened, mock.patch.object(r.os, 'fsync') as sync, \
             mock.patch.object(r.os, 'close') as close:
            r.sync_parent_directory(path)
        sync.assert_called_once_with(52); close.assert_called_once_with(52)

    def test_TLS_warmup_supplies_required_operation_ID_and_strict_errors(self):
        responses = []
        for status, value in [(401, {'errCode': 1501, 'errMsg': 'Chat session expired or revoked'}),
                              (200, {'errCode': 1503, 'errMsg': 'TokenMalformedError'}),
                              (200, {'code': 0})]:
            response = mock.MagicMock(); response.status = status
            response.__enter__.return_value = response
            response.read.return_value = json.dumps(value).encode()
            responses.append(response)
        opener = mock.Mock(); opener.open.side_effect = responses
        with mock.patch.object(r.ssl, 'create_default_context'), mock.patch.object(r.urllib.request, 'build_opener', return_value=opener):
            self.assertTrue(r.warm_authentication_entry())
        self.assertEqual(opener.open.call_count, 3)
        for call in opener.open.call_args_list:
            self.assertTrue(call.args[0].get_header('Operationid'))
            self.assertTrue(call.args[0].full_url.startswith('https://192.168.10.11:9885/'))

    def test_signature_and_malformed_proofs_reject_transient_errors(self):
        self.assertTrue(r.invalid_native_rejected(200, {'errCode': 1503, 'errMsg': 'TokenMalformedError'}))
        self.assertTrue(r.unregistered_im_signature_verified(200, {'errCode': 1507, 'errMsg': 'TokenNotExistError'}))
        self.assertTrue(r.old_im_signature_rejected(200, {'errCode': 1505, 'errMsg': 'signature is invalid'}))
        for status, body in ((503, {'errCode': 1505, 'errMsg': 'signature is invalid'}),
                             (200, {'errCode': 90004, 'errMsg': 'network error'}),
                             (200, {'errCode': 1501, 'errMsg': 'TokenExpiredError'})):
            self.assertFalse(r.old_im_signature_rejected(status, body))
            self.assertFalse(r.invalid_native_rejected(status, body))
            self.assertFalse(r.unregistered_im_signature_verified(status, body))

    def test_IM_probe_is_nonadmin_unregistered_short_token_without_raw_key(self):
        key = bundle()['new']['im']
        token = r.im_key_token(key)
        self.assertNotIn(key, token)
        payload = token.split('.')[1]
        claims = json.loads(base64.urlsafe_b64decode(payload+'='*((-len(payload))%4)))
        self.assertEqual(claims['UserID'], 'puty-runtime-key-proof-unregistered')
        self.assertEqual(claims['PlatformID'], 2)
        self.assertLessEqual(claims['exp']-claims['iat'], 65)
        self.assertNotIn('secret', claims)

    def test_three_process_argument_patches_match_the_generator(self):
        spec = importlib.util.spec_from_file_location('generate_for_rotation_test', ROOT/'tools/generate_instance.py')
        generator = importlib.util.module_from_spec(spec); spec.loader.exec_module(generator)
        original = {'services': {'mongo': {'healthcheck': {'test': ['old'], 'interval': '10s'}, 'environment': {'keep': 'same'}},
                                 'redis': {'command': ['old'], 'healthcheck': {'test': ['old']}, 'volumes': ['./relative:/keep']}},
                    'networks': {'owned': {}}, 'volumes': {'owned': {}}}
        expected = generator.harden_storage_process_arguments(copy.deepcopy(original))
        self.assertEqual(r.harden_storage_process_arguments(copy.deepcopy(original)), expected)
        self.assertEqual(expected['services']['redis']['volumes'], ['./relative:/keep'])
        for key in bundle()['new'].values():
            self.assertNotIn(key, json.dumps(expected))

    def render_fixture(self, root, value, redis_db=6):
        for name in r.FILES:
            target = root/name; target.parent.mkdir(parents=True, exist_ok=True)
            target.write_text('unchanged: fixture\n')
        (root/'openim/secrets.json').write_text(json.dumps({**{k: v for k, v in value['old'].items() if k != 'bridge'}, 'chat': 'keep-independent-chat'}))
        (root/'gosuxin/runtime-environment.json').write_text(json.dumps({
            'GOSUXIN_IM_REDIS_URL': 'redis://:'+value['old']['redis']+'@127.0.0.1:16379/'+str(redis_db),
            'GOSUXIN_IM_HMAC_KEY': value['old']['bridge'], 'WECOM_CREDENTIAL_KEY': 'keep-wecom',
            'GOSUXIN_SYNC_START_AT': 'keep-cutoff'}))
        compose = {'name': r.PROJECT, 'services': {
            'mongo': {'environment': {'MONGO_INITDB_ROOT_PASSWORD': value['old']['mongo']}, 'healthcheck': {'test': ['old']}},
            'redis': {'command': ['old'], 'environment': {'REDIS_PASSWORD': value['old']['redis']}, 'healthcheck': {'test': ['old']}}},
            'networks': {'same': {}}, 'volumes': {'same': {}}}
        (root/'openim/compose.yaml').write_text(yaml.safe_dump(compose))
        return compose

    def test_render_preserves_redis_DB6_cutoff_wecom_and_independent_chat(self):
        value = bundle()
        with tempfile.TemporaryDirectory() as folder:
            root = pathlib.Path(folder).resolve(); self.render_fixture(root, value)
            historical = root/'history.json'; historical.write_text(json.dumps(value['old']))
            rows = r.render_files(root, value)
            proposed = {row['path']: base64.b64decode(row['after']) for row in rows}
            runtime = json.loads(proposed['gosuxin/runtime-environment.json'])
            self.assertTrue(runtime['GOSUXIN_IM_REDIS_URL'].endswith('/6'))
            self.assertIn(value['new']['redis'], runtime['GOSUXIN_IM_REDIS_URL'])
            self.assertEqual(runtime['WECOM_CREDENTIAL_KEY'], 'keep-wecom')
            self.assertEqual(runtime['GOSUXIN_SYNC_START_AT'], 'keep-cutoff')
            self.assertEqual(json.loads(proposed['openim/secrets.json'])['chat'], 'keep-independent-chat')
            self.assertEqual(json.loads(historical.read_text()), value['old'])
            self.assertNotIn('history.json', proposed)

    def test_wrong_HRM_redis_database_fails_before_render_is_saved(self):
        with tempfile.TemporaryDirectory() as folder:
            root = pathlib.Path(folder).resolve(); self.render_fixture(root, bundle(), redis_db=2)
            with self.assertRaisesRegex(RuntimeError, 'Redis_database'):
                r.render_files(root, bundle())

    def test_starting_HRM_is_stopped_and_polled_before_DB_changes(self):
        stopped = types.SimpleNamespace(returncode=0, stdout=b'')
        with mock.patch.object(r, 'hrm_state', side_effect=['STARTING', 'STOPPING', 'STOPPED']) as state, \
             mock.patch.object(r.subprocess, 'run', return_value=stopped) as command, \
             mock.patch.object(r, 'all_containers', return_value=[]), mock.patch.object(r.time, 'sleep'):
            r.stop_writers({'instanceID': 'fixture', 'writers': []})
        self.assertEqual(state.call_count, 3)
        command.assert_called_once()
        self.assertEqual(command.call_args.args[0][-2:], ['stop', 'puty-hrm'])

    def test_starting_HRM_is_not_started_twice(self):
        with mock.patch.object(r, 'validate_storage_ownership'), mock.patch.object(r, 'compose'), \
             mock.patch.object(r, 'wait_containers'), mock.patch.object(r, 'hrm_state', side_effect=['STARTING', 'RUNNING']), \
             mock.patch.object(r, 'execute') as command, mock.patch.object(r.time, 'sleep'):
            r.restart_services({'instanceID': 'fixture', 'writers': [], 'hrmRunning': True, 'volumes': {}})
        command.assert_not_called()

    def volume_fixture(self):
        owner = 'b'*32
        spec = {'name': r.PROJECT, 'services': {}, 'volumes': {}}
        inspected, containers = {}, []
        for service in r.STORAGE:
            logical, name = service+'-data', r.PROJECT+'_'+service+'-data'
            source = str(pathlib.Path('/var/lib/docker/volumes')/name/'_data')
            spec['volumes'][logical] = {'name': name}
            spec['services'][service] = {'volumes': [{'type': 'volume', 'source': logical, 'target': '/owned/'+service}]}
            inspected[name] = {'Name': name, 'Driver': 'local', 'Mountpoint': source, 'Labels': {'com.puty.suite': owner}}
            containers.append({'Config': {'Labels': {'com.docker.compose.service': service}},
                               'Mounts': [{'Type': 'volume', 'Name': name, 'Source': source,
                                           'Destination': '/owned/'+service, 'RW': True}]})
        return owner, spec, inspected, containers

    def test_six_volume_owner_mount_destination_and_base_bind_are_enforced(self):
        owner, spec, inspected, containers = self.volume_fixture()
        def inspect(args, **_):
            return json.dumps([inspected[args[-1]]]).encode()
        with mock.patch.object(r, 'compose', return_value=json.dumps(spec).encode()), \
             mock.patch.object(r, 'execute', side_effect=inspect), mock.patch.object(r, 'all_containers', return_value=containers), \
             mock.patch.object(pathlib.Path, 'is_dir', return_value=True), mock.patch.object(pathlib.Path, 'resolve', lambda path: path):
            good = r.validate_storage_ownership(owner)
            self.assertEqual(len(good), 6)
            containers[0]['Mounts'][0]['Destination'] = '/foreign'
            with self.assertRaisesRegex(RuntimeError, 'wrong_volume'):
                r.validate_storage_ownership(owner)
            containers[0]['Mounts'][0]['Destination'] = spec['services'][containers[0]['Config']['Labels']['com.docker.compose.service']]['volumes'][0]['target']
            inspected[next(iter(inspected))]['Labels']['com.puty.suite'] = 'foreign'
            with self.assertRaisesRegex(RuntimeError, 'foreign'):
                r.validate_storage_ownership(owner)

    def test_reviewed_volume_set_cannot_change_during_recovery(self):
        owner, spec, inspected, containers = self.volume_fixture()
        with mock.patch.object(r, 'compose', return_value=json.dumps(spec).encode()), \
             mock.patch.object(r, 'execute', side_effect=lambda args, **_: json.dumps([inspected[args[-1]]]).encode()), \
             mock.patch.object(r, 'all_containers', return_value=containers), mock.patch.object(pathlib.Path, 'is_dir', return_value=True), \
             mock.patch.object(pathlib.Path, 'resolve', lambda path: path):
            with self.assertRaisesRegex(RuntimeError, 'identity_changed'):
                r.validate_storage_ownership(owner, expected={})

    def test_exact_eight_keys_and_unique_entropy(self):
        value = bundle(); r.bundle_valid(value)
        value['new']['chat'] = '0'*64
        with self.assertRaisesRegex(RuntimeError, 'schema'):
            r.bundle_valid(value)
        value = bundle(); value['new']['redis'] = value['old']['redis']
        with self.assertRaisesRegex(RuntimeError, 'unique'):
            r.bundle_valid(value)

    def test_rewrap_preserves_all_other_BSON_and_inner_JSON_bytes(self):
        value = bundle()
        for kind in r.KINDS:
            before, clear, binary = document(value['old']['session'], kind=kind)
            row = r.rewrap_documents([before], value['old']['session'], value['new']['session'])[0]
            old_doc, new_doc = json.loads(before), json.loads(row['after'])
            old_data, new_data = old_doc.pop('data'), new_doc.pop('data')
            self.assertEqual(old_doc, new_doc)
            proposed = base64.b64decode(new_data['$binary']['base64'])
            old_cipher = json.loads(binary)['Ciphertext'].encode()
            new_cipher = json.loads(proposed)['Ciphertext'].encode()
            self.assertEqual(proposed.replace(new_cipher, old_cipher), binary)
            encrypted = base64.b64decode(new_cipher)
            self.assertEqual(AESGCM(bytes.fromhex(value['new']['session'])).decrypt(encrypted[:12], encrypted[12:], r.AAD), clear)
            with self.assertRaises(InvalidTag):
                AESGCM(bytes.fromhex(value['old']['session'])).decrypt(encrypted[:12], encrypted[12:], r.AAD)
            self.assertEqual(new_data['$binary']['subType'], old_data['$binary']['subType'])

    def test_rewrap_rejects_wrong_key_and_unapproved_kind(self):
        value = bundle(); raw, _, _ = document(value['old']['session'])
        with self.assertRaises(InvalidTag):
            r.rewrap_documents([raw], value['new']['session'], value['old']['session'])
        raw, _, _ = document(value['old']['session'], kind='business-history')
        with self.assertRaisesRegex(RuntimeError, 'kind'):
            r.rewrap_documents([raw], value['old']['session'], value['new']['session'])

    def test_rollback_rewraps_current_new_records_preserving_newest_semantics(self):
        value = bundle()
        old, _, _ = document(value['old']['session'], identity='existing-old')
        fresh, clear, binary = document(value['new']['session'], identity='created-after-startup', completed=True)
        with mock.patch.object(r, 'mongo_scan', return_value=[old, fresh]), mock.patch.object(r, 'mongo_rewrap') as update:
            r.rollback_documents('synthetic-container', value['new']['mongo'], value['old']['session'], value['new']['session'])
        rows = update.call_args.args[2]
        self.assertEqual(len(rows), 1)
        self.assertEqual(rows[0]['before'], fresh)
        after = json.loads(rows[0]['after'])
        inner = json.loads(base64.b64decode(after['data']['$binary']['base64']))
        self.assertTrue(inner['Complete'])
        self.assertEqual(inner['Epoch'], 9223372036854775807)
        encrypted = base64.b64decode(inner['Ciphertext'])
        self.assertEqual(AESGCM(bytes.fromhex(value['old']['session'])).decrypt(encrypted[:12], encrypted[12:], r.AAD), clear)

    def test_mongo_CAS_uses_canonical_BSON_and_only_sets_data(self):
        value = bundle(); before, _, _ = document(value['old']['session'])
        rows = r.rewrap_documents([before], value['old']['session'], value['new']['session'])
        with mock.patch.object(r, 'mongo', return_value={'ok': True, 'changed': 1, 'unchanged': 0}) as command:
            r.mongo_rewrap('synthetic', value['old']['mongo'], rows)
        script = command.call_args.args[2]
        self.assertIn('relaxed:false', script)
        self.assertIn('updateOne(before,{$set:{data:after.data}})', script)
        self.assertNotIn('replaceOne', script)

    def test_durable_commit_refuses_rollback_before_touching_services(self):
        with tempfile.TemporaryDirectory() as folder:
            path = pathlib.Path(folder).resolve(); r.save(path/'journal.json', {'status': 'committed'})
            with mock.patch.object(r, '_rollback') as change, mock.patch.object(r, 'stop_writers') as stop:
                with self.assertRaisesRegex(RuntimeError, 'committed'):
                    r.rollback(bundle(), {}, {'status': 'applying'}, path)
            change.assert_not_called(); stop.assert_not_called()

    def test_failed_rollback_keeps_writers_stopped(self):
        with tempfile.TemporaryDirectory() as folder:
            path = pathlib.Path(folder).resolve(); r.save(path/'journal.json', {'status': 'applying'})
            with mock.patch.object(r, '_rollback', side_effect=RuntimeError('synthetic failure')), mock.patch.object(r, 'stop_writers') as stop:
                with self.assertRaisesRegex(RuntimeError, 'kept_stopped'):
                    r.rollback(bundle(), {}, {}, path)
            stop.assert_called_once()
            self.assertEqual(json.loads((path/'journal.json').read_text())['status'], 'rollbackFailed')

    def test_recovery_starts_missing_stopped_initializing_storage_before_auth(self):
        plan = {'files': [], 'volumes': {}, 'instanceID': 'b'*32, '_newMongo': 'newm', '_oldMongo': 'oldm', '_newRedis': 'newr', '_oldRedis': 'oldr'}
        running = {'mongo': {'Id': 'm', 'State': {'Running': True, 'Health': {'Status': 'starting'}}},
                   'redis': {'Id': 'r', 'State': {'Running': True}}, 'minio': {'Id': 's', 'State': {'Running': True}}}
        stopped = {name: {'Id': item['Id'], 'State': {'Running': False}} for name, item in running.items()}
        with mock.patch.object(r, 'compose') as start, mock.patch.object(r, 'validate_storage_ownership'), mock.patch.object(r, 'current_containers', side_effect=[{}, stopped, running]), \
             mock.patch.object(r, 'mongo_auth', side_effect=lambda _, key: key == 'oldm'), \
             mock.patch.object(r, 'redis_auth', side_effect=lambda key: key == 'oldr'), mock.patch.object(r.time, 'sleep'):
            self.assertEqual(r.recover_storage_processes(plan), ('m', 'oldm', 'oldr'))
        start.assert_called_once_with('up', '-d', '--no-build', '--no-deps', *r.CHANGED_STORAGE)

    def test_HRM_stop_is_idempotent(self):
        result = types.SimpleNamespace(stdout=b'puty-hrm STOPPED', returncode=3)
        with mock.patch.object(r.subprocess, 'run', return_value=result), mock.patch.object(r, 'execute') as command, \
             mock.patch.object(r, 'all_containers', return_value=[]):
            r.stop_writers({'instanceID': 'synthetic', 'writers': []})
        command.assert_not_called()

    def test_receipt_publication_failure_after_commit_does_not_roll_back(self):
        value = bundle()
        plan = {'bundleSHA256': r.sha(r.canonical(value)), 'instanceID': value['instanceID'], 'files': [], 'documents': [], 'protectedFiles': {}, 'volumes': {}}
        with tempfile.TemporaryDirectory() as folder:
            directory = pathlib.Path(folder).resolve(); original_save = r.save
            def write(path, data):
                if path.name == 'receipt.json':
                    raise OSError('synthetic interrupted publication')
                original_save(path, data)
            with mock.patch.object(r, 'runtime_validate', return_value={'mongo': {'Id': 'synthetic'}}), \
                 mock.patch.object(r, 'validate_storage_ownership'), \
                 mock.patch.object(r, 'validate_ownership'), mock.patch.object(r, 'save', side_effect=write), \
                 mock.patch.object(r, 'watchdog', return_value=123), mock.patch.object(r, 'stop_writers'), \
                 mock.patch.object(r, 'mongo_scan', return_value=[]), mock.patch.object(r, 'mongo_rewrap'), \
                 mock.patch.object(r, 'mongo_password'), mock.patch.object(r, 'redis_password'), \
                 mock.patch.object(r, 'install_files'), mock.patch.object(r, 'restart_services'), \
                 mock.patch.object(r, 'application_proof', return_value={'syntheticProof': True}), \
                 mock.patch.object(r, 'rollback') as revert, mock.patch.object(r.os, 'waitpid', create=True):
                with self.assertRaisesRegex(RuntimeError, 'receipt_publication_pending'):
                    r.apply(value, plan, directory)
            revert.assert_not_called()
            journal = json.loads((directory/'journal.json').read_text())
            self.assertEqual(journal['status'], 'committed')
            self.assertEqual(journal['receipt']['status'], 'committed')
            self.assertIn('signature', journal['receipt'])

    def test_watchdog_rereads_commit_after_draining_parent_group(self):
        class Finished(BaseException):
            pass
        with tempfile.TemporaryDirectory() as folder:
            directory = pathlib.Path(folder).resolve(); r.save(directory/'journal.json', {'status': 'applying'})
            def signal_group(_pid, sig):
                if sig == r.signal.SIGTERM:
                    r.save(directory/'journal.json', {'status': 'committed'})
            with mock.patch.object(r.os, 'getpgrp', return_value=321, create=True), \
                 mock.patch.object(r.os, 'fork', return_value=0, create=True), \
                 mock.patch.object(r.os, 'setsid', create=True), mock.patch.object(r.os, 'dup2'), \
                 mock.patch.object(r.signal, 'SIGKILL', 9, create=True), \
                 mock.patch.object(r.os, 'killpg', side_effect=signal_group, create=True), \
                 mock.patch.object(r, 'process_start', side_effect=['identity', 'identity', None, None]), \
                 mock.patch.object(r.time, 'sleep'), mock.patch.object(r, 'rollback') as revert, \
                 mock.patch.object(r.os, '_exit', side_effect=Finished):
                with self.assertRaises(Finished):
                    r.watchdog(bundle(), {}, directory, 321, maximum=0)
            revert.assert_not_called()

    def test_watchdog_kills_orphaned_commands_before_compensation(self):
        class Finished(BaseException):
            pass
        order = []
        with tempfile.TemporaryDirectory() as folder:
            directory = pathlib.Path(folder).resolve(); r.save(directory/'journal.json', {'status': 'applying'})
            with mock.patch.object(r.os, 'getpgrp', return_value=321, create=True), \
                 mock.patch.object(r.os, 'fork', return_value=0, create=True), \
                 mock.patch.object(r.os, 'setsid', create=True), mock.patch.object(r.os, 'dup2'), \
                 mock.patch.object(r.signal, 'SIGKILL', 9, create=True), \
                 mock.patch.object(r.os, 'killpg', side_effect=lambda *_: order.append('kill-orphan-group'), create=True), \
                 mock.patch.object(r, 'process_start', side_effect=['identity', None, None]), \
                 mock.patch.object(r, 'rollback', side_effect=lambda *_: order.append('rollback-current-data')), \
                 mock.patch.object(r.os, '_exit', side_effect=Finished):
                with self.assertRaises(Finished):
                    r.watchdog(bundle(), {}, directory, 321, maximum=0)
            self.assertEqual(order, ['kill-orphan-group', 'rollback-current-data'])


if __name__ == '__main__':
    unittest.main()
