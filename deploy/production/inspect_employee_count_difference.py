#!/usr/bin/env python3
"""Read one reviewed real-employee count difference; never modify business data."""
import argparse
import datetime
import hashlib
import importlib.util
import json
import os
import pathlib
import re
import secrets
import stat
import subprocess
import sys

BASE = pathlib.Path('/srv/puty-suite')
PROJECT = 'puty-suite-prod'
INSTANCE = '5ac7c644d9cd4923e2b701f66b52485d'
HELPER_SHA256 = '20c11459c67a9ece3405fac3568e0550a9d06af5ea88dc4c7097c98dfee9870c'


def require(ok, message):
    if not ok:
        raise RuntimeError(message)


def canonical(value):
    return json.dumps(value, sort_keys=True, separators=(',', ':'), ensure_ascii=False).encode()


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def regular(path):
    require(path.resolve() == path and path.is_file() and not path.is_symlink()
            and stat.S_ISREG(path.stat().st_mode) and path.stat().st_nlink == 1,
            'Canonical regular proof/input required')
    return path


def validate_input(value):
    require(value.get('format') == 'puty-organization-count-probe-v1' and value.get('instanceID') == INSTANCE,
            'Exact reviewed probe owner required')
    original, current = value.get('baselineEmployee'), value.get('capturedProjection')
    require(isinstance(original, dict) and isinstance(current, dict)
            and re.fullmatch(r'[A-Za-z0-9_-]{1,64}', original.get('id', ''))
            and original.get('status') == 'active' and int(original.get('user_id') or 0) == 0
            and not str(original.get('employee_no') or '').strip()
            and isinstance(original.get('wecom_userid'), str) and 0 < len(original['wecom_userid']) <= 256
            and isinstance(original.get('name'), str) and original['name'],
            'One exact previously unprovisioned baseline identity required')
    require(current.get('id') == original['id'] and current.get('status') == 'waiting'
            and current.get('enabled') is False and current.get('hostUID') == 0
            and current.get('appliedVersion') == 0 and type(current.get('desiredVersion')) is int
            and current['desiredVersion'] > 0 and current.get('userID') ==
            'emp_' + sha(original['id'].encode())[:32], 'Exact disabled captured projection required')
    require(all(isinstance(value.get(key), str) and re.fullmatch('[a-f0-9]{64}', value[key])
                for key in ('baselineResponseSHA256', 'captureResponseSHA256', 'reasonSHA256')),
            'Baseline/capture evidence digests required')
    return original, current


def exclusion_proof(employee, member, departments, rules):
    require(isinstance(rules, dict), 'Explicit exclusion configuration unavailable')
    arrays = {}
    for key in ('exclude_department_ids', 'exclude_department_names', 'exclude_userids', 'exclude_user_names'):
        arrays[key] = rules.get(key) or []
        require(isinstance(arrays[key], list), 'Invalid explicit exclusion rule shape')
    recursive = rules.get('include_subdepartments')
    recursive = True if recursive is None else recursive
    require(type(recursive) is bool, 'Invalid explicit exclusion recursion flag')
    userid = employee.get('wecom_userid') or ''
    name = str((member or {}).get('name') if member else employee.get('name') or '').strip()
    by_id = {int(d['id']): d for d in departments}
    hits = {'explicitUserID': userid in arrays['exclude_userids'],
            'explicitUserName': bool(name and name != userid and name in arrays['exclude_user_names']),
            'explicitDepartmentID': False, 'explicitDepartmentName': False}
    ancestry = []
    if member:
        try:
            ids = json.loads(member.get('department_ids') or '[]')
            if not isinstance(ids, list):
                ids = []
        except (TypeError, ValueError):
            ids = []
        for identity in ids:
            seen = set()
            while int(identity) > 0 and int(identity) not in seen:
                identity = int(identity)
                seen.add(identity)
                row = by_id.get(identity, {'id': identity, 'pid': 0, 'name': ''})
                ancestry.append(row)
                hits['explicitDepartmentID'] |= identity in arrays['exclude_department_ids']
                hits['explicitDepartmentName'] |= bool(str(row.get('name') or '').strip()
                                                       and str(row['name']).strip() in arrays['exclude_department_names'])
                if not recursive:
                    break
                identity = int(row.get('pid') or 0)
    return {'excludedByExplicitRules': any(hits.values()), 'ruleHits': hits,
            'includeSubdepartments': recursive, 'targetDepartmentAncestry': ancestry,
            'restrictedRuleSHA256': sha(canonical(rules))}


def collect(db, helpers, value):
    original, captured = validate_input(value)
    literal = helpers.literal
    employee_id, userid = literal(original['id']), literal(original['wecom_userid'])
    # This transaction is explicitly READ ONLY. No FOR UPDATE, repair or DML.
    db.query('SET TRANSACTION ISOLATION LEVEL REPEATABLE READ; START TRANSACTION READ ONLY')
    employees = db.json("SELECT COALESCE(JSON_ARRAYAGG(JSON_OBJECT('id',id,'user_id',user_id,"
        "'wecom_userid',wecom_userid,'employee_no',employee_no,'name',name,'status',status)),JSON_ARRAY()) "
        "FROM biz_hrm_employee WHERE id=" + employee_id)
    require(len(employees) <= 1, 'Unique exact employee source required')
    projections = db.json("SELECT COALESCE(JSON_ARRAYAGG(JSON_OBJECT('employeeID',employee_id,'nativeUserID',openim_user_id,"
        "'uid',uid,'enabled',enabled,'status',status,'provisioned',provisioned,'desiredVersion',desired_version,'appliedVersion',applied_version,"
        "'projection',JSON_OBJECT('userID',JSON_UNQUOTE(JSON_EXTRACT(projection_json,'$.userID')),"
        "'employeeID',JSON_UNQUOTE(JSON_EXTRACT(projection_json,'$.employeeID')),"
        "'hostUID',JSON_EXTRACT(projection_json,'$.hostUID'),'enabled',JSON_EXTRACT(projection_json,'$.enabled'),"
        "'reason',JSON_UNQUOTE(JSON_EXTRACT(projection_json,'$.reason'))))),JSON_ARRAY()) "
        "FROM biz_openim_employee WHERE employee_id=" + employee_id)
    require(len(projections) == 1, 'Exactly one retained projection required')
    p = projections[0]
    nested = p['projection']
    projection_bound = (p['employeeID'] == original['id'] and p['nativeUserID'] == captured['userID']
        and int(p.get('uid') or 0) == 0 and p['enabled'] == 0 and p['status'] == 'waiting'
        and p['appliedVersion'] == 0 and p.get('provisioned') == 0
        and p['desiredVersion'] == captured['desiredVersion'] and nested.get('employeeID') == original['id']
        and nested.get('userID') == captured['userID'] and int(nested.get('hostUID') or 0) == 0
        and nested.get('enabled') is False and sha(str(nested.get('reason') or '').encode()) == value['reasonSHA256'])
    require(projection_bound, 'Retained projection no longer matches the reviewed disabled identity')
    members = db.json("SELECT COALESCE(JSON_ARRAYAGG(JSON_OBJECT('wecom_userid',wecom_userid,'name',name,"
                      "'department_ids',department_ids)),JSON_ARRAY()) FROM wecom_user WHERE wecom_userid=" + userid)
    require(len(members) <= 1, 'Unique baseline WeCom mapping required')
    departments = db.json("SELECT COALESCE(JSON_ARRAYAGG(JSON_OBJECT('id',wecom_id,'pid',parent_id,'name',name)),JSON_ARRAY()) FROM wecom_department")
    rule_rows = db.json("SELECT COALESCE(JSON_ARRAYAGG(JSON_OBJECT("
        "'exclude_department_ids',JSON_EXTRACT(settings_json,'$.exclude_department_ids'),"
        "'exclude_department_names',JSON_EXTRACT(settings_json,'$.exclude_department_names'),"
        "'exclude_userids',JSON_EXTRACT(settings_json,'$.exclude_userids'),"
        "'exclude_user_names',JSON_EXTRACT(settings_json,'$.exclude_user_names'),"
        "'include_subdepartments',JSON_EXTRACT(settings_json,'$.include_subdepartments'))),JSON_ARRAY()) "
        "FROM wecom_settings WHERE id=1")
    require(len(rule_rows) <= 1, 'Unique explicit exclusion configuration required')
    current = employees[0] if employees else None
    mapping = bool(current and current['id'] == original['id']
        and current.get('wecom_userid') == original['wecom_userid']
        and int(current.get('user_id') or 0) == int(original.get('user_id') or 0)
        and str(current.get('employee_no') or '') == str(original.get('employee_no') or ''))
    proof = exclusion_proof(current or original, members[0] if members else None, departments,
                            rule_rows[0] if rule_rows else {})
    status = current.get('status') if current else 'absent'
    require(status in ('active', 'inactive', 'resigned', 'filtered', 'absent'), 'Unknown source status category')
    hidden = bool(current and (status == 'filtered' or proof['excludedByExplicitRules']))
    safe = {'employeeRowCount': len(employees), 'employeeStatusCategory': status,
        'baselineWeComUIDAndNoHostUIDAndEmployeeNumberMatch': mapping,
        'employeeNameMatchesBaseline': bool(current and current.get('name') == original['name']),
        'retainedProjectionCount': 1, 'retainedProjectionStrictlyBoundDisabledAndNeverApplied': projection_bound,
        'rawWeComMappingCount': len(members), 'excludedByStatusFiltered': status == 'filtered',
        'excludedByExplicitRules': proof['excludedByExplicitRules'], 'explicitRuleHits': proof['ruleHits'],
        'excludedFromCurrentHRMViewByExactSourceRules': hidden,
        'sourcePhysicallyAbsent': not employees,
        'historicalClassificationProven': (not employees or mapping and hidden),
        'businessDataModified': False}
    db.query('ROLLBACK')
    return safe, {'baselineEmployee': original, 'currentEmployee': current, 'retainedProjection': p,
                  'targetWeComMember': members[0] if members else None, 'explicitExclusionProof': proof}


def main():
    import fcntl
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--input', type=pathlib.Path, required=True)
    parser.add_argument('--input-sha256', required=True)
    parser.add_argument('--database-helper', type=pathlib.Path, default=BASE / 'staging/fixtures_cleanup.py')
    args = parser.parse_args()
    require(os.name == 'posix' and os.geteuid() == 0 and BASE.resolve() == BASE, 'Owned Linux root operation required')
    stage = BASE / '.backup-stage'
    require(stage.is_dir() and stage.resolve() == stage and not stage.is_symlink(), 'Owned private evidence stage required')
    require(args.input.is_relative_to(stage) and sha(regular(args.input).read_bytes()) == args.input_sha256,
            'Exact reviewed private input required')
    value = json.loads(args.input.read_text())
    validate_input(value)
    marker = json.loads(regular(BASE / 'openim/instance.json').read_text())
    activation = json.loads(regular(BASE / 'activation.json').read_text())
    require(marker.get('instanceID') == activation.get('instanceID') == INSTANCE
            and marker.get('production') is True and marker.get('testOnly') is False
            and marker.get('root') == str(BASE / 'openim') and marker.get('stackName') == PROJECT, 'Owner mismatch')
    helper = args.database_helper
    require(helper.is_relative_to(BASE / 'staging') and sha(regular(helper).read_bytes()) == HELPER_SHA256,
            'Frozen safe database helper required')
    spec = importlib.util.spec_from_file_location('safe_count_probe_database', helper)
    helpers = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(helpers)
    lock = regular(stage / 'backup.lock').open('rb')
    with lock:
        fcntl.flock(lock, fcntl.LOCK_SH | fcntl.LOCK_NB)
        ids = subprocess.check_output(['docker', 'ps', '-q', '--filter', 'label=com.docker.compose.project=' + PROJECT,
            '--filter', 'label=com.docker.compose.service=mysql'], timeout=20).decode().split()
        require(len(ids) == 1, 'Exactly one owned MySQL container required')
        labels = json.loads(subprocess.check_output(['docker', 'inspect', '--format', '{{json .Config.Labels}}', ids[0]], timeout=20))
        require(labels.get('com.puty.suite') == INSTANCE and labels.get('com.docker.compose.project') == PROJECT
                and labels.get('com.docker.compose.service') == 'mysql', 'Foreign MySQL container refused')
        db = helpers.Database(ids[0])
        try:
            safe, detail = collect(db, helpers, value)
        finally:
            db.close()
        report = {'format': 'puty-organization-count-proof-v1', 'instanceID': INSTANCE,
            'capturedAtUTC': datetime.datetime.now(datetime.timezone.utc).isoformat(),
            'inputSHA256': args.input_sha256, 'baselineResponseSHA256': value['baselineResponseSHA256'],
            'captureResponseSHA256': value['captureResponseSHA256'], 'safe': safe, 'privateEvidence': detail}
        target = stage / ('organization-count-proof-' + secrets.token_hex(8) + '.private.json')
        descriptor = os.open(target, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        with os.fdopen(descriptor, 'wb') as output:
            output.write(canonical(report) + b'\n')
            output.flush()
            os.fsync(output.fileno())
        print(json.dumps({'readOnly': True, 'snapshot': str(target), 'snapshotSHA256': sha(target.read_bytes()), **safe}))


if __name__ == '__main__':
    try:
        main()
    except Exception as error:
        print(json.dumps({'probeFailed': True, 'category': type(error).__name__,
                          'message': str(error) if type(error) is RuntimeError else 'private_details_suppressed'}))
        sys.exit(1)
