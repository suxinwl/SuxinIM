"""Apply one reviewed production draft plan, with optimistic guards in one transaction.

Default use (--emit-sql) is offline and read-only. --apply is Ubuntu-only, needs
the reviewed digest, deployed package digest, backup file and audit actor. It
does not publish rules, create people, enqueue calculation or change settings.
"""
from __future__ import annotations

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys


def stable(value):
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":"))


def sha(value):
    return hashlib.sha256(stable(value).encode()).hexdigest()


def literal(value):
    if value is None:
        return "NULL"
    if isinstance(value, bool):
        return "1" if value else "0"
    if isinstance(value, int):
        return str(value)
    encoded = str(value).encode().hex()
    return "CONVERT(0x" + encoded + " USING utf8mb4)" if encoded else "''"


def validate(plan, snapshot, expected):
    content = {k: v for k, v in plan.items() if k != "plan_sha256"}
    if plan.get("plan_sha256") != expected or sha(content) != expected:
        raise ValueError("Plan digest differs from the reviewed plan")
    if plan["mode"] != "prepare_only" or plan["database_writes"]:
        raise ValueError("Unexpected plan mode")
    if sha(snapshot["rule"]) != snapshot["rules_sha256"] or snapshot["rules_sha256"] != plan["rules_before_sha256"]:
        raise ValueError("Rule snapshot differs from reviewed plan")
    if sha({k: v for k, v in snapshot.items() if k != "snapshot_sha256"}) != snapshot["snapshot_sha256"] or snapshot["snapshot_sha256"] != plan["snapshot_sha256"]:
        raise ValueError("Snapshot digest differs from reviewed plan")
    by_id = {row["id"]: row for row in snapshot["rule"]}
    claimed, new_ids = set(), set()
    for action in plan["actions"]:
        if not action["ready_to_apply_after_review"] or action["blocked_reasons"] or action["ready_to_publish"]:
            raise ValueError("Blocked or publishable action is outside this draft migration")
        new = action["new_rule"]
        if new["status"] != "draft" or new["config"]["category"] not in ("员工", "生产职员") or new["id"] in by_id or new["id"] in new_ids:
            raise ValueError("Invalid draft identity or category")
        new_ids.add(new["id"])
        if action["archive_ids"] != [row["id"] for row in action["before"]]:
            raise ValueError("Archive list does not match evidence")
        for old in action["before"]:
            if old["id"] in claimed or old != by_id.get(old["id"]) or sha(old) != action["before_hashes"][old["id"]] or old["status"] != "draft":
                raise ValueError("Predecessor differs from reviewed snapshot")
            claimed.add(old["id"])
        if new["family_id"] != action["before"][0]["family_id"]:
            raise ValueError("New version must preserve the primary family")
    if len(new_ids) != plan["summary"]["new_drafts"] or len(claimed) != plan["summary"]["old_drafts_to_archive"]:
        raise ValueError("Plan action count mismatch")


def row_matches(row):
    clauses = []
    for column, value in row.items():
        if not re.fullmatch(r"[a-z_]+", column):
            raise ValueError("Unexpected column")
        # MySQL JSON serializes DATETIME with six fractional digits. Comparing
        # dates as dates preserves that representation without string guessing.
        left = "`" + column + "`"
        if isinstance(value, str) and not column.endswith("_at"):
            left = "BINARY " + left
        clauses.append("(" + left + " <=> " + literal(value) + ")")
    return " AND ".join(clauses)


def build_sql(plan, snapshot, actor_id):
    if actor_id < 1:
        raise ValueError("A real audit actor is required")
    statements = [
        "SET NAMES utf8mb4;",
        "SET SESSION TRANSACTION ISOLATION LEVEL REPEATABLE READ;",
        "SET SESSION innodb_lock_wait_timeout=15;",
        "CREATE TEMPORARY TABLE attendance_plan_assertion (id INT PRIMARY KEY) ENGINE=MEMORY;",
        "INSERT INTO attendance_plan_assertion VALUES (1);",
        "START TRANSACTION;",
    ]

    def check(condition):
        # mysql batch mode (without --force) aborts at the first duplicate key.
        # A failed pre/postcondition disconnects before COMMIT and rolls back.
        statements.append("INSERT INTO attendance_plan_assertion SELECT 1 WHERE NOT (" + condition + ");")

    check("(SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name IN ('biz_att_rule','biz_att_audit','biz_hrm_employee','gf_auth_dept','wecom_department','wecom_user','wecom_settings') AND engine='InnoDB')=7")
    check("(SELECT COUNT(*) FROM gf_admin WHERE id=" + str(actor_id) + ")=1")
    definitions = [
        ("rule", "biz_att_rule", "id"),
        ("employee", "biz_hrm_employee", "id"),
        ("department", "gf_auth_dept", "id"),
        ("wecom_department", "wecom_department", "wecom_id"),
        ("wecom_user", "wecom_user", "wecom_userid"),
        ("rule_audit", "biz_att_audit", "id"),
    ]
    for kind, table, key in definitions:
        where = " WHERE entity='rule'" if kind == "rule_audit" else ""
        statements.append("SELECT " + key + " FROM " + table + where + " ORDER BY " + key + " FOR UPDATE;")
        rows = snapshot.get(kind, [])
        check("(SELECT COUNT(*) FROM " + table + where + ")=" + str(len(rows)))
        for row in rows:
            check("(SELECT COUNT(*) FROM " + table + " WHERE " + row_matches(row) + ")=1")
    statements.append("SELECT id FROM wecom_settings WHERE id=1 FOR UPDATE;")
    check("(SELECT COUNT(*) FROM wecom_settings WHERE id=1)=" + str(len(snapshot.get("exclusions", []))))
    for settings in snapshot.get("exclusions", []):
        for key, value in settings.items():
            if not re.fullmatch(r"[a-z_]+", key):
                raise ValueError("Unexpected exclusion key")
            expected = "NULL" if value is None else "CAST(" + literal(stable(value)) + " AS JSON)"
            check("(SELECT JSON_EXTRACT(settings_json,'$." + key + "') <=> " + expected + " FROM wecom_settings WHERE id=1)")
    statements.append("SET @attendance_change_at=NOW();")
    reason = "按已确认生产员工/生产职员周休规则重建导入草稿；保留旧版本与来源，不发布、不重算。工作簿 " + plan["source_sha256"] + "；计划 " + plan["plan_sha256"]

    def audit(entity_id, action, before, after):
        audit_id = "prod-audit-" + sha([plan["plan_sha256"], entity_id, action])[:40]
        after_value = "JSON_SET(" + literal(stable(after)) + ",'$.updated_at',DATE_FORMAT(@attendance_change_at,'%Y-%m-%d %H:%i:%s'))"
        if action == "rebuild_work_schedule":
            after_value = "JSON_SET(" + after_value + ",'$.created_at',DATE_FORMAT(@attendance_change_at,'%Y-%m-%d %H:%i:%s'),'$.created_by'," + str(actor_id) + ")"
        values = [literal(v) for v in (audit_id, "rule", entity_id, action, stable(before))] + [after_value] + [literal(reason), literal(actor_id)]
        statements.append("INSERT INTO biz_att_audit (id,entity,entity_id,action,before_json,after_json,reason,created_by,created_at) VALUES (" + ",".join(values) + ",@attendance_change_at);")

    for action in plan["actions"]:
        new = action["new_rule"]
        for old in action["before"]:
            statements.append("UPDATE biz_att_rule SET status='archived',updated_at=@attendance_change_at WHERE BINARY id=" + literal(old["id"]) + " AND status='draft';")
            check("ROW_COUNT()=1")
            after = dict(old, status="archived", updated_at="transaction_timestamp", replaced_by=new["id"])
            audit(old["id"], "archive_work_schedule", old, after)
        values = [new[k] for k in ("id", "family_id", "version", "name", "status", "effective_from", "effective_to")]
        values += [stable(new[k]) for k in ("config", "scope", "conflicts", "source")]
        values += [actor_id]
        statements.append("INSERT INTO biz_att_rule (id,family_id,version,name,status,effective_from,effective_to,config_json,scope_json,conflicts_json,source_json,created_by,created_at,updated_at) VALUES (" + ",".join(literal(v) for v in values) + ",@attendance_change_at,@attendance_change_at);")
        audit(new["id"], "rebuild_work_schedule", action["before"], new)
        check("(SELECT COUNT(*) FROM biz_att_rule WHERE BINARY id=" + literal(new["id"]) + " AND status='draft')=1")
    changed = {rid for action in plan["actions"] for rid in action["archive_ids"]}
    for row in snapshot["rule"]:
        if row["id"] not in changed:
            check("(SELECT COUNT(*) FROM biz_att_rule WHERE " + row_matches(row) + ")=1")
    check("(SELECT COUNT(*) FROM biz_att_rule)=" + str(len(snapshot["rule"]) + len(plan["actions"])))
    check("(SELECT COUNT(*) FROM biz_att_audit WHERE entity='rule')=" + str(len(snapshot.get("rule_audit", [])) + len(changed) + len(plan["actions"])))
    statements.extend(["COMMIT;", "SELECT 'ATTENDANCE_DRAFT_PLAN_COMMITTED';"])
    return "\n".join(statements) + "\n"


def main():
    sys.stdout.reconfigure(encoding="utf-8")
    parser = argparse.ArgumentParser()
    parser.add_argument("--plan", required=True)
    parser.add_argument("--snapshot", required=True)
    parser.add_argument("--expect-plan-sha256", required=True)
    parser.add_argument("--actor-id", required=True, type=int)
    mode = parser.add_mutually_exclusive_group(required=True)
    mode.add_argument("--emit-sql")
    mode.add_argument("--apply", action="store_true")
    parser.add_argument("--expected-package")
    parser.add_argument("--backup-file")
    parser.add_argument("--report")
    args = parser.parse_args()
    plan = json.loads(Path(args.plan).read_text(encoding="utf-8-sig"))
    snapshot = json.loads(Path(args.snapshot).read_text(encoding="utf-8-sig"))
    validate(plan, snapshot, args.expect_plan_sha256)
    sql = build_sql(plan, snapshot, args.actor_id)
    if args.emit_sql:
        Path(args.emit_sql).write_text(sql, encoding="utf-8")
        print(json.dumps({"mode": "sql_preview_only", "database_writes": False, "plan_sha256": plan["plan_sha256"], "sql_sha256": hashlib.sha256(sql.encode()).hexdigest(), "summary": plan["summary"]}))
        return
    if os.name == "nt" or os.geteuid() != 0:
        raise ValueError("Apply requires sudo on the existing Ubuntu host")
    if not args.expected_package or not re.fullmatch(r"[0-9a-f]{64}", args.expected_package) or not args.backup_file or not args.report:
        raise ValueError("Apply requires exact deployed package digest, existing database backup and report path")
    active = json.loads(Path("/srv/gosuxin-suite/shared/gosuxin/storage/plugins/biz-suite/active.json").read_text())
    if active.get("package") != args.expected_package or active.get("disabled"):
        raise ValueError("Deployed plugin is not the reviewed active package")
    backup = Path(args.backup_file).resolve(strict=True)
    if not backup.is_file() or backup.stat().st_size < 100:
        raise ValueError("Database backup is missing or empty")
    report_path = Path(args.report)
    if report_path.exists():
        raise ValueError("Refusing to overwrite an earlier execution report; inspect prior outcome first")
    report = {"plan_sha256": plan["plan_sha256"], "deployed_package": args.expected_package, "backup_path": str(backup), "sql_sha256": hashlib.sha256(sql.encode()).hexdigest(), "actor_id": args.actor_id, "status": "started", "summary": plan["summary"]}
    report_path.write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding="utf-8")
    report_path.chmod(0o600)
    command = ["docker", "exec", "-i", "gosuxin-suite-mysql-1", "mysql", "--defaults-extra-file=/run/secrets/backup.cnf", "--default-character-set=utf8mb4", "-N", "-B", "--raw", "kaoqin_dev"]
    result = subprocess.run(command, input=sql.encode(), capture_output=True, timeout=120)
    committed = result.returncode == 0 and b"ATTENDANCE_DRAFT_PLAN_COMMITTED" in result.stdout
    report["status"] = "committed" if committed else "failed_or_uncertain_inspect_before_retry"
    report["mysql_exit_code"] = result.returncode
    # Never print full SQL, employee records or database diagnostics.
    error = re.search(rb"ERROR\s+(\d+)\s+\([^)]*\)\s+at line\s+(\d+)", result.stderr)
    if error:
        report["mysql_error_code"], report["sql_error_line"] = int(error[1]), int(error[2])
    report_path.write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding="utf-8")
    print(json.dumps(report, ensure_ascii=False))
    if not committed:
        raise RuntimeError("Apply did not confirm commit. Inspect the report and fresh read-only snapshot before any retry")


if __name__ == "__main__":
    main()
