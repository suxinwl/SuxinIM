"""Run the isolated final 50+50 real-SDK window, with an automatic ready barrier.

Linux test host only. Reuses one specifically inspected stopped load container;
never creates/removes a container or touches production/HRM/client accounts101+.
Run from /srv/openim38-upgrade using sudo python3 tests/start-final-load.py.
"""
from __future__ import annotations

import argparse
import datetime
import hashlib
import json
import os
import pathlib
import re
import shutil
import subprocess
import threading
import time
import urllib.request

ROOT = pathlib.Path('/srv/openim38-upgrade')
NAME = 'openim38-upgrade-load'
DOCKER = ['docker', '--host', 'unix:///var/run/docker.sock']
DURATION = 1800
ORIGIN = 'https://192.168.10.11:29880'
EXPECTED_MOUNTS = {
    '/ca': (str(ROOT / 'certs'), False), '/fixtures': (str(ROOT / '.private'), False),
    '/load': (str(ROOT / 'load'), True), '/root': (str(ROOT / '.private/load-home'), True),
}


def fail(message):
    raise RuntimeError(message)


def sha(path):
    with pathlib.Path(path).open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def command(args, *, input_data=None, timeout=120):
    result = subprocess.run(args, input=input_data, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=timeout)
    if result.returncode:
        # Do not echo subprocess stderr: configuration and SDK errors can contain secrets.
        fail('Checked command failed: ' + args[0] + ' exit ' + str(result.returncode))
    return result.stdout


def write_json(path, value):
    path = pathlib.Path(path)
    temporary = path.with_name(path.name + '.tmp')
    temporary.write_text(json.dumps(value, indent=2) + '\n', encoding='utf-8')
    temporary.replace(path)


def inspect_container():
    return json.loads(command(DOCKER + ['inspect', NAME]))[0]


def validate_container(container, lock, require_stopped=True):
    config, host = container['Config'], container['HostConfig']
    if container['Name'] != '/' + NAME or container['Id'] != lock['containerID']:
        fail('Refusing a container other than the reviewed load-container identity')
    labels = config.get('Labels') or {}
    if labels.get('com.openim38.role') != 'load-test' or labels.get('com.docker.compose.project'):
        fail('Unexpected load-container ownership labels')
    if config.get('Image') != lock['image'] or config.get('Entrypoint') != ['sleep'] or config.get('Cmd') != ['infinity'] or config.get('WorkingDir') != '/load':
        fail('Unexpected load image, entrypoint, command or workdir')
    if require_stopped and (container['State']['Running'] or container['State']['Status'] != 'exited'):
        fail('Load container must be stopped before this exclusive run')
    if host.get('NetworkMode') != 'host' or host.get('Privileged') or host.get('PidMode') not in (None, '') or host.get('CapAdd'):
        fail('Unexpected host namespace, privilege or capabilities')
    if (host.get('Memory'), host.get('NanoCpus'), host.get('PidsLimit'), host.get('ShmSize')) != (18 * 1024**3, 8_000_000_000, 1536, 1024**3):
        fail('Load resource limits differ from the reviewed container')
    mounts = container.get('Mounts', [])
    actual = {item['Destination']: (item['Source'], item['RW']) for item in mounts}
    if len(mounts) != 4 or actual != EXPECTED_MOUNTS or any(item['Type'] != 'bind' for item in mounts):
        fail('Load bind-mount ownership or read-only settings differ')
    for value in config.get('Env') or []:
        if value.startswith(('NODE_OPTIONS=', 'NODE_TLS_REJECT_UNAUTHORIZED=', 'PLAYWRIGHT_CHROMIUM_EXECUTABLE=')):
            fail('Unexpected environment override in reviewed container')


def validate_sessions(value):
    users = value.get('users') if isinstance(value, dict) else value
    if not isinstance(users, list) or len(users) != 100:
        fail('Exactly100 minimal synthetic sessions are required')
    if isinstance(value, dict) and value.get('baseURL', ORIGIN) != ORIGIN:
        fail('Unexpected synthetic-session API origin')
    for index, user in enumerate(users, 1):
        if (not isinstance(user, dict) or set(user) != {'userID', 'imToken', 'platformID'}
                or user.get('userID') != f'upgrade38_u{index:03}' or user.get('platformID') != 5
                or not isinstance(user.get('imToken'), str) or len(user['imToken']) < 20):
            fail('Session range/order/fields must be exactly synthetic001–100 on platform5')
    return len(users)


def valid_ready(report, offset, run_id):
    return (report.get('sharedRun') == run_id and report.get('phase') == 'preparing'
            and report.get('offset') == offset and report.get('count') == 50
            and report.get('clients') == 50 and report.get('connected') == 50
            and report.get('failed') == 0 and report.get('disconnections') == 0 and not report.get('pageErrors'))


def summarize(parts):
    checks = {
        'sameRun': len({part['sharedRun'] for part in parts}) == 1,
        'exact100DistinctSlots': sorted((part['offset'], part['count']) for part in parts) == [(0, 50), (50, 50)],
        'allComplete': all(part['phase'] == 'complete' for part in parts),
        'atLeast30Minutes': all(part['elapsedSeconds'] >= DURATION for part in parts),
        'allConnectedAtCompletion': all(part['connected'] == 50 and part['clients'] == 50 for part in parts),
        'noFailedSends': sum(part['failed'] for part in parts) == 0,
        'noDisconnections': sum(part['disconnections'] for part in parts) == 0,
        'noRuntimeErrors': not any(part['pageErrors'] for part in parts),
        'allSentMessagesReceived': sum(part['sent'] for part in parts) == sum(part['received'] for part in parts) > 0,
    }
    start = max(datetime.datetime.fromisoformat(part['startedAt']) for part in parts)
    end = min(datetime.datetime.fromisoformat(part['finishedAt']) for part in parts)
    common = (end - start).total_seconds()
    checks['commonWindowAtLeast30Minutes'] = common >= DURATION
    return {'run': parts[0]['sharedRun'], 'passed': all(checks.values()), 'checks': checks,
            'clients': sum(part['clients'] for part in parts), 'sent': sum(part['sent'] for part in parts),
            'received': sum(part['received'] for part in parts), 'failed': sum(part['failed'] for part in parts),
            'disconnections': sum(part['disconnections'] for part in parts), 'commonWindowSeconds': common,
            'partitions': parts, 'latencyNote': 'Keep partition percentiles separately; never average percentiles.',
            'physicalDeviceOrAVCapacityConclusion': False}


def business_snapshot(lock):
    ids = command(DOCKER + ['ps', '-q', '--filter', 'label=com.docker.compose.project=openim38-upgrade']).decode().split()
    containers = json.loads(command(DOCKER + ['inspect', *ids]))
    if len(containers) != 24:
        fail('Expected24 running isolated business containers')
    expected = lock['businessBinarySHA256']
    binaries = {}
    services = {}
    for item in containers:
        labels = item['Config'].get('Labels') or {}
        if labels.get('com.docker.compose.project') != 'openim38-upgrade' or not item['State']['Running']:
            fail('Unexpected business ownership/state')
        service = labels['com.docker.compose.service']
        services[service] = item['Id']
        for mount in item.get('Mounts', []):
            if mount['Destination'].startswith('/app/bin/'):
                source = pathlib.Path(mount['Source'])
                if source.resolve().parent != ROOT / 'bin' or mount.get('RW'):
                    fail('Unexpected isolated executable mount')
                mounted, running = sha(source), sha(f"/proc/{item['State']['Pid']}/exe")
                if mounted != expected.get(service) or running != mounted:
                    fail('Business executable does not match the reviewed final snapshot')
                binaries[service] = {'binary': source.name, 'mountedSHA256': mounted, 'processSHA256': running}
    if set(binaries) != set(expected):
        fail('Business executable set differs from final snapshot')
    return {'utc': datetime.datetime.now(datetime.timezone.utc).isoformat(), 'containers': 24,
            'containerIDs': services, 'binaries': binaries}


def resource_sampler(start_at, output, container_ids, stop, errors):
    try:
        with output.open('x', encoding='utf-8') as stream:
            while not stop.is_set():
                raw = command(DOCKER + ['stats', '--no-stream', '--format', '{{json .}}', *container_ids], timeout=30).decode()
                values = [json.loads(value) for value in raw.splitlines() if value.strip()]
                info = dict(value.split(':', 1) for value in pathlib.Path('/proc/meminfo').read_text().splitlines())
                sample = {'utc': datetime.datetime.now(datetime.timezone.utc).isoformat(),
                          'secondsFromStart': round(time.time() - start_at / 1000, 2),
                          'hostMemoryKB': {key: info[key].strip() for key in ['MemTotal', 'MemAvailable', 'SwapTotal', 'SwapFree']},
                          'loadAverage': pathlib.Path('/proc/loadavg').read_text().split()[:3],
                          'loadGenerator': [value for value in values if value.get('Name') == NAME],
                          'businessContainers': [value for value in values if value.get('Name') != NAME]}
                stream.write(json.dumps(sample) + '\n'); stream.flush()
                stop.wait(15)
    except Exception as error:
        errors.append(type(error).__name__)


def copy_reports(work, evidence):
    reports = {}
    for group in ('linux50a', 'linux50b'):
        folders = list((work / group / 'test-results').glob('load-*'))
        if len(folders) > 1:
            fail('Multiple loader result directories in exclusive final workdir')
        if not folders:
            continue
        target = evidence / group
        target.mkdir(exist_ok=True)
        for name in ('progress.json', 'summary.json'):
            source = folders[0] / name
            if source.is_file():
                try:
                    value = json.loads(source.read_text())
                except json.JSONDecodeError:
                    continue  # Loader writes progress in place; retry a concurrent write.
                write_json(target / name, value)
                reports[group] = value
    return reports


def verify_trust(container_id):
    expected = command(['openssl', 'x509', '-in', str(ROOT / 'certs/ca.crt'), '-outform', 'DER'])
    for args in (['cat', '/usr/local/share/ca-certificates/openim38.crt'],
                 ['certutil', '-L', '-a', '-d', 'sql:/root/.pki/nssdb', '-n', 'OpenIM38']):
        pem = command(DOCKER + ['exec', container_id, *args])
        actual = command(['openssl', 'x509', '-outform', 'DER'], input_data=pem)
        if actual != expected:
            fail('Existing system/NSS trust does not match the isolated public CA')
    trust = command(DOCKER + ['exec', container_id, 'certutil', '-L', '-d', 'sql:/root/.pki/nssdb']).decode()
    if not re.search(r'^OpenIM38\s+[^\s]*C[^\s]*,', trust, re.MULTILINE):
        fail('Isolated CA is not a trusted NSS SSL authority')
    return hashlib.sha256(expected).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--run-id', default='final-100-' + datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ').lower())
    parser.add_argument('--prepare-timeout', type=int, default=1200)
    parser.add_argument('--check-only', action='store_true', help='Validate ownership/files/minimal session fields; do not start anything')
    args = parser.parse_args()
    if not re.fullmatch(r'[a-z0-9][a-z0-9-]{1,63}', args.run_id) or not 120 <= args.prepare_timeout <= 1800:
        fail('Invalid final run ID or preparation timeout')
    if ROOT.resolve() != ROOT or pathlib.Path.cwd().resolve() != ROOT:
        fail('Run from the real isolated root /srv/openim38-upgrade')
    if (ROOT / 'tests/load-final').resolve() != ROOT / 'tests/load-final':
        fail('Final evidence parent must not be a symlink')
    state = json.loads((ROOT / 'instance.json').read_text())
    if state.get('stackName') != 'openim38-upgrade' or state.get('testOnly') is not True or state.get('originalDataMigrated') is not False:
        fail('Not the expected synthetic instance')
    lock = json.loads((ROOT / 'tests/load-final/launcher-lock.json').read_text())
    container = inspect_container()
    validate_container(container, lock)
    for path, _ in EXPECTED_MOUNTS.values():
        if pathlib.Path(path).resolve() != pathlib.Path(path):
            fail('Expected bind source must not be a symlink')
    image = json.loads(command(DOCKER + ['image', 'inspect', lock['image']]))[0]
    if image['Id'] != container['Image']:
        fail('Load image identity mismatch')
    if command(['ss', '-H', '-ltn', 'sport', '=', ':7778']).strip():
        fail('Port7778 already belongs to another process; nothing was stopped')
    for name, expected in lock['loadFilesSHA256'].items():
        path = ROOT / 'load' / name
        if path.resolve().is_relative_to(ROOT / 'load') is False or not path.is_file() or sha(path) != expected:
            fail('Fixed loader/static SDK file missing or changed: ' + name)
    for name in ('load-100.mjs', 'load-static-server.mjs', 'static/load-client.js'):
        value = (ROOT / 'load' / name).read_text()
        if re.search(r'ignoreHTTPSErrors|ignore-certificate-errors|setIgnoreCertificateErrors|rejectUnauthorized\s*:\s*false|NODE_TLS_REJECT_UNAUTHORIZED', value):
            fail('TLS-bypass pattern in loader source')
    fixture = ROOT / '.private/final-load-sessions.json'
    if fixture.is_symlink() or fixture.stat().st_mode & 0o077:
        fail('Minimal session fixture must be private600 and not a symlink')
    validate_sessions(json.loads(fixture.read_text()))
    barrier = ROOT / '.private/final-load-start-at.json'
    work = ROOT / 'load' / args.run_id
    evidence = ROOT / 'tests/load-final' / args.run_id
    if barrier.exists() or work.exists() or evidence.exists():
        fail('Final barrier/work/evidence already exists; preserve it and choose an explicitly reviewed new run')
    available = int(re.search(r'^MemAvailable:\s+(\d+)', pathlib.Path('/proc/meminfo').read_text(), re.MULTILINE).group(1)) * 1024
    if available < 12 * 1024**3:
        fail('Less than12GiB host available memory; do not start100 SDK instances')
    if args.check_only:
        print(json.dumps({'validated': True, 'syntheticSessions': 100, 'partitions': [50, 50],
                          'fixtureContentsPrinted': False, 'noContainersStarted': True})); return 0
    with (ROOT / '.private/final-load-launch.lock').open('x', encoding='utf-8') as stream:
        os.chmod(ROOT / '.private/final-load-launch.lock', 0o600)
        json.dump({'runID': args.run_id, 'pid': os.getpid(), 'purpose': 'exclusive final SDK run; preserve for review'}, stream)
    evidence.mkdir(mode=0o700, parents=True)
    work.mkdir(mode=0o700)
    for group in ('linux50a', 'linux50b'):
        (work / group).mkdir(mode=0o700)
    started = False
    jobs, streams = [], []
    sampler_stop, sampler_errors = threading.Event(), []
    sampler = None
    status = {'runID': args.run_id, 'phase': 'preflight', 'passed': False, 'durationSeconds': DURATION,
              'syntheticUsers': '001–100 only', 'partitions': [{'offset': 0, 'count': 50}, {'offset': 50, 'count': 50}],
              'loaderSHA256': lock['loadFilesSHA256'], 'minimalFixtureSHA256': sha(fixture),
              'launcherSHA256': sha(pathlib.Path(__file__).resolve()), 'TLSBypass': False,
              'oldEvidenceOverwritten': False}
    write_json(evidence / 'run.json', status)
    try:
        before = business_snapshot(lock)
        write_json(evidence / 'running-binaries-before.json', before)
        command(DOCKER + ['start', container['Id']]); started = True
        validate_container(inspect_container(), lock, require_stopped=False)
        status['trustedCASHA256'] = verify_trust(container['Id'])
        server_log = (evidence / 'static-server.log').open('x'); streams.append(server_log)
        server = subprocess.Popen(DOCKER + ['exec', '-e', 'LOAD_STATIC_PORT=7778', '-w', '/load', container['Id'], 'node', '/load/load-static-server.mjs', '/load/static'], stdout=server_log, stderr=subprocess.STDOUT)
        jobs.append(server)
        http = urllib.request.build_opener(urllib.request.ProxyHandler({}))
        for attempt in range(40):
            if server.poll() is not None:
                fail('Exclusive static server exited')
            try:
                with http.open('http://127.0.0.1:7778/load-host.html', timeout=2) as response:
                    if response.status == 200 and response.headers.get('Cross-Origin-Embedder-Policy') == 'require-corp':
                        break
            except OSError:
                time.sleep(0.5)
        else:
            fail('Static SDK host did not become ready')
        partition_jobs = []
        for group, offset in (('linux50a', 0), ('linux50b', 50)):
            log = (evidence / (group + '.log')).open('x'); streams.append(log)
            env = {'LOAD_OFFSET': offset, 'LOAD_COUNT': 50, 'LOAD_SECONDS': DURATION, 'LOAD_RUN_ID': args.run_id,
                   'LOAD_START_FILE': '/fixtures/final-load-start-at.json', 'LOAD_START_AT': 0, 'LOAD_API_ORIGIN': ORIGIN,
                   'LOAD_PAGE_URL': 'http://127.0.0.1:7778/load.html', 'LOAD_TMP_DIR': '/tmp/' + args.run_id + '-' + group,
                   'TMPDIR': '/tmp/' + args.run_id + '-' + group}
            cli = DOCKER + ['exec', '-w', '/load/' + args.run_id + '/' + group]
            for key, value in env.items():
                cli.extend(['-e', f'{key}={value}'])
            job = subprocess.Popen(cli + [container['Id'], 'node', '/load/load-100.mjs', '/fixtures/final-load-sessions.json'], stdout=log, stderr=subprocess.STDOUT)
            jobs.append(job); partition_jobs.append(job)
        status['phase'] = 'connecting'; write_json(evidence / 'run.json', status)
        deadline = time.monotonic() + args.prepare_timeout
        while time.monotonic() < deadline:
            if any(job.poll() is not None for job in partition_jobs) or server.poll() is not None:
                fail('A final SDK partition exited before the common barrier')
            progress = copy_reports(work, evidence)
            if valid_ready(progress.get('linux50a', {}), 0, args.run_id) and valid_ready(progress.get('linux50b', {}), 50, args.run_id):
                break
            time.sleep(2)
        else:
            fail('100 SDK connections did not become ready before preparation timeout')
        # Never write a start time until BOTH real-SDK partitions report50 connected.
        start_at = int(time.time() * 1000) + 15000
        barrier_value = {'runID': args.run_id, 'startAt': start_at, 'requiredSeconds': DURATION,
                         'linuxGroups': [50, 50], 'connectedBeforeBarrier': 100}
        with barrier.open('x', encoding='utf-8') as stream:
            os.chmod(barrier, 0o600); json.dump(barrier_value, stream)
        write_json(evidence / 'barrier.json', barrier_value)
        sampler = threading.Thread(target=resource_sampler, args=(start_at, evidence / 'resources.jsonl',
                                   list(before['containerIDs'].values()) + [container['Id']], sampler_stop, sampler_errors))
        sampler.start()
        status.update({'phase': 'common-window', 'startAt': start_at, 'connectedBeforeBarrier': 100})
        write_json(evidence / 'run.json', status)
        deadline = start_at / 1000 + DURATION + 180
        while time.time() < deadline:
            copy_reports(work, evidence)
            if all(job.poll() is not None for job in partition_jobs):
                break
            if any(job.poll() not in (None, 0) for job in partition_jobs) or server.poll() is not None:
                fail('SDK partition/static host failed during the common window')
            time.sleep(5)
        else:
            fail('Final SDK partitions did not finish within the bounded window')
        copy_reports(work, evidence)
        if any(job.returncode != 0 for job in partition_jobs):
            fail('Final SDK partition process failed')
        parts = [json.loads((evidence / group / 'summary.json').read_text()) for group in ('linux50a', 'linux50b')]
        summary = summarize(parts)
        after = business_snapshot(lock)
        write_json(evidence / 'running-binaries-after.json', after)
        summary['checks']['businessContainersUnchanged'] = before['containerIDs'] == after['containerIDs']
        summary['checks']['businessBinariesUnchanged'] = before['binaries'] == after['binaries']
        summary['checks']['resourceSamplerNoErrors'] = not sampler_errors
        summary['passed'] = all(summary['checks'].values())
        write_json(evidence / 'summary.json', summary)
        status.update({'phase': 'complete', 'passed': summary['passed']})
    except Exception as error:
        status.update({'phase': 'failed', 'failureType': type(error).__name__, 'failure': str(error)[:300]})
    finally:
        sampler_stop.set()
        if sampler is not None:
            sampler.join(timeout=40)
        try:
            copy_reports(work, evidence)
        except Exception as error:
            status['reportCopyFailureType'] = type(error).__name__; status['passed'] = False
        if started:
            # Exclusive ownership was verified while stopped; stop only this exact
            # load container, preserving its writable layer and all old evidence.
            try:
                validate_container(inspect_container(), lock, require_stopped=False)
                command(DOCKER + ['stop', '--time', '30', container['Id']], timeout=60)
                status['ownedLoadContainerStopped'] = True
            except Exception as error:
                status['cleanupFailureType'] = type(error).__name__; status['passed'] = False
        for job in jobs:
            try:
                job.wait(timeout=10)
            except subprocess.TimeoutExpired:
                job.terminate()  # Only the CLI subprocess created by this launcher.
        for stream in streams:
            stream.close()
        status['resourceSamplerErrors'] = sampler_errors
        if sampler_errors:
            status['passed'] = False
        summary_path = evidence / 'summary.json'
        if summary_path.is_file():
            summary = json.loads(summary_path.read_text())
            summary['checks']['resourceSamplerNoErrors'] = not sampler_errors
            summary['checks']['ownedLoadContainerStopped'] = status.get('ownedLoadContainerStopped') is True
            summary['passed'] = all(summary['checks'].values()) and status['passed']
            write_json(summary_path, summary)
        write_json(evidence / 'run.json', status)
    print(json.dumps({'evidence': str(evidence.relative_to(ROOT)), 'phase': status['phase'],
                      'passed': status['passed'], 'tokensPrinted': False}), flush=True)
    return 0 if status['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
