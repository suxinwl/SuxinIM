#!/usr/bin/env python3
"""Root-operated final SQL phase for two released synthetic HRM employees.

No SSH, no credential arguments, no Redis/Mongo/Minio mutation. Inspect first.
All MySQL checks and the two-row delete share one serializable transaction.
"""
import argparse
import contextlib
import datetime
import hashlib
import json
import os
import pathlib
import re
import secrets
import subprocess
import tempfile

BASE = pathlib.Path('/srv/puty-suite')
PROJECT = 'puty-suite-prod'
SCHEMA = 'gosuxin_prod'
UID_COLUMNS = {'uid', 'user_id', 'account_id', 'created_by', 'updated_by', 'actor_uid', 'host_id', 'leader_uid'}
ATTACHMENT_IDS = {308: '4e13456024a5c8e5c5afc265e1d3742432302d9b556477a7ab8ace10ef173610',
                  309: '8522e8f8383f6896d073918ab8980bde58c2ec43d0a626ee5485c4f2fe929b3d'}


def require(value, message):
    if not value:
        raise RuntimeError(message)


def canonical(value):
    return json.dumps(value, sort_keys=True, separators=(',', ':'), ensure_ascii=False).encode()


def identifier(value):
    require(isinstance(value, str) and re.fullmatch('[A-Za-z0-9_]+', value), 'Unsafe SQL identifier')
    return '`' + value + '`'


def literal(value):
    # Explicit binary collation also makes the original fixture identity checks
    # case-exact across the host and plugin tables' different default collations.
    return "CONVERT(X'" + str(value).encode().hex() + "' USING utf8mb4) COLLATE utf8mb4_bin"


def retained(table):
    # These exact authentication rows belong to the retained soft-deleted
    # account. They are revocation/security state, never employee business data.
    security = {'gf_account_security', 'gf_account_failure', 'gf_account_change_ticket'}
    return table in security or table.startswith('biz_openim_') or table == 'gf_auth_role_access' or 'audit' in table or table.endswith('_log')


def validate_plan(plan, owner):
    require(plan.get('format') == 'puty-fixture-cleanup-v1' and plan.get('instanceID') == owner,
            'Cleanup plan ownership mismatch')
    fixtures = [row['expected'] for row in plan.get('fixtures', [])]
    require(len(fixtures) == 2 and len({int(a['uid']) for a in fixtures}) == 2
            and len({a['employeeID'] for a in fixtures}) == 2, 'Exactly two distinct fixtures required')
    for fixture in fixtures:
        require(int(fixture['uid']) > 1 and re.fullmatch('[A-Za-z0-9_-]{1,20}', fixture['username'])
                and 'qa' in fixture['username'].lower()
                and re.fullmatch('[A-Za-z0-9_-]{1,64}', fixture['employeeID']) and fixture['name'],
                'Original named synthetic fixtures required')
    if plan.get('namespace') is not None:
        namespace = plan.get('namespace')
        wanted = {'organization-20261103': {306: 'qa_261103_c', 307: 'qa_261103_d'},
                  'attachment-20261103': {308: 'qa_261103_e', 309: 'qa_261103_f'}}
        require(namespace in wanted and plan.get('departmentID') == 53
                and {int(a['uid']): a['username'] for a in fixtures} == wanted[namespace],
                'Exact fixture namespace/UID/account binding required')
        excluded = plan.get('excludedOriginalUIDs', [])
        excluded_employees = plan.get('excludedOriginalEmployeeIDs', [])
        old_count = 4 if namespace == 'attachment-20261103' else 2
        require(len(excluded) == len(set(excluded)) == old_count and not set(wanted[namespace]).intersection(excluded)
                and all(type(uid) is int and uid > 1 for uid in excluded)
                and len(excluded_employees) == len(set(excluded_employees)) == old_count
                and not {a['employeeID'] for a in fixtures}.intersection(excluded_employees),
                'Original fixtures must remain disjoint and excluded')
        for fixture in fixtures:
            require(fixture.get('departmentID') == 53 and isinstance(fixture.get('mobile'), str)
                    and isinstance(fixture.get('hostMobile'), str), 'Exact original fixture mobile/department binding required')
        if namespace == 'attachment-20261103':
            require({306, 307}.issubset(excluded), 'All four earlier identities must remain excluded')
            for fixture in fixtures:
                uid = int(fixture['uid'])
                suffix = 'E' if uid == 308 else 'F'
                require(fixture['name'] == 'IM attachment acceptance ' + suffix
                        and fixture['mobile'] == fixture['hostMobile'] == ('00000000005' if uid == 308 else '00000000006')
                        and fixture.get('hostDepartmentID') == 53
                        and hashlib.sha256(fixture['employeeID'].encode()).hexdigest() == ATTACHMENT_IDS[uid],
                        'Exact original E/F employee ID, mobile, name and host department required')
    return fixtures


def employee_predicate(fixtures):
    terms = []
    for fixture in fixtures:
        term = ('(e.id=' + literal(fixture['employeeID']) + ' AND e.user_id=' + str(int(fixture['uid']))
                     + ' AND e.employee_no=' + literal(fixture['username']) + ' AND e.name=' + literal(fixture['name'])
                     + ' AND a.username=' + literal(fixture['username']) + ' AND a.name=' + literal(fixture['name']))
        if 'departmentID' in fixture:
            term += ' AND e.dept_id=' + str(int(fixture['departmentID']))
        if 'hostDepartmentID' in fixture:
            term += ' AND a.dept_id=' + str(int(fixture['hostDepartmentID']))
        if 'mobile' in fixture:
            term += " AND COALESCE(e.mobile,'')=" + literal(fixture['mobile'])
        if 'hostMobile' in fixture:
            term += " AND COALESCE(a.mobile,'')=" + literal(fixture['hostMobile'])
        terms.append(term + ')')
    return '(' + ' OR '.join(terms) + ") AND COALESCE(e.wecom_userid,'')=''"


def check_attachment_department(db, plan):
    if plan.get('namespace') != 'attachment-20261103':
        return
    require(int(db.one('SELECT COUNT(*) FROM gf_auth_dept WHERE id=53 AND pid=0 AND business_id=1 AND status=0 '
                       'AND deletetime IS NULL AND im_group_enabled=0 AND leader_uid=0 AND name='
                       + literal('TEMP QA 20261103') + ' AND remark='
                       + literal('Temporary synthetic organization acceptance department'))) == 1,
            'Protected department changed; attachment cleanup cannot alter departments or groups')


class Database:
    def __init__(self, container):
        self.errors = tempfile.TemporaryFile()
        self.process = subprocess.Popen(['docker', 'exec', '-i', container, 'sh', '-ec',
            'export MYSQL_PWD="$MYSQL_ROOT_PASSWORD"; exec mysql -uroot --database=gosuxin_prod '
            '--batch --raw --skip-column-names --unbuffered --connect-timeout=10 --default-character-set=utf8mb4'],
            stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=self.errors)

    def query(self, sql):
        marker = 'puty_end_' + secrets.token_hex(16)
        self.process.stdin.write((sql.rstrip(';') + ";\nSELECT '" + marker + "';\n").encode())
        self.process.stdin.flush()
        lines = []
        while True:
            line = self.process.stdout.readline()
            if not line:
                self.errors.seek(0)
                diagnostic = self.errors.read(1024 * 1024)
                match = re.search(rb'ERROR\s+(\d+)\s+\(([0-9A-Z]{5})\)', diagnostic)
                safe = ('ERROR ' + match[1].decode() + ' SQLSTATE ' + match[2].decode()) if match else 'diagnostic unavailable'
                raise RuntimeError('MySQL operation failed: ' + safe)
            text = line.decode().rstrip('\r\n')
            if text == marker:
                return lines
            lines.append(text)

    def one(self, sql):
        values = self.query(sql)
        require(len(values) == 1, 'Unexpected SQL result shape')
        return values[0]

    def json(self, sql):
        return json.loads(self.one(sql))

    def close(self):
        with contextlib.suppress(Exception):
            self.query('ROLLBACK')
        with contextlib.suppress(Exception):
            self.process.stdin.close()
        try:
            self.process.wait(timeout=10)
        except subprocess.TimeoutExpired:
            self.process.kill()
            self.process.wait(timeout=10)
        self.errors.close()


def columns(db):
    return db.json("SELECT COALESCE(JSON_ARRAYAGG(JSON_OBJECT('table',TABLE_NAME,'column',COLUMN_NAME,'type',DATA_TYPE)),JSON_ARRAY()) "
                   "FROM information_schema.COLUMNS WHERE TABLE_SCHEMA='gosuxin_prod'")


def rows(db, table, where, schema):
    names = [c['column'] for c in schema if c['table'] == table and c['column'] not in {'password', 'salt'}]
    require(names, 'Required fixture table missing')
    fields = ','.join(literal(c) + ',' + identifier(c) for c in names)
    return db.json('SELECT COALESCE(JSON_ARRAYAGG(JSON_OBJECT(' + fields + ')),JSON_ARRAY()) FROM '
                   + identifier(table) + ' WHERE ' + where)


def references(db, fixtures, schema):
    employee_ids = ','.join(literal(a['employeeID']) for a in fixtures)
    uids = ','.join(str(int(a['uid'])) for a in fixtures)
    checks = []
    for column in schema:
        table, name = column['table'], column['column']
        if table == 'biz_hrm_employee' or retained(table):
            continue
        field = identifier(name)
        predicate = None
        if name == 'employee_id' or name.endswith('_id') and column['type'] in {'char', 'varchar'} and name not in UID_COLUMNS:
            predicate = field + ' IN (' + employee_ids + ')'
        elif name in UID_COLUMNS:
            predicate = field + ' IN (' + uids + ')'
        elif name == 'employee_ids' or name.endswith('_json') or column['type'] == 'json':
            searches = []
            for fixture in fixtures:
                pattern = fixture['employeeID'].replace('!', '!!').replace('_', '!_').replace('%', '!%')
                searches.append('JSON_SEARCH(' + field + ",'one'," + literal(pattern) + ",'!') IS NOT NULL")
            # Malformed nonempty declared JSON is conservatively a dependency;
            # never skip it while deciding whether a source employee can vanish.
            predicate = "(COALESCE(" + field + ",'')<>'' AND (NOT JSON_VALID(" + field + ') OR CASE WHEN JSON_VALID(' + field + ') THEN (' + ' OR '.join(searches) + ') ELSE FALSE END))'
        if predicate:
            count = int(db.one('SELECT COUNT(*) FROM ' + identifier(table) + ' WHERE ' + predicate))
            checks.append({'table': table, 'column': name, 'count': count})
    foreign = db.json("SELECT COALESCE(JSON_ARRAYAGG(JSON_OBJECT('table',TABLE_NAME,'column',COLUMN_NAME,'target',REFERENCED_COLUMN_NAME)),JSON_ARRAY()) "
                      "FROM information_schema.KEY_COLUMN_USAGE WHERE REFERENCED_TABLE_SCHEMA='gosuxin_prod' AND REFERENCED_TABLE_NAME='biz_hrm_employee'")
    for relation in foreign:
        target = relation['target']
        require(target in {'id', 'user_id', 'employee_no'}, 'Unknown employee foreign-key target')
        values = employee_ids if target == 'id' else uids if target == 'user_id' else ','.join(literal(a['username']) for a in fixtures)
        count = int(db.one('SELECT COUNT(*) FROM ' + identifier(relation['table']) + ' WHERE ' + identifier(relation['column']) + ' IN (' + values + ')'))
        checks.append({**relation, 'kind': 'foreign-key', 'count': count})
    return checks, foreign


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('command', choices=('inspect', 'delete-employees'))
    parser.add_argument('--plan', type=pathlib.Path, required=True)
    parser.add_argument('--plan-sha256', required=True)
    parser.add_argument('--fixtures-released', action='store_true')
    args = parser.parse_args()
    require(os.name == 'posix' and os.geteuid() == 0, 'Linux root operator required')
    import fcntl
    os.umask(0o077)
    require(BASE.resolve() == BASE and (BASE / 'openim').resolve() == BASE / 'openim', 'Owned runtime redirected')
    marker = BASE / 'openim/instance.json'
    require(marker.is_file() and not marker.is_symlink(), 'Regular owner marker required')
    owner = json.loads(marker.read_text())
    require(owner.get('root') == '/srv/puty-suite/openim' and owner.get('production') is True
            and owner.get('testOnly') is False and owner.get('stackName') == PROJECT
            and re.fullmatch('[a-f0-9]{32}', owner.get('instanceID', '')), 'Production ownership mismatch')
    stage = BASE / '.backup-stage'
    require(not stage.is_symlink(), 'Linked maintenance stage refused')
    stage.mkdir(mode=0o700, exist_ok=True)
    require(args.plan.is_file() and not args.plan.is_symlink() and args.plan.resolve().is_relative_to(stage.resolve()), 'Private owned staged plan required')
    raw = args.plan.read_bytes()
    require(hashlib.sha256(raw).hexdigest() == args.plan_sha256, 'Reviewed plan checksum mismatch')
    plan = json.loads(raw)
    fixtures = validate_plan(plan, owner['instanceID'])
    if args.command == 'delete-employees':
        require(args.fixtures_released, 'Fixture owner release required before employee deletion')
    lock = (stage / 'backup.lock').open('a+b')
    fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
    identifiers = subprocess.check_output(['docker', 'ps', '-q', '--filter', 'label=com.docker.compose.project=' + PROJECT,
                                         '--filter', 'label=com.docker.compose.service=mysql']).decode().split()
    require(len(identifiers) == 1, 'Exactly one production MySQL container required')
    container = json.loads(subprocess.check_output(['docker', 'inspect', identifiers[0]]))[0]
    require(container['Config']['Labels'].get('com.puty.suite') == owner['instanceID'], 'Foreign MySQL container refused')
    db = Database(identifiers[0])
    snapshot = {'format': plan['format'], 'planSHA256': args.plan_sha256, 'instanceID': owner['instanceID'],
                'createdAt': datetime.datetime.now(datetime.timezone.utc).isoformat(), 'command': args.command,
                'employeeRowsDeleted': 0, 'productionBusinessRowsTouched': False}
    try:
        db.query('SET SESSION innodb_lock_wait_timeout=10; SET TRANSACTION ISOLATION LEVEL SERIALIZABLE; START TRANSACTION')
        schema = columns(db)
        require(db.one("SELECT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA='gosuxin_prod' AND TABLE_NAME='biz_hrm_employee'") == 'InnoDB',
                'Transactional InnoDB employee table required')
        check_attachment_department(db, plan)
        ids = ','.join(literal(a['employeeID']) for a in fixtures)
        uids = ','.join(str(int(a['uid'])) for a in fixtures)
        db.query('SELECT id FROM biz_hrm_employee WHERE id IN (' + ids + ') FOR UPDATE')
        predicate = employee_predicate(fixtures)
        require(int(db.one('SELECT COUNT(*) FROM biz_hrm_employee e JOIN gf_admin a ON a.id=e.user_id WHERE ' + predicate)) == 2,
                'Exact original synthetic username/UID/employeeID/name/empty-WeCom bindings no longer match')
        snapshot['employees'] = rows(db, 'biz_hrm_employee', 'id IN (' + ids + ')', schema)
        snapshot['hostAccounts'] = rows(db, 'gf_admin', 'id IN (' + uids + ')', schema)
        snapshot['references'], snapshot['foreignKeys'] = references(db, fixtures, schema)
        snapshot['businessReferenceCount'] = sum(r['count'] for r in snapshot['references'])
        snapshot_prefix = plan.get('namespace', 'fixture')
        snapshot_path = stage / (snapshot_prefix + '-cleanup-' + args.command + '-' + secrets.token_hex(8) + '.private.json')
        snapshot_path.write_bytes(canonical(snapshot) + b'\n')
        require(snapshot['businessReferenceCount'] == 0, 'Business or foreign-key references exist; private snapshot retained, employees preserved')
        if args.command == 'delete-employees':
            final_predicate = predicate + " AND e.status='inactive' AND a.status=1 AND a.deletetime IS NOT NULL"
            require(int(db.one('SELECT COUNT(*) FROM biz_hrm_employee e JOIN gf_admin a ON a.id=e.user_id WHERE ' + final_predicate)) == 2,
                    'Quiesce, inactive employee and soft-deleted host account are required first')
            require(int(db.one('SELECT COUNT(*) FROM biz_openim_employee WHERE employee_id IN (' + ids + ") AND enabled=0 AND desired_version=applied_version AND last_error=''")) == 2,
                    'Disabled IM projections have not converged')
            require(int(db.one('SELECT COUNT(*) FROM biz_openim_employee_access WHERE employee_id IN (' + ids + ') AND allow_lan=0 AND allow_pc=0 AND allow_mobile=0 AND allow_internet=0 AND allow_internet_download=0')) == 2,
                    'All five fixture access permissions must be closed')
            user_ids = ','.join(literal('emp_' + hashlib.sha256(a['employeeID'].encode()).hexdigest()[:32]) for a in fixtures)
            require(int(db.one("SELECT COUNT(*) FROM biz_openim_outbox WHERE status IN ('pending','running','failed') AND (aggregate_id IN (" + ids + ',' + user_ids + ") OR JSON_UNQUOTE(JSON_EXTRACT(payload,'$.userID')) IN (" + user_ids + '))')) == 0,
                    'Fixture revocation or projection work is still pending')
            require(int(db.one('SELECT COUNT(*) FROM biz_openim_epoch WHERE uid IN (' + uids + ') AND epoch>0')) == 2,
                    'Persistent revocation epochs must remain present')
            affected = db.one('DELETE e FROM biz_hrm_employee e JOIN gf_admin a ON a.id=e.user_id WHERE ' + final_predicate + '; SELECT ROW_COUNT()')
            require(int(affected) == 2, 'Exact two-row delete failed; rolling back')
            require(int(db.one('SELECT COUNT(*) FROM biz_hrm_employee WHERE id IN (' + ids + ')')) == 0,
                    'Synthetic employee rows remain')
            db.query('COMMIT')
            snapshot.update(employeeRowsDeleted=2, retained=['soft-deleted host accounts', 'IM projections', 'epochs',
                            'tombstones', 'audit history', 'outbox history', 'all Redis and Mongo data'])
            snapshot_path.write_bytes(canonical(snapshot) + b'\n')
        else:
            db.query('ROLLBACK')
        print(json.dumps({'snapshot': str(snapshot_path), 'businessReferenceCount': snapshot['businessReferenceCount'],
                          'foreignKeyCount': len(snapshot['foreignKeys']), 'employeeRowsDeleted': snapshot['employeeRowsDeleted'],
                          'planSHA256': args.plan_sha256, 'passed': True}))
    finally:
        db.close()
        lock.close()


if __name__ == '__main__':
    main()
