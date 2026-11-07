"""Independent offline verification of cleanup backups; no database access."""
import argparse
from collections import Counter
import hashlib
import json
from pathlib import Path


def sha(value):
    return hashlib.sha256(json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode()).hexdigest()


def load(path):
    return json.loads(Path(path).read_text(encoding="utf-8-sig"))


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--report", required=True)
    parser.add_argument("--backup-dir")
    parser.add_argument("--expected-hidden", type=int, required=True)
    parser.add_argument("--output", required=True)
    args = parser.parse_args()
    report = load(args.report)
    folder = Path(args.backup_dir) if args.backup_dir else Path(report["backup_path"]).parent
    before, after, plan = [load(folder / name) for name in ("snapshot-before.json", "snapshot-after.json", "plan.json")]
    checks = {"commit_confirmed": report["status"] == "committed", "plan_digest": sha({k: v for k, v in plan.items() if k != "plan_sha256"}) == plan["plan_sha256"] == report["plan_sha256"]}
    backup = folder / "rules-and-audits-before.sql"
    checks["backup_bytes_hash"] = backup.stat().st_size == report["backup_bytes"] and hashlib.sha256(backup.read_bytes()).hexdigest() == report["backup_sha256"]
    for label, data in (("before", before), ("after", after)):
        checks[label + "_snapshot_integrity"] = data["snapshot_sha256"] == sha({k: v for k, v in data.items() if k != "snapshot_sha256"}) and data["rules_sha256"] == sha(data["rules"]) and data["audits_sha256"] == sha(data["audits"])
    old, new = [{r["id"]: r for r in data["rules"]} for data in (before, after)]
    actions = {a["id"]: a for a in plan["actions"]}
    checks["identical_rule_ids"] = set(old) == set(new)
    checks["expected_selection"] = len(actions) == args.expected_hidden
    hidden = []
    for rid, original in old.items():
        actual = new.get(rid, {})
        changed_fields = {key for key, value in original.items() if actual.get(key) != value}
        if rid not in actions:
            checks["untouched:" + rid] = original == actual
        else:
            source = json.loads(actual["source_json"])
            meta = source.pop("cleanup", None)
            original_source = json.loads(original["source_json"])
            checks["fields:" + rid] = changed_fields <= {"status", "source_json", "updated_at"} and actual["status"] == "archived" and original["status"] in ("draft", "archived")
            checks["original_source_preserved:" + rid] = source == original_source
            checks["cleanup_metadata:" + rid] = isinstance(meta, dict) and meta.get("hidden") is True and meta.get("batch_id") == plan["batch_id"] and meta.get("reason") == actions[rid]["reason"] and meta.get("previous_status") == original["status"] and bool(meta.get("cleaned_at")) and meta.get("timezone") == "Asia/Shanghai"
            checks["identity_config_scope:" + rid] = all(actual[k] == original[k] for k in ("id", "family_id", "version", "name", "config_json", "scope_json", "conflicts_json", "effective_from", "effective_to", "created_by", "created_at"))
        if json.loads(actual.get("source_json", "{}") or "{}").get("cleanup", {}).get("hidden") is True:
            hidden.append(rid)
    checks["exact_hidden_ids"] = set(hidden) == set(actions)
    audits_before, audits_after = [{a["id"]: a for a in data["audits"]} for data in (before, after)]
    checks["all_prior_audits_unchanged"] = all(audits_after.get(key) == value for key, value in audits_before.items())
    expected_audits = set()
    for rid, action in actions.items():
        key = "rule-cleanup-" + sha([plan["plan_sha256"], rid])[:40]
        expected_audits.add(key)
        audit = audits_after.get(key, {})
        checks["complete_audit:" + rid] = audit.get("entity_id") == rid and audit.get("entity") == "rule" and audit.get("action") == "cleanup_hide" and audit.get("created_by") == report["actor_id"] and json.loads(audit.get("before_json", "null")) == old[rid] and json.loads(audit.get("after_json", "null")) == new[rid]
    checks["exact_audit_additions"] = set(audits_after) - set(audits_before) == expected_audits
    checks["all_protected_table_checksums_unchanged"] = before["protected_checksums"] == after["protected_checksums"]
    result = {"read_only_file_verification": True, "database_operations": 0, "passed": all(checks.values()), "plan_sha256": plan["plan_sha256"], "backup_sha256": report["backup_sha256"], "counts": {"total_rules": len(new), "hidden": len(hidden), "retained": len(new) - len(hidden), "new_audits": len(expected_audits), "retained_statuses": dict(Counter(new[rid]["status"] for rid in new if rid not in hidden))}, "protected_checksums": after["protected_checksums"], "checks": checks}
    result["verification_sha256"] = sha(result)
    with Path(args.output).open("x", encoding="utf-8") as target:
        json.dump(result, target, ensure_ascii=False, indent=2)
    print(json.dumps({k: result[k] for k in ("passed", "counts", "verification_sha256")}))
    if not result["passed"]:
        print(json.dumps([key for key, value in checks.items() if not value]))
        raise RuntimeError("Independent cleanup verification failed")


if __name__ == "__main__":
    main()
