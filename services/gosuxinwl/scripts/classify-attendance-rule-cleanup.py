"""Build an offline review list from the read-only public reference graph."""
from collections import Counter
import hashlib
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent


def sha(value):
    return hashlib.sha256(json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode()).hexdigest()


def main():
    audit = json.loads((ROOT / "runtime/attendance-rule-cleanup-public-audit.json").read_text(encoding="utf-8"))
    plan = json.loads((ROOT / "runtime/production-work-schedule-plan-confirmed.json").read_text(encoding="utf-8"))
    presets = json.loads((ROOT / "runtime/production-work-schedule-presets-final.json").read_text(encoding="utf-8"))
    current = {a["new_rule"]["id"] for a in plan["actions"]}
    replaced = {rid for a in plan["actions"] for rid in a["archive_ids"]}
    unmatched = {a["id"] for a in plan["unmatched"]}
    moved = {name: p["source"] for p in presets["presets"] for name in ("谭志邦", "黄安彬") if name == p["source"].get("names_text")}
    rows = []
    for rule in audit["rules"]:
        source, config = json.loads(rule["source_json"]), json.loads(rule["config_json"])
        references = audit["references"].get(rule["id"], [])
        operational = [r for r in references if r["table"] not in ("biz_att_rule", "biz_att_audit", "biz_att_import")]
        source_row = source.get("source") if isinstance(source.get("source"), dict) else {}
        moved_name = next((n for n in moved if n in rule["name"]), None)
        if rule["id"] in current:
            category, reason = "keep_current_production_draft", "刚重建的本地生产制度；保留并补齐窗口/范围，不因仍是草稿而清除"
        elif rule["id"] in replaced:
            category, reason = "keep_archived_history", "已被新版替代；仅从日常入口隐藏，保留新制度来源链和完整审计，不作为完全无引用数据删除"
        elif rule["status"] == "published":
            category, reason = "keep_published_until_valid_replacement", "唯一已发布制度；存在原始保存/发布审计，新一部草稿范围未设不能当作等价替代；七天全空周期另列业务缺陷"
        elif source.get("snapshot_id"):
            category, reason = "archive_obsolete_wecom_reference", "已停止使用企业微信考勤规则；未绑定范围且无业务引用，可归档退出制度候选，原始快照及导入审计保留"
        elif rule["id"] in unmatched:
            category, reason = "archive_obsolete_unmatched_import", "旧员工工作表泛部门行在最新生产预设中不存在；未绑定范围、无业务引用，可归档，不按名称套用生产职员个人制度"
        elif moved_name:
            category, reason = "replace_reclassified_draft_before_removal", "新版工作簿已将该人移入生产职员，当前只有旧职员草稿；应先建立对应新版本再归档，不能仅因分类过时直接清空"
        else:
            category, reason = "keep_distinct_ordinary_staff_draft", "不同部门/人员及免卡、单休、大小周、客服等独立适用制度，当前无替代新版；不是重复生产规则"
        rows.append({"id": rule["id"], "name": rule["name"], "status": rule["status"], "category": category, "reason": reason, "row_sha256": sha(rule), "personnel_category": config.get("category"), "scope": json.loads(rule["scope_json"]), "source_key": source_row.get("source_key"), "replacement_source_key": moved[moved_name].get("source_key") if moved_name and category == "replace_reclassified_draft_before_removal" else None, "operational_references": operational, "source_or_audit_references": [r for r in references if r not in operational]})
    counts = dict(Counter(r["category"] for r in rows))
    archive = [r for r in rows if r["category"].startswith("archive_obsolete_")]
    assert all(not r["operational_references"] and r["status"] == "draft" and not r["scope"].get("employee_ids") and not r["scope"].get("department_ids") and not r["scope"].get("is_default") for r in archive)
    report = {"read_only": True, "writes_performed": False, "rules_sha256": audit["rules_sha256"], "reference_audit_sha256": audit["audit_sha256"], "counts": counts, "recommended_new_archives": len(archive), "hard_delete_recommended": 0, "would_remain_in_current_list": len(rows) - len(replaced) - len(archive), "direct_business_reference_count": sum(len(r["operational_references"]) for r in rows), "table_counts": audit["table_counts"], "result_summary": audit.get("result_summary"), "notes": ["104=22新生产草稿+27历史归档+22企微参考+29普通职员草稿+3旧员工泛部门草稿+1已发布", "27旧归档每份都有新版source引用及两条迁移审计；无排班/结果引用不等于完全无引用", "29普通职员中谭志邦/黄安彬需依据最新生产职员行另建替代版本，其余27份保留", "同上班时间、同部门但员工分类或人员范围不同，不作为删除重复的依据", "仅建议新增归档25份；历史记录/快照/核算结果/审计不删除"], "rules": rows}
    report["review_sha256"] = sha(report)
    target = ROOT / "runtime/attendance-rule-cleanup-public-review.json"
    target.write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding="utf-8")
    print(json.dumps({k: report[k] for k in ("counts", "recommended_new_archives", "would_remain_in_current_list", "hard_delete_recommended", "review_sha256")}))


if __name__ == "__main__":
    main()
