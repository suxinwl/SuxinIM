"""Pure local tests. No production fixture, credential, CA or endpoint is read."""
import base64
import contextlib
import copy
import importlib.util
import io
import json
import pathlib
import subprocess
import sys
import tempfile
import types
import unittest
from unittest.mock import patch

SPEC = importlib.util.spec_from_file_location('security_resources', pathlib.Path(__file__).with_name('production_security_resources.py'))
m = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(m)


def inputs():
    accounts = []
    for i in range(2):
        employee = 'new-security-synthetic-' + str(i)
        accounts.append({'uid': 9001 + i, 'employeeID': employee, 'userID': 'emp_' + m.sha(employee.encode())[:32],
                         'username': m.ACCOUNTS[i], 'name': m.NAMES[i], 'mobile': '000000000' + ('09' if i == 0 else '10'),
                         'wecom_userid': '', 'remark': m.NAMESPACE + ' fixture ' + 'a' * 32 + ' ' + str(i)})
    fixture = {'namespace': m.NAMESPACE, 'instanceID': m.INSTANCE, 'departmentID': 53, 'runID': 'a' * 32,
               'createdAt': 2000000000, 'accounts': accounts, 'excludedUIDs': list(range(10, 16)),
               'excludedEmployeeIDs': ['old-bound-' + str(i) for i in range(6)]}
    state = copy.deepcopy(fixture)
    state['sessions'] = {}
    for slot, platform, client in zip(m.SLOTS, (5, 3), ('web_desktop', 'pc')):
        state['sessions'][slot] = {'userID': accounts[0]['userID'], '_platform': platform, '_client': client,
                                  'policyClient': client, 'imToken': 'synthetic-im-' + client,
                                  'chatToken': 'synthetic-chat-' + client, 'sessionToken': 'synthetic-SSO-' + client}
    return fixture, state


def response(status=200, value=None, body=b'', headers=None):
    return status, value, body, {'cache-control': 'private, no-store', **(headers or {})}


class Fake:
    def __init__(self, expected, body, sessions, durable):
        self.origin, self.zone = m.ORIGINS['lan'], 'lan'
        self.expected, self.body, self.sessions, self.durable = expected, body, sessions, durable
        self.calls, self.grants = [], {}
        owner = expected['name'].split('/')[0]
        self.key = 'openim/direct/20330518/' + owner + '/' + 'c' * 32 + '.txt'
        self.upload_url = self.origin + '/attachments/' + '8' * 64 + '/openim38'
        self.no_store, self.wrong_scope, self.false_deny_status = True, False, None
        self.saved_before_initiate = False

    def reply(self, value):
        return response(value={'errCode': 0, 'data': value}, headers={'cache-control': 'no-store' if self.no_store else 'public'})

    def request(self, target, method='GET', session=None, value=None, raw=None, headers=None):
        headers = headers or {}
        self.calls.append((target, method, copy.deepcopy(session), copy.deepcopy(value), dict(headers)))
        if target == '/enterprise/v1/session/state':
            client = session['_client']
            return self.reply({'userID': session['userID'], 'platform': session['_platform'], 'logicalClient': client,
                               'sessionID': 'bound-sid-' + client, 'epoch': 7, 'loginAllowed': True, 'saveAllowed': client == 'pc',
                               'effectivePolicy': {'zone': 'lan', 'logicalClient': client, 'login': True, 'preview': True,
                                                   'save': client == 'pc', 'securityVersion': 7}})
        if target == '/api/object/initiate_form_data':
            self.saved_before_initiate = bool(self.durable and self.durable[-1]['name'] == value['name']
                                              and self.durable[-1]['bodySHA256'] == m.sha(self.body))
            if not self.saved_before_initiate:
                raise AssertionError('Initiated before a durable expected object')
            meta = {**value, 'key': self.key}
            return self.reply({'id': base64.b64encode(m.canonical(meta)).decode().rstrip('='), 'url': self.upload_url,
                               'file': 'file', 'header': {}, 'formData': {'key': self.key, 'Content-Type': 'text/plain', 'policy': 'synthetic'},
                               'successCodes': [200]})
        if target == self.upload_url:
            assert method == 'POST' and session is None and raw.count(self.body) == 1
            return response()
        if target == '/api/object/complete_form_data':
            return self.reply({'url': self.origin + '/api/object/' + self.expected['name']})
        if target == '/enterprise/v1/attachment/authorize':
            if value['purpose'] == 'save' and session['_platform'] == 5:
                return response(self.false_deny_status or 403, {'errCode': 403})
            web = session['_platform'] == 5
            proof = ('a' if web else 'b') * 64
            key = self.key + '-other' if self.wrong_scope else self.key
            url = self.origin + '/attachments/' + ('1' if web else '2') * 64 + '/openim38/' + key + '?X-Amz-Signature=synthetic'
            self.grants[url] = (proof, session)
            return self.reply({'url': url, 'purpose': value['purpose'], 'resourceProof': proof, 'objectID': 'f' * 64})
        proof, owner = self.grants[target]
        authorized = headers.get(m.PROOF) == proof
        if session is not None:
            authorized = authorized and all(session.get(k) == owner.get(k) for k in ('userID', '_platform', 'imToken', 'chatToken'))
        if not authorized:
            return response(401, {'errCode': 1002})
        if method == 'HEAD':
            return response(headers={'content-length': '436', 'cache-control': 'no-store' if self.no_store else 'public'})
        if headers.get('Range'):
            return response(206, body=self.body[2:10], headers={'content-range': 'bytes 2-9/436'})
        return response(body=self.body, headers={'cache-control': 'no-store' if self.no_store else 'public'})


class ResourceContracts(unittest.TestCase):
    def setUp(self):
        self.fixture, self.state = inputs()
        self.sessions = m.validate_inputs(self.fixture, self.state, 'a' * 32, 2000000010)
        self.expected, self.body = m.new_expected(self.fixture, 2000000010, 'b' * 24, 'd' * 64)
        self.saved, self.checks = [], []
        self.fake = Fake(self.expected, self.body, self.sessions, self.saved)
        self.engine = m.Acceptance(self.fake, self.fixture, self.sessions, self.expected, self.body,
                                   lambda value: self.saved.append(copy.deepcopy(value)),
                                   lambda label, passed, status: self.checks.append({'check': label, 'passed': passed, 'status': status}),
                                   now=lambda: 2000000020)

    def test_default_dry_run_reads_nothing_and_subprocess_exits_zero(self):
        with patch.object(m, 'snapshot', side_effect=AssertionError('Private input read')), patch.object(m, 'Transport', side_effect=AssertionError('Network')):
            output = io.StringIO()
            with contextlib.redirect_stdout(output):
                self.assertEqual(m.main([]), 0)
            self.assertEqual(json.loads(output.getvalue())['privateInputsRead'], 0)
        result = subprocess.run([sys.executable, m.__file__, '--dry-run'], capture_output=True, text=True, check=True)
        self.assertEqual(result.stderr, '')
        self.assertEqual(json.loads(result.stdout)['productionRequests'], 0)

    def test_input_snapshot_parsed_value_and_receipt_SHA_use_one_byte_sequence(self):
        with tempfile.TemporaryDirectory() as temporary:
            file = pathlib.Path(temporary).resolve() / 'synthetic.json'
            raw = b'\xef\xbb\xbf{ "synthetic":true }\r\n'
            file.write_bytes(raw)
            value, digest = m.snapshot(file)
            self.assertEqual(value, {'synthetic': True})
            self.assertEqual(digest, m.sha(raw))

    def test_full_preview_save_and_cross_SID_suite_uses_only_existing_two_slots(self):
        self.engine.run()
        self.assertTrue(all(x['passed'] for x in self.checks))
        self.assertGreaterEqual(len(self.checks), 25)
        self.assertTrue(self.fake.saved_before_initiate)
        self.assertTrue(self.saved[-1]['completed'])
        api = [p for p, _, _, _, _ in self.fake.calls if p.startswith('/')]
        self.assertEqual(set(api), m.API_PATHS)
        self.assertFalse(any('/sso/' in p or '/admin/' in p or 'logout' in p or 'refresh' in p for p in api))
        self.assertEqual(sum(p == '/api/object/initiate_form_data' for p in api), 1)

    def test_safe_report_never_contains_tokens_proofs_or_URL(self):
        self.engine.run()
        encoded = json.dumps(self.checks)
        for session in self.sessions:
            for key in ('imToken', 'chatToken', 'sessionToken'):
                self.assertNotIn(session[key], encoded)
        self.assertNotIn(self.fake.origin, encoded)
        self.assertNotIn('a' * 64, encoded)
        self.assertNotIn('b' * 64, encoded)

    def test_wrong_namespace_nonce_or_previous_identity_refused_without_requests(self):
        changes = [lambda x: x.update(namespace='attachment-20261103'), lambda x: x.update(runID='b' * 32),
                   lambda x: x['accounts'][0].update(uid=10), lambda x: x['accounts'][0].update(username='qa_261103_e'),
                   lambda x: x['accounts'][1].update(remark='wrong'), lambda x: x['accounts'][0].update(wecom_userid='real-upstream')]
        for change in changes:
            state = copy.deepcopy(self.state)
            change(state)
            with self.assertRaises(m.Rejected):
                m.validate_inputs(self.fixture, state, 'a' * 32, 2000000010)
        self.assertEqual(self.fake.calls, [])

    def test_employee_hash_and_immutable_account_mapping_must_match(self):
        for key, value in [('userID', 'emp_' + '0' * 32), ('employeeID', 'different'), ('mobile', 'other')]:
            state = copy.deepcopy(self.state)
            state['accounts'][0][key] = value
            with self.assertRaises(m.Rejected):
                m.validate_inputs(self.fixture, state, 'a' * 32, 2000000010)

    def test_wrong_logical_platform_duplicate_pair_or_missing_pause_refused(self):
        for mutate in (lambda x: x['sessions'].pop(m.SLOTS[0]),
                       lambda x: x['sessions'][m.SLOTS[0]].update(_platform=2),
                       lambda x: x['sessions'][m.SLOTS[1]].update(policyClient='android'),
                       lambda x: x['sessions'][m.SLOTS[1]].update(imToken=x['sessions'][m.SLOTS[0]]['imToken'])):
            state = copy.deepcopy(self.state)
            mutate(state)
            with self.assertRaises(m.Rejected):
                m.validate_inputs(self.fixture, state, 'a' * 32, 2000000010)

    def test_expected_body_matches_unchanged_server_cleanup_marker(self):
        spec = importlib.util.spec_from_file_location('old_identity_server', pathlib.Path(__file__).with_name('production_identity_acceptance_server.py'))
        old = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(old)
        self.assertEqual(len(self.body), 436)
        self.assertTrue(old.identity_fixture_body(self.body))
        self.assertEqual(self.expected['size'], 436)
        self.assertIn('/production-acceptance-' + 'b' * 24 + '.txt', self.expected['name'])

    def test_record_is_persisted_before_network_and_write_fault_prevents_initiate(self):
        self.engine.persist = lambda value: (_ for _ in ()).throw(OSError('synthetic ENOSPC'))
        with self.assertRaises(OSError):
            self.engine.upload()
        self.assertEqual(self.fake.calls, [])

    def test_upload_id_exact_name_size_group_contenttype_and_owner_key_are_bound(self):
        metadata = {k: self.expected[k] for k in ('name', 'size', 'contentType', 'group')}
        metadata['key'] = self.fake.key
        for key, value in [('name', 'foreign/object.txt'), ('size', 437), ('group', 'other'), ('contentType', 'image/png'), ('key', self.fake.key.replace(self.sessions[0]['userID'], 'emp_' + '0' * 32))]:
            bad = {**metadata, key: value}
            with self.assertRaises(m.Rejected):
                m.decode_upload({'id': base64.b64encode(m.canonical(bad)).decode()}, self.expected)

    def test_grant_origin_path_and_no_proof_query_are_strict(self):
        valid = self.fake.origin + '/attachments/' + 'e' * 64 + '/openim38/exact?X-Amz-Signature=synthetic'
        self.assertEqual(m.grant_url(valid, self.fake.origin), valid)
        for bad in (valid.replace('https:', 'http:'), valid.replace('192.168.10.11', '127.0.0.1'),
                    valid + '&resourceProof=x', valid + '&token=x', valid + '#x',
                    valid.replace('/attachments/', '/other/'), valid + '&x=' + 'a' * 64):
            with self.assertRaises(m.Rejected):
                m.grant_url(bad, self.fake.origin, 'a' * 64)

    def test_authorized_other_object_is_rejected(self):
        self.fake.wrong_scope = True
        with self.assertRaisesRegex(m.Rejected, 'grant_must_identify_exact_uploaded_key'):
            self.engine.run()

    def test_missing_no_store_is_failure(self):
        self.fake.no_store = False
        with self.assertRaisesRegex(m.Rejected, 'no_store'):
            self.engine.run()

    def test_dependency_failure_is_not_save_denial(self):
        self.fake.false_deny_status = 503
        with self.assertRaisesRegex(m.Rejected, 'web_save_authorization_denied'):
            self.engine.run()
        self.assertFalse(m.rejection(response(503)))
        self.assertFalse(m.rejection(response(404)))
        self.assertFalse(m.rejection(response(200, {'errCode': 1006})))

    def test_complete_failure_keeps_exact_orphan_review_input(self):
        request = self.fake.request
        def fail(target, *args, **kwargs):
            if target == '/api/object/complete_form_data':
                return response(503, {'errCode': 1006})
            return request(target, *args, **kwargs)
        self.fake.request = fail
        with self.assertRaises(m.Rejected):
            self.engine.run()
        value = m.inventory_input(self.fixture, self.expected, 2000000021)
        self.assertTrue(value['requiresExactOrphanReview'])
        self.assertEqual(value['expectedObjects'][0]['objectKey'], self.fake.key)
        self.assertFalse(value['expectedObjects'][0]['completed'])

    def test_nonclient_inventory_input_only_new_pair_bound_time_exact_record(self):
        self.engine.run()
        value = m.inventory_input(self.fixture, self.expected, 2000000021)
        self.assertEqual(value['serverAction'], 'attachment-inventory')
        self.assertEqual(value['cleanupAction'], 'attachment-cleanup')
        self.assertNotIn('clientUploads', value)
        self.assertEqual(len(value['fixtureEmployeeIDs']), 2)
        self.assertEqual(value['expectedObjectCount'], 1)
        self.assertEqual(value['expectedObjects'][0]['bodySHA256'], m.sha(self.body))
        self.assertTrue(value['rootMustVerifyExactNameKeySHAAndCountBeforeApply'])
        self.assertFalse(value['requiresExactOrphanReview'])
        self.assertLess(value['createdBefore'] - value['createdAfter'], 86400)
        with self.assertRaises(m.Rejected):
            m.inventory_input(self.fixture, self.expected, self.expected['createdAt'] + 86400)

    def test_only_final_durable_inventory_ack_can_unlock_next_operator_phase(self):
        checks = [{'check': 'synthetic partial success', 'passed': True}]
        pending = m.report_document(checks, 'a' * 64, 'b' * 64, 'c' * 32)
        self.assertFalse(pending['passed'])
        self.assertFalse(pending['resourceChecksComplete'])
        complete = m.report_document(checks, 'a' * 64, 'b' * 64, 'c' * 32, complete=True, inventory_sha='d' * 64)
        self.assertTrue(complete['passed'])
        self.assertTrue(complete['resourceChecksComplete'])
        self.assertEqual(complete['fixtureSourceSHA256'], 'a' * 64)
        self.assertEqual(complete['pausedStateSHA256'], 'b' * 64)
        self.assertEqual(complete['inventoryInputSHA256'], 'd' * 64)

    def test_invalid_multipart_fields_and_unexpected_headers_refused(self):
        valid = {'file': 'file', 'formData': {'key': self.fake.key, 'Content-Type': 'text/plain'}, 'header': {}}
        raw, headers = m.multipart(valid, self.body)
        self.assertEqual(raw.count(self.body), 1)
        self.assertTrue(headers['Content-Type'].startswith('multipart/form-data; boundary='))
        for bad in ({**valid, 'header': {'Authorization': 'bad'}}, {**valid, 'file': 'bad"'},
                    {**valid, 'formData': {'key': 'bad\r\nvalue'}}):
            with self.assertRaises(m.Rejected):
                m.multipart(bad, self.body)

    def test_transport_has_no_cookie_jar_and_rejects_nonallowlisted_routes(self):
        text = pathlib.Path(m.__file__).read_text()
        self.assertNotIn('HTTPCookieProcessor(', text)
        self.assertIn('ProxyHandler({})', text)
        fake = object.__new__(m.Transport)
        fake.origin = self.fake.origin
        for path in ('/sso/login', '/sso/logout', '/internal/hrm/revoke', '/admin/system/account/save'):
            with self.assertRaises(m.Rejected):
                fake.request(path)

    def test_durable_output_protects_before_data_and_cleans_failed_temporary(self):
        with tempfile.TemporaryDirectory() as temporary:
            home = pathlib.Path(temporary).resolve()
            output = home / 'resource-acceptance'
            output.mkdir()
            events = []
            backup = types.SimpleNamespace(protect=lambda path: events.append((path.name, path.stat().st_size if path.is_file() else None)))
            with patch.object(m, 'HOME', home), patch.object(m, 'OUTPUT', output), patch.dict(sys.modules, {'production_backup': backup}):
                file = output / 'private.json'
                m.durable(file, {'synthetic': True})
                self.assertEqual(json.loads(file.read_text()), {'synthetic': True})
                self.assertTrue(any(name.startswith('private.json.next-') and size == 0 for name, size in events))
                with patch.object(m.os, 'fsync', side_effect=OSError('synthetic disk failure')):
                    with self.assertRaises(OSError):
                        m.durable(file, {'synthetic': False})
                self.assertEqual(json.loads(file.read_text()), {'synthetic': True})
                self.assertEqual(sorted(x.name for x in output.iterdir()), ['private.json'])


if __name__ == '__main__':
    unittest.main()
