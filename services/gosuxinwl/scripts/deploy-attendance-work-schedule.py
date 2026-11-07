"""Deploy an audited attendance plugin upgrade without replacing the R6 host."""
import argparse
import hashlib
import json
import os
import re
from pathlib import Path
import runpy
import shutil
import subprocess
import time
import tempfile
import zipfile
from datetime import datetime
from zoneinfo import ZoneInfo

ROOT = Path('/srv/gosuxin-suite/shared/gosuxin/storage/plugins/biz-suite')
OLD = '92d59c1d1e196606b108a89193b0bdccd02320f27a36da6940594693b98ec2ff'
SERVICE = 'gosuxin-hrm.service'
MYSQL = ['docker', 'exec', '-i', 'gosuxin-suite-mysql-1', 'mysql', '--defaults-extra-file=/run/secrets/backup.cnf', '--default-character-set=utf8mb4', '-N', '-B', 'kaoqin_dev']
HELPER = runpy.run_path(str(Path(__file__).with_name('deploy-attendance-null-hotfix.py')))

def sha(data):
    return hashlib.sha256(data).hexdigest()

def db(query):
    result = subprocess.run(MYSQL, input=query.encode(), capture_output=True, timeout=60)
    if result.returncode:
        raise RuntimeError('Database verification failed; private output suppressed')
    return result.stdout

def snapshot():
    names = db("SELECT table_name FROM information_schema.tables WHERE table_schema='kaoqin_dev' AND (table_name LIKE 'biz_att_%' OR table_name LIKE 'biz_hrm_attendance_%' OR table_name LIKE 'wecom_%') ORDER BY table_name;").decode().split()
    if not names or any(not re.fullmatch(r'[a-z0-9_]+', name) for name in names):
        raise RuntimeError('Unexpected business table inventory')
    checksums = db('CHECKSUM TABLE ' + ','.join(names) + ' EXTENDED;')
    if b'NULL' in checksums:
        raise RuntimeError('Business table checksum unavailable')
    return {
        'rules_sha256': sha(db('SELECT * FROM biz_att_rule ORDER BY id;')),
        'core_accounts_sha256': sha(db('SELECT id,username,name,status,dept_id FROM gf_admin ORDER BY id;')),
        'departments_sha256': sha(db('SELECT * FROM gf_auth_dept ORDER BY id;')),
        'counts': db('SELECT (SELECT COUNT(*) FROM biz_att_rule),(SELECT COUNT(*) FROM biz_att_roster),(SELECT COUNT(*) FROM biz_att_result),(SELECT COUNT(*) FROM biz_hrm_attendance_clock),(SELECT COUNT(*) FROM wecom_checkin_raw);').decode().strip(),
        'old_schema_sha256': sha(db('SELECT * FROM biz_schema_version WHERE version<=5 ORDER BY version;')),
        'attendance_and_wecom_table_checksums_sha256': sha(checksums),
    }

def require_idle():
    pending = db("SELECT (SELECT COUNT(*) FROM biz_att_batch WHERE status IN ('queued','running'))+(SELECT COUNT(*) FROM biz_att_dirty WHERE status='pending')+(SELECT COUNT(*) FROM wecom_attendance_outbox WHERE status='pending')+(SELECT COUNT(*) FROM wecom_sync_run WHERE status='running');")
    if int(pending):
        raise RuntimeError('Background work is active or pending; finish it before upgrading')
    now = datetime.now(ZoneInfo('Asia/Shanghai'))
    minute = now.hour * 60 + now.minute
    if any(min((minute-slot)%1440, (slot-minute)%1440) <= 8 for slot in [510,600,870,1439]):
        raise RuntimeError('Too close to a scheduled sync; upgrade after this scheduled run finishes')

def main():
    global OLD
    parser = argparse.ArgumentParser()
    parser.add_argument('archive', type=Path)
    parser.add_argument('--package', required=True)
    parser.add_argument('--archive-sha256', required=True)
    parser.add_argument('--expected-current', default=OLD)
    parser.add_argument('--apply', action='store_true')
    args = parser.parse_args()
    if not re.fullmatch(r'[0-9a-f]{64}', args.expected_current):
        raise RuntimeError('Expected current package must be an exact digest')
    OLD = args.expected_current
    if os.geteuid() != 0:
        raise RuntimeError('sudo is required')
    active = ROOT / 'active.json'
    prior = active.read_bytes()
    state = json.loads(prior)
    if state.get('package') != OLD or state.get('disabled'):
        raise RuntimeError('Active package changed; inspect before deploying')
    current = ROOT / 'packages' / OLD
    old_raw = (current / 'plugin.json').read_bytes()
    old = json.loads(old_raw)
    if sha(old_raw) != OLD or any(sha((current / path).read_bytes()) != expected for path, expected in old['files'].items()):
        raise RuntimeError('Previous package integrity changed')
    if sha(args.archive.read_bytes()) != args.archive_sha256:
        raise RuntimeError('Unreviewed archive')
    with zipfile.ZipFile(args.archive) as archive:
        manifest = archive.read('plugin.json')
        if sha(manifest) != args.package:
            raise RuntimeError('Unreviewed manifest')
        new = json.loads(manifest)
        for key in ['format', 'name', 'protocol', 'os', 'arch']:
            if new[key] != old[key]:
                raise RuntimeError('Plugin target changed')
        if not new['version'].startswith('0.3.2'):
            raise RuntimeError('Unexpected release version')
        if set(archive.namelist()) != set(new['files']) | {'plugin.json'} or len(archive.namelist()) != len(new['files']) + 1:
            raise RuntimeError('Archive has unlisted or duplicate files')
        contents = {}
        for name, expected in new['files'].items():
            path = Path(name)
            if path.is_absolute() or '..' in path.parts or '\\' in name:
                raise RuntimeError('Unsafe archive member')
            contents[name] = archive.read(name)
            if sha(contents[name]) != expected:
                raise RuntimeError('Member checksum mismatch')
    if not HELPER['healthy'](OLD):
        raise RuntimeError('Existing service is not healthy')
    require_idle()
    before = snapshot()
    report = {'applied': False, 'old_package': OLD, 'package': args.package, 'version': new['version'], 'before': before}
    if not args.apply:
        print(json.dumps(report))
        return
    backup = ROOT / 'backups' / ('attendance-work-schedule-' + time.strftime('%Y%m%d-%H%M%S'))
    backup.mkdir(mode=0o700)
    (backup / 'active-before.json').write_bytes(prior)
    (backup / 'active-before.json').chmod(0o600)
    with zipfile.ZipFile(backup / 'package-before.zip', 'x', compression=zipfile.ZIP_DEFLATED) as archive:
        archive.write(current / 'plugin.json', 'plugin.json')
        for path in old['files']:
            archive.write(current / path, path)
    (backup / 'package-before.zip').chmod(0o600)
    target = ROOT / 'packages' / args.package
    target.mkdir(mode=0o700)
    for name, data in {**contents, 'plugin.json': manifest}.items():
        destination = target / name
        destination.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
        destination.write_bytes(data)
        destination.chmod(0o700 if name == 'bin/biz-suite' else 0o600)
    for path in [target, *target.rglob('*')]:
        shutil.chown(path, 'gosuxin', 'gosuxin')
    if sha((target / 'plugin.json').read_bytes()) != args.package or any(sha((target / name).read_bytes()) != expected for name, expected in new['files'].items()):
        raise RuntimeError('Staged package verification failed')
    def replace(data):
        descriptor, name = tempfile.mkstemp(prefix='active.work-schedule-', suffix='.json', dir=active.parent)
        temp = Path(name)
        try:
            with os.fdopen(descriptor, 'wb') as output:
                output.write(data)
            temp.chmod(0o600)
            shutil.chown(temp, 'gosuxin', 'gosuxin')
            os.replace(temp, active)
        finally:
            temp.unlink(missing_ok=True)
    if active.read_bytes() != prior:
        raise RuntimeError('Concurrent deployment detected')
    try:
        subprocess.run(['systemctl', 'stop', SERVICE], check=True, timeout=60)
        require_idle()
        before = snapshot()
        dump = backup / 'kaoqin_dev-before.sql'
        with dump.open('xb') as output:
            dump.chmod(0o600)
            result = subprocess.run(['docker', 'exec', 'gosuxin-suite-mysql-1', 'mysqldump', '--defaults-extra-file=/run/secrets/backup.cnf', '--single-transaction', '--quick', '--routines', '--triggers', '--events', '--hex-blob', '--set-gtid-purged=OFF', '--skip-dump-date', '--order-by-primary', '--databases', 'kaoqin_dev'], stdout=output, stderr=subprocess.PIPE, timeout=180)
        if result.returncode or dump.stat().st_size < 1000:
            raise RuntimeError('Database backup failed; private dump output suppressed')
        with dump.open('rb') as input_file:
            dump_sha = hashlib.file_digest(input_file, 'sha256').hexdigest()
        (backup / 'database-backup.sha256').write_text(dump_sha + '\n')
        replace(json.dumps({**state, 'package': args.package}).encode())
        subprocess.run(['systemctl', 'start', SERVICE], check=True, timeout=60)
        for _ in range(50):
            if HELPER['healthy'](args.package):
                time.sleep(2)
                if HELPER['healthy'](args.package):
                    break
            time.sleep(1)
        else:
            raise RuntimeError('New plugin failed startup health')
        after = snapshot()
        if before != after:
            raise RuntimeError('Unexpected business data changes during program upgrade')
        versions = db('SELECT version FROM biz_schema_version ORDER BY version;').decode().split()
        if versions != ['1','2','3','4','5','6','7']:
            raise RuntimeError('Schema migration incomplete')
        if json.loads(active.read_bytes()).get('package') != args.package:
            raise RuntimeError('Active package changed during health verification')
        report.update(applied=True, backup=str(backup), database_backup_bytes=dump.stat().st_size, database_backup_sha256=dump_sha, before=before, after=after, business_data_unchanged=True, schema_versions=versions)
    except Exception:
        # Do not hand a new-format rule, saved concurrently by an administrator,
        # to the previous engine. Its writes must be preserved for inspection.
        try:
            rules_unchanged = sha(db('SELECT * FROM biz_att_rule ORDER BY id;')) == before['rules_sha256']
        except Exception:
            # Unknown database state must not leave a stopped service down or
            # downgrade a potentially new-format rule.
            subprocess.run(['systemctl', 'start', SERVICE], check=True, timeout=60)
            print(json.dumps({'rollback_held_for_unknown_database_state': True, 'backup': str(backup)}))
            raise
        if not rules_unchanged:
            subprocess.run(['systemctl', 'start', SERVICE], check=True, timeout=60)
            print(json.dumps({'rollback_held_for_new_rule_data': True, 'backup': str(backup)}))
            raise
        try:
            replace(prior)
        finally:
            subprocess.run(['systemctl', 'restart', SERVICE], check=True, timeout=60)
        restored = False
        for _ in range(40):
            if HELPER['healthy'](OLD):
                restored = True
                break
            time.sleep(1)
        print(json.dumps({'program_rolled_back': restored, 'backup': str(backup), 'database_restored': False}))
        raise
    (backup / 'deployment-report.json').write_text(json.dumps(report, indent=2))
    print(json.dumps(report))

if __name__ == '__main__':
    main()
