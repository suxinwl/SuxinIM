"""Reviewable, recoverable attendance rule cleanup for Windows and Ubuntu.

snapshot and plan are read-only; emit-sql is offline. apply requires the reviewed
plan hash and exact active package. All old rows remain, only status/source
cleanup metadata/updated_at change, with complete before/after audit and backup.
"""
import argparse
from collections import Counter
import copy
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys

ROOT = Path(__file__).resolve().parent.parent
RULE_COLUMNS = "id family_id version name status effective_from effective_to config_json scope_json conflicts_json source_json created_by created_at updated_at".split()
AUDIT_COLUMNS = "id entity entity_id action before_json after_json reason created_by created_at".split()
PROTECTED = ("biz_hrm_employee", "gf_auth_dept", "wecom_user", "wecom_department", "wecom_settings", "biz_att_roster", "biz_att_result", "biz_att_batch", "biz_att_month", "biz_att_publication", "biz_att_dirty", "wecom_attendance_outbox")


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


def row_object(columns):
    return "JSON_OBJECT(" + ",".join("'%s',`%s`" % (key, key) for key in columns) + ")"


def row_matches(row):
    out = []
    for key, value in row.items():
        if not re.fullmatch("[a-z_]+", key):
            raise ValueError("Unexpected column")
        column = "`" + key + "`"
        if isinstance(value, str) and not key.endswith("_at"):
            column = "BINARY " + column
        out.append("(" + column + " <=> " + literal(value) + ")")
    return " AND ".join(out)


def connection():
    return "windows-local" if os.name == "nt" else "ubuntu-192.168.10.11"


def active_package():
    path = ROOT / "storage/plugins/biz-suite/active.json" if os.name == "nt" else Path("/srv/gosuxin-suite/shared/gosuxin/storage/plugins/biz-suite/active.json")
    data = json.loads(path.read_text(encoding="utf-8-sig"))
    if data.get("disabled"):
        raise ValueError("Plugin is disabled")
    return data["package"]


def run_db(sql="", backup=False):
    if os.name == "nt":
        command = [os.environ.get("NODE_EXE", "node"), str(Path(__file__).with_name("attendance-cleanup-local-db.mjs")), "backup" if backup else "query"]
    else:
        command = ["docker", "exec"] + ([] if backup else ["-i"]) + ["gosuxin-suite-mysql-1", "mysqldump" if backup else "mysql", "--defaults-extra-file=/run/secrets/backup.cnf", "--default-character-set=utf8mb4"]
        command += ["--single-transaction", "--quick", "--hex-blob", "--set-gtid-purged=OFF", "--skip-dump-date", "--order-by-primary", "kaoqin_dev", "biz_att_rule", "biz_att_audit"] if backup else ["-N", "-B", "--raw", "kaoqin_dev"]
    result = subprocess.run(command, input=None if backup else sql.encode(), capture_output=True, timeout=130, **({"creationflags": subprocess.CREATE_NO_WINDOW} if os.name == "nt" else {}))
    if result.returncode:
        error = re.search(rb"(?:ERROR\s+|MYSQL_ERROR\s+)(\d+)(?:\s+\([^)]*\)\s+at line\s+|\s+LINE\s+)(\d+)", result.stderr)
        code = " (code %s, SQL line %s)" % (error[1].decode(), error[2].decode()) if error else ""
        raise RuntimeError("Database operation failed" + code + "; private SQL and credentials suppressed")
    return result.stdout


def snapshot():
    sql = ["START TRANSACTION READ ONLY, WITH CONSISTENT SNAPSHOT;"]
    for table, key, columns in (("biz_att_rule", "rules", RULE_COLUMNS), ("biz_att_audit", "audits", AUDIT_COLUMNS)):
        sql.append("SELECT JSON_OBJECT('kind','%s','row',%s) FROM %s ORDER BY id;" % (key, row_object(columns), table))
    sql.append("COMMIT;")
    result = {"read_only": True, "connection": connection(), "database": "kaoqin_dev", "active_package": active_package(), "rules": [], "audits": []}
    for line in run_db("\n".join(sql)).decode().splitlines():
        data = json.loads(line)
        result[data["kind"]].append(data["row"])
    checksums = run_db("CHECKSUM TABLE " + ",".join(PROTECTED) + " EXTENDED;").decode().splitlines()
    result["protected_checksums"] = {}
    for line in checksums:
        name, checksum = line.split("\t")
        if checksum == "NULL":
            raise RuntimeError("Protected table checksum unavailable")
        result["protected_checksums"][name.split(".")[-1]] = checksum
    result["rules_sha256"], result["audits_sha256"] = sha(result["rules"]), sha(result["audits"])
    result["snapshot_sha256"] = sha(result)
    return result


def validate_snapshot(data):
    if sha({k: v for k, v in data.items() if k != "snapshot_sha256"}) != data["snapshot_sha256"] or sha(data["rules"]) != data["rules_sha256"] or sha(data["audits"]) != data["audits_sha256"]:
        raise ValueError("Snapshot integrity mismatch")


def prepare(data, candidates, expected_count):
    validate_snapshot(data)
    selected = candidates["candidates"]
    ids = [r["id"] for r in selected]
    if len(ids) != expected_count or len(ids) != len(set(ids)) or not ids:
        raise ValueError("Candidate count/uniqueness does not match the explicit selection")
    batch = candidates["batch_id"]
    if not re.fullmatch(r"[A-Za-z0-9_-]{1,80}", batch):
        raise ValueError("Invalid cleanup batch")
    rules = {r["id"]: r for r in data["rules"]}
    actions = []
    for candidate in selected:
        old = rules[candidate["id"]]
        if old["status"] not in ("draft", "archived"):
            raise ValueError("Published, superseded or unknown-status rule cannot be cleaned")
        source = json.loads(old["source_json"])
        if not isinstance(source, dict) or source.get("cleanup"):
            raise ValueError("Existing cleanup metadata or non-object source requires separate review")
        # Empty legacy drafts may be removed from the daily list; bound or
        # default drafts need individual approval and are deliberately rejected.
        scope = json.loads(old["scope_json"])
        if old["status"] == "draft" and (scope.get("employee_ids") or scope.get("department_ids") or scope.get("is_default")):
            raise ValueError("A scoped/default draft cannot be automatically cleaned")
        reason = candidate.get("reason") or candidates.get("reason")
        if not isinstance(reason, str) or not reason.strip() or len(reason) > 600:
            raise ValueError("Every cleanup candidate requires a concise audit reason")
        cleaned = copy.deepcopy(source)
        cleaned["cleanup"] = {"hidden": True, "batch_id": batch, "reason": reason, "previous_status": old["status"]}
        actions.append({"id": old["id"], "name": old["name"], "before": old, "before_sha256": sha(old), "after_source": cleaned, "after_status": "archived", "reason": reason})
    result = {"mode": "recoverable_cleanup_plan", "database_writes": False, "connection": data["connection"], "database": data["database"], "snapshot_sha256": data["snapshot_sha256"], "rules_before_sha256": data["rules_sha256"], "audits_before_sha256": data["audits_sha256"], "batch_id": batch, "created_at": datetime.now(timezone.utc).isoformat(), "actions": actions, "summary": {"total_rules": len(data["rules"]), "before_statuses": dict(Counter(r["status"] for r in data["rules"])), "selected": len(actions), "selected_statuses": dict(Counter(a["before"]["status"] for a in actions)), "unmodified_rules": len(data["rules"]) - len(actions), "hard_deletes": 0}}
    result["plan_sha256"] = sha(result)
    return result


def validate_plan(plan, data, expected):
    validate_snapshot(data)
    if sha({k: v for k, v in plan.items() if k != "plan_sha256"}) != expected or plan["plan_sha256"] != expected:
        raise ValueError("Reviewed plan hash mismatch")
    if plan["snapshot_sha256"] != data["snapshot_sha256"] or plan["mode"] != "recoverable_cleanup_plan" or plan["database_writes"]:
        raise ValueError("Plan snapshot or mode mismatch")
    by_id = {r["id"]: r for r in data["rules"]}
    if len(plan["actions"]) != plan["summary"]["selected"] or len({a["id"] for a in plan["actions"]}) != len(plan["actions"]):
        raise ValueError("Action identity/count mismatch")
    for action in plan["actions"]:
        old = action["before"]
        if by_id.get(action["id"]) != old or sha(old) != action["before_sha256"] or old["status"] not in ("draft", "archived") or action["after_status"] != "archived":
            raise ValueError("Candidate changed or illegal status")
        expected_source = json.loads(old["source_json"])
        expected_source["cleanup"] = {"hidden": True, "batch_id": plan["batch_id"], "reason": action["reason"], "previous_status": old["status"]}
        if action["after_source"] != expected_source:
            raise ValueError("Only the reviewed cleanup source metadata may change")


def build_sql(plan, data, actor):
    if actor < 1:
        raise ValueError("An existing audit actor is required")
    sql = ["SET NAMES utf8mb4;", "SET time_zone='+08:00';", "SET SESSION TRANSACTION ISOLATION LEVEL REPEATABLE READ;", "SET SESSION innodb_lock_wait_timeout=15;", "CREATE TEMPORARY TABLE attendance_cleanup_assertion (id INT PRIMARY KEY) ENGINE=MEMORY;", "INSERT INTO attendance_cleanup_assertion VALUES (1);", "START TRANSACTION;"]

    def check(test):
        sql.append("INSERT INTO attendance_cleanup_assertion SELECT 1 WHERE NOT (" + test + ");")

    check("(SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name IN ('biz_att_rule','biz_att_audit') AND engine='InnoDB')=2")
    check("(SELECT COUNT(*) FROM gf_admin WHERE id=" + str(actor) + ")=1")
    for table, rows in (("biz_att_rule", data["rules"]), ("biz_att_audit", data["audits"])):
        sql.append("SELECT id FROM " + table + " ORDER BY id FOR UPDATE;")
        check("(SELECT COUNT(*) FROM " + table + ")=" + str(len(rows)))
        for row in rows:
            check("(SELECT COUNT(*) FROM " + table + " WHERE " + row_matches(row) + ")=1")
    sql.append("SET @attendance_cleanup_time=NOW();")
    changed = {a["id"] for a in plan["actions"]}
    for action in plan["actions"]:
        source = "JSON_SET(" + literal(stable(action["after_source"])) + ",'$.cleanup.cleaned_at',DATE_FORMAT(@attendance_cleanup_time,'%Y-%m-%d %H:%i:%s'),'$.cleanup.timezone','Asia/Shanghai')"
        predicate = "BINARY id=" + literal(action["id"])
        sql.append("UPDATE biz_att_rule SET status='archived',source_json=" + source + ",updated_at=@attendance_cleanup_time WHERE " + predicate + " AND status IN ('archived','draft');")
        check("ROW_COUNT()=1")
        audit_id = "rule-cleanup-" + sha([plan["plan_sha256"], action["id"]])[:40]
        reason = action["reason"] + "；可恢复清理；计划 " + plan["plan_sha256"]
        sql.append("INSERT INTO biz_att_audit (id,entity,entity_id,action,before_json,after_json,reason,created_by,created_at) SELECT " + ",".join(literal(v) for v in (audit_id, "rule", action["id"], "cleanup_hide", stable(action["before"]))) + "," + row_object(RULE_COLUMNS) + "," + literal(reason) + "," + str(actor) + ",@attendance_cleanup_time FROM biz_att_rule WHERE " + predicate + ";")
        check("ROW_COUNT()=1")
        check("(SELECT COUNT(*) FROM biz_att_rule WHERE " + predicate + " AND status='archived' AND JSON_EXTRACT(source_json,'$.cleanup.hidden')=TRUE)=1")
    for rule in data["rules"]:
        if rule["id"] not in changed:
            check("(SELECT COUNT(*) FROM biz_att_rule WHERE " + row_matches(rule) + ")=1")
    for audit in data["audits"]:
        check("(SELECT COUNT(*) FROM biz_att_audit WHERE " + row_matches(audit) + ")=1")
    check("(SELECT COUNT(*) FROM biz_att_rule)=" + str(len(data["rules"])))
    check("(SELECT COUNT(*) FROM biz_att_audit)=" + str(len(data["audits"]) + len(changed)))
    sql += ["COMMIT;", "SELECT 'ATTENDANCE_RECOVERABLE_CLEANUP_COMMITTED';"]
    return "\n".join(sql) + "\n"


def verify_after(plan, before, after, actor):
    rows, old = ({r["id"]: r for r in d["rules"]} for d in (after, before))
    checks = {"same_rule_ids": set(rows) == set(old), "protected_tables_unchanged": before["protected_checksums"] == after["protected_checksums"]}
    actions = {a["id"]: a for a in plan["actions"]}
    for rid, original in old.items():
        actual = rows.get(rid, {})
        if rid not in actions:
            checks["unchanged:" + rid] = actual == original
            continue
        source = json.loads(actual.get("source_json", "{}"))
        cleanup = source.get("cleanup", {})
        recorded_time = cleanup.pop("cleaned_at", None)
        recorded_zone = cleanup.pop("timezone", None)
        checks["cleaned:" + rid] = actual.get("status") == "archived" and bool(recorded_time) and recorded_zone == "Asia/Shanghai" and source == actions[rid]["after_source"] and all(actual.get(k) == v for k, v in original.items() if k not in ("status", "source_json", "updated_at"))
    old_audits, new_audits = ({a["id"]: a for a in d["audits"]} for d in (before, after))
    checks["old_audits_unchanged"] = all(new_audits.get(k) == v for k, v in old_audits.items())
    expected_ids = set()
    for rid, action in actions.items():
        key = "rule-cleanup-" + sha([plan["plan_sha256"], rid])[:40]
        expected_ids.add(key)
        audit = new_audits.get(key, {})
        checks["audit:" + rid] = audit.get("entity") == "rule" and audit.get("entity_id") == rid and audit.get("action") == "cleanup_hide" and audit.get("created_by") == actor and json.loads(audit.get("before_json", "null")) == action["before"] and json.loads(audit.get("after_json", "null")) == rows.get(rid)
    checks["exact_new_audits"] = set(new_audits) - set(old_audits) == expected_ids
    return {"passed": all(checks.values()), "checks": checks, "rules_after_sha256": after["rules_sha256"], "audits_after_sha256": after["audits_sha256"]}


def save(path, data):
    target = Path(path)
    if target.exists():
        raise ValueError("Refusing to overwrite an earlier artifact: " + str(target))
    target.write_text(json.dumps(data, ensure_ascii=False, indent=2), encoding="utf-8")
    if os.name != "nt":
        target.chmod(0o600)
        import shutil
        shutil.chown(target, "suxin", "suxin")


def main():
    sys.stdout.reconfigure(encoding="utf-8")
    parser = argparse.ArgumentParser()
    parser.add_argument("mode", choices=("snapshot", "plan", "emit-sql", "apply"))
    parser.add_argument("--snapshot")
    parser.add_argument("--candidates")
    parser.add_argument("--expected-count", type=int)
    parser.add_argument("--plan")
    parser.add_argument("--expect-plan-sha256")
    parser.add_argument("--actor-id", type=int, default=1)
    parser.add_argument("--expected-package")
    parser.add_argument("--backup-dir")
    parser.add_argument("--output", required=True)
    args = parser.parse_args()
    if args.mode == "snapshot":
        data = snapshot()
        save(args.output, data)
        print(json.dumps({"read_only": True, "snapshot_sha256": data["snapshot_sha256"], "rules": len(data["rules"]), "audits": len(data["audits"]), "statuses": dict(Counter(r["status"] for r in data["rules"]))}))
        return
    data = json.loads(Path(args.snapshot).read_text(encoding="utf-8-sig"))
    if args.mode == "plan":
        result = prepare(data, json.loads(Path(args.candidates).read_text(encoding="utf-8-sig")), args.expected_count)
        save(args.output, result)
        print(json.dumps({"database_writes": False, "plan_sha256": result["plan_sha256"], "summary": result["summary"]}))
        return
    plan = json.loads(Path(args.plan).read_text(encoding="utf-8-sig"))
    validate_plan(plan, data, args.expect_plan_sha256)
    sql = build_sql(plan, data, args.actor_id)
    if args.mode == "emit-sql":
        with Path(args.output).open("x", encoding="utf-8") as output:
            output.write(sql)
        print(json.dumps({"database_writes": False, "sql_sha256": hashlib.sha256(sql.encode()).hexdigest()}))
        return
    if data["connection"] != connection() or not args.backup_dir or not args.expected_package or active_package() != args.expected_package:
        raise ValueError("Connection, backup directory or exact deployed package mismatch")
    if os.name != "nt" and os.geteuid() != 0:
        raise ValueError("Ubuntu apply requires sudo")
    report_path = Path(args.output)
    if report_path.exists():
        raise ValueError("Earlier report exists; inspect outcome before any retry")
    current = snapshot()
    if current["rules_sha256"] != data["rules_sha256"] or current["audits_sha256"] != data["audits_sha256"] or current["protected_checksums"] != data["protected_checksums"]:
        raise ValueError("Database changed since review; prepare a new snapshot/plan")
    backup = Path(args.backup_dir)
    backup.mkdir(parents=True, exist_ok=False)
    if os.name != "nt":
        backup.chmod(0o700)
    raw = run_db(backup=True)
    if len(raw) < 1000 or b"CREATE TABLE `biz_att_rule`" not in raw or b"CREATE TABLE `biz_att_audit`" not in raw:
        raise ValueError("Full rule and audit SQL backup failed")
    backup_file = backup / "rules-and-audits-before.sql"
    with backup_file.open("xb") as target:
        target.write(raw)
    if os.name != "nt":
        backup_file.chmod(0o600)
    save(backup / "snapshot-before.json", current)
    save(backup / "plan.json", plan)
    report = {"mode": "recoverable_cleanup", "plan_sha256": plan["plan_sha256"], "connection": connection(), "package": args.expected_package, "backup_path": str(backup_file), "backup_sha256": hashlib.sha256(raw).hexdigest(), "backup_bytes": len(raw), "sql_sha256": hashlib.sha256(sql.encode()).hexdigest(), "status": "started", "actor_id": args.actor_id, "summary": plan["summary"]}
    save(report_path, report)
    try:
        result = run_db(sql)
        if b"ATTENDANCE_RECOVERABLE_CLEANUP_COMMITTED" not in result:
            raise RuntimeError("Commit confirmation absent")
        report["status"] = "committed"
        after = snapshot()
        save(backup / "snapshot-after.json", after)
        report["verification"] = verify_after(plan, current, after, args.actor_id)
        if not report["verification"]["passed"]:
            raise RuntimeError("Committed, but post-check failed; inspect report before any other action")
    except Exception as error:
        if report["status"] != "committed":
            report["status"] = "failed_or_uncertain_inspect_before_retry"
        report["error"] = str(error)
        raise
    finally:
        report_path.write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding="utf-8")
        print(json.dumps({k: v for k, v in report.items() if k != "verification"}))


if __name__ == "__main__":
    main()
