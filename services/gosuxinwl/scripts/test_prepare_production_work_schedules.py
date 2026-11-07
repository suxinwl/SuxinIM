"""Offline regression checks for conservative production-draft preparation."""
import contextlib
import importlib.util
import io
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("prepare_production", Path(__file__).with_name("prepare-production-work-schedules.py"))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class PrepareProductionTests(unittest.TestCase):
    def fixture(self, wanted="测试甲", mode="production_staff"):
        category = "生产职员" if mode != "production_employee" else "员工"
        source = {"source_key": "生产职员!A3", "department": "二部", "directory_department": "印刷事业部", "names_text": wanted, "group": category}
        config = {"category": category, "rule": {"window": {"in_before_minutes": 0}}, "work_schedule": {"mode": mode}, "shifts": [{"id": "day", "segments": [{"local_windows": {"in_start_second": None}}]}]}
        preset = {"source": source, "sources": [source], "config": config, "conflicts": []}
        old_source = {"department": "二部", "names_text": "", "group": wanted}
        old_config = {"category": category, "rule": {"window": {"in_before_minutes": 0}}, "shifts": []}
        rule = {"id": "old", "family_id": "family", "version": 1, "name": "二部  " + wanted, "status": "draft", "effective_from": "", "effective_to": "", "created_at": "2026-10-29", "config_json": json.dumps(old_config), "scope_json": json.dumps({"employee_ids": None, "department_ids": None, "is_default": False}), "source_json": json.dumps({"source": old_source})}
        snapshot = {"rule": [rule], "employee": [{"id": "employee1", "name": "测试甲", "employee_no": "0080", "dept_id": 1, "status": "active", "wecom_userid": "w1"}], "department": [{"id": 1, "pid": 0, "name": "印刷事业部"}], "wecom_department": [], "wecom_user": [], "exclusions": [{}], "rule_audit": [], "active_plugin": {"package": "read-only-fixture"}, "snapshot_sha256": "fixture"}
        bundle = {"source_sha256": "fixture-source", "source_filename": "fixture.xlsx", "presets": [preset]}
        return snapshot, bundle

    def run_prepare(self, snapshot, bundle, overrides=None):
        snapshot["rules_sha256"] = module.sha(snapshot["rule"])
        before = module.stable(snapshot)
        with tempfile.TemporaryDirectory(prefix="attendance-prepare-test-") as folder:
            folder = Path(folder)
            (folder / "snapshot.json").write_text(json.dumps(snapshot), encoding="utf-8")
            (folder / "presets.json").write_text(json.dumps(bundle), encoding="utf-8")
            (folder / "overrides.json").write_text(json.dumps(overrides or {}), encoding="utf-8")
            with patch.object(module.subprocess, "run", side_effect=AssertionError("Preparation must not call a database or SSH")), contextlib.redirect_stdout(io.StringIO()):
                module.prepare(folder / "snapshot.json", folder / "presets.json", folder / "plan.json", folder / "overrides.json")
            plan = json.loads((folder / "plan.json").read_text(encoding="utf-8"))
        self.assertEqual(module.stable(snapshot), before)
        return plan

    def test_precise_identity_keeps_number_and_published_rules(self):
        snapshot, bundle = self.fixture()
        published = dict(snapshot["rule"][0], id="published", status="published")
        snapshot["rule"].append(published)
        plan = self.run_prepare(snapshot, bundle)
        action = plan["actions"][0]
        self.assertEqual(action["new_rule"]["name"], "二部  测试甲")
        self.assertEqual(action["new_rule"]["scope"]["employee_ids"], ["employee1"])
        self.assertEqual(action["resolved_people"][0]["employee_no"], "0080")
        self.assertEqual(plan["preserved"][0]["id"], "published")
        self.assertEqual(action["new_rule"]["config"]["shifts"][0]["segments"][0]["local_windows"]["in_start_second"], None)

    def test_ambiguous_or_filtered_identity_is_not_guessed(self):
        for case in ("duplicate", "filtered", "explicit_exclusion"):
            with self.subTest(case=case):
                snapshot, bundle = self.fixture()
                if case == "duplicate":
                    snapshot["employee"].append(dict(snapshot["employee"][0], id="employee2", employee_no=None))
                elif case == "filtered":
                    snapshot["employee"][0]["status"] = "filtered"
                else:
                    snapshot["exclusions"] = [{"exclude_userids": ["w1"]}]
                action = self.run_prepare(snapshot, bundle)["actions"][0]
                self.assertEqual(action["new_rule"]["scope"]["employee_ids"], [])
                self.assertTrue(action["identity_issues"])
                self.assertFalse(action["ready_to_publish"])

    def test_existing_scope_and_manual_windows_are_not_discarded(self):
        snapshot, bundle = self.fixture()
        scope = {"employee_ids": ["employee1"], "department_ids": [], "include_children": False, "is_default": False}
        snapshot["rule"][0]["scope_json"] = json.dumps(scope)
        config = json.loads(snapshot["rule"][0]["config_json"])
        config["rule"]["window"]["in_before_minutes"] = 60
        snapshot["rule"][0]["config_json"] = json.dumps(config)
        action = self.run_prepare(snapshot, bundle)["actions"][0]
        self.assertEqual(action["new_rule"]["scope"], scope)
        self.assertFalse(action["ready_to_apply_after_review"])
        self.assertTrue(action["old_manual_windows"][0]["windows"])

    def test_duty_sources_merge_whole_group_and_keep_unresolved_scope_empty(self):
        snapshot, bundle = self.fixture("测试甲、测试乙、测试丙", "after_sales")
        prototype = snapshot["rule"][0]
        snapshot["rule"] = []
        for i, name in enumerate(("测试甲", "测试乙", "测试丙")):
            source = {"department": "二部", "names_text": "", "group": name}
            snapshot["rule"].append(dict(prototype, id="old"+str(i), family_id="family"+str(i), name="售后 "+name, source_json=json.dumps({"source": source})))
        plan = self.run_prepare(snapshot, bundle)
        self.assertEqual(len(plan["actions"]), 1)
        action = plan["actions"][0]
        self.assertEqual(len(action["before"]), 3)
        self.assertEqual(action["new_rule"]["config"]["work_schedule"]["duty_expected_members"], 3)
        self.assertEqual(action["new_rule"]["scope"]["employee_ids"], [])
        self.assertEqual(len(action["identity_issues"]), 2)

    def test_explicit_override_keeps_confirmation_and_exact_number(self):
        snapshot, bundle = self.fixture("旧姓名")
        override = {"source_name": "旧姓名", "employee_id": "employee1", "current_name": "测试甲", "employee_no": "0080", "department": "印刷事业部", "confirmed_by": "user", "confirmation": "用户确认旧姓名对应工号0080"}
        action = self.run_prepare(snapshot, bundle, {"overrides": [override]})["actions"][0]
        self.assertEqual(action["new_rule"]["scope"]["employee_ids"], ["employee1"])
        self.assertEqual(action["new_rule"]["source"]["work_schedule_rebuild"]["confirmed_identity_overrides"], [override])
        self.assertEqual(action["identity_issues"], [])
        for column, wrong in (("employee_no", "80"), ("current_name", "另一个姓名"), ("department", "错误部门"), ("confirmed_by", "inferred")):
            with self.subTest(column=column), self.assertRaises(ValueError):
                self.run_prepare(snapshot, bundle, {"overrides": [dict(override, **{column: wrong})]})
        snapshot["employee"][0]["status"] = "filtered"
        with self.assertRaises(ValueError):
            self.run_prepare(snapshot, bundle, {"overrides": [override]})


if __name__ == "__main__":
    unittest.main()
