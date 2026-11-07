"""Read-only production-rule snapshot and reviewable migration preparation.

This tool never executes database INSERT/UPDATE/DELETE and has no apply mode.
Remote snapshots use the existing backup client configuration, not chat secrets.
"""
from __future__ import annotations

import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import copy
import re
import unicodedata
from collections import Counter, defaultdict


def stable(value):
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":"))


def sha(value):
    return hashlib.sha256(stable(value).encode()).hexdigest()


def remote_snapshot(destination):
    if os.name == "nt" or os.geteuid() != 0:
        raise RuntimeError("Remote read-only snapshot requires sudo on the existing Ubuntu host")
    queries = ["START TRANSACTION READ ONLY, WITH CONSISTENT SNAPSHOT;"]
    definitions = [
        ("rule", "biz_att_rule", ["id", "family_id", "version", "name", "status", "effective_from", "effective_to", "config_json", "scope_json", "conflicts_json", "source_json", "created_by", "created_at", "updated_at"], "id"),
        ("employee", "biz_hrm_employee", ["id", "name", "employee_no", "dept_id", "status", "wecom_userid"], "id"),
        ("department", "gf_auth_dept", ["id", "pid", "name"], "id"),
        ("wecom_department", "wecom_department", ["wecom_id", "parent_id", "name", "host_dept_id"], "wecom_id"),
        ("wecom_user", "wecom_user", ["wecom_userid", "name", "department_ids"], "wecom_userid"),
        ("rule_audit", "biz_att_audit WHERE entity='rule'", ["id", "entity_id", "action", "reason", "before_json", "after_json", "created_by", "created_at"], "created_at,id"),
    ]
    for kind, table, columns, order in definitions:
        pairs = ",".join("'%s',`%s`" % (column, column) for column in columns)
        queries.append("SELECT JSON_OBJECT('record_type','%s','data',JSON_OBJECT(%s)) FROM %s ORDER BY %s;" % (kind, pairs, table, order))
    keys = ["exclude_department_ids", "exclude_department_names", "exclude_userids", "exclude_user_names", "include_subdepartments"]
    pairs = ",".join("'%s',JSON_EXTRACT(settings_json,'$.%s')" % (key, key) for key in keys)
    queries.append("SELECT JSON_OBJECT('record_type','exclusions','data',JSON_OBJECT(%s)) FROM wecom_settings WHERE id=1;" % pairs)
    queries.append("COMMIT;")
    command = ["docker", "exec", "-i", "gosuxin-suite-mysql-1", "mysql", "--defaults-extra-file=/run/secrets/backup.cnf", "--default-character-set=utf8mb4", "-N", "-B", "--raw", "kaoqin_dev"]
    result = subprocess.run(command, input="\n".join(queries).encode(), capture_output=True, timeout=90)
    if result.returncode:
        raise RuntimeError("Read-only snapshot query failed; private database output suppressed")
    data = {"host": "192.168.10.11", "database": "kaoqin_dev", "read_only": True}
    for line in result.stdout.decode().splitlines():
        row = json.loads(line)
        data.setdefault(row["record_type"], []).append(row["data"])
    active = Path("/srv/gosuxin-suite/shared/gosuxin/storage/plugins/biz-suite/active.json")
    data["active_plugin"] = json.loads(active.read_text())
    data["rules_sha256"] = sha(data.get("rule", []))
    data["snapshot_sha256"] = sha(data)
    target = Path(destination)
    target.write_text(json.dumps(data, ensure_ascii=False, indent=2), encoding="utf-8")
    target.chmod(0o600)
    # This is a non-secret snapshot; allow the authorized SSH user to retrieve it.
    import shutil
    shutil.chown(target, "suxin", "suxin")
    print(json.dumps({"read_only": True, "rules": len(data.get("rule", [])), "employees": len(data.get("employee", [])), "rules_sha256": data["rules_sha256"], "snapshot_sha256": data["snapshot_sha256"]}))


def names(value):
    value = re.sub(r"[（(][^）)]*[）)]", "", str(value or ""))
    return sorted(set(token.strip() for token in re.split(r"[、,，;；\s]+", value) if token.strip()))


def normalized(value):
    return " ".join(unicodedata.normalize("NFKC", str(value or "")).split())


def source_names(source):
    if source.get("names_text"):
        return names(source["names_text"])
    group = source.get("group", "")
    return [] if group in ("", "员工", "生产职员", "职员") else names(group)


def explicit_windows(value, path="config"):
    found = []
    if isinstance(value, dict):
        for key, child in value.items():
            here = path + "." + key
            if key == "local_windows" and isinstance(child, dict) and any(v is not None for v in child.values()):
                found.append({"path": here, "value": child})
            elif key == "window" and isinstance(child, dict) and any(isinstance(v, (int, float)) and v != 0 for v in child.values()):
                found.append({"path": here, "value": child})
            found.extend(explicit_windows(child, here))
    elif isinstance(value, list):
        for i, child in enumerate(value):
            found.extend(explicit_windows(child, path + "[" + str(i) + "]"))
    return found


def active_directory(snapshot):
    rules = (snapshot.get("exclusions") or [{}])[0]
    excluded_ids = set(rules.get("exclude_department_ids") or [])
    excluded_names = set(rules.get("exclude_department_names") or [])
    parents = {r["wecom_id"]: r["parent_id"] for r in snapshot["wecom_department"]}
    dept_names = {r["wecom_id"]: r["name"].strip() for r in snapshot["wecom_department"]}
    descend = rules.get("include_subdepartments") is not False

    def excluded_department(dept):
        seen = set()
        while dept and dept not in seen:
            if dept in excluded_ids or dept_names.get(dept) in excluded_names:
                return True
            seen.add(dept)
            dept = parents.get(dept) if descend else None
        return False

    excluded_users = set(rules.get("exclude_userids") or [])
    excluded_user_names = set(rules.get("exclude_user_names") or [])
    for user in snapshot["wecom_user"]:
        if user["name"].strip() in excluded_user_names or any(excluded_department(i) for i in json.loads(user["department_ids"] or "[]")):
            excluded_users.add(user["wecom_userid"])
    departments = {r["id"]: r for r in snapshot["department"]}
    result = {}
    for employee in snapshot["employee"]:
        if employee["status"] != "active" or employee["wecom_userid"] in excluded_users or employee["name"].strip() in excluded_user_names:
            continue
        row = {k: employee.get(k) for k in ("id", "name", "employee_no", "dept_id", "status")}
        row["department"] = departments.get(row["dept_id"], {}).get("name", "")
        result[row["id"]] = row
    return result, departments


def match_named_people(wanted, department, active, departments, overrides=None):
    matched, issues = [], []
    targets = {i for i, dept in departments.items() if dept["name"].strip() == department.strip()}

    def in_department(dept):
        seen = set()
        while dept and dept not in seen:
            if dept in targets:
                return True
            seen.add(dept)
            dept = departments.get(dept, {}).get("pid")
        return False

    for name in wanted:
        override = (overrides or {}).get(name)
        if override is not None:
            candidate = active.get(override.get("employee_id"))
            if (not candidate or candidate["name"] != override.get("current_name") or
                    str(candidate.get("employee_no") or "") != override.get("employee_no") or
                    candidate["department"] != override.get("department") or not in_department(candidate["dept_id"])):
                raise ValueError("Confirmed employee override no longer matches the active name/number/department: " + name)
            matched.append(candidate)
            continue
        candidates = [p for p in active.values() if p["name"].strip() == name and in_department(p["dept_id"])]
        if len(candidates) != 1 or not str(candidates[0].get("employee_no") or "").strip():
            issues.append({"code": "employee_identity_unconfirmed", "name": name, "expected_department": department,
                           "reason": "没有唯一的在职人员及工号匹配", "candidates": candidates})
        else:
            matched.append(candidates[0])
    return matched, issues


def prepare(snapshot_path, presets_path, output_path, identity_overrides_path=None):
    snapshot = json.loads(Path(snapshot_path).read_text(encoding="utf-8-sig"))
    bundle = json.loads(Path(presets_path).read_text(encoding="utf-8-sig"))
    if snapshot["rules_sha256"] != sha(snapshot["rule"]):
        raise RuntimeError("Snapshot rule hash does not match its contents")
    presets = bundle["presets"]
    override_document = json.loads(Path(identity_overrides_path).read_text(encoding="utf-8-sig")) if identity_overrides_path else {}
    overrides = {}
    for override in override_document.get("overrides", []):
        key = override.get("source_name")
        if not key or key in overrides or not override.get("confirmation") or override.get("confirmed_by") != "user":
            raise ValueError("Identity overrides require unique source name and explicit user confirmation")
        overrides[key] = override
    active, departments = active_directory(snapshot)
    plan = {"mode": "prepare_only", "database_writes": False, "source_sha256": bundle["source_sha256"],
            "snapshot_sha256": snapshot["snapshot_sha256"], "rules_before_sha256": snapshot["rules_sha256"],
            "active_plugin": snapshot["active_plugin"], "identity_overrides": override_document, "actions": [], "preserved": [], "unmatched": [], "unmatched_presets": [],
            "invariants": ["Only explicitly matched imported production drafts may change", "Never publish, recalculate, create employees or restore filtered records", "New draft version in primary family; prior draft rows keep full source/config and are archived with audit", "Published and ordinary staff rules remain byte-for-byte unchanged", "Unconfirmed identities remain unassigned and block publication", "Existing non-empty scope is preserved; manual windows require separate review"]}
    matched = []
    family_versions = defaultdict(int)
    for rule in snapshot["rule"]:
        family_versions[rule["family_id"]] = max(family_versions[rule["family_id"]], rule["version"])
        config, provenance = json.loads(rule["config_json"]), json.loads(rule["source_json"])
        source = provenance.get("source")
        reason = None
        if rule["status"] != "draft":
            reason = "non_draft_preserved"
        elif provenance.get("snapshot_id"):
            reason = "wecom_reference_preserved"
        elif config.get("category") not in ("员工", "生产职员"):
            reason = "ordinary_staff_preserved"
        elif not isinstance(source, dict):
            reason = "not_an_imported_source"
        elif config.get("work_schedule"):
            reason = "already_has_work_schedule"
        if reason:
            plan["preserved"].append({"id": rule["id"], "name": rule["name"], "reason": reason, "row_sha256": sha(rule)})
            continue
        old_names = source_names(source)
        choices = []
        for i, preset in enumerate(presets):
            if preset["config"]["category"] != config["category"]:
                continue
            for item in preset["sources"]:
                new_names = source_names(item)
                if normalized(source["department"]) != normalized(item["department"]):
                    continue
                if old_names and not set(old_names).issubset(new_names) or not old_names and new_names:
                    continue
                choices.append(i)
                break
        if len(choices) != 1:
            plan["unmatched"].append({"id": rule["id"], "name": rule["name"], "reason": "source_not_uniquely_mapped", "source_department": source["department"], "source_names": old_names, "candidate_source_keys": [presets[i]["source"]["source_key"] for i in choices], "before": rule})
            continue
        matched.append((choices[0], rule, old_names))
    groups = defaultdict(list)
    for index, rule, wanted in matched:
        mode = presets[index]["config"]["work_schedule"]["mode"]
        # Keep familiar individual production-staff rules. Shared departmental
        # day/night source rows and the three-person duty group are merged.
        key = (index, "merge") if mode == "after_sales" or not wanted else (index, rule["id"])
        groups[key].append((rule, wanted))
    consumed = set()
    for (index, _), predecessors in sorted(groups.items(), key=lambda item: item[1][0][0]["id"]):
        preset = presets[index]
        consumed.add(index)
        before = sorted([r for r, _ in predecessors], key=lambda r: (r["created_at"], r["id"]))
        old_ids = [r["id"] for r in before]
        primary = before[0]
        mode = preset["config"]["work_schedule"]["mode"]
        wanted = source_names(preset["source"]) if mode == "after_sales" else sorted(set(n for _, listed in predecessors for n in listed))
        department = preset["source"].get("directory_department") or preset["source"]["department"]
        used_overrides = [overrides[name] for name in wanted if name in overrides]
        people, identity_issues = match_named_people(wanted, department, active, departments, overrides)
        if len({p["id"] for p in people}) != len(people):
            raise ValueError("Distinct source names must not resolve to the same employee")
        scopes = [json.loads(row["scope_json"]) for row in before]
        nonempty_scopes = [s for s in scopes if s.get("employee_ids") or s.get("department_ids") or s.get("is_default")]
        blockers, pending = [], []
        if nonempty_scopes:
            if len({stable(s) for s in nonempty_scopes}) != 1:
                blockers.append("旧草稿的非空适用范围不一致，不能自动合并")
            scope = copy.deepcopy(nonempty_scopes[0])
            if any(i not in active for i in scope.get("employee_ids") or []):
                blockers.append("旧范围包含当前无效人员，不能恢复或静默删除")
        else:
            # Ambiguous groups are wholly unassigned, never partially enabled.
            scope = {"employee_ids": [] if identity_issues else [p["id"] for p in people], "department_ids": [], "include_children": False, "is_default": False}
        if not wanted and not nonempty_scopes:
            pending.append("原草稿未选择适用人员；请按员工分类核对具体人员，不能直接扩为整个部门")
        for issue in identity_issues:
            pending.append(issue["name"] + "：当前在职人员、工号和部门不能唯一对应，需确认后选择完整人员")
        windows = [{"rule_id": row["id"], "windows": explicit_windows(json.loads(row["config_json"]))} for row in before]
        if any(item["windows"] for item in windows):
            blockers.append("已有非空人工取卡窗口，须逐段核对迁移；本计划不会覆盖")
        audits = [a for a in snapshot["rule_audit"] if a["entity_id"] in old_ids and a["action"] in ("save", "update", "edit")]
        if audits:
            blockers.append("草稿存在人工保存审计，须审核其修改后再生成替代版本")
        new = {"id": "prod-week-" + sha([bundle["source_sha256"], old_ids, preset["config"]])[:40], "family_id": primary["family_id"],
               "version": family_versions[primary["family_id"]] + 1, "name": primary["name"], "status": "draft",
               "effective_from": primary["effective_from"] or "2026-10-01", "effective_to": primary["effective_to"],
               "config": copy.deepcopy(preset["config"]), "scope": scope,
               "conflicts": list(dict.fromkeys(preset["conflicts"] + pending)),
               "source": {"source": preset["source"], "sources": preset["sources"], "work_schedule_rebuild": {"source_filename": bundle["source_filename"], "source_sha256": bundle["source_sha256"], "previous_rule_ids": old_ids, "previous_sources": [json.loads(r["source_json"]) for r in before], "identity_issues": identity_issues, "confirmed_identity_overrides": used_overrides}}}
        if mode == "after_sales":
            new["name"] = "售后部（三人留守）"
            # Fixed declaration from the source list, never from resolved scope.
            new["config"]["work_schedule"]["duty_expected_members"] = len(source_names(preset["source"]))
        elif len(before) > 1:
            new["name"] = " / ".join(dict.fromkeys(r["name"].strip() for r in before))
        new["config"]["rule"]["id"] = new["id"]
        new["config"]["rule"]["version"] = str(new["version"])
        old_dates = {(row["effective_from"], row["effective_to"]) for row in before}
        if len(old_dates) > 1:
            blockers.append("被合并草稿的有效日期不一致，请人工确认")
        plan["actions"].append({"kind": "merge_drafts" if len(before) > 1 else "new_draft_version", "ready_to_apply_after_review": not blockers,
                                "ready_to_publish": False, "blocked_reasons": blockers, "pending_configuration": pending + ["缺少的本地秒级打卡窗口保持待配置，不自动填入默认值"],
                                "source_key": preset["source"]["source_key"], "before": before, "before_hashes": {r["id"]: sha(r) for r in before},
                                "archive_ids": old_ids, "new_rule": new, "resolved_people": people, "identity_issues": identity_issues, "confirmed_identity_overrides": used_overrides,
                                "old_manual_windows": windows, "old_human_audits": audits,
                                "audit_requirement": "事务中写入完整before/after、来源工作簿hash和明确修订原因；不修改归档行的config/source"})
    for i, preset in enumerate(presets):
        if i not in consumed:
            plan["unmatched_presets"].append({"source_key": preset["source"]["source_key"], "department": preset["source"]["department"], "names": source_names(preset["source"]), "reason": "没有分类与来源均匹配的现有生产草稿；不修改普通职员或自动新建制度"})
    claimed = [rid for action in plan["actions"] for rid in action["archive_ids"]]
    assert len(claimed) == len(set(claimed)), "A prior rule was assigned to multiple replacements"
    assert all(r["status"] == "draft" for action in plan["actions"] for r in action["before"])
    assert all(i in active for action in plan["actions"] for i in action["new_rule"]["scope"].get("employee_ids") or [])
    assert all(action["new_rule"]["status"] == "draft" for action in plan["actions"])
    plan["summary"] = {"rules_before": len(snapshot["rule"]), "active_people_after_exclusions": len(active), "presets": len(presets),
                       "new_drafts": len(plan["actions"]), "old_drafts_to_archive": len(claimed), "merges": sum(a["kind"] == "merge_drafts" for a in plan["actions"]),
                       "ready_after_review": sum(a["ready_to_apply_after_review"] for a in plan["actions"]), "blocked_actions": sum(not a["ready_to_apply_after_review"] for a in plan["actions"]),
                       "identity_pending_actions": sum(bool(a["identity_issues"]) for a in plan["actions"]), "explicitly_bound_people": len(set(i for a in plan["actions"] for i in a["new_rule"]["scope"].get("employee_ids") or [])),
                       "preserved": len(plan["preserved"]), "unmatched_old_drafts": len(plan["unmatched"]), "unmatched_presets": len(plan["unmatched_presets"]), "published_or_calculated": 0}
    plan["plan_sha256"] = sha(plan)
    Path(output_path).write_text(json.dumps(plan, ensure_ascii=False, indent=2), encoding="utf-8")
    print(json.dumps({"plan": str(output_path), "plan_sha256": plan["plan_sha256"], "summary": plan["summary"]}, ensure_ascii=False))


def main():
    sys.stdout.reconfigure(encoding="utf-8")
    parser = argparse.ArgumentParser()
    parser.add_argument("--remote-snapshot")
    parser.add_argument("--snapshot")
    parser.add_argument("--presets")
    parser.add_argument("--output")
    parser.add_argument("--identity-overrides")
    arguments = parser.parse_args()
    if arguments.remote_snapshot:
        remote_snapshot(arguments.remote_snapshot)
    elif arguments.snapshot and arguments.presets and arguments.output:
        prepare(arguments.snapshot, arguments.presets, arguments.output, arguments.identity_overrides)
    else:
        parser.error("Specify --remote-snapshot; this tool has no apply mode")


if __name__ == "__main__":
    main()
