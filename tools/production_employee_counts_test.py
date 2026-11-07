"""Offline checks for exact QA exclusion and final employee-count semantics."""
import copy
import unittest

from tools import production_employee_counts as counts


def inputs(after=False):
    fixtures = [{'employeeID': 'qa-a', 'uid': 901, 'username': 'qa_a', 'name': 'Synthetic A'},
                {'employeeID': 'qa-b', 'uid': 902, 'username': 'qa_b', 'name': 'Synthetic B'}]
    plan = {'format': 'puty-fixture-cleanup-v1', 'fixtures': [{'expected': row} for row in fixtures]}
    employees = [
        {'id': 'real-ready', 'user_id': 11, 'status': 'active', 'employee_no': 'R1', 'wecom_userid': 'source-ready'},
        {'id': 'real-missing', 'user_id': 12, 'status': 'active', 'employee_no': '', 'wecom_userid': 'source-missing'},
        {'id': 'real-resigned', 'user_id': 13, 'status': 'resigned', 'employee_no': 'R3', 'wecom_userid': 'source-resigned'},
    ]
    if not after:
        employees += [{'id': f['employeeID'], 'user_id': f['uid'], 'employee_no': f['username'],
                       'host_username': f['username'], 'name': f['name'], 'wecom_userid': '', 'status': 'active'}
                      for f in fixtures]
    projections = []
    for employee in employees[:3]:
        enabled = employee['id'] == 'real-ready'
        projections.append({'id': employee['id'], 'status': 'ready' if enabled else 'waiting',
                            'desiredVersion': 1, 'appliedVersion': 1 if enabled else 0, 'error': '',
                            'projection': {'userID': counts.im_id(employee['id']), 'hostUID': employee['user_id'],
                                           'enabled': enabled, 'reason': '' if enabled else 'fixed synthetic reason'}})
    for f in fixtures:
        projections.append({'id': f['employeeID'], 'status': 'ready', 'desiredVersion': 2, 'appliedVersion': 2,
                            'error': '', 'projection': {'employeeID': f['employeeID'], 'hostUID': f['uid'],
                            'account': f['username'], 'userID': counts.im_id(f['employeeID']), 'enabled': not after}})
    responses = {'hrmEmployees': {'code': 0, 'data': employees},
                 'hrmSummary': {'code': 0, 'data': {'employees': len(employees)}},
                 'imProjections': {'code': 0, 'data': projections}}
    return responses, plan


class EmployeeCounts(unittest.TestCase):
    def test_before_cleanup_excludes_exact_two_fixtures(self):
        report = counts.summarize(*inputs(), 'before-cleanup')
        self.assertEqual(report['HRMEmployees']['currentTotal'], 5)
        self.assertEqual(report['HRMEmployees']['realCount'], 3)
        self.assertEqual(report['IMProjections']['realStates'], {'ready': 1, 'waiting': 2})

    def test_after_cleanup_preserves_but_excludes_disabled_ready_tombstones(self):
        report = counts.summarize(*inputs(after=True), 'after-cleanup')
        self.assertEqual(report['HRMEmployees']['syntheticCount'], 0)
        self.assertEqual(report['IMProjections']['syntheticStates'], {'ready': 2})
        self.assertEqual(report['IMProjections']['syntheticDisabledCount'], 2)
        self.assertEqual(report['IMProjections']['realCount'], 3)
        self.assertEqual(report['HRMEmployees']['realStatus'], {'active': 2, 'resigned': 1})
        self.assertEqual(report['IMProjections']['realWaitingCategories'],
                         {'missing_explicit_wecom_employee_number': 1, 'non_active_employee': 1})

    def test_partial_or_ambiguous_fixture_binding_is_rejected(self):
        responses, plan = inputs()
        responses['imProjections']['data'][-1]['projection']['hostUID'] = 999
        with self.assertRaisesRegex(RuntimeError, 'QA projection binding'):
            counts.summarize(responses, plan, 'before-cleanup')

    def test_incomplete_pages_and_enabled_ready_inconsistency_are_rejected(self):
        responses, plan = inputs(after=True)
        changed = copy.deepcopy(responses)
        changed['hrmSummary']['data']['employees'] += 1
        with self.assertRaisesRegex(RuntimeError, 'list and summary'):
            counts.summarize(changed, plan, 'after-cleanup')
        responses['imProjections']['data'][0]['projection']['enabled'] = False
        with self.assertRaisesRegex(RuntimeError, 'ready projection'):
            counts.summarize(responses, plan, 'after-cleanup')

    def test_after_cleanup_rejects_remaining_qa_employee_or_enabled_projection(self):
        with self.assertRaisesRegex(RuntimeError, 'QA employee rows remain'):
            counts.summarize(*inputs(), 'after-cleanup')
        responses, plan = inputs(after=True)
        responses['imProjections']['data'][-1]['projection']['enabled'] = True
        with self.assertRaisesRegex(RuntimeError, 'QA projection remains enabled'):
            counts.summarize(responses, plan, 'after-cleanup')


if __name__ == '__main__':
    unittest.main()
