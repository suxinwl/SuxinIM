"""Offline-only C/D cleanup identity, mobile, department and reference guards."""
import contextlib
import importlib.util
import io
import json
import pathlib
import tempfile
import types
import unittest
from unittest import mock

ROOT = pathlib.Path(__file__).resolve().parents[1]


def load(name, path):
    spec = importlib.util.spec_from_file_location(name, ROOT / path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


local = load('organization_cleanup_tested', 'tools/production_organization_fixture_cleanup.py')
remote = load('organization_cleanup_remote_tested', 'deploy/production/fixtures_cleanup.py')
original = load('original_cleanup_untouched', 'tools/production_fixture_cleanup.py')


class OrganizationCleanupTests(unittest.TestCase):
    def source(self):
        new = {'namespace': local.NAMESPACE, 'departmentID': 53, 'accounts': [
            {'uid': 306, 'username': 'qa_261103_c', 'name': 'Synthetic C', 'employeeID': 'fixture-c', 'mobile': 'synthetic-phone-c', 'password': 'never-persist'},
            {'uid': 307, 'username': 'qa_261103_d', 'name': 'Synthetic D', 'employeeID': 'fixture-d', 'mobile': 'synthetic-phone-d', 'password': 'never-persist'}]}
        old = {'accounts': [{'uid': 904, 'employeeID': 'fixture-a'}, {'uid': 905, 'employeeID': 'fixture-b'}]}
        return new, old

    def expected(self):
        return local.source_scope(*self.source())[0]

    def plan(self):
        expected = [{**row, 'departmentID': 53, 'hostMobile': ''} for row in self.expected()]
        return {'format': 'puty-fixture-cleanup-v1', 'instanceID': 'owner', 'namespace': local.NAMESPACE,
                'departmentID': 53, 'fixtures': [{'expected': row} for row in expected],
                'excludedOriginalUIDs': [904, 905], 'excludedOriginalEmployeeIDs': ['fixture-a', 'fixture-b'],
                'fixtureSourceSHA256': 'new-source', 'originalFixtureSourceSHA256': 'old-source',
                'protectedGroupIDs': [local.PROTECTED_GROUP],
                'departmentSnapshot': {'id': 53, 'name': 'Synthetic department', 'pid': 0, 'remark': 'preserve',
                                       'status': 0, 'weigh': 53, 'business_id': 0}}

    def test_exact_namespace_uid_account_and_original_disjointness(self):
        expected, old_uids, old_ids = local.source_scope(*self.source())
        self.assertEqual({v['uid'] for v in expected}, {306, 307})
        self.assertFalse({v['uid'] for v in expected}.intersection(old_uids))
        self.assertNotIn('password', expected[0])
        self.assertEqual(expected[0]['mobile'], 'synthetic-phone-c')
        for change in ('namespace', 'uid', 'username', 'employee-overlap', 'uid-overlap', 'wecom'):
            new, old = self.source()
            if change == 'namespace': new['namespace'] = 'original'
            if change == 'uid': new['accounts'][0]['uid'] = 904
            if change == 'username': new['accounts'][0]['username'] = 'qa_other'
            if change == 'employee-overlap': new['accounts'][0]['employeeID'] = 'fixture-a'
            if change == 'uid-overlap': old['accounts'][0]['uid'] = 306
            if change == 'wecom': new['accounts'][0]['wecom_userid'] = 'real-source'
            with self.subTest(change=change), self.assertRaises(RuntimeError):
                local.source_scope(new, old)

    def test_original_defaults_and_evidence_paths_remain_distinct(self):
        before = (original.FIXTURES, original.PLAN, original.PROGRESS)
        common = local.configured(self.expected())
        self.assertEqual((original.FIXTURES, original.PLAN, original.PROGRESS), before)
        self.assertEqual((common.FIXTURES, common.PLAN, common.PROGRESS), (local.FIXTURES, local.PLAN, local.PROGRESS))
        self.assertNotEqual(local.PLAN, original.PLAN)
        self.assertNotEqual(local.PROGRESS, original.PROGRESS)

    def test_mobile_department_and_empty_wecom_bindings_are_exact(self):
        expected = self.expected()
        common = local.configured(expected)
        row = expected[0]
        host = {'id': row['uid'], 'username': row['username'], 'name': row['name'], 'mobile': ''}
        employee = {'id': row['employeeID'], 'user_id': row['uid'], 'employee_no': row['username'],
                    'name': row['name'], 'mobile': row['mobile'], 'dept_id': 53, 'wecom_userid': ''}
        common.binding({**row, 'hostMobile': ''}, host, employee)
        for changed in ({**employee, 'mobile': ''}, {**employee, 'dept_id': 54},
                        {**employee, 'wecom_userid': 'real'}, {**employee, 'name': 'Another person'}):
            with self.assertRaises(RuntimeError):
                common.binding(row, host, changed)
        with self.assertRaises(RuntimeError):
            common.binding({**row, 'hostMobile': 'unexpected'}, host, employee)

    def test_no_release_rejected_before_private_read_or_api(self):
        with mock.patch.object(local, 'load_scope') as source, mock.patch.object(local, 'configured') as api:
            with self.assertRaisesRegex(RuntimeError, 'release'):
                local.quiesce(False)
            source.assert_not_called()
            api.assert_not_called()

    def test_department_leader_cleared_normally_before_generic_quiesce_without_group_mutation(self):
        value = self.plan()
        dept = {**value['departmentSnapshot'], 'leader_uid': 307, 'im_group_enabled': 0}
        calls, saves = [], []
        def api(path, method='GET', payload=None):
            calls.append((path, method, payload))
            if method == 'GET':
                return [{'id': 1, 'children': [dict(dept)]}]
            self.assertEqual(path, '/admin/system/dept/save')
            self.assertEqual(payload, {'id': 53, 'name': 'Synthetic department', 'pid': 0, 'remark': 'preserve',
                                       'status': 0, 'weigh': 53, 'im_group_enabled': 0, 'leader_uid': 0})
            dept['leader_uid'] = 0
        def quiesce(released):
            self.assertTrue(released)
            self.assertEqual(dept['leader_uid'], 0)
            calls.append(('generic-quiesce', None, None))
        common = types.SimpleNamespace(api=api, items=lambda value: value, current_rows=lambda rows: None,
                                       save=lambda path, v: saves.append((path, v)), quiesce=quiesce)
        with tempfile.TemporaryDirectory() as tmp:
            plan = pathlib.Path(tmp) / 'organization-plan.json'
            plan.write_text(json.dumps(value), encoding='utf-8')
            with mock.patch.object(local, 'PLAN', plan), mock.patch.object(local, 'PROGRESS', pathlib.Path(tmp) / 'progress.json'), \
                 mock.patch.object(local, 'load_scope', return_value=(self.expected(), [904, 905], ['fixture-a', 'fixture-b'], 'new-source', 'old-source')), \
                 mock.patch.object(local, 'configured', return_value=common):
                local.quiesce(True)
        self.assertEqual(calls[-1][0], 'generic-quiesce')
        self.assertTrue(saves)
        self.assertEqual(saves[0][1]['namespace'], local.NAMESPACE)
        self.assertTrue(saves[0][1]['steps'][0].startswith('department_53_OFF'))
        self.assertFalse(any('dismiss' in c[0] or 'messages/' in c[0] for c in calls))

    def test_changed_department_snapshot_stops_before_any_mutation(self):
        value = self.plan()
        dept = {**value['departmentSnapshot'], 'remark': 'changed since review', 'leader_uid': 307, 'im_group_enabled': 0}
        calls = []
        common = types.SimpleNamespace(items=lambda v: v, current_rows=lambda rows: None,
                                       api=lambda path, method='GET', data=None: calls.append(method) or [dept])
        with tempfile.TemporaryDirectory() as tmp:
            plan = pathlib.Path(tmp) / 'plan.json'
            plan.write_text(json.dumps(value), encoding='utf-8')
            with mock.patch.object(local, 'PLAN', plan), \
                 mock.patch.object(local, 'load_scope', return_value=(self.expected(), [904, 905], ['fixture-a', 'fixture-b'], 'new-source', 'old-source')), \
                 mock.patch.object(local, 'configured', return_value=common), self.assertRaises(RuntimeError):
                local.quiesce(True)
        self.assertEqual(calls, ['GET'])

    def test_server_new_scope_retains_original_validator_and_exact_phone_sql(self):
        value = self.plan()
        fixtures = remote.validate_plan(value, 'owner')
        predicate = remote.employee_predicate(fixtures)
        for text in ('e.dept_id=53', "COALESCE(e.mobile,'')=", "COALESCE(a.mobile,'')=", "COALESCE(e.wecom_userid,'')=''"):
            self.assertIn(text, predicate)
        self.assertNotIn('synthetic-phone', predicate)  # Values remain hex SQL literals.
        for changed in ({**value, 'namespace': 'anything'}, {**value, 'departmentID': 54},
                        {**value, 'excludedOriginalUIDs': [306, 905]}):
            with self.assertRaises(RuntimeError):
                remote.validate_plan(changed, 'owner')

    def test_department_leader_uid_is_a_blocking_business_reference(self):
        queries = []
        db = types.SimpleNamespace(json=lambda query: [], one=lambda query: queries.append(query) or '1')
        refs, foreign = remote.references(db, remote.validate_plan(self.plan(), 'owner'),
                                         [{'table': 'gf_auth_dept', 'column': 'leader_uid', 'type': 'bigint'}])
        self.assertEqual(refs, [{'table': 'gf_auth_dept', 'column': 'leader_uid', 'count': 1}])
        self.assertEqual(sum(r['count'] for r in refs), 1)
        self.assertIn('`leader_uid` IN (306,307)', queries[0])
        self.assertFalse(foreign)


if __name__ == '__main__':
    unittest.main()
