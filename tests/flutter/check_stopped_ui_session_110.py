"""Read-only validity checkpoint for the one approved stopped110 UI pair."""
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import ssl
import urllib.request
import revoke_stopped_ui_session as target

EXPECTED_TOOL = 'f80069e5cdf02bdbe90e8371accbe2c7f9d3422a6fe3767af2f1438800baf24e'
EXPECTED_FIXTURE = 'd7c7ba4cadb8c78aeceb9deb496cea0991d7eed549d0a80fb39ed0791b49a89e'
EXPECTED_PAIR = 'aaa6db634208103f2b74188e49a0edaa9fe9aaf2b0d50c14f73e341c746eae52'

def main():
    output = target.ROOT / 'tests/flutter/android-110-stopped-session-valid-before-revoke.json'
    if output.exists():
        raise target.Refusal('checkpoint already exists')
    tool_sha = target.sha(Path(target.__file__).read_bytes())
    if tool_sha != EXPECTED_TOOL:
        raise target.Refusal('approved helper source changed')
    raw = target.FIXTURE.read_bytes()
    if target.sha(raw) != EXPECTED_FIXTURE:
        raise target.Refusal('captured fixture changed')
    cert = target.checked_pair(json.loads(raw))
    if target.pair_fingerprint(cert) != EXPECTED_PAIR:
        raise target.Refusal('captured pair changed')
    target.verify_stopped()
    before_xml, before_pair = target.local_snapshot()
    if before_pair != EXPECTED_PAIR:
        raise target.Refusal('local pair differs from captured pair')
    context = ssl.create_default_context(cafile=str(target.CA))
    opener = urllib.request.build_opener(target.NoRedirects(),
        urllib.request.HTTPSHandler(context=context), urllib.request.ProxyHandler({}))
    checks = {
        'enterprise': target.request(opener, cert, '/enterprise/v1/rtc/events?after=-1', None,
            'stopped-synthetic104-valid-before-revoke-enterprise'),
        'im': target.request(opener, cert, '/api/user/get_users_info', {'userIDs': [target.USER]},
            'stopped-synthetic104-valid-before-revoke-im'),
        'chat': target.request(opener, cert, '/chat/user/find/full', {'userIDs': [target.USER]},
            'stopped-synthetic104-valid-before-revoke-chat'),
    }
    target.verify_stopped()
    after_xml, after_pair = target.local_snapshot()
    result = {'schemaVersion': 1, 'sampledAt': datetime.now(timezone.utc).isoformat(),
        'operation': 'read-only exact stopped UI pair validity before logout',
        'helperSourceSHA256': tool_sha, 'fixtureSHA256': target.sha(raw),
        'pairSHA256': EXPECTED_PAIR, 'userID': target.USER, 'platformID': 2,
        'normalTLSValidation': True, 'caSHA256': target.sha(target.CA.read_bytes()),
        'api': checks, 'localCertificatePreserved': before_xml == after_xml and after_pair == EXPECTED_PAIR,
        'logoutRequestsSent': 0}
    result['passed'] = all(r.get('status') == 200 and r.get('errCode') == 0 for r in checks.values()) and result['localCertificatePreserved']
    with output.open('x', encoding='utf-8') as stream:
        json.dump(result, stream, indent=2); stream.write('\n')
    print(json.dumps({'report': str(output.relative_to(target.ROOT)), 'passed': result['passed'], 'api': checks}))
    return 0 if result['passed'] else 1

if __name__ == '__main__':
    try:
        raise SystemExit(main())
    except Exception as error:
        print(json.dumps({'passed': False, 'errorType': type(error).__name__,
            'detail': str(error) if isinstance(error, target.Refusal) else 'sensitive details suppressed'}))
        raise SystemExit(1)
