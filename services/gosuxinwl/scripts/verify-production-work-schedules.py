"""Read-only post-migration verification; never applies or publishes anything."""
import argparse
import hashlib
import json
from pathlib import Path
import re
import subprocess
import sys


def stable(value):
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":"))


def digest(value):
    return hashlib.sha256(stable(value).encode()).hexdigest()


TABLES = ("biz_hrm_employee", "gf_auth_dept", "wecom_department", "wecom_user", "wecom_settings", "biz_att_roster", "biz_att_batch", "biz_att_result", "biz_att_dirty", "wecom_attendance_outbox")


def inserts(path=None, content=None):
    result = {name: hashlib.sha256() for name in TABLES}
    counts = {name: 0 for name in TABLES}
    lines = Path(path).open("rb") if path else content.splitlines(keepends=True)
    try:
        for line in lines:
            match = re.match(rb"INSERT INTO `([a-z_]+)` VALUES ", line)
            if match and match[1].decode() in result:
                name = match[1].decode()
                result[name].update(line.rstrip(b"\r\n") + b"\n")
                counts[name] += 1
    finally:
        if path:
            lines.close()
    return {name: {"insert_sha256": result[name].hexdigest(), "insert_statements": counts[name]} for name in TABLES}


def verify_untouched(backup, output):
    command = ["docker", "exec", "gosuxin-suite-mysql-1", "mysqldump", "--defaults-extra-file=/run/secrets/backup.cnf", "--single-transaction", "--quick", "--hex-blob", "--set-gtid-purged=OFF", "--skip-dump-date", "--order-by-primary", "--no-create-info", "kaoqin_dev", *TABLES]
    current = subprocess.run(command, capture_output=True, timeout=90)
    if current.returncode:
        raise RuntimeError("Read-only table comparison failed; private dump output suppressed")
    old, new = inserts(path=backup), inserts(content=current.stdout)
    report = {"read_only": True, "backup_path": str(backup), "tables": {name: {"before": old[name], "after": new[name], "unchanged": old[name] == new[name]} for name in TABLES}}
    report["all_unchanged"] = all(item["unchanged"] for item in report["tables"].values())
    Path(output).write_text(json.dumps(report, indent=2))
    print(json.dumps(report))
    if not report["all_unchanged"]:
        raise RuntimeError("An unmodified table differs from the pre-upgrade backup")


def verify(plan_path, before_path, after_path, untouched_path, output):
    plan, before, after, untouched = [json.loads(Path(p).read_text(encoding="utf-8-sig")) for p in (plan_path, before_path, after_path, untouched_path)]
    checks = []

    def check(name, condition):
        checks.append({"check": name, "passed": bool(condition)})

    check("reviewed_plan_digest", digest({k: v for k, v in plan.items() if k != "plan_sha256"}) == plan["plan_sha256"])
    check("before_snapshot_matches_plan", before["snapshot_sha256"] == plan["snapshot_sha256"] and before["rules_sha256"] == plan["rules_before_sha256"])
    check("before_snapshot_integrity", digest({k: v for k, v in before.items() if k != "snapshot_sha256"}) == before["snapshot_sha256"])
    check("after_snapshot_integrity", digest({k: v for k, v in after.items() if k != "snapshot_sha256"}) == after["snapshot_sha256"])
    old_rules, new_rules = ({row["id"]: row for row in data["rule"]} for data in (before, after))
    expected_new = {action["new_rule"]["id"] for action in plan["actions"]}
    archives = {rid for action in plan["actions"] for rid in action["archive_ids"]}
    check("exactly_104_rules_and_22_expected_new_ids", len(new_rules) == 104 and set(new_rules) - set(old_rules) == expected_new and not set(old_rules) - set(new_rules))
    for action in plan["actions"]:
        wanted = action["new_rule"]
        actual = new_rules.get(wanted["id"], {})
        check("new_draft_fields:" + wanted["id"], all(actual.get(k) == wanted[k] for k in ("id", "family_id", "version", "name", "status", "effective_from", "effective_to")) and actual.get("status") == "draft" and actual.get("created_by") == 1)
        check("new_draft_json:" + wanted["id"], all(json.loads(actual.get(k + "_json", "null")) == wanted[k] for k in ("config", "scope", "conflicts", "source")))
    for rid, old in old_rules.items():
        actual = new_rules.get(rid, {})
        if rid in archives:
            changed = {key for key in old if old[key] != actual.get(key)}
            check("archive_only_status_time:" + rid, actual.get("status") == "archived" and changed <= {"status", "updated_at"} and "status" in changed)
        else:
            check("original_preserved:" + rid, old == actual)
    for key in ("employee", "department", "wecom_department", "wecom_user", "exclusions", "active_plugin"):
        check("unchanged_snapshot:" + key, before[key] == after[key])
    old_audits, new_audits = ({row["id"]: row for row in data["rule_audit"]} for data in (before, after))
    check("existing_audits_unchanged", all(new_audits.get(key) == value for key, value in old_audits.items()))
    added = {key: value for key, value in new_audits.items() if key not in old_audits}
    expected_audits = {}
    for action in plan["actions"]:
        wanted = action["new_rule"]
        expected_audits["prod-audit-" + digest([plan["plan_sha256"], wanted["id"], "rebuild_work_schedule"])[:40]] = (wanted["id"], "rebuild_work_schedule", action["before"], wanted)
        for old in action["before"]:
            expected_audits["prod-audit-" + digest([plan["plan_sha256"], old["id"], "archive_work_schedule"])[:40]] = (old["id"], "archive_work_schedule", old, dict(old, status="archived", replaced_by=wanted["id"]))
    check("exactly_49_expected_audits", len(added) == 49 and set(added) == set(expected_audits))
    for key, (entity_id, action, wanted_before, wanted_after) in expected_audits.items():
        actual = added.get(key, {})
        check("audit_evidence:" + key, actual.get("entity_id") == entity_id and actual.get("action") == action and actual.get("created_by") == 1 and plan["plan_sha256"] in actual.get("reason", "") and json.loads(actual.get("before_json", "null")) == wanted_before)
        after_evidence = json.loads(actual.get("after_json", "{}"))
        check("audit_after:" + key, all(after_evidence.get(k) == v for k, v in wanted_after.items() if k not in ("created_at", "updated_at")))
    sales = [action["new_rule"] for action in plan["actions"] if action["new_rule"]["config"]["work_schedule"]["mode"] == "after_sales"]
    employees = {p["id"]: p for p in after["employee"]}
    sales_members = [employees[i] for i in sales[0]["scope"]["employee_ids"]]
    check("after_sales_exact_three_confirmed_members", len(sales) == 1 and len(sales_members) == 3 and {p["employee_no"] for p in sales_members} == {"2485", "3958", "3720"} and sales[0]["config"]["work_schedule"]["duty_expected_members"] == 3)
    check("full_untouched_tables_match_backup", untouched["all_unchanged"] and set(untouched["tables"]) == set(TABLES))
    report = {"read_only": True, "passed": all(c["passed"] for c in checks), "plan_sha256": plan["plan_sha256"], "before_rules_sha256": before["rules_sha256"], "after_rules_sha256": after["rules_sha256"], "after_snapshot_sha256": after["snapshot_sha256"], "counts": {"rules": len(new_rules), "new_drafts": len(expected_new), "archived": len(archives), "original_preserved": len(old_rules) - len(archives), "new_audits": len(added)}, "after_sales_members": [{k: p[k] for k in ("id", "name", "employee_no", "status")} for p in sales_members], "untouched_tables": untouched, "checks": checks}
    report["verification_sha256"] = digest(report)
    Path(output).write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding="utf-8")
    print(json.dumps({k: report[k] for k in ("read_only", "passed", "counts", "verification_sha256")}))
    if not report["passed"]:
        print(json.dumps([c for c in checks if not c["passed"]]))
        raise RuntimeError("Post-migration verification failed")


def main():
    sys.stdout.reconfigure(encoding="utf-8")
    parser = argparse.ArgumentParser()
    parser.add_argument("--backup")
    parser.add_argument("--plan")
    parser.add_argument("--before")
    parser.add_argument("--after")
    parser.add_argument("--untouched")
    parser.add_argument("--output", required=True)
    args = parser.parse_args()
    if args.backup:
        verify_untouched(args.backup, args.output)
    elif all((args.plan, args.before, args.after, args.untouched)):
        verify(args.plan, args.before, args.after, args.untouched, args.output)
    else:
        parser.error("Use --backup or provide all plan/snapshot/untouched paths")


if __name__ == "__main__":
    main()
