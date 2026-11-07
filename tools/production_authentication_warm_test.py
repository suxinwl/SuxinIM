"""Offline checks for the no-registration recovery warmup helper."""
import hashlib
import hmac
import contextlib
import io
import json
import pathlib
import tempfile
import time
import types
import unittest
from unittest import mock

import production_authentication_warm_server as worker


class WarmTest(unittest.TestCase):
    secret = 'a'*64

    def test_instance_and_container_ownership_before_environment_read(self):
        import copy
        marker = {'instanceID': 'f'*32, 'production': True, 'testOnly': False, 'stackName': worker.PROJECT}
        activation = {'instanceID': marker['instanceID']}
        labels = {'com.puty.suite': marker['instanceID'], 'com.docker.compose.project': worker.PROJECT, 'com.docker.compose.service': 'enterprise-service'}
        container = {'Name': '/'+worker.CONTAINER, 'State': {'Running': True}, 'Config': {'Labels': labels, 'Env': ['OPENIM_SECRET=synthetic']}}
        self.assertEqual(worker.owned_environment(container, marker, activation), {'OPENIM_SECRET': 'synthetic'})
        for field in labels:
            wrong = copy.deepcopy(container)
            wrong['Config']['Labels'][field] = 'foreign'
            wrong['Config'].pop('Env')
            with self.assertRaises(worker.Failure):
                worker.owned_environment(wrong, marker, activation)
        for field, value in (('testOnly', True), ('production', False), ('instanceID', '0'*32), ('stackName', 'foreign')):
            with self.assertRaises(worker.Failure):
                worker.owned_environment(container, {**marker, field: value}, activation)

    def test_probe_tokens_are_unregistered_non_admin_and_short(self):
        import base64
        for chat in (True, False):
            raw = worker.probe_token(self.secret, chat)
            body = raw.split('.')[1]
            claims = json.loads(base64.urlsafe_b64decode(body+'='*(-len(body) % 4)))
            self.assertEqual(claims['UserID'], worker.PROBE_USER)
            self.assertFalse(claims['UserID'].startswith('emp_'))
            self.assertLessEqual(claims['exp']-time.time(), 60)
            self.assertEqual(claims['PlatformID'], 0 if chat else 2)
            if chat:
                self.assertEqual(claims['UserType'], 1)

    def test_warmup_only_sends_two_unregistered_probe_tokens(self):
        replies = [(401, {'errCode': 1501, 'errMsg': 'Chat session expired or revoked'}), (200, {'errCode': 1507, 'errMsg': 'TokenNotExistError'})]
        with mock.patch.object(worker, 'request', side_effect=replies) as request:
            worker.warm_once(None, 'unregistered-chat-token', 'unregistered-im-token')
        self.assertEqual(request.call_count, 2)
        self.assertEqual(request.call_args_list[0].args[3], {'chatToken': 'unregistered-chat-token', 'token': 'unregistered-im-token', 'platformID': '2'})
        self.assertEqual(request.call_args_list[1].args[2], {'token': 'unregistered-im-token'})

    def test_dependency_errors_never_count_as_registry_ready(self):
        good_chat = (401, {'errCode': 1501, 'errMsg': 'Chat session expired or revoked'})
        for bad in ((503, {'errCode': 1006, 'errMsg': 'authentication service temporarily unavailable'}), (200, {'errCode': 1503, 'errMsg': 'TokenMalformedError'}), (401, {'errCode': 1501, 'errMsg': 'HRM session is expired or revoked'})):
            with mock.patch.object(worker, 'request', side_effect=[good_chat, bad]):
                with self.assertRaises(worker.Failure):
                    worker.warm_once(None, 'chat', 'im')

    def test_enterprise_wrong_denial_category_cannot_count_as_Chat_registry_ready(self):
        for status, code, message in ((503, 1006, 'authentication service temporarily unavailable'), (403, 1002, 'HRM access denied'), (401, 1501, 'OpenIM session expired or revoked')):
            with mock.patch.object(worker, 'request', return_value=(status, {'errCode': code, 'errMsg': message})):
                with self.assertRaises(worker.Failure):
                    worker.warm_once(None, 'chat', 'im')

    def test_main_does_not_read_registry_or_Redis_credentials_and_reports_private_IM_gap(self):
        marker = {'instanceID': 'f'*32, 'production': True, 'testOnly': False, 'stackName': worker.PROJECT}
        labels = {'com.puty.suite': marker['instanceID'], 'com.docker.compose.project': worker.PROJECT, 'com.docker.compose.service': 'enterprise-service'}
        container = {'Name': '/'+worker.CONTAINER, 'State': {'Running': True}, 'Config': {'Labels': labels, 'Env': ['OPENIM_SECRET='+self.secret]}}
        with tempfile.TemporaryDirectory() as folder:
            root = pathlib.Path(folder).resolve()
            files = {'openim/instance.json': marker, 'activation.json': {'instanceID': marker['instanceID']}, 'openim/config/chat/chat-rpc-admin.yml': {'secret': self.secret}}
            for name, body in files.items():
                target = root/name
                target.parent.mkdir(parents=True, exist_ok=True)
                target.write_text(json.dumps(body))
            cert = root/'openim/certs/puty-root.crt'
            cert.parent.mkdir(parents=True)
            cert.write_text('synthetic certificate')
            output = io.StringIO()
            # No redis.yml or any token registry exists in this fixture.
            with mock.patch.object(worker, 'BASE', root), mock.patch.object(worker.subprocess, 'check_output', return_value=json.dumps([container]).encode()) as command, mock.patch.object(worker.ssl, 'create_default_context'), mock.patch.object(worker.urllib.request, 'build_opener'), mock.patch.object(worker, 'warm_once') as warm, mock.patch.dict('sys.modules', {'yaml': types.SimpleNamespace(safe_load=json.loads)}), mock.patch('sys.argv', ['warm.py']), contextlib.redirect_stdout(output):
                worker.main()
            self.assertEqual(command.call_args.args[0], ['docker', 'inspect', worker.CONTAINER])
            self.assertEqual(len(warm.call_args.args), 3)
            report = json.loads(output.getvalue())
            self.assertTrue(report['authenticationWarmupPassed'])
            self.assertFalse(report['enterprisePrivateIMRPCPathWarmed'])
            self.assertFalse(report['ChatLoginForbiddenMongoPathWarmed'])
            self.assertEqual(report['registeredCredentialsRead'], 0)
            self.assertEqual(report['newTokenRegistrations'], 0)

    def test_only_two_read_only_HTTP_routes_can_be_requested(self):
        with self.assertRaises(worker.Failure):
            worker.request(None, '/api/auth/get_user_token', {})
        with self.assertRaises(worker.Failure):
            worker.request(None, '/sso/login', {})


if __name__ == '__main__':
    unittest.main()
