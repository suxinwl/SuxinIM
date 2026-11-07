"""Offline cleanup guard checks; no HTTP, SSH or Docker is contacted."""
import importlib.util
import io
import pathlib
import types
import unittest

HERE = pathlib.Path(__file__).resolve().parent


def load(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


remote = load('fixtures_cleanup', HERE.parent / 'deploy/production/fixtures_cleanup.py')
local = load('production_fixture_cleanup', HERE / 'production_fixture_cleanup.py')


class CleanupGuards(unittest.TestCase):
    def fixtures(self):
        return [{'uid': 901, 'username': 'qa_a', 'employeeID': 'fixture-a', 'name': 'Synthetic A'},
                {'uid': 902, 'username': 'qa_b', 'employeeID': 'fixture-b', 'name': 'Synthetic B'}]

    def test_only_two_original_named_nonadmin_fixtures_are_accepted(self):
        plan = {'format': 'puty-fixture-cleanup-v1', 'instanceID': 'owner',
                'fixtures': [{'expected': v} for v in self.fixtures()]}
        self.assertEqual(remote.validate_plan(plan, 'owner'), self.fixtures())
        for changed in ({**plan, 'instanceID': 'other'}, {**plan, 'fixtures': plan['fixtures'][:1]},
                        {**plan, 'fixtures': [{'expected': {**v, 'uid': 1}} for v in self.fixtures()]},
                        {**plan, 'fixtures': [{'expected': {**v, 'username': 'employee'}} for v in self.fixtures()]}):
            with self.assertRaises(RuntimeError):
                remote.validate_plan(changed, 'owner')

    def test_delete_predicate_binds_every_identity_and_empty_wecom(self):
        predicate = remote.employee_predicate(self.fixtures())
        for required in ('e.id=', 'e.user_id=901', 'e.user_id=902', 'e.employee_no=', 'e.name=',
                         'a.username=', 'a.name=', "COALESCE(e.wecom_userid,'')=''"):
            self.assertIn(required, predicate)
        self.assertNotIn('LIKE', predicate)

    def test_values_are_hex_literals_and_identifiers_cannot_inject(self):
        value = "qa'; DROP TABLE gf_admin; --"
        encoded = remote.literal(value)
        self.assertNotIn('DROP', encoded)
        self.assertIn(value.encode().hex(), encoded)
        self.assertIn('COLLATE utf8mb4_bin', encoded)
        with self.assertRaises(RuntimeError):
            remote.identifier('table;DELETE')

    def test_audit_and_identity_history_are_preserved(self):
        for table in ('biz_openim_employee', 'biz_openim_epoch', 'biz_openim_outbox',
                      'biz_openim_access_audit', 'gf_login_log', 'biz_att_audit', 'gf_auth_role_access'):
            self.assertTrue(remote.retained(table))
        for table in ('biz_hrm_attendance_clock', 'biz_att_ledger', 'wecom_user', 'biz_org_binding'):
            self.assertFalse(remote.retained(table))

    def test_only_three_exact_authentication_state_tables_are_retained_not_business_or_similarly_named_tables(self):
        for table in ('gf_account_security', 'gf_account_failure', 'gf_account_change_ticket'):
            self.assertTrue(remote.retained(table))
        for table in ('biz_account_security', 'gf_account_security_business', 'gf_account_security_backup',
                      'gf_account_failure_business', 'gf_account_change_ticket_refs', 'gf_account_profile',
                      'biz_hrm_employee', 'biz_hrm_attendance_clock', 'biz_contract'):
            self.assertFalse(remote.retained(table))

    def test_exact_security_retention_still_counts_business_references_and_even_security_table_foreign_keys(self):
        statements = []
        db = types.SimpleNamespace(
            one=lambda sql: (statements.append(sql) or '1'),
            json=lambda sql: [{'table': 'gf_account_security', 'column': 'employee_id', 'target': 'id'}],
        )
        schema = [{'table': table, 'column': 'uid', 'type': 'bigint'} for table in
                  ('gf_account_security', 'gf_account_failure', 'gf_account_change_ticket',
                   'gf_account_security_business', 'biz_contract')]
        checks, foreign = remote.references(db, self.fixtures(), schema)
        self.assertEqual({r['table'] for r in checks if r.get('kind') != 'foreign-key'},
                         {'gf_account_security_business', 'biz_contract'})
        self.assertEqual(len(foreign), 1)
        self.assertEqual(checks[-1]['kind'], 'foreign-key')
        self.assertEqual(checks[-1]['count'], 1)
        self.assertEqual(sum(row['count'] for row in checks), 3)
        self.assertEqual(len(statements), 3)

    def test_mutation_requires_explicit_fixture_release_before_any_network_call(self):
        with self.assertRaisesRegex(RuntimeError, 'release'):
            local.quiesce(False)

    def test_local_binding_rejects_nonempty_wecom_or_wrong_uid(self):
        fixture = self.fixtures()[0]
        host = {'id': fixture['uid'], 'username': fixture['username'], 'name': fixture['name']}
        employee = {'id': fixture['employeeID'], 'user_id': fixture['uid'], 'employee_no': fixture['username'],
                    'name': fixture['name'], 'wecom_userid': ''}
        local.binding(fixture, host, employee)
        for changed in ({**employee, 'wecom_userid': 'real-source'}, {**employee, 'user_id': 999}):
            with self.assertRaises(RuntimeError):
                local.binding(fixture, host, changed)

    def test_mysql_error_diagnostic_exposes_only_number_and_sqlstate(self):
        db = remote.Database.__new__(remote.Database)
        db.process = types.SimpleNamespace(stdin=io.BytesIO(), stdout=io.BytesIO())
        db.errors = io.BytesIO(b"ERROR 1267 (HY000) at line 3: private SQL credentials or personal data")
        with self.assertRaisesRegex(RuntimeError, '^MySQL operation failed: ERROR 1267 SQLSTATE HY000$'):
            db.query('SELECT synthetic_fixture')


if __name__ == '__main__':
    unittest.main()
