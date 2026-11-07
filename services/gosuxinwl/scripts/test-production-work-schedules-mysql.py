"""Exercise the exact migration on connection-local temporary table copies only.

Reads production source rows into temporary copies preserving real collations.
Every DML target is statically allowlisted to a temporary qa_* table. No account,
business record, schema, configuration or permanent table is created or changed.
"""
import argparse
import hashlib
import importlib.util
import json
from pathlib import Path
import re
import subprocess

spec = importlib.util.spec_from_file_location("apply_plan", Path(__file__).with_name("apply-production-work-schedules.py"))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
TABLES = ("biz_att_rule", "biz_att_audit", "biz_hrm_employee", "gf_auth_dept", "wecom_department", "wecom_user", "wecom_settings", "gf_admin")
MYSQL = ["docker", "exec", "-i", "gosuxin-suite-mysql-1", "mysql", "--defaults-extra-file=/run/secrets/backup.cnf", "--default-character-set=utf8mb4", "-N", "-B", "--raw", "kaoqin_dev"]


def run(sql):
    result = subprocess.run(MYSQL, input=sql.encode(), capture_output=True, timeout=120)
    error = re.search(rb"ERROR\s+(\d+)\s+\([^)]*\)\s+at line\s+(\d+)", result.stderr)
    return result, {"exit_code": result.returncode, "mysql_error_code": int(error[1]) if error else None, "sql_error_line": int(error[2]) if error else None}


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--plan", required=True)
    parser.add_argument("--snapshot", required=True)
    parser.add_argument("--report", required=True)
    args = parser.parse_args()
    plan = json.loads(Path(args.plan).read_text())
    snapshot = json.loads(Path(args.snapshot).read_text())
    module.validate(plan, snapshot, plan["plan_sha256"])
    exact_sql = module.build_sql(plan, snapshot, 1)
    # Reproduce the original first-UPDATE comparison using a read-only SELECT.
    old_id = plan["actions"][0]["before"][0]["id"]
    original = "SET NAMES utf8mb4; SELECT COUNT(*) FROM biz_att_rule WHERE id=" + module.literal(old_id) + ";"
    _, reproduction = run(original)
    corrected, correction = run(original.replace("WHERE id=", "WHERE BINARY id="))
    if corrected.returncode or corrected.stdout.strip() != b"1":
        raise RuntimeError("Read-only exact ID comparison failed")
    setup = []
    for table in TABLES:
        setup += ["CREATE TEMPORARY TABLE qa_" + table + " LIKE " + table + ";", "INSERT INTO qa_" + table + " SELECT * FROM " + table + ";"]
    translated = []
    for line in exact_sql.splitlines():
        if "information_schema.tables" not in line:
            for table in TABLES:
                line = re.sub(r"\b" + table + r"\b", "qa_" + table, line)
        translated.append(line)
    sql = "\n".join(setup + translated)
    for target in re.findall(r"(?:INSERT INTO|UPDATE|DELETE FROM)\s+([a-z_]+)", sql):
        if target not in {"qa_" + table for table in TABLES} | {"attendance_plan_assertion"}:
            raise ValueError("Refusing any DML on a non-temporary table: " + target)
    result, execution = run(sql)
    execution["committed_only_to_temporary_tables"] = result.returncode == 0 and b"ATTENDANCE_DRAFT_PLAN_COMMITTED" in result.stdout
    report = {"permanent_database_writes": False, "plan_sha256": plan["plan_sha256"], "sql_sha256": hashlib.sha256(exact_sql.encode()).hexdigest(), "temporary_table_copies": list(TABLES), "original_read_only_comparison": reproduction, "corrected_read_only_comparison": correction, "temporary_full_migration": execution}
    Path(args.report).write_text(json.dumps(report, indent=2))
    print(json.dumps(report))
    if not execution["committed_only_to_temporary_tables"]:
        raise RuntimeError("Temporary-copy regression failed; no permanent database DML was performed")


if __name__ == "__main__":
    main()
