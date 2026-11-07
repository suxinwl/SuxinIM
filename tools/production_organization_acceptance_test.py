"""Offline scope regression: no production authentication module or HTTP."""
import contextlib
import hashlib
import importlib.util
import io
import json
import pathlib
import sys
import tempfile
import types
import unittest
from unittest import mock

ROOT = pathlib.Path(__file__).resolve().parents[1]


def load():
    spec = importlib.util.spec_from_file_location('organization_scopes_tested', ROOT / 'tools/production_organization_acceptance.py')
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class ScopeTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.private = pathlib.Path(self.temp.name).resolve()
        self.addCleanup(self.temp.cleanup)
        self.addCleanup(mock.patch.stopall)
        self.mod = load()
        self.mod.PRIVATE = self.private
        self.mod.protect = mock.Mock()
        self.mod.hrm_client = lambda: types.SimpleNamespace(ORIGIN='https://192.0.2.1:9880')
        for name, suffixes, first in [('test-accounts.json', ('a', 'b'), 1001),
                                      ('organization-test-accounts.json', ('c', 'd'), 2001)]:
            value = {'namespace': 'organization-20261103' if first == 2001 else 'original-AB', 'departmentID': 53,
                     'accounts': [{'uid': first + i, 'username': 'qa_261103_' + suffix, 'name': 'Synthetic ' + suffix,
                                   'employeeID': 'old-' + suffix, 'password': 'not-a-production-password'}
                                  for i, suffix in enumerate(suffixes)]}
            (self.private / name).write_text(json.dumps(value))
        (self.private / 'identity-captcha-aweb.json').write_bytes(b'OLD CAPTCHA MUST STAY')
        (self.private / 'organization-identity-state.json').write_bytes(b'OLD STATE MUST STAY')
        self.before = {p.name: p.read_bytes() for p in self.private.iterdir() if p.is_file()}
        self.calls = []
        self.dept = {'id': 53, 'pid': 0, 'business_id': 1, 'status': 0, 'im_group_enabled': 0, 'leader_uid': 0,
                     'name': 'TEMP QA 20261103', 'remark': 'Temporary synthetic organization acceptance department'}

    def api(self, path, method='GET', value=None):
        self.calls.append((path, method, value))
        if path == '/admin/system/dept/getList' and method == 'GET':
            return [dict(self.dept)]
        if path.startswith('/admin/system/account/getList') and method == 'GET':
            return []
        if path == '/admin/system/account/save' and method == 'POST':
            return {'qa_261103_e': 9008, 'qa_261103_f': 9009}[value['username']]
        if path == '/plugins/biz-suite/api/admin/hrm/employees' and method == 'POST':
            return {'id': 'new-' + value['employee_no'][-1]}
        raise AssertionError('Unexpected mock API action')

    def created(self):
        self.mod.configure_scope('attachment-20261103')
        self.mod.api = self.api
        with contextlib.redirect_stdout(io.StringIO()):
            self.mod.create()
        return json.loads(self.mod.FIXTURE.read_text())

    def old_unchanged(self):
        self.assertEqual({p.name: p.read_bytes() for p in self.private.iterdir() if p.is_file()}, self.before)

    def test_default_profile_paths_and_legacy_names_unchanged(self):
        self.mod.configure_scope('attachment-20261103')
        self.mod.configure_scope('organization-20261103')
        self.assertEqual(self.mod.FIXTURE, self.private / 'organization-test-accounts.json')
        self.assertEqual(self.mod.STATE, self.private / 'organization-identity-state.json')
        self.assertEqual(self.mod.REPORT, self.private / 'organization-acceptance-report.json')
        self.assertEqual(self.mod.PAIR, [('c', '00000000003'), ('d', '00000000004')])
        self.assertEqual(self.mod.fixture()['accounts'][0]['uid'], 2001)
        self.old_unchanged()

    def test_attachment_create_only_new_EF_and_no_department_or_group_writes(self):
        value = self.created()
        self.assertEqual(value['namespace'], 'attachment-20261103')
        self.assertEqual(value['departmentID'], 53)
        self.assertEqual(value['excludedPreviousUIDs'], [1001, 1002, 2001, 2002])
        self.assertEqual([a['username'] for a in value['accounts']], ['qa_261103_e', 'qa_261103_f'])
        self.assertEqual(len({a['password'] for a in value['accounts']}), 2)
        self.assertTrue(all(len(a['password']) >= 24 for a in value['accounts']))
        self.assertTrue(all(method == 'GET' for path, method, _ in self.calls if '/dept/' in path))
        self.assertFalse(any('/groups' in path for path, _, _ in self.calls))
        account_writes = [body for path, method, body in self.calls if path.endswith('/account/save')]
        self.assertEqual(len(account_writes), 2)
        for row, account in zip(account_writes, value['accounts']):
            self.assertEqual(row['password'], hashlib.md5(account['password'].encode()).hexdigest())
            self.assertEqual(row['tel'], '')
            self.assertEqual(row['roleid'], [])
        self.old_unchanged()
        with self.assertRaises(AssertionError):
            self.mod.create()

    def test_department_any_drift_fails_before_private_or_account_writes(self):
        self.mod.configure_scope('attachment-20261103')
        self.mod.api = self.api
        for key, changed in [('business_id', 0), ('pid', 1), ('leader_uid', 307), ('im_group_enabled', 1),
                             ('name', 'Changed'), ('remark', 'Changed'), ('status', 1)]:
            original = self.dept[key]
            self.dept[key] = changed
            self.calls.clear()
            with self.subTest(key=key), self.assertRaises(AssertionError):
                self.mod.create()
            self.assertFalse(self.mod.FIXTURE.exists())
            self.assertTrue(all(method == 'GET' for _, method, _ in self.calls))
            self.dept[key] = original
        self.old_unchanged()

    def test_old_identity_reuse_is_rejected(self):
        self.created()
        source = json.loads(self.mod.FIXTURE.read_text())
        for field, old in [('uid', 1001), ('employeeID', 'old-c')]:
            value = json.loads(json.dumps(source))
            value['accounts'][0][field] = old
            self.mod.FIXTURE.write_text(json.dumps(value))
            with self.subTest(field=field), self.assertRaises(AssertionError):
                self.mod.fixture()
        self.old_unchanged()

    def test_access_updates_only_exact_EF_with_LAN_permissions(self):
        value = self.created()
        writes = []
        def api(path, method='GET', body=None):
            if path == self.mod.BASE + 'access/employees':
                return [{'employeeID': account['employeeID'], 'uid': account['uid'], 'eligible': True, 'revision': 3}
                        for account in value['accounts']] + [{'employeeID': 'unrelated', 'uid': 555, 'eligible': True, 'revision': 1}]
            if path == self.mod.BASE + 'access/employee' and method == 'POST':
                writes.append(body)
                return {}
            return self.api(path, method, body)
        self.mod.api = api
        with contextlib.redirect_stdout(io.StringIO()):
            self.mod.access()
        self.assertEqual({row['employeeID'] for row in writes}, {'new-e', 'new-f'})
        self.assertTrue(all(row['expectedRevision'] == 3 and row['allowLAN'] and row['allowPC'] and row['allowMobile']
                            and not row['allowInternet'] and not row['allowInternetDownload'] for row in writes))
        self.old_unchanged()

    def test_attachment_forbids_group_tests_before_any_HTTP_or_credentials(self):
        self.mod.api = mock.Mock(side_effect=AssertionError('No HTTP permitted'))
        self.mod.hrm_client = mock.Mock(side_effect=AssertionError('No private authentication read permitted'))
        for action in ('department', 'chat', 'directory'):
            with self.subTest(action=action), self.assertRaisesRegex(AssertionError, 'forbids'):
                self.mod.main([action, '--fixture-scope', 'attachment-20261103'])
        self.mod.api.assert_not_called()
        self.mod.hrm_client.assert_not_called()
        self.old_unchanged()

    def test_captcha_login_state_and_report_are_isolated_and_default_restored(self):
        value = self.created()
        fake = types.SimpleNamespace(PRIVATE=self.private, SLOTS={'aweb': (0, 'web'), 'bweb': (1, 'web')}, save=lambda *args: None)
        def captchas(slots):
            self.assertEqual(slots, ['eweb', 'fweb'])
            for slot in slots:
                fake.save(fake.PRIVATE / ('identity-captcha-' + slot + '.json'), {'show': True})
        def login(slot, answers):
            index = fake.SLOTS[slot][0]
            self.assertEqual(answers[slot], 'offline-answer')
            session = {'userID': 'emp_' + hashlib.sha256(value['accounts'][index]['employeeID'].encode()).hexdigest()[:32],
                       'sessionToken': 'offline-session-not-a-credential'}
            fake.save(fake.STATE, {slot: session})
            fake.save(fake.REPORT, [{'passed': True}])
            return session
        fake.captchas, fake.login = captchas, login
        with mock.patch.dict(sys.modules, {'production_identity_acceptance': fake}):
            self.mod.main(['captchas', '--fixture-scope', 'attachment-20261103'])
            answers = self.mod.EVIDENCE / 'captcha-answers.json'
            answers.write_text(json.dumps({'eweb': 'offline-answer', 'fweb': 'offline-answer'}))
            self.mod.main(['login', '--fixture-scope', 'attachment-20261103', '--answers', str(answers)])
            self.assertEqual(fake.PRIVATE, self.private / 'attachment-20261103')
            self.assertTrue(self.mod.STATE.is_file() and self.mod.REPORT.is_file())
            self.assertTrue((fake.PRIVATE / 'identity-captcha-eweb.json').is_file())
            self.mod.configure_scope('organization-20261103')
            self.mod.identity_module()
            self.assertEqual(fake.PRIVATE, self.private)
            self.assertEqual(set(fake.SLOTS), {'aweb', 'bweb'})
        self.old_unchanged()


if __name__ == '__main__':
    unittest.main()
