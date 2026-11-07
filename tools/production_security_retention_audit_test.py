"""Offline synthetic tests only; never reads production inputs or contacts services."""
import ast
import importlib.util
import json
import pathlib
import sys
import tempfile
import types
import unittest
from unittest import mock

ROOT = pathlib.Path(__file__).resolve().parents[1]
if 'fcntl' not in sys.modules and sys.platform == 'win32':
    sys.modules['fcntl'] = types.SimpleNamespace(LOCK_SH=1, LOCK_NB=4, flock=lambda *args: None)
spec = importlib.util.spec_from_file_location('retention_audit', ROOT / 'deploy/production/audit_security_retention.py')
audit = importlib.util.module_from_spec(spec)
spec.loader.exec_module(audit)


def containers():
    return {name: {'Id': name, 'Config': {'Labels': {'com.docker.compose.service': name,
        'com.docker.compose.project': audit.PROJECT, 'com.puty.suite': audit.INSTANCE,
        'com.puty.environment': 'production'}, 'Env': []}, 'State': {'Running': True}}
        for name in audit.SERVICES}


class RetentionAuditTests(unittest.TestCase):
    def test_exact_24_running_service_labels_and_no_duplicates(self):
        values = list(containers().values())
        def run(args, *rest):
            return ('\n'.join(row['Id'] for row in values).encode() if args[1] == 'ps'
                    else json.dumps(values).encode())
        with mock.patch.object(audit, 'run', side_effect=run):
            self.assertEqual(set(audit.owned_containers()), audit.SERVICES)
            values[0]['Config']['Labels']['com.puty.suite'] = 'foreign'
            with self.assertRaisesRegex(RuntimeError, 'ownership'):
                audit.owned_containers()
            values[0]['Config']['Labels']['com.puty.suite'] = audit.INSTANCE
            values[0]['State']['Running'] = False
            with self.assertRaisesRegex(RuntimeError, 'ownership'):
                audit.owned_containers()

    def test_most_specific_actual_config_mount_cannot_be_shadowed(self):
        source = pathlib.Path('/srv/puty-suite/openim/config/openim-server/openim-crontask.yml')
        container = {'Mounts': [{'Type': 'bind', 'Source': str(source.parent),
                     'Destination': '/app/config', 'RW': False}]}
        audit.mounted_file(container, source)
        container['Mounts'].append({'Type': 'bind', 'Source': '/foreign/cron.yml',
            'Destination': '/app/config/openim-crontask.yml', 'RW': False})
        with self.assertRaisesRegex(RuntimeError, 'mismatch'):
            audit.mounted_file(container, source)

    def test_permanent_retention_requires_all_three_explicit_exact_fields(self):
        value = {'retainChatRecords': 0, 'fileExpireTime': 0, 'deleteObjectType': []}
        self.assertTrue(audit.cron_summary(value)['permanent'])
        value['retainChatRecords'] = 90
        self.assertFalse(audit.cron_summary(value)['permanent'])
        value['retainChatRecords'] = False
        with self.assertRaisesRegex(RuntimeError, 'invalid'):
            audit.cron_summary(value)
        with self.assertRaisesRegex(RuntimeError, 'missing'):
            audit.cron_summary({})

    def test_mongo_all_index_ttl_summary_redacts_dynamic_names(self):
        value = {'ok': True, 'databases': 4, 'collections': 31, 'indexes': 80, 'ttl': [
            {'database': 'openim38', 'collection': 'synthetic-personal-collection',
             'index': 'synthetic-private-index', 'seconds': '0', 'partial': True, 'internal': False},
            {'database': 'config', 'collection': 'system.sessions', 'index': 'ttl',
             'seconds': '1800', 'partial': False, 'internal': True}]}
        result = audit.mongo_summary(value)
        self.assertEqual(result['businessTTLCount'], 1)
        self.assertEqual(result['ttl'][0]['expireAfterSeconds'], '0')
        self.assertNotIn('synthetic-personal', json.dumps(result))
        self.assertNotIn('synthetic-private', json.dumps(result))
        value['ttl'][0]['seconds'] = 'NaN'
        with self.assertRaisesRegex(RuntimeError, 'invalid'):
            audit.mongo_summary(value)

    def test_minio_lifecycle_distinguishes_expiration_from_incomplete_upload_cleanup(self):
        raw = b'''<LifecycleConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
        <Rule><Status>Enabled</Status><Expiration><Days>30</Days></Expiration></Rule>
        <Rule><Status>Disabled</Status><NoncurrentVersionExpiration><NoncurrentDays>5</NoncurrentDays></NoncurrentVersionExpiration></Rule>
        <Rule><Status>Enabled</Status><AbortIncompleteMultipartUpload><DaysAfterInitiation>7</DaysAfterInitiation></AbortIncompleteMultipartUpload></Rule>
        </LifecycleConfiguration>'''
        result = audit.lifecycle_summary(200, raw)
        self.assertEqual(result['enabledExpirationRules'], 1)
        self.assertEqual(result['enabledAbortMultipartRules'], 1)
        self.assertFalse(audit.lifecycle_summary(404,
            b'<Error><Code>NoSuchLifecycleConfiguration</Code></Error>')['configured'])
        with self.assertRaisesRegex(RuntimeError, 'unavailable'):
            audit.lifecycle_summary(404, b'<Error><Code>NoSuchBucket</Code></Error>')
        with self.assertRaisesRegex(RuntimeError, 'invalid'):
            audit.lifecycle_summary(200, b'<!DOCTYPE a><LifecycleConfiguration/>')

    def test_loopback_s3_only_get_and_no_redirect_or_environment_proxy(self):
        class Response:
            status = 200
            def __enter__(self): return self
            def __exit__(self, *args): return None
            def read(self, *args): return b'<LifecycleConfiguration/>'
        opener = mock.Mock()
        opener.open.return_value = Response()
        with mock.patch.object(audit.urllib.request, 'build_opener', return_value=opener) as build:
            self.assertEqual(audit.s3_get('a' * 64, '/openim38', 'lifecycle=')[0], 200)
            request = opener.open.call_args.args[0]
            self.assertEqual(request.get_method(), 'GET')
            self.assertEqual(request.full_url, 'http://127.0.0.1:19000/openim38?lifecycle=')
            handlers = build.call_args.args
            self.assertEqual(handlers[0].proxies, {})
            self.assertIsNone(handlers[1].redirect_request(None, None, 302, '', {}, 'https://foreign.invalid'))
        with self.assertRaisesRegex(RuntimeError, 'scope'):
            audit.s3_get('a' * 64, '/openim38', 'delete=')

    def test_data_capacity_requires_owned_source_volume_and_exact_readonly_alias(self):
        with tempfile.TemporaryDirectory() as temporary:
            values = containers()
            volumes = {}
            for kind, target in (('mongo', '/data/db'), ('minio', '/bitnami/minio/data')):
                source = pathlib.Path(temporary, kind)
                source.mkdir()
                source = source.resolve()
                values[kind]['Mounts'] = [{'Type': 'volume', 'Name': kind, 'Source': str(source),
                    'Destination': target, 'RW': True}]
                values['enterprise-service'].setdefault('Mounts', []).append({'Type': 'volume',
                    'Source': str(source), 'Destination': '/capacity/' + kind, 'RW': False})
                volumes[kind] = [{'Driver': 'local', 'Scope': 'local', 'Mountpoint': str(source),
                    'Labels': {'com.puty.suite': audit.INSTANCE, 'com.docker.compose.project': audit.PROJECT,
                               'com.docker.compose.volume': kind + '-data'}}]
            values['mongo']['Mounts'].append({'Type': 'volume', 'Name': 'anonymous-configdb', 'Destination': '/data/configdb'})
            values['minio']['Mounts'].append({'Type': 'volume', 'Name': 'anonymous-certs', 'Destination': '/certs'})
            fake_stat = types.SimpleNamespace(f_blocks=100, f_frsize=4096, f_bavail=20)
            with mock.patch.object(audit, 'run', side_effect=lambda argv: json.dumps(volumes[argv[-1]]).encode()), \
                 mock.patch.object(audit.os, 'statvfs', return_value=fake_stat, create=True):
                result = audit.capacity_summary(values)
                self.assertTrue(all(row['capacityAliasReadonly'] for row in result))
                self.assertFalse(any(row['operatorVerified'] for row in result))
                values['minio']['Mounts'][0]['Destination'] = '/data'
                with self.assertRaisesRegex(RuntimeError, 'data_volume_mount_invalid'):
                    audit.capacity_summary(values)
                values['minio']['Mounts'][0]['Destination'] = '/bitnami/minio/data'
                values['enterprise-service']['Mounts'][0]['RW'] = True
                self.assertFalse(audit.capacity_summary(values)[0]['capacityAliasReadonly'])
                volumes['mongo'][0]['Labels']['com.puty.suite'] = 'foreign'
                with self.assertRaisesRegex(RuntimeError, 'owned'):
                    audit.capacity_summary(values)

    def test_task_results_never_echo_command_content_or_credentials(self):
        result = audit.task_summary(b'0 1 * * * rm /srv/puty-suite/private/synthetic-secret', '/etc/cron.d/test')
        self.assertTrue(result['requiresManualReview'])
        self.assertNotIn('synthetic-secret', json.dumps(result))
        self.assertNotIn('/etc/cron.d/test', json.dumps(result))

    def test_root_guard_fails_before_any_subprocess_or_file_read(self):
        with mock.patch.object(audit.os, 'geteuid', return_value=999, create=True), \
             mock.patch.object(audit, 'run') as run:
            with self.assertRaisesRegex(RuntimeError, 'root'):
                audit.audit()
            run.assert_not_called()

    def test_worker_contains_no_filesystem_mutation_or_mongo_write_operation(self):
        tree = ast.parse((ROOT / 'deploy/production/audit_security_retention.py').read_text(encoding='utf-8'))
        forbidden = {'write_text', 'write_bytes', 'unlink', 'remove', 'mkdir', 'rename', 'replace',
                     'chmod', 'chown', 'rmtree'}
        self.assertFalse(any(isinstance(node, ast.Call) and isinstance(node.func, ast.Attribute)
            and node.func.attr in forbidden for node in ast.walk(tree)))
        self.assertNotRegex(audit.MONGO_SCRIPT, r'\.(?:insert|delete|update|drop|create|remove|save)[A-Z_a-z]*\s*\(')


if __name__ == '__main__':
    unittest.main()
