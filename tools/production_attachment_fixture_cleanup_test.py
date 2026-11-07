"""Offline exact E/F cleanup profile; no credentials, HTTP or database access."""
import copy
import hashlib
import importlib.util
import json
import pathlib
import tempfile
import types
import unittest
from unittest import mock

ROOT = pathlib.Path(__file__).resolve().parents[1]


def load(name, relative):
    spec = importlib.util.spec_from_file_location(name, ROOT / relative)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


local = load('attachment_cleanup_local', 'tools/production_organization_fixture_cleanup.py')
remote = load('attachment_cleanup_remote', 'deploy/production/fixtures_cleanup.py')


class AttachmentCleanupTests(unittest.TestCase):
    def setUp(self):
        local.configure_scope('attachment-20261103')
        self.hashes = {308: hashlib.sha256(b'fixture-e').hexdigest(), 309: hashlib.sha256(b'fixture-f').hexdigest()}
        self.patches = [mock.patch.dict(module.ATTACHMENT_IDS, self.hashes, clear=True) for module in (local, remote)]
        for patch in self.patches:
            patch.start()

    def tearDown(self):
        for patch in self.patches:
            patch.stop()
        local.configure_scope('organization-20261103')

    def sources(self):
        old = {'accounts': [{'uid': 904, 'employeeID': 'fixture-a'}, {'uid': 905, 'employeeID': 'fixture-b'},
                            {'uid': 306, 'employeeID': 'fixture-c'}, {'uid': 307, 'employeeID': 'fixture-d'}]}
        source = {'namespace': 'attachment-20261103', 'departmentID': 53,
                  'excludedPreviousUIDs': [306, 307, 904, 905],
                  'excludedPreviousEmployeeIDs': ['fixture-a', 'fixture-b', 'fixture-c', 'fixture-d'],
                  'accounts': [{'uid': uid, 'username': 'qa_261103_' + suffix, 'name': 'IM attachment acceptance ' + suffix.upper(),
                                'employeeID': 'fixture-' + suffix, 'mobile': phone, 'password': 'never-in-plan'}
                               for uid, suffix, phone in ((308, 'e', '00000000005'), (309, 'f', '00000000006'))]}
        return source, old

    def plan(self):
        expected, excluded, excluded_ids = local.source_scope(*self.sources())
        return {'format': 'puty-fixture-cleanup-v1', 'instanceID': 'owner', 'namespace': local.NAMESPACE,
                'departmentID': 53, 'fixtures': [{'expected': {**e, 'departmentID': 53, 'hostDepartmentID': 53, 'hostMobile': e['mobile']}} for e in expected],
                'excludedOriginalUIDs': excluded, 'excludedOriginalEmployeeIDs': excluded_ids,
                'fixtureSourceSHA256': 'new-source', 'originalFixtureSourceSHA256': 'four-old-sources',
                'protectedGroupIDs': [local.PROTECTED_GROUP],
                'departmentSnapshot': {'id': 53, 'name': 'TEMP QA 20261103', 'pid': 0,
                    'remark': 'Temporary synthetic organization acceptance department', 'status': 0, 'weigh': 9999, 'business_id': 1}}

    def test_exact_pair_four_exclusions_and_separate_default_paths(self):
        expected, excluded, _ = local.source_scope(*self.sources())
        self.assertEqual({e['uid'] for e in expected}, {308, 309})
        self.assertEqual(len(excluded), 4)
        self.assertNotIn('password', expected[0])
        self.assertIn('attachment-20261103', str(local.PLAN))
        self.assertIn('attachment-20261103', str(local.PROGRESS))
        local.configure_scope('organization-20261103')
        self.assertEqual(local.ACCOUNTS, {306: 'qa_261103_c', 307: 'qa_261103_d'})
        self.assertEqual(local.PLAN.name, 'organization-20261103-cleanup-plan.private.json')
        self.assertEqual(local.PROGRESS.name, 'organization-20261103-cleanup-progress.private.json')

    def test_namespace_uid_employee_sha_mobile_and_old_overlap_fail_closed(self):
        for change in ('namespace', 'uid', 'employee', 'name', 'mobile', 'wecom', 'old-uid', 'old-id', 'exclusion'):
            source, old = self.sources()
            if change == 'namespace': source['namespace'] = 'organization-20261103'
            if change == 'uid': source['accounts'][0]['uid'] = 306
            if change == 'employee': source['accounts'][0]['employeeID'] = 'different-employee'
            if change == 'name': source['accounts'][0]['name'] = 'Other identity'
            if change == 'mobile': source['accounts'][0]['mobile'] = 'not-original'
            if change == 'wecom': source['accounts'][0]['wecom_userid'] = 'upstream-real'
            if change == 'old-uid': old['accounts'][0]['uid'] = 308
            if change == 'old-id': old['accounts'][0]['employeeID'] = 'fixture-e'
            if change == 'exclusion': source['excludedPreviousUIDs'] = [904, 905]
            with self.subTest(change=change), self.assertRaises(RuntimeError):
                local.source_scope(source, old)

    def test_no_release_fails_before_private_read_or_network(self):
        with mock.patch.object(local, 'load_scope') as load_scope, mock.patch.object(local, 'configured') as client:
            with self.assertRaisesRegex(RuntimeError, 'release'):
                local.quiesce(False)
            load_scope.assert_not_called()
            client.assert_not_called()

    def test_attachment_quiesce_only_reads_protected_department(self):
        value = self.plan()
        expected, old, old_ids = local.source_scope(*self.sources())
        dept = {**value['departmentSnapshot'], 'leader_uid': 0, 'im_group_enabled': 0}
        calls, saves = [], []
        def api(path, method='GET', payload=None):
            calls.append((path, method))
            self.assertEqual((path, method), ('/admin/system/dept/getList', 'GET'))
            return [dict(dept)]
        common = types.SimpleNamespace(api=api, items=lambda v: v, current_rows=lambda rows: None,
                                       save=lambda path, v: saves.append((path, v)),
                                       quiesce=lambda released: calls.append(('generic-scoped-quiesce', released)))
        with tempfile.TemporaryDirectory() as tmp:
            plan = pathlib.Path(tmp) / 'plan.json'; plan.write_text(json.dumps(value))
            with mock.patch.object(local, 'PLAN', plan), mock.patch.object(local, 'PROGRESS', pathlib.Path(tmp) / 'progress.json'), \
                 mock.patch.object(local, 'load_scope', return_value=(expected, old, old_ids, 'new-source', 'four-old-sources')), \
                 mock.patch.object(local, 'configured', return_value=common):
                local.quiesce(True)
        self.assertEqual(calls[-1], ('generic-scoped-quiesce', True))
        self.assertTrue(saves[0][1]['steps'][0].startswith('department_53_read_only'))

    def test_department_drift_fails_without_writing(self):
        for field, changed in [('leader_uid', 309), ('im_group_enabled', 1), ('business_id', 0), ('name', 'Other'), ('remark', 'Other'), ('pid', 1)]:
            dept = {**self.plan()['departmentSnapshot'], 'leader_uid': 0, 'im_group_enabled': 0, field: changed}
            calls = []
            def api(path, method='GET'):
                calls.append(method)
                return [dept]
            common = types.SimpleNamespace(api=api, items=lambda v: v)
            with self.subTest(field=field), self.assertRaises(RuntimeError):
                local.department(common)
            self.assertEqual(calls, ['GET'])

    def test_server_exact_plan_and_employee_predicate(self):
        plan = self.plan()
        rows = remote.validate_plan(plan, 'owner')
        sql = remote.employee_predicate(rows)
        for predicate in ('e.dept_id=53', 'a.dept_id=53', 'e.user_id=308', 'e.user_id=309', "COALESCE(e.wecom_userid,'')=''"):
            self.assertIn(predicate, sql)
        for field, changed in [('uid', 306), ('employeeID', 'other'), ('hostMobile', ''), ('hostDepartmentID', 54), ('name', 'Other')]:
            wrong = copy.deepcopy(plan); wrong['fixtures'][0]['expected'][field] = changed
            with self.subTest(field=field), self.assertRaises(RuntimeError):
                remote.validate_plan(wrong, 'owner')

    def test_server_department_guard_is_one_read_only_query_and_zero_drift(self):
        queries = []
        db = types.SimpleNamespace(one=lambda sql: queries.append(sql) or '1')
        remote.check_attachment_department(db, self.plan())
        self.assertEqual(len(queries), 1)
        self.assertTrue(queries[0].startswith('SELECT COUNT(*)'))
        self.assertIn('im_group_enabled=0 AND leader_uid=0', queries[0])
        with self.assertRaises(RuntimeError):
            remote.check_attachment_department(types.SimpleNamespace(one=lambda sql: '0'), self.plan())


if __name__ == '__main__':
    unittest.main()
