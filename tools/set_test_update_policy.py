"""Temporarily change only the exact isolated Android Chat release's force flag.

Publish and verify the final immutable APK first with publish_test_artifacts.py.
Use this tool again with --force false after the actual device update test.
The official Chat API changes only that record's policy. It does not access
databases directly or change server programs, immutable artifact metadata,
Windows policy or production endpoints.
"""
import argparse
import datetime
import hashlib
import json
import pathlib
import re
import secrets
import ssl
import urllib.request

ROOT = pathlib.Path(__file__).resolve().parents[1]
BASE = 'https://192.168.10.11:29880'


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        raise RuntimeError('Redirects are not allowed for the test policy API')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--expected-build', required=True, type=int)
    parser.add_argument('--expected-sha256', required=True)
    parser.add_argument('--force', required=True, choices=['true', 'false'])
    parser.add_argument('--report-name', required=True)
    args = parser.parse_args()
    if not re.fullmatch(r'[a-zA-Z0-9_.-]+\.json', args.report_name):
        raise RuntimeError('Report must be a new JSON basename in tests/')
    output = ROOT / 'tests' / args.report_name
    if output.exists():
        raise RuntimeError('Report already exists; refusing to overwrite evidence')
    digest = args.expected_sha256.lower()
    if not re.fullmatch(r'[0-9a-f]{64}', digest):
        raise RuntimeError('Expected SHA-256 is invalid')
    match = re.search(r'^version:\s*(\d+\.\d+\.\d+)\+(\d+)\s*$',
                      (ROOT / 'clients/flutter/pubspec.yaml').read_text(), re.M)
    if not match or int(match[2]) != args.expected_build or args.expected_build < 2:
        raise RuntimeError('Current application build does not match the selected test')
    version, build = match[1], int(match[2])
    apk = ROOT / f'artifacts/android/OpenIM38-{version}+{build}-development-arm64.apk'
    with apk.open('rb') as stream:
        if hashlib.file_digest(stream, 'sha256').hexdigest() != digest:
            raise RuntimeError('Current formal APK hash does not match')
    expected = {'platform': 'android', 'version': version, 'build': build,
                'url': f'{BASE}/downloads/OpenIM38-{version}-{build}-development-arm64.apk',
                'sha256': digest, 'size': apk.stat().st_size}
    published = json.loads((ROOT / 'tests/release-downloads.json').read_text(encoding='utf-8'))
    if (published.get('published') is not True or published.get('fullHTTPSDownloadsVerified') is not True or
            not any(all(item.get(k) == v for k, v in expected.items())
                    for item in published.get('artifacts', []))):
        raise RuntimeError('The exact APK must first have a verified full HTTPS publication report')
    tls = ssl.create_default_context(cafile=str(ROOT / 'artifacts/dev-cert/OpenIM38-Development-CA.crt'))
    http = urllib.request.build_opener(urllib.request.ProxyHandler({}), NoRedirect(),
                                       urllib.request.HTTPSHandler(context=tls))

    def call(path, body, token=''):
        request = urllib.request.Request(BASE + path, json.dumps(body).encode(),
                    {'Content-Type': 'application/json', 'operationID': secrets.token_hex(12),
                     'token': token, 'chatToken': token})
        with http.open(request, timeout=30) as response:
            raw = response.read(2 * 1024 * 1024 + 1)
        if len(raw) > 2 * 1024 * 1024:
            raise RuntimeError('Unexpectedly large policy response')
        value = json.loads(raw)
        if value.get('errCode') != 0:
            raise RuntimeError(f'Policy API failed: {path}, code {value.get("errCode")}')
        return value.get('data') or {}

    baseline = {'platform': 'android', 'version': version, 'build': build - 1}

    def latest():
        value = call('/enterprise/v1/application/latest', baseline)
        if value.get('available') is not True or any(value.get(k) != v for k, v in expected.items()):
            raise RuntimeError('Published immutable Android artifact does not match the APK')
        return value

    initial = latest()
    state = json.loads((ROOT / '.private/test-accounts.json').read_text(encoding='utf-8'))
    admin = call('/enterprise/v1/admin/login', {'account': state['admin']['account'],
                 'password': hashlib.md5(state['admin']['password'].encode()).hexdigest()})['adminToken']
    route = '/enterprise/v1/admin/chat/application/'

    def catalog():
        record = call(route + 'latest_version', {'platform': 'android'}, admin).get('version', {})
        if (not re.fullmatch(r'[0-9a-f]{24}', record.get('id', '')) or
                any(record.get(k) != expected[k] for k in ['platform', 'version', 'url']) or
                record.get('latest') is not True or record.get('hot') is not False or
                not isinstance(record.get('force'), bool)):
            raise RuntimeError('Unexpected current Chat Android record')
        return record

    before = catalog()
    if before['force'] is not initial.get('force'):
        raise RuntimeError('Chat and enterprise policy disagree before mutation')
    desired = args.force == 'true'
    report = {'utc': datetime.datetime.now(datetime.timezone.utc).isoformat(),
              'scope': 'Isolated openim38-upgrade Android policy only',
              'artifact': expected, 'catalogID': before['id'], 'beforeForce': before['force'],
              'requestedForce': desired, 'status': 'prepared', 'mutationAttempted': False,
              'limits': ['This policy check is not device update acceptance.',
                         'Artifact URL/build/size/SHA remain immutable; Chat is the live policy record.']}
    with output.open('x', encoding='utf-8') as stream:
        json.dump(report, stream, indent=2)

    def save():
        output.write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')

    phase = 'mutation'
    try:
        if before['force'] is not desired:
            report['mutationAttempted'] = True
            save()
            # openim.protobuf.BoolValue uses a raw JSON boolean, not {value:...}.
            call(route + 'update_version', {'id': before['id'], 'force': desired}, admin)
        phase = 'verify-catalog'
        after = catalog()
        if ({k: v for k, v in before.items() if k != 'force'} !=
                {k: v for k, v in after.items() if k != 'force'} or after['force'] is not desired):
            raise RuntimeError('A field other than force changed or the policy did not apply')
        phase = 'verify-enterprise-policy'
        visible = latest()
        if visible.get('force') is not desired:
            raise RuntimeError('Enterprise latest did not expose the expected Chat force flag')
        phase = 'verify-current-version'
        current = call('/enterprise/v1/application/latest', {'platform': 'android',
                       'version': version, 'build': build})
        if current.get('available') is not False:
            raise RuntimeError('Current APK is incorrectly offered an update')
        report.update(status='passed', afterForce=after['force'],
                      immutableArtifactUnchanged=True, currentVersionNotOffered=True)
    except Exception as error:
        report.update(status='failed', errorType=type(error).__name__, failedPhase=phase)
        if report['mutationAttempted']:
            try:
                now = catalog()
                if now['id'] != before['id']:
                    raise RuntimeError('Current record identity changed; refusing rollback of a different record')
                call(route + 'update_version', {'id': before['id'], 'force': before['force']}, admin)
                report['rollbackVerified'] = (catalog() == before and latest()['force'] is before['force'])
            except Exception as rollback_error:
                report['rollbackVerified'] = False
                report['rollbackErrorType'] = type(rollback_error).__name__
        save()
        raise RuntimeError('Policy verification failed; inspect the sanitized report and restore optional policy if needed') from None
    save()
    print(json.dumps({'status': report['status'], 'build': build, 'force': desired,
                      'report': output.relative_to(ROOT).as_posix()}))


if __name__ == '__main__':
    main()
