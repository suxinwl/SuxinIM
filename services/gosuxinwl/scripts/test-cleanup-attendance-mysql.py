"""Run cleanup SQL on connection-local temporary copies only, on either host."""
import argparse
import importlib.util
import json
from pathlib import Path
import re

spec = importlib.util.spec_from_file_location("cleanup", Path(__file__).with_name("cleanup-attendance-rules.py"))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--plan", required=True)
    parser.add_argument("--snapshot", required=True)
    parser.add_argument("--output", required=True)
    args = parser.parse_args()
    plan, data = [json.loads(Path(p).read_text(encoding="utf-8-sig")) for p in (args.plan, args.snapshot)]
    module.validate_plan(plan, data, plan["plan_sha256"])
    before = module.snapshot()
    if before["rules_sha256"] != data["rules_sha256"] or before["audits_sha256"] != data["audits_sha256"]:
        raise RuntimeError("Database differs from test snapshot")
    exact = module.build_sql(plan, data, 1)
    tables = ("biz_att_rule", "biz_att_audit", "gf_admin")
    sql = []
    for table in tables:
        sql += ["CREATE TEMPORARY TABLE qa_" + table + " LIKE " + table + ";", "INSERT INTO qa_" + table + " SELECT * FROM " + table + ";"]
    for line in exact.splitlines():
        if "information_schema.tables" not in line:
            for table in tables:
                line = re.sub(r"\b" + table + r"\b", "qa_" + table, line)
        sql.append(line)
    for kind, table, columns in (("rules", "qa_biz_att_rule", module.RULE_COLUMNS), ("audits", "qa_biz_att_audit", module.AUDIT_COLUMNS)):
        sql.append("SELECT JSON_OBJECT('kind','%s','row',%s) FROM %s ORDER BY id;" % (kind, module.row_object(columns), table))
    text = "\n".join(sql)
    for target in re.findall(r"(?:INSERT INTO|UPDATE|DELETE FROM)\s+([a-z_]+)", text):
        if target not in {"qa_" + t for t in tables} | {"attendance_cleanup_assertion"}:
            raise RuntimeError("Refusing DML on permanent table: " + target)
    output = module.run_db(text).decode()
    temporary = dict(before, rules=[], audits=[])
    for line in output.splitlines():
        if line.startswith('{'):
            record = json.loads(line)
            if record.get("kind") in ("rules", "audits"):
                temporary[record["kind"]].append(record["row"])
    temporary["rules_sha256"], temporary["audits_sha256"] = module.sha(temporary["rules"]), module.sha(temporary["audits"])
    verification = module.verify_after(plan, before, temporary, 1)
    after = module.snapshot()
    unchanged = all(before[k] == after[k] for k in ("rules_sha256", "audits_sha256", "protected_checksums"))
    backup = module.run_db(backup=True)
    backup_ok = b"CREATE TABLE `biz_att_rule`" in backup and b"CREATE TABLE `biz_att_audit`" in backup and len(backup) > 1000
    result = {"permanent_business_writes": False, "plan_sha256": plan["plan_sha256"], "temporary_transaction_verified": verification, "permanent_tables_unchanged": unchanged, "full_sql_backup_command_verified": backup_ok, "backup_read_bytes": len(backup), "selected": len(plan["actions"])}
    module.save(args.output, result)
    print(json.dumps({k: v for k, v in result.items() if k != "temporary_transaction_verified"}))
    if not (verification["passed"] and unchanged and backup_ok and "ATTENDANCE_RECOVERABLE_CLEANUP_COMMITTED" in output):
        raise RuntimeError("Temporary-copy cleanup verification failed; inspect report")


if __name__ == "__main__":
    main()
