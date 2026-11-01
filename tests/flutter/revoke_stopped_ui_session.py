"""Explicit opt-in, exact-pair revoke for the stopped synthetic104 test app only.

This tool never starts/stops an app, clears local storage, invokes the SDK,
changes AVD state, logs credentials, follows redirects or disables TLS checks.
It is intentionally not a general remote logout tool.
"""
from __future__ import annotations

import argparse
import hashlib
import json
from pathlib import Path
import re
import ssl
import subprocess
import sys
import urllib.error
import urllib.request
import xml.etree.ElementTree as ET

ROOT = Path(__file__).resolve().parents[2]
ADB = Path('E:/AndroidDev/sdk/platform-tools/adb.exe')
SERIAL = 'emulator-5588'
AVD = 'openim38_api30'
PACKAGE = 'io.openim.upgrade38.test.dev'
USER = 'upgrade38_u104'
BASE = 'https://192.168.10.11:29880'
FIXTURE = ROOT / '.private/android-ui-session.json'
CA = ROOT / 'artifacts/dev-cert/OpenIM38-Development-CA.crt'
HEX = re.compile(r'[a-fA-F0-9]{64}\Z')


class Refusal(RuntimeError):
    """Messages are static labels, never interpolated private input."""


def sha(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def pair_fingerprint(cert: dict) -> str:
    selected = {key: cert[key] for key in ('userID', 'imToken', 'chatToken')}
    selected['platformID'] = 2
    return sha(json.dumps(selected, sort_keys=True, separators=(',', ':')).encode())


def checked_pair(value: object) -> dict:
    if not isinstance(value, dict) or not {'userID', 'imToken', 'chatToken'} <= value.keys():
        raise Refusal('fixture does not contain the required captured pair')
    if set(value) - {'userID', 'imToken', 'chatToken', 'platformID'}:
        raise Refusal('unexpected fixture fields; use the minimal session snapshot')
    if value['userID'] != USER or value.get('platformID', 2) != 2:
        raise Refusal('fixture is not the approved synthetic mobile user')
    if any(not isinstance(value[k], str) or not value[k] or len(value[k]) > 16384
           for k in ('imToken', 'chatToken')):
        raise Refusal('fixture token format is invalid')
    return value


def adb(*args: str, allow_empty_pid: bool = False) -> str:
    result = subprocess.run([str(ADB), '-s', SERIAL, *args], capture_output=True,
                            timeout=15, check=False)
    # Do not include stdout/stderr in exceptions: a shared-prefs response is secret.
    if result.returncode != 0 and not (allow_empty_pid and result.returncode == 1):
        raise Refusal('read-only ADB check failed')
    if len(result.stdout) > 1024 * 1024:
        raise Refusal('unexpectedly large ADB response')
    return result.stdout.decode('utf-8')


def verify_stopped() -> None:
    if adb('get-state').strip() != 'device':
        raise Refusal('approved emulator is unavailable')
    if adb('shell', 'getprop', 'ro.kernel.qemu').strip() != '1':
        raise Refusal('target is not an emulator')
    if adb('shell', 'getprop', 'ro.build.version.sdk').strip() != '30':
        raise Refusal('target is not the approved API30 emulator')
    boot_avd_name = adb('shell', 'getprop', 'ro.boot.qemu.avd_name').strip()
    kernel_avd_name = adb('shell', 'getprop', 'ro.kernel.qemu.avd_name').strip()
    names = (boot_avd_name, kernel_avd_name)
    if not any(names) or any(name and name != AVD for name in names):
        raise Refusal('AVD ownership/name mismatch')
    console_name = adb('emu', 'avd', 'name').split('\n')
    if console_name and console_name[-1] == '':
        console_name.pop()
    console_name = [line.rstrip('\r') for line in console_name]
    if console_name != [AVD, 'OK']:
        raise Refusal('AVD console ownership/name mismatch')
    if adb('shell', 'pidof', PACKAGE, allow_empty_pid=True).strip():
        raise Refusal('test app is still running; helper will not force-stop it')
    package = adb('shell', 'dumpsys', 'package', PACKAGE)
    if not re.search(r'User 0:[^\r\n]*\bstopped=true\b', package):
        raise Refusal('package is not explicitly force-stopped for Android user0')


def local_snapshot() -> tuple[str, str]:
    xml = adb('shell', 'cat', f'/data/user/0/{PACKAGE}/shared_prefs/FlutterSharedPreferences.xml')
    parsed = ET.fromstring(xml)
    matching = [node.text for node in parsed if node.attrib.get('name') == 'flutter.loginCertificate']
    if len(matching) != 1 or matching[0] is None:
        raise Refusal('exactly one retained local login certificate is required')
    cert = checked_pair(json.loads(matching[0]))
    return sha(xml.encode()), pair_fingerprint(cert)


class NoRedirects(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        raise Refusal('HTTPS redirect refused; credentials are restricted to the fixed test origin')


def request(opener, cert: dict, path: str, data: object | None, operation: str) -> dict:
    if path not in {'/enterprise/v1/session/logout', '/enterprise/v1/rtc/events?after=-1',
                    '/api/user/get_users_info', '/chat/user/find/full'}:
        raise Refusal('request path is outside the exact test allowlist')
    headers = {'token': cert['imToken'], 'chatToken': cert['chatToken'], 'platformID': '2',
               'operationID': operation, 'Content-Type': 'application/json'}
    # Chat public user endpoints use the Chat token as their token header.
    if path.startswith('/chat/'):
        headers['token'] = cert['chatToken']
    req = urllib.request.Request(BASE + path,
            data=None if data is None else json.dumps(data).encode(), headers=headers)
    try:
        with opener.open(req, timeout=20) as response:
            status, raw = response.status, response.read(1024 * 1024)
    except urllib.error.HTTPError as error:
        status, raw = error.code, error.read(1024 * 1024)
    body = json.loads(raw) if raw else {}
    result = {'status': int(status)}
    code = body.get('errCode') if isinstance(body, dict) else None
    if isinstance(code, int) and not isinstance(code, bool):
        result['errCode'] = code
    payload = body.get('data') if isinstance(body, dict) else None
    if isinstance(payload, dict) and isinstance(payload.get('revoked'), bool):
        result['revoked'] = payload['revoked']
    return result


def denied(result: dict) -> bool:
    # Known auth errors only: validation/not-found/server failures are not proof.
    return result['status'] in (401, 403) or (
        result['status'] == 200 and result.get('errCode') in {
            401, 403, 1501, 1502, 1503, 1504, 1505, 1506, 1507, 20101})


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--execute', action='store_true')
    parser.add_argument('--revoke-own-stopped-synthetic-session', action='store_true')
    parser.add_argument('--expected-fixture-sha256', required=True)
    parser.add_argument('--expected-pair-sha256', required=True,
                        help='SHA256 of canonical JSON userID/imToken/chatToken/platformID=2; never pass tokens')
    parser.add_argument('--output', type=Path, default=ROOT / 'tests/flutter/stopped-ui-session-revoke-105.json')
    args = parser.parse_args()
    if not args.execute or not args.revoke_own_stopped_synthetic_session:
        parser.error('both explicit execution flags are required; no ADB/files/network actions performed')
    if not HEX.fullmatch(args.expected_fixture_sha256) or not HEX.fullmatch(args.expected_pair_sha256):
        parser.error('both expected fingerprints must be SHA-256 hex values')
    output = args.output.absolute()
    if output.parent.resolve() != (ROOT / 'tests/flutter').resolve() or output.exists() or output.is_symlink():
        raise Refusal('output must be a new ordinary report directly inside tests/flutter')
    if FIXTURE.is_symlink() or not FIXTURE.is_file() or FIXTURE.stat().st_size > 65536:
        raise Refusal('fixture is not the expected bounded ordinary session file')
    raw = FIXTURE.read_bytes()
    if sha(raw) != args.expected_fixture_sha256.lower():
        raise Refusal('captured fixture hash mismatch')
    cert = checked_pair(json.loads(raw))
    fingerprint = pair_fingerprint(cert)
    if fingerprint != args.expected_pair_sha256.lower():
        raise Refusal('captured pair hash mismatch')
    verify_stopped()
    before_xml, before_pair = local_snapshot()
    if before_pair != fingerprint:
        raise Refusal('retained local certificate differs from captured fixture pair')
    context = ssl.create_default_context(cafile=str(CA))
    opener = urllib.request.build_opener(NoRedirects(), urllib.request.HTTPSHandler(context=context),
                                        urllib.request.ProxyHandler({}))
    verify_stopped()
    first = request(opener, cert, '/enterprise/v1/session/logout', {}, 'stopped-synthetic104-exact-logout-1')
    second = request(opener, cert, '/enterprise/v1/session/logout', {}, 'stopped-synthetic104-exact-logout-2')
    checks = {
        'enterprise': request(opener, cert, '/enterprise/v1/rtc/events?after=-1', None, 'stopped-synthetic104-check-enterprise'),
        'im': request(opener, cert, '/api/user/get_users_info', {'userIDs': [USER]}, 'stopped-synthetic104-check-im'),
        'chat': request(opener, cert, '/chat/user/find/full', {'userIDs': [USER]}, 'stopped-synthetic104-check-chat'),
    }
    verify_stopped()
    after_xml, after_pair = local_snapshot()
    result = {
        'schemaVersion': 1, 'source': 'explicit exact-pair logout while approved synthetic UI is force-stopped',
        'user': USER, 'serial': SERIAL, 'avd': AVD, 'package': PACKAGE, 'androidAPI': 30,
        'serverOrigin': BASE, 'normalTLSValidation': True, 'caSHA256': sha(CA.read_bytes()),
        'fixtureSHA256': sha(raw), 'pairSHA256': fingerprint,
        'firstLogout': first, 'idempotentLogout': second, 'oldPairAPI': checks,
        'localCertificatePreserved': before_xml == after_xml and before_pair == after_pair == fingerprint,
        'localSDKCache': 'No local storage writes, SDK calls, force-stop/start, data clear or app install performed by this helper.',
        'coldStartRegression': 'NOT_RUN_BY_THIS_TOOL',
    }
    result['passed'] = (all(r.get('status') == 200 and r.get('errCode') == 0 and r.get('revoked') is True
                            for r in (first, second)) and all(map(denied, checks.values()))
                        and result['localCertificatePreserved'])
    with output.open('x', encoding='utf-8') as stream:
        json.dump(result, stream, ensure_ascii=False, indent=2); stream.write('\n')
    print(json.dumps({'report': str(output.resolve().relative_to(ROOT.resolve())), 'passed': result['passed'],
                      'localCertificatePreserved': result['localCertificatePreserved']}))
    return 0 if result['passed'] else 1


if __name__ == '__main__':
    try:
        raise SystemExit(main())
    except Exception as error:
        # Private fixture/native/network response bodies must not reach stdout.
        print(json.dumps({'passed': False, 'errorType': type(error).__name__,
                          'detail': str(error) if isinstance(error, Refusal) else 'operation failed; sensitive details suppressed'}))
        raise SystemExit(1)
