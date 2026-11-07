"""Offline safety and audit tests; never calls a database."""
import copy
import importlib.util
import json
from pathlib import Path
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("cleanup", Path(__file__).with_name("cleanup-attendance-rules.py"))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class CleanupTests(unittest.TestCase):
    def fixture(self):
        row = {"id": "old", "family_id": "family", "version": 1, "name": "历史制度", "status": "archived", "effective_from": "2026-10-01", "effective_to": "", "config_json": '{"rule":{}}', "scope_json": '{"employee_ids":["person"]}', "conflicts_json": "[]", "source_json": '{"original":{"v":1}}', "created_by": 1, "created_at": "2026-10-29 00:00:00.000000", "updated_at": "2026-10-29 00:00:00.000000"}
        data = {"rules": [row], "audits": [], "protected_checksums": {}, "connection": "fixture", "database": "kaoqin_dev", "read_only": True}
        data["rules_sha256"], data["audits_sha256"] = module.sha(data["rules"]), module.sha(data["audits"])
        data["snapshot_sha256"] = module.sha(data)
        candidates = {"batch_id": "test-cleanup", "candidates": [{"id": "old", "reason": "历史版本退出日常列表"}]}
        return data, candidates

    def test_plan_retains_full_original_and_only_adds_cleanup(self):
        data, candidates = self.fixture()
        with patch.object(module, "run_db", side_effect=AssertionError("Offline preparation cannot call DB")):
            plan = module.prepare(data, candidates, 1)
            module.validate_plan(plan, data, plan["plan_sha256"])
        action = plan["actions"][0]
        self.assertEqual(action["before"], data["rules"][0])
        self.assertEqual(action["after_source"]["original"], {"v": 1})
        self.assertTrue(action["after_source"]["cleanup"]["hidden"])

    def test_rejects_active_scoped_and_already_cleaned_candidates(self):
        for changes in ({"status": "published"}, {"status": "superseded"}, {"status": "draft"}, {"source_json": '{"cleanup":{"hidden":true}}'}):
            data, candidates = self.fixture()
            data["rules"][0].update(changes)
            data["rules_sha256"] = module.sha(data["rules"])
            data["snapshot_sha256"] = module.sha({k: v for k, v in data.items() if k != "snapshot_sha256"})
            with self.subTest(changes=changes), self.assertRaises(ValueError):
                module.prepare(data, candidates, 1)

    def test_empty_draft_allowed_but_candidate_count_exact(self):
        data, candidates = self.fixture()
        data["rules"][0].update(status="draft", scope_json="{}")
        data["rules_sha256"] = module.sha(data["rules"])
        data["snapshot_sha256"] = module.sha({k: v for k, v in data.items() if k != "snapshot_sha256"})
        self.assertEqual(module.prepare(data, candidates, 1)["actions"][0]["after_status"], "archived")
        with self.assertRaises(ValueError):
            module.prepare(data, candidates, 2)

    def test_sql_is_single_transaction_no_delete_or_business_write(self):
        data, candidates = self.fixture()
        plan = module.prepare(data, candidates, 1)
        sql = module.build_sql(plan, data, 1)
        self.assertEqual(sql.count("START TRANSACTION;"), 1)
        self.assertEqual(sql.count("COMMIT;"), 1)
        self.assertNotIn("DELETE", sql)
        self.assertNotIn("UPDATE biz_hrm", sql)
        self.assertIn("WHERE BINARY id=", sql)
        self.assertIn("SELECT id FROM biz_att_audit ORDER BY id FOR UPDATE", sql)
        self.assertIn("JSON_OBJECT('id',`id`", sql)
        self.assertIn("'$.cleanup.cleaned_at'", sql)

    def test_mutated_plan_or_snapshot_rejected(self):
        data, candidates = self.fixture()
        plan = module.prepare(data, candidates, 1)
        changed = copy.deepcopy(plan)
        changed["actions"][0]["after_source"]["original"]["v"] = 2
        with self.assertRaisesRegex(ValueError, "hash"):
            module.validate_plan(changed, data, plan["plan_sha256"])
        data["rules"][0]["name"] = "changed"
        with self.assertRaisesRegex(ValueError, "integrity"):
            module.validate_snapshot(data)


if __name__ == "__main__":
    unittest.main()
