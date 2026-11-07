"""Offline six-QA exclusion with the original historical proof left intact."""
import copy
import hashlib
import pathlib
import sys
import unittest
from unittest import mock

ROOT = pathlib.Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))
from tools.production_organization_employee_counts_test import counts, historical_inputs
from tools.production_attachment_fixture_cleanup_test import remote


class AttachmentCountsTests(unittest.TestCase):
    def setUp(self):
        hashes = {308: hashlib.sha256(b'qa-e').hexdigest(), 309: hashlib.sha256(b'qa-f').hexdigest()}
        self.hashes = mock.patch.dict(remote.ATTACHMENT_IDS, hashes, clear=True)
        self.hashes.start()
        self.validator = mock.patch.object(counts, 'validate_attachment_scope', side_effect=remote.validate_plan)
        self.validator.start()

    def tearDown(self):
        self.validator.stop()
        self.hashes.stop()

    def inputs(self, after=False):
        replies, old, org, historical = historical_inputs(after=True, full=True)
        accounts = [{'uid': uid, 'employeeID': 'qa-' + suffix, 'username': 'qa_261103_' + suffix,
                     'name': 'IM attachment acceptance ' + suffix.upper(), 'mobile': mobile, 'hostMobile': mobile,
                     'departmentID': 53, 'hostDepartmentID': 53}
                    for uid, suffix, mobile in ((308, 'e', '00000000005'), (309, 'f', '00000000006'))]
        plan = {'format': 'puty-fixture-cleanup-v1', 'instanceID': 'owner', 'namespace': 'attachment-20261103',
                'departmentID': 53, 'fixtures': [{'expected': account} for account in accounts],
                'excludedOriginalUIDs': [306, 307, 901, 902],
                'excludedOriginalEmployeeIDs': ['qa-a', 'qa-b', 'qa-c', 'qa-d']}
        for account in accounts:
            if not after:
                replies['hrmEmployees']['data'].append({'id': account['employeeID'], 'user_id': account['uid'],
                    'employee_no': account['username'], 'host_username': account['username'], 'name': account['name'],
                    'mobile': account['mobile'], 'dept_id': 53, 'status': 'active', 'wecom_userid': ''})
            replies['imProjections']['data'].append({'id': account['employeeID'], 'status': 'ready' if not after else 'disabled',
                'desiredVersion': 2, 'appliedVersion': 2, 'error': '', 'projection': {'employeeID': account['employeeID'],
                    'hostUID': account['uid'], 'account': account['username'], 'userID': counts.base.im_id(account['employeeID']),
                    'enabled': not after}})
        replies['hrmSummary']['data']['employees'] = len(replies['hrmEmployees']['data'])
        return replies, old, org, historical, plan

    def report(self, after=False):
        replies, old, org, historical, plan = self.inputs(after)
        return counts.summarize(replies, old, org, 'after-cleanup' if after else 'before-cleanup', historical, plan)

    def test_before_excludes_only_exact_ef_and_preserves_raw_responses(self):
        replies, old, org, historical, plan = self.inputs()
        original = copy.deepcopy(replies)
        report = counts.summarize(replies, old, org, 'before-cleanup', historical, plan)
        self.assertEqual(original, replies)
        self.assertEqual(report['HRMEmployees']['currentTotal'], 303)
        self.assertEqual(report['HRMEmployees']['realCount'], 301)
        self.assertEqual(report['HRMEmployees']['syntheticByCohort'], {'originalAB': 0, 'organizationCD': 0, 'attachmentEF': 2})
        self.assertEqual(report['IMProjections']['syntheticCount'], 6)
        self.assertTrue(report['expectedProductionCountsMatch'])

    def test_after_requires_zero_source_and_six_disabled_retained(self):
        report = self.report(True)
        self.assertEqual(report['HRMEmployees']['syntheticCount'], 0)
        self.assertEqual(report['IMProjections']['syntheticDisabledCount'], 6)
        self.assertEqual(report['IMProjections']['retainedQAByCohort'], {'originalAB': 2, 'organizationCD': 2, 'attachmentEF': 2})
        self.assertTrue(report['sixQAScopesExactlyBoundAndExcluded'])

    def test_retained_history_is_separate_and_never_physically_deleted(self):
        report = self.report(True)
        self.assertEqual(report['HRMEmployees']['realCount'], 301)
        self.assertEqual(report['IMProjections']['realStates'], {'ready': 300, 'waiting': 1})
        self.assertEqual(report['historicalExclusionAudit']['count'], 1)
        self.assertIs(report['historicalExclusionAudit']['sourcePhysicallyDeleted'], False)

    def test_wrong_owner_exact_ids_or_exclusions_fail(self):
        for change in ('owner', 'uid', 'employee', 'exclusion'):
            replies, old, org, historical, plan = self.inputs()
            if change == 'owner': plan['instanceID'] = 'another'
            if change == 'uid': plan['fixtures'][0]['expected']['uid'] = 306
            if change == 'employee': plan['fixtures'][0]['expected']['employeeID'] = 'other'
            if change == 'exclusion': plan['excludedOriginalUIDs'] = [306, 307, 901, 999]
            with self.subTest(change=change), self.assertRaises(RuntimeError):
                counts.summarize(replies, old, org, 'before-cleanup', historical, plan)

    def test_alias_wrong_phone_and_missing_projection_fail(self):
        for change in ('employee-alias', 'projection-alias', 'mobile', 'missing'):
            replies, old, org, historical, plan = self.inputs()
            if change == 'employee-alias': replies['hrmEmployees']['data'][0]['user_id'] = 308
            if change == 'projection-alias': replies['imProjections']['data'][0]['projection']['hostUID'] = 309
            if change == 'mobile': replies['hrmEmployees']['data'][-1]['mobile'] = 'other'
            if change == 'missing': replies['imProjections']['data'].pop()
            with self.subTest(change=change), self.assertRaises(RuntimeError):
                counts.summarize(replies, old, org, 'before-cleanup', historical, plan)

    def test_after_rejects_live_source_enabled_or_unapplied_projection(self):
        for change in ('source', 'enabled', 'version'):
            replies, old, org, historical, plan = self.inputs(after=change != 'source')
            if change == 'enabled': replies['imProjections']['data'][-1]['projection']['enabled'] = True
            if change == 'version': replies['imProjections']['data'][-1]['appliedVersion'] = 1
            with self.subTest(change=change), self.assertRaises(RuntimeError):
                counts.summarize(replies, old, org, 'after-cleanup', historical, plan)


if __name__ == '__main__':
    unittest.main()
