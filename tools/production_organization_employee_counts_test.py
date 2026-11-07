"""Offline four-QA exclusion and post-cleanup counting; no credentials/HTTP."""
import copy
import hashlib
import importlib.util
import json
import pathlib
import sys
import tempfile
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))
from tools.production_employee_counts_test import inputs as original_inputs

spec = importlib.util.spec_from_file_location('four_qa_counts_tested', ROOT / 'tools/production_organization_employee_counts.py')
counts = importlib.util.module_from_spec(spec)
spec.loader.exec_module(counts)


def inputs(after=False, disabled=True, full=False):
    replies, old = original_inputs(after=True)
    old['instanceID'] = 'owner'
    current = [{'uid': 306, 'employeeID': 'qa-c', 'username': 'qa_261103_c', 'name': 'Synthetic C',
                'mobile': 'synthetic-mobile-c', 'hostMobile': '', 'departmentID': 53},
               {'uid': 307, 'employeeID': 'qa-d', 'username': 'qa_261103_d', 'name': 'Synthetic D',
                'mobile': 'synthetic-mobile-d', 'hostMobile': '', 'departmentID': 53}]
    new = {'format': 'puty-fixture-cleanup-v1', 'instanceID': 'owner', 'namespace': 'organization-20261103',
           'departmentID': 53, 'fixtures': [{'expected': row} for row in current],
           'excludedOriginalUIDs': [901, 902], 'excludedOriginalEmployeeIDs': ['qa-a', 'qa-b']}
    if not after:
        replies['hrmEmployees']['data'].extend({'id': row['employeeID'], 'user_id': row['uid'],
            'employee_no': row['username'], 'host_username': row['username'], 'name': row['name'], 'wecom_userid': '',
            'mobile': row['mobile'], 'dept_id': 53, 'status': 'inactive' if disabled else 'active'} for row in current)
    for row in current:
        replies['imProjections']['data'].append({'id': row['employeeID'], 'status': 'ready',
            'desiredVersion': 2, 'appliedVersion': 2, 'error': '', 'projection': {
                'employeeID': row['employeeID'], 'hostUID': row['uid'], 'account': row['username'],
                'userID': counts.base.im_id(row['employeeID']), 'enabled': not disabled}})
    if full:
        for number in range(299):
            identity = 'real-extra-' + str(number)
            uid = 1000 + number
            replies['hrmEmployees']['data'].append({'id': identity, 'user_id': uid, 'status': 'active'})
            replies['imProjections']['data'].append({'id': identity, 'status': 'ready', 'desiredVersion': 1,
                'appliedVersion': 1, 'error': '', 'projection': {'userID': counts.base.im_id(identity),
                                                             'hostUID': uid, 'enabled': True}})
    replies['hrmSummary']['data']['employees'] = len(replies['hrmEmployees']['data'])
    return replies, old, new


def historical_inputs(after=True, full=False):
    replies, old, new = inputs(after=after, full=full)
    baseline = copy.deepcopy([r for r in replies['hrmEmployees']['data'] if not r['id'].startswith('qa-')])
    original = next(r for r in baseline if r['id'] == 'real-missing')
    original.update(user_id=0, name='Synthetic historical source')
    replies['hrmEmployees']['data'] = [r for r in replies['hrmEmployees']['data'] if r['id'] != original['id']]
    replies['hrmSummary']['data']['employees'] = len(replies['hrmEmployees']['data'])
    row = next(r for r in replies['imProjections']['data'] if r['id'] == original['id'])
    row['projection'].update(employeeID=original['id'], hostUID=0, account='')
    current = {**original, 'status': 'filtered'}
    request = {'baselineEmployee': original, 'reasonSHA256': hashlib.sha256(row['projection']['reason'].encode()).hexdigest()}
    safe = {'historicalClassificationProven': True, 'sourcePhysicallyAbsent': False, 'businessDataModified': False,
            'employeeRowCount': 1, 'employeeStatusCategory': 'filtered', 'excludedByStatusFiltered': True,
            'baselineWeComUIDAndNoHostUIDAndEmployeeNumberMatch': True, 'employeeNameMatchesBaseline': True,
            'retainedProjectionStrictlyBoundDisabledAndNeverApplied': True,
            'excludedFromCurrentHRMViewByExactSourceRules': True, 'excludedByExplicitRules': True,
            'explicitRuleHits': {'explicitUserName': True}}
    proof = {'format': 'puty-organization-count-proof-v1', 'instanceID': old['instanceID'], 'safe': safe,
             'privateEvidence': {'baselineEmployee': original, 'currentEmployee': current, 'retainedProjection': {
                 'employeeID': original['id'], 'nativeUserID': row['projection']['userID'], 'uid': 0,
                 'enabled': 0, 'provisioned': 0, 'status': 'waiting', 'desiredVersion': row['desiredVersion'],
                 'appliedVersion': 0, 'projection': copy.deepcopy(row['projection'])}}}
    evidence = {'proof': proof, 'request': request, 'baseline': baseline, 'captured': copy.deepcopy(row), 'proofSHA256': 'f' * 64}
    return replies, old, new, evidence


class OrganizationCountsTests(unittest.TestCase):
    def test_before_cleanup_excludes_new_pair_and_preserves_raw_api_responses(self):
        replies, old, new = inputs(disabled=False)
        untouched = copy.deepcopy(replies)
        report = counts.summarize(replies, old, new, 'before-cleanup')
        self.assertEqual(replies, untouched)
        self.assertEqual(report['HRMEmployees']['realCount'], 3)
        self.assertEqual(report['HRMEmployees']['currentTotal'], 5)
        self.assertEqual(report['HRMEmployees']['syntheticCount'], 2)
        self.assertEqual(report['IMProjections']['total'], 7)
        self.assertEqual(report['IMProjections']['syntheticCount'], 4)
        self.assertEqual(report['IMProjections']['syntheticDisabledCount'], 2)
        self.assertFalse(report['expectedProductionCountsMatch'])

    def test_after_cleanup_has_four_disabled_retained_projections_and_no_qa_employees(self):
        report = counts.summarize(*inputs(after=True), 'after-cleanup')
        self.assertEqual(report['HRMEmployees']['syntheticByCohort'], {'originalAB': 0, 'organizationCD': 0})
        self.assertEqual(report['IMProjections']['retainedQAByCohort'], {'originalAB': 2, 'organizationCD': 2})
        self.assertEqual(report['IMProjections']['syntheticDisabledCount'], 4)
        self.assertEqual(report['IMProjections']['realStates'], {'ready': 1, 'waiting': 2})
        self.assertEqual(report['IMProjections']['realWaitingCategories'],
                         {'missing_explicit_wecom_employee_number': 1, 'non_active_employee': 1})

    def test_expected_302_and_300_real_ready_assertion_is_computed_from_actual_rows(self):
        report = counts.summarize(*inputs(after=True, full=True), 'after-cleanup')
        self.assertTrue(report['expectedProductionCountsMatch'])
        self.assertEqual(report['HRMEmployees']['realCount'], 302)
        self.assertEqual(report['IMProjections']['realStates'], {'ready': 300, 'waiting': 2})
        self.assertEqual(report['HRMEmployees']['realStatus'], {'active': 301, 'resigned': 1})

    def test_after_cleanup_rejects_remaining_cd_source_or_enabled_retained_token_identity(self):
        with self.assertRaisesRegex(RuntimeError, 'C/D employee rows remain'):
            counts.summarize(*inputs(), 'after-cleanup')
        with self.assertRaisesRegex(RuntimeError, 'C/D projection remains enabled'):
            counts.summarize(*inputs(after=True, disabled=False), 'after-cleanup')

    def test_partial_wrong_mobile_or_alias_bindings_rejected(self):
        for changed in ('mobile', 'department', 'alias-employee', 'alias-projection', 'missing-projection'):
            replies, old, new = inputs()
            if changed == 'mobile': replies['hrmEmployees']['data'][-1]['mobile'] = 'different'
            if changed == 'department': replies['hrmEmployees']['data'][-1]['dept_id'] = 54
            if changed == 'alias-employee': replies['hrmEmployees']['data'][0]['user_id'] = 306
            if changed == 'alias-projection': replies['imProjections']['data'][0]['projection']['hostUID'] = 307
            if changed == 'missing-projection': replies['imProjections']['data'].pop()
            with self.subTest(changed=changed), self.assertRaises(RuntimeError):
                counts.summarize(replies, old, new, 'before-cleanup')

    def test_supplemental_scope_must_bind_original_pair_disjoint_namespace_owner_and_exact_cd(self):
        for changed in ('namespace', 'old-uid', 'old-employee', 'owner', 'username'):
            replies, old, new = inputs()
            if changed == 'namespace': new['namespace'] = 'another'
            if changed == 'old-uid': new['excludedOriginalUIDs'] = [306, 902]
            if changed == 'old-employee': new['excludedOriginalEmployeeIDs'] = ['unrelated', 'qa-b']
            if changed == 'owner': new['instanceID'] = 'other'
            if changed == 'username': new['fixtures'][0]['expected']['username'] = 'qa_other'
            with self.subTest(changed=changed), self.assertRaises(RuntimeError):
                counts.summarize(replies, old, new, 'before-cleanup')

    def test_original_counter_defaults_untouched(self):
        self.assertEqual(counts.base.PLAN.name, 'fixture-cleanup-plan.private.json')
        self.assertNotEqual(counts.ORG_PLAN, counts.base.PLAN)
        self.assertEqual(counts.ORG_PLAN.name, 'organization-20261103-cleanup-plan.private.json')

    def test_filtered_one_is_audited_as_history_and_302_to_301_is_visible(self):
        replies, old, new, history = historical_inputs(full=True)
        original = copy.deepcopy(replies)
        report = counts.summarize(replies, old, new, 'after-cleanup', history)
        self.assertEqual(replies, original)
        self.assertEqual(report['HRMEmployees']['realCount'], 301)
        self.assertEqual(report['HRMEmployees']['realStatus'], {'active': 300, 'resigned': 1})
        self.assertEqual(report['HRMEmployees']['historicalBaselineRealCount'], 302)
        self.assertEqual(report['HRMEmployees']['currentViewChangeFromBaseline'], -1)
        self.assertEqual(report['HRMEmployees']['physicallyDeletedHistoricalSourceRowCount'], 0)
        self.assertEqual(report['IMProjections']['realStates'], {'ready': 300, 'waiting': 1})
        self.assertEqual(report['IMProjections']['historicalFilteredCount'], 1)
        self.assertEqual(report['IMProjections']['totalRealIncludingHistorical'], 302)
        self.assertEqual(report['IMProjections']['syntheticCount'], 4)
        self.assertEqual(report['IMProjections']['total'], 306)
        self.assertFalse(report['original302BaselineCountsUnchanged'])
        self.assertTrue(report['expectedProductionCountsMatch'])

    def test_historical_is_opt_in_and_never_ignores_any_other_unmatched_identity(self):
        replies, old, new, history = historical_inputs()
        with self.assertRaisesRegex(RuntimeError, 'identities differ'):
            counts.summarize(replies, old, new, 'after-cleanup')
        extra = copy.deepcopy(replies['imProjections']['data'][0])
        extra['id'] = 'unexpected-real-orphan'
        replies['imProjections']['data'].append(extra)
        with self.assertRaisesRegex(RuntimeError, 'identities differ'):
            counts.summarize(replies, old, new, 'after-cleanup', history)

    def test_historical_sql_null_and_api_coalesced_empty_are_same_unassigned_fields(self):
        replies, old, new, history = historical_inputs()
        current = history['proof']['privateEvidence']['currentEmployee']
        current.update(employee_no=None, user_id=None)
        self.assertEqual(counts.summarize(replies, old, new, 'after-cleanup', history)['HRMEmployees']['realCount'], 2)
        for field, value in [('employee_no', 'not-empty'), ('user_id', 5)]:
            changed = copy.deepcopy(history)
            changed['proof']['privateEvidence']['currentEmployee'][field] = value
            with self.assertRaises(RuntimeError):
                counts.summarize(replies, old, new, 'after-cleanup', changed)

    def test_historical_mapping_rule_status_enabled_and_applied_changes_rejected(self):
        for change in ('mapping', 'rule', 'status', 'enabled', 'applied', 'provisioned', 'source-visible', 'capture'):
            replies, old, new, history = historical_inputs()
            detail, safe = history['proof']['privateEvidence'], history['proof']['safe']
            row = next(r for r in replies['imProjections']['data'] if r['id'] == 'real-missing')
            if change == 'mapping': detail['currentEmployee']['wecom_userid'] = 'different'
            if change == 'rule': safe['explicitRuleHits']['explicitUserName'] = False
            if change == 'status': detail['currentEmployee']['status'] = 'active'
            if change == 'enabled': row['projection']['enabled'] = True
            if change == 'applied': row['appliedVersion'] = 1
            if change == 'provisioned': detail['retainedProjection']['provisioned'] = 1
            if change == 'source-visible': replies['hrmEmployees']['data'].append(detail['currentEmployee']); replies['hrmSummary']['data']['employees'] += 1
            if change == 'capture': row['projection']['reason'] = 'changed'
            with self.subTest(change=change), self.assertRaises(RuntimeError):
                counts.summarize(replies, old, new, 'after-cleanup', history)

    def test_private_capture_is_exclusive_preserved_and_protected(self):
        with tempfile.TemporaryDirectory() as folder:
            protected = []
            path = pathlib.Path(folder)
            counts.save_private(path, 'hrmEmployees', {'code': 0, 'data': []}, protected.append)
            self.assertEqual(json.loads((path / 'hrmEmployees.private.json').read_text()), {'code': 0, 'data': []})
            self.assertEqual(protected, [path / 'hrmEmployees.private.json'])
            with self.assertRaises(FileExistsError):
                counts.save_private(path, 'hrmEmployees', {'other': True}, protected.append)

    def test_unreviewed_historical_proof_digest_is_rejected(self):
        with tempfile.TemporaryDirectory() as folder:
            proof = pathlib.Path(folder) / 'proof.json'
            proof.write_text('{}')
            with self.assertRaisesRegex(RuntimeError, 'independently reviewed exact SQL proof'):
                counts.load_historical_proof(proof, '0' * 64)


if __name__ == '__main__':
    unittest.main()
