"""Verify named delivery inputs and collect the scoped 3.8.6 acceptance evidence."""
import datetime
import hashlib
import json
import pathlib
import re
import ssl
import urllib.request

ROOT = pathlib.Path(__file__).resolve().parents[1]
OUT = ROOT / 'tests/delivery'
BASE = 'https://192.168.10.11:29880'
EXPECTED_INSTALLER = 'b529a90986d4c70176922e3965c1c88d6ef69432743e71a09f7f991a7e0018c3'


def sha(path):
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def load(name):
    return json.loads((ROOT / name).read_text(encoding='utf-8'))


def record(name):
    path = ROOT / name
    return {'path': name, 'bytes': path.stat().st_size, 'sha256': sha(path)}


def main():
    tls = ssl.create_default_context(cafile=str(ROOT / 'artifacts/dev-cert/OpenIM38-Development-CA.crt'))
    http = urllib.request.build_opener(urllib.request.ProxyHandler({}), urllib.request.HTTPSHandler(context=tls))
    checked = []

    def check_http(route, local=None):
        req = urllib.request.Request(BASE + route, headers={'Accept': 'text/html,*/*'})
        with http.open(req, timeout=30) as response:
            data = response.read()
            assert response.status == 200
        digest = hashlib.sha256(data).hexdigest()
        if local:
            assert digest == sha(ROOT / local), route
        checked.append({'route': route, 'bytes': len(data), 'sha256': digest,
                        'matchesLocal': local})
        return data

    index = check_http('/', 'clients/web-desktop/dist-web/index.html').decode()
    check_http('/downloads/', 'artifacts/downloads/index.html')
    check_http('/admin/')
    for route in re.findall(r'(?:src|href)="(/assets/[^\"]+)"', index):
        check_http(route, 'clients/web-desktop/dist-web' + route)
    check_http('/sdk-assets.json', 'clients/web-desktop/dist-web/sdk-assets.json')
    published = load('tests/release-downloads.json')
    assert published['published'] and published['fullHTTPSDownloadsVerified']
    assert len(published['artifacts']) == 1
    artifact = published['artifacts'][0]
    assert artifact['platform'] == 'windows' and artifact['version'] == '3.8.6'
    assert artifact['build'] == 380601 and artifact['force'] is False
    assert artifact['sha256'] == EXPECTED_INSTALLER and artifact['size'] == 160967316
    manifest = load('clients/web-desktop/release-manifest.json')
    assert manifest['version'] == '3.8.6' and manifest['build'] == 380601
    assert manifest['artifacts'][0]['sha256'] == EXPECTED_INSTALLER
    smoke = load('tests/delivery/windows-386-native-smoke.json')
    assert smoke['runtime']['app'] == '3.8.6' and smoke['errors'] == []
    assert smoke['cacheAccess']['status'] == 206
    tests = (OUT / 'windows-386-final-adapter-tests.log').read_text(encoding='utf-8')
    assert '# tests 57' in tests and '# pass 57' in tests and '# fail 0' in tests
    deployment = load('tests/delivery/windows-386-deployment.json')
    assert deployment['scope'] == '/srv/openim38-upgrade/www'
    assert deployment['installer']['sha256'] == EXPECTED_INSTALLER
    frozen = {}
    inputs = [
        ('tests/delivery/windows-386-read-receipts-review.json', 'inputsAfter'),
        ('tests/delivery/windows-386-moments-review.json', 'sourceFiles'),
        ('tests/delivery/windows-386-screenshot-fix-review-final.json', 'files'),
        ('tests/delivery/windows-386-documentation-review.json', 'files'),
    ]
    for name, key in inputs:
        for item in load(name)[key]:
            assert sha(ROOT / item['path']) == item['sha256'], item['path']
            frozen[item['path']] = record(item['path'])
    asar = load('tests/delivery/windows-386-screenshot-packaging-review.json')['asar']
    assert sha(pathlib.Path(asar['path'])) == asar['sha256']
    evidence_names = [
        'windows-386-final-adapter-tests.log', 'windows-386-final-build.log',
        'windows-386-read-receipts-review.json', 'windows-386-read-runtime-review.json',
        'windows-386-read-object-error-diagnostic-review.json',
        'windows-386-moments-review.json', 'windows-386-moments-ui/report.json',
        'windows-386-screenshot-fix-review-final.json',
        'windows-386-screenshot-packaging-review.json', 'windows-386-native-smoke.json',
        'windows-386-native-smoke-run.log', 'windows-386-native-smoke-process.log',
        'windows-386-installer-fixture-review.json',
        'windows-386-documentation-review.json', 'windows-386-web-bundle.json',
        'windows-386-deployment.json',
    ]
    evidence = [record('tests/delivery/' + name) for name in evidence_names]
    evidence.append(record('tests/release-downloads.json'))
    # Exact-match secrets are read in memory only. No value or matching text is exported.
    private = load('.private/test-accounts.json')
    secrets = set()

    def collect(value):
        if isinstance(value, dict):
            for key, child in value.items():
                if re.search(r'password|token|secret', key, re.I) and isinstance(child, str) and len(child) >= 8:
                    secrets.add(child)
                    if 'password' in key.lower():
                        secrets.add(hashlib.md5(child.encode()).hexdigest())
                else:
                    collect(child)
        elif isinstance(value, list):
            for child in value:
                collect(child)
    collect(private)
    text_names = {item['path'] for item in evidence}
    text_extensions = {'.json', '.log', '.txt', '.md', '.html', '.ts', '.tsx', '.js', '.mjs', '.patch', '.py', '.ps1'}
    text_names.update(item['path'] for item in frozen.values()
                      if pathlib.PurePosixPath(item['path']).suffix in text_extensions)
    text_names.update(['tools/prepare_windows386_delivery.py', 'tools/deploy_windows386_isolated.py',
                       'tools/finalize_windows386_delivery.py', 'tools/release_manifest.py'])
    findings, text_bytes = [], 0
    for name in sorted(text_names):
        data = (ROOT / name).read_bytes()
        text = data.decode('utf-8')
        text_bytes += len(data)
        if any(value in text for value in secrets) or re.search(r'eyJ[A-Za-z0-9_-]{12,}\.[A-Za-z0-9_-]{12,}\.[A-Za-z0-9_-]{12,}', text):
            findings.append(name)
    assert not findings, 'Public evidence credential scan failed; see file names only: ' + ', '.join(findings)
    result = {
        'utc': datetime.datetime.now(datetime.timezone.utc).isoformat(),
        'identity': 'OpenIM original enterprise upgrade; four reported PC/Web fixes',
        'application': {'version': '3.8.6', 'build': 380601, 'sdk': '3.8.3-patch.15.1'},
        'status': 'PASS_FOR_SCOPED_FOUR_FIXES', 'fullUpgradeAccepted': False,
        'installer': record('clients/web-desktop/release/OpenIM38-Enterprise-3.8.6-win-x64.exe'),
        'acceptance': {
            'typeScript': 'Passed during implementation/build verification',
            'adapterTests': {'passed': 57, 'failed': 0},
            'readReceipts': 'Real WASM and normal Electron window: foreground read; two messages stay unread through tray-hidden sampling; restore reads both. Passed twice.',
            'workCircle': 'Seven original-UI cases passed, own mutations immediate; peer events and fallback verified (5062 ms observed).',
            'screenshots': 'Real Electron sandbox preload/IPC and selection toolbar with synthetic screenshot input; confirm/cancel, duplicate events and restore passed. Final ASAR patch verified.',
            'packagedSmoke': 'Actual final packaged executable: version, normal TLS, WASM/SQLite startup, managed local files and HTTP Range passed.',
            'delivery': 'New installer full HTTPS bytes/SHA verified and optional Windows update published; Web/resources/download page hashes match deployed content.',
        },
        'normalTLSHttpChecks': checked, 'sourceAndDocumentationFrozenInputs': list(frozen.values()),
        'evidence': evidence,
        'finitePublicTextCredentialScan': {'fileCount': len(text_names), 'bytes': text_bytes,
            'findings': [], 'scope': 'Exactly the named UTF-8 inputs; no binary/OCR/full-project claim.'},
        'limits': [
            'Previously recorded read receipts cannot be rolled back; validate new messages.',
            'Deliberate Mark as read remains available. Native minimize/unfocused cases have behavior coverage, not a separate final native GUI run.',
            'One earlier Object pageerror remains unattributed. A single 30.357-second diagnostic rerun passed the same core flow with zero pageerrors/exceptions; this is not proof of a fix or harmlessness.',
            'Read/work-circle UI tests used current source; screenshot GUI used synthetic input. Packaged smoke and ASAR verification are separate evidence, not full NSIS installed-login acceptance or all display hardware tests.',
            'Windows test installer is unsigned. No in-place user-data upgrade acceptance was added in this task.',
            'No Android or public release, no real business data migration. Audio/video/calls/meetings remain deferred per user instruction.',
        ],
    }
    (OUT / 'windows-386-delivery.json').write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'result': result['status'], 'httpChecks': len(checked),
                      'frozenInputs': len(frozen), 'credentialScanFiles': len(text_names),
                      'credentialMatches': len(findings), 'installerSha256': EXPECTED_INSTALLER}))


if __name__ == '__main__':
    main()
