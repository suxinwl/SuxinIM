"""Offline policy defaults only: no generator execution or private-file reads."""
import ast
import pathlib
import unittest

import yaml

ROOT = pathlib.Path(__file__).resolve().parents[1]


class ProductionSecurityDefaults(unittest.TestCase):
    def test_source_and_future_production_retention_are_permanent(self):
        config = yaml.safe_load((ROOT / "services/openim-server/config/openim-crontask.yml").read_text(encoding="utf-8"))
        self.assertEqual((config["retainChatRecords"], config["fileExpireTime"], config["deleteObjectType"]), (0, 0, []))
        tree = ast.parse((ROOT / "tools/prepare_production.py").read_text(encoding="utf-8"))
        updates = [node for node in ast.walk(tree) if isinstance(node, ast.Call) and isinstance(node.func, ast.Attribute) and isinstance(node.func.value, ast.Name) and node.func.value.id == "retention" and node.func.attr == "update"]
        self.assertEqual(len(updates), 1)
        self.assertEqual({item.arg: ast.literal_eval(item.value) for item in updates[0].keywords}, {"retainChatRecords": 0, "fileExpireTime": 0, "deleteObjectType": []})

    def test_data_volume_sampling_is_readonly_and_unverified_by_default(self):
        tree = ast.parse((ROOT / "tools/prepare_production.py").read_text(encoding="utf-8"))
        constants = {node.value for node in ast.walk(tree) if isinstance(node, ast.Constant) and isinstance(node.value, str)}
        self.assertIn("mongo-data:/capacity/mongo:ro", constants)
        self.assertIn("minio-data:/capacity/minio:ro", constants)
        definitions = [node for node in ast.walk(tree) if isinstance(node, ast.Dict)]
        self.assertTrue(any(any(isinstance(key, ast.Constant) and key.value == "STORAGE_CAPACITY_DATA_VERIFIED" and isinstance(value, ast.Constant) and value.value == "false" for key, value in zip(node.keys, node.values)) for node in definitions))

    def test_existing_monitor_rules_keep_usage_available_and_thresholds(self):
        config = yaml.safe_load((ROOT / "deploy/production/capacity-alerts.yml").read_text(encoding="utf-8"))
        self.assertEqual(set(config), {"groups"})
        rules = config["groups"][0]["rules"]
        self.assertEqual([row["record"] for row in rules[:4]], ["puty_storage_usage_bytes", "puty_storage_available_bytes", "puty_storage_usage_ratio", "puty_storage_state"])
        self.assertIn("0.70", rules[4]["expr"])
        self.assertIn("0.85", rules[5]["expr"])
        self.assertEqual([row["labels"]["severity"] for row in rules[4:]], ["warning", "critical"])
        self.assertFalse(any("or vector(0)" in row["expr"] for row in rules))


if __name__ == "__main__":
    unittest.main()
