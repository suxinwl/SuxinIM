"""Read-only rule inventory and reference graph for cleanup review."""
import argparse
from collections import Counter, defaultdict
import hashlib
import json
from pathlib import Path
import re
import subprocess

MYSQL = ["docker", "exec", "-i", "gosuxin-suite-mysql-1", "mysql", "--defaults-extra-file=/run/secrets/backup.cnf", "--default-character-set=utf8mb4", "-N", "-B", "--raw", "kaoqin_dev"]


def query(sql):
    result = subprocess.run(MYSQL, input=sql.encode(), capture_output=True, timeout=90)
    if result.returncode:
        raise RuntimeError("Read-only audit query failed; private output suppressed")
    return result.stdout.decode().splitlines()


def stable(value):
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":"))


def sha(value):
    return hashlib.sha256(stable(value).encode()).hexdigest()


def scan(value, ids, path="", depth=0):
    if depth > 30:
        return []
    out = []
    if isinstance(value, dict):
        for key, child in value.items():
            out += scan(child, ids, path + "." + key, depth + 1)
    elif isinstance(value, list):
        for index, child in enumerate(value):
            out += scan(child, ids, path + "[" + str(index) + "]", depth + 1)
    elif isinstance(value, str):
        if value in ids:
            out.append((value, path))
        elif value.lstrip().startswith(("{", "[")):
            try:
                out += scan(json.loads(value), ids, path, depth + 1)
            except (ValueError, TypeError):
                pass
    return out


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", required=True)
    args = parser.parse_args()
    schemas = defaultdict(list)
    for line in query("SELECT JSON_OBJECT('table',table_name,'column',column_name) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name LIKE 'biz_att_%' ORDER BY table_name,ordinal_position;"):
        row = json.loads(line)
        if not re.fullmatch(r"[a-z0-9_]+", row["table"]) or not re.fullmatch(r"[a-z0-9_]+", row["column"]):
            raise RuntimeError("Unexpected schema identifier")
        schemas[row["table"]].append(row["column"])
    sql = ["START TRANSACTION READ ONLY, WITH CONSISTENT SNAPSHOT;"]
    selected = {}
    for table, columns in schemas.items():
        # Scope is rule references and batch/month links, not raw attendance
        # employee lists or credentials. Only rule rows are retained in full.
        chosen = columns if table == "biz_att_rule" else [c for c in columns if c.endswith("_json") or c in ("id", "month", "rule_id", "batch_id", "family_id", "entity", "entity_id", "source_ref", "status", "action", "work_date", "revision")]
        if not chosen:
            continue
        selected[table] = chosen
        pairs = ",".join("'%s',`%s`" % (column, column) for column in chosen)
        sql.append("SELECT JSON_OBJECT('table','%s','data',JSON_OBJECT(%s)) FROM `%s`;" % (table, pairs, table))
    sql.append("COMMIT;")
    data = defaultdict(list)
    for line in query("\n".join(sql)):
        record = json.loads(line)
        data[record["table"]].append(record["data"])
    rules = sorted(data["biz_att_rule"], key=lambda row: row["id"])
    ids = {r["id"] for r in rules}
    refs = defaultdict(list)
    for table, rows in data.items():
        for row in rows:
            row_id = row.get("id", row.get("month", ""))
            hits = defaultdict(set)
            for rule_id, path in scan(row, ids):
                if table == "biz_att_rule" and rule_id == row_id:
                    continue
                hits[rule_id].add(path)
            for rule_id, paths in hits.items():
                refs[rule_id].append({"table": table, "row_id": row_id, "status": row.get("status"), "entity": row.get("entity"), "action": row.get("action"), "paths": sorted(paths)})
    batches = [{k: r.get(k) for k in ("id", "status", "request_json")} for r in data.get("biz_att_batch", [])]
    months = data.get("biz_att_month", [])
    publications = data.get("biz_att_publication", [])
    result_batches = defaultdict(set)
    for row in data.get("biz_att_result", []):
        if row.get("rule_id") in ids:
            result_batches[row["rule_id"]].add(row["batch_id"])
    results = data.get("biz_att_result", [])
    report = {"read_only": True, "rules": rules, "rules_sha256": sha(rules), "references": refs, "table_counts": {k: len(data.get(k, [])) for k in selected}, "scanned_columns": selected, "batches": batches, "months": months, "publications": publications, "result_batches": {key: sorted(value) for key, value in result_batches.items()}, "result_summary": {"rows": len(results), "blank_rule_ids": sum(not r.get("rule_id") for r in results), "status_counts": dict(Counter(r.get("status") for r in results))}}
    report["audit_sha256"] = sha(report)
    Path(args.output).write_text(json.dumps(report, ensure_ascii=False, indent=2))
    print(json.dumps({"read_only": True, "rule_count": len(rules), "status_counts": dict(Counter(r["status"] for r in rules)), "rules_sha256": report["rules_sha256"], "audit_sha256": report["audit_sha256"], "table_counts": report["table_counts"]}))


if __name__ == "__main__":
    main()
