"""Offline SQL invariants for the explicitly reviewed draft-only migration."""
import copy
import importlib.util
import json
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location("apply_production", Path(__file__).with_name("apply-production-work-schedules.py"))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
ROOT = Path(__file__).resolve().parent.parent


class ApplyProductionTests(unittest.TestCase):
    def setUp(self):
        self.plan = json.loads((ROOT / "runtime/production-work-schedule-plan-final.json").read_text(encoding="utf-8"))
        self.snapshot = json.loads((ROOT / "runtime/production-work-schedule-snapshot.json").read_text(encoding="utf-8"))

    def test_exact_reviewed_plan(self):
        module.validate(self.plan, self.snapshot, self.plan["plan_sha256"])
        changed = copy.deepcopy(self.plan)
        changed["actions"][0]["new_rule"]["scope"]["employee_ids"] = ["unauthorized"]
        with self.assertRaisesRegex(ValueError, "digest"):
            module.validate(changed, self.snapshot, self.plan["plan_sha256"])

    def test_preview_is_one_draft_only_transaction(self):
        sql = module.build_sql(self.plan, self.snapshot, 1)
        self.assertEqual(sql.count("START TRANSACTION;"), 1)
        self.assertEqual(sql.count("COMMIT;"), 1)
        self.assertEqual(sql.count("UPDATE biz_att_rule SET status='archived'"), 27)
        self.assertEqual(sql.count("INSERT INTO biz_att_rule ("), 22)
        self.assertEqual(sql.count("INSERT INTO biz_att_audit ("), 49)
        self.assertNotIn("DELETE", sql)
        self.assertNotIn("INSERT INTO biz_att_batch", sql)
        self.assertNotIn("UPDATE biz_hrm_employee", sql)
        self.assertLess(sql.index("FOR UPDATE"), sql.index("UPDATE biz_att_rule"))
        self.assertGreater(sql.rindex("INSERT INTO attendance_plan_assertion"), sql.rindex("INSERT INTO biz_att_rule ("))
        self.assertIn("ROW_COUNT()=1", sql)
        self.assertEqual(sql.count("WHERE BINARY id="), 49)

    def test_real_snapshot_tampering_aborts(self):
        changed = copy.deepcopy(self.snapshot)
        changed["employee"][0]["name"] += "changed"
        with self.assertRaisesRegex(ValueError, "Snapshot digest"):
            module.validate(self.plan, changed, self.plan["plan_sha256"])

    def test_sql_text_is_hex_encoded(self):
        text = "测试'\\\n; COMMIT; --"
        encoded = module.literal(text)
        self.assertNotIn("COMMIT", encoded)
        self.assertIn(text.encode().hex(), encoded)
        self.assertEqual(module.literal(None), "NULL")
        with self.assertRaisesRegex(ValueError, "actor"):
            module.build_sql(self.plan, self.snapshot, 0)


if __name__ == "__main__":
    unittest.main()
