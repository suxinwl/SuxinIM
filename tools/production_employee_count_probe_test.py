"""Pure offline tests; synthetic identities only, no database or credential reads."""
import copy
import hashlib
import importlib.util
import pathlib
import re
import types
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location('count_difference_probe', ROOT / 'deploy/production/inspect_employee_count_difference.py')
probe = importlib.util.module_from_spec(spec)
spec.loader.exec_module(probe)


def inputs():
    employee = {'id': 'synthetic-proof', 'name': 'Synthetic Proof', 'user_id': 0,
                'employee_no': '', 'wecom_userid': 'synthetic-wecom', 'status': 'active'}
    return {'format': 'puty-organization-count-probe-v1', 'instanceID': probe.INSTANCE,
            'baselineEmployee': employee,
            'capturedProjection': {'id': employee['id'], 'userID': 'emp_' + probe.sha(employee['id'].encode())[:32],
                                   'enabled': False, 'hostUID': 0, 'status': 'waiting',
                                   'desiredVersion': 2, 'appliedVersion': 0},
            'baselineResponseSHA256': 'a' * 64, 'captureResponseSHA256': 'b' * 64,
            'reasonSHA256': probe.sha(b'synthetic reason')}


class Database:
    def __init__(self, value, *, source=True, filtered=False, rule=False):
        original, captured = value['baselineEmployee'], value['capturedProjection']
        employee = copy.deepcopy(original)
        if filtered:
            employee['status'] = 'filtered'
        self.outputs = [([employee] if source else []), [{
            'employeeID': original['id'], 'nativeUserID': captured['userID'], 'uid': 0,
            'enabled': 0, 'status': 'waiting', 'provisioned': 0, 'desiredVersion': 2, 'appliedVersion': 0,
            'projection': {'employeeID': original['id'], 'userID': captured['userID'],
                           'hostUID': 0, 'enabled': False, 'reason': 'synthetic reason'}}],
            [{'wecom_userid': original['wecom_userid'], 'name': original['name'], 'department_ids': '[8]'}],
            [{'id': 8, 'pid': 1, 'name': 'Synthetic Team'}, {'id': 1, 'pid': 0, 'name': 'Synthetic Parent'}],
            [{'exclude_userids': [original['wecom_userid']]}] if rule else [{}]]
        self.statements = []

    def query(self, sql):
        self.statements.append(sql)

    def json(self, sql):
        self.statements.append(sql)
        stripped = re.sub(r"'(?:''|[^'])*'", "''", sql)
        self_test = sum(ch == '(' for ch in stripped) == sum(ch == ')' for ch in stripped)
        if not self_test:
            raise AssertionError('Unbalanced SQL function parentheses')
        return self.outputs.pop(0)


class ProbeTests(unittest.TestCase):
    helpers = types.SimpleNamespace(literal=lambda value: "X'" + value.encode().hex() + "'")

    def test_exact_input_rejects_other_owner_enabled_or_applied(self):
        value = inputs()
        probe.validate_input(value)
        for path, new in [('instanceID', 'other'), ('enabled', True), ('appliedVersion', 1), ('hostUID', 5)]:
            changed = copy.deepcopy(value)
            (changed if path == 'instanceID' else changed['capturedProjection'])[path] = new
            with self.assertRaises(RuntimeError):
                probe.validate_input(changed)

    def test_explicit_name_user_and_department_source_parity(self):
        employee = inputs()['baselineEmployee']
        member = {'name': employee['name'], 'department_ids': '[8]'}
        departments = [{'id': 8, 'pid': 1, 'name': 'Team'}, {'id': 1, 'pid': 8, 'name': ' Parent '}]
        proof = probe.exclusion_proof(employee, member, departments, {'exclude_department_names': ['Parent']})
        self.assertTrue(proof['ruleHits']['explicitDepartmentName'])
        proof = probe.exclusion_proof(employee, member, departments,
                                     {'exclude_department_ids': [1], 'include_subdepartments': False})
        self.assertFalse(proof['excludedByExplicitRules'])
        proof = probe.exclusion_proof(employee, None, [], {'exclude_user_names': [employee['name']]})
        self.assertTrue(proof['ruleHits']['explicitUserName'])

    def test_exact_read_only_filtered_and_unfiltered_distinguished(self):
        value = inputs()
        for filtered, rule, expected in [(True, False, True), (False, True, True), (False, False, False)]:
            db = Database(value, filtered=filtered, rule=rule)
            safe, _ = probe.collect(db, self.helpers, value)
            self.assertEqual(safe['historicalClassificationProven'], expected)
            self.assertEqual(safe['excludedFromCurrentHRMViewByExactSourceRules'], expected)
            self.assertFalse(safe['sourcePhysicallyAbsent'])
            self.assertIn('READ ONLY', db.statements[0])
            self.assertEqual(db.statements[-1], 'ROLLBACK')
            for statement in db.statements[1:-1]:
                self.assertTrue(statement.startswith('SELECT '))
                self.assertNotIn('FOR UPDATE', statement)

    def test_absent_is_separate_and_changed_projection_is_rejected(self):
        value = inputs()
        db = Database(value, source=False)
        safe, _ = probe.collect(db, self.helpers, value)
        self.assertTrue(safe['sourcePhysicallyAbsent'])
        self.assertEqual(safe['employeeStatusCategory'], 'absent')
        db = Database(value)
        db.outputs[1][0]['provisioned'] = 1
        with self.assertRaises(RuntimeError):
            probe.collect(db, self.helpers, value)


if __name__ == '__main__':
    unittest.main()
