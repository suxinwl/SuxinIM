"""Bind the Windows 3.8.7 source, runtime and isolated delivery evidence."""
import datetime
import hashlib
import json
import pathlib
import re
import ssl
import urllib.request

ROOT = pathlib.Path(__file__).resolve().parents[1]


def sha(path):
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def read(relative):
    return json.loads((ROOT / relative).read_text(encoding='utf-8'))


def record(relative):
    path = ROOT / relative
    return {'path': relative, 'bytes': path.stat().st_size, 'sha256': sha(path)}


def main():
    current = read('clients/web-desktop/build-version.json')
    assert current == {'version': '3.8.7', 'build': 380701}
    bundle = read('tests/delivery/windows-387-web-bundle.json')
    deployed = read('tests/delivery/windows-387-deployment.json')
    released = read('tests/release-downloads.json')
    package = bundle['installer']
    assert package == deployed['installer']
    assert sha(ROOT / 'clients/web-desktop/release' / package['name']) == package['sha256']
    assert released['published'] and released['fullHTTPSDownloadsVerified']
    release = next(a for a in released['artifacts'] if a['platform'] == 'windows')
    assert release['sha256'] == package['sha256'] and release['build'] == current['build']
    assert release['size'] == package['bytes'] and not release['force']
    smoke = read('tests/delivery/windows-387-native-smoke.json')
    assert smoke['runtime']['app'] == '3.8.7' and smoke['platform'] == 3
    assert smoke['isolated'] and not smoke['errors'] and smoke['cacheAccess']['status'] == 206
    tests = (ROOT / 'tests/delivery/windows-387-final-adapter-tests.log').read_text(encoding='utf-8')
    assert all(re.search(r'^' + re.escape(line) + '$', tests, re.M) for line in ['# tests 94', '# pass 94', '# fail 0', '# skipped 0'])

    tls = ssl.create_default_context(cafile=str(ROOT / 'artifacts/dev-cert/OpenIM38-Development-CA.crt'))
    http = urllib.request.build_opener(urllib.request.ProxyHandler({}), urllib.request.HTTPSHandler(context=tls))
    base = 'https://192.168.10.11:29880'
    verified = []
    def verify(relative, path):
        with http.open(base + path, timeout=30) as response:
            body = response.read()
            assert response.status == 200
        expected = (ROOT / relative).read_bytes()
        assert body == expected
        verified.append({'url': base + path, 'bytes': len(body), 'sha256': hashlib.sha256(body).hexdigest()})
        return body
    index = verify('clients/web-desktop/dist-web/index.html', '/')
    verify('artifacts/downloads/index.html', '/downloads/')
    modules = re.findall(r'<script[^>]+src="([^"]+)"', index.decode('utf-8'))
    assert modules
    for url in modules:
        path = pathlib.PurePosixPath(url.removeprefix('/')).as_posix()
        assert '..' not in pathlib.PurePosixPath(path).parts
        assert path in {item['path'] for item in bundle['files']}
        verify('clients/web-desktop/dist-web/' + path, '/' + path)

    evidence = [
        'tests/delivery/windows-387-actual-client-panic-review.json',
        'tests/delivery/windows-387-actual-client-panic-followup.json',
        'tests/delivery/windows-387-quote-facade-review.json',
        'tests/delivery/windows-387-forward-native-frozen-386-1793555657332/report.json',
        'tests/delivery/windows-387-forward-native-386-quote-panic-1793556107285/report.json',
        'tests/delivery/windows-387-forward-native-fixed-quote-1793556273854/report.json',
        'tests/delivery/windows-387-search-review.json',
        'tests/delivery/windows-387-video-download-review.json',
        'tests/delivery/windows-387-final-adapter-tests.log',
        'tests/delivery/windows-387-final-typecheck.log',
        'tests/delivery/windows-387-build.log',
        'tests/delivery/windows-387-native-smoke.json',
        'tests/delivery/windows-387-packaging-review.json',
        'tests/delivery/windows-387-documentation-review.json',
        'tests/delivery/windows-387-web-bundle.json',
        'tests/delivery/windows-387-deployment.json',
        'tests/release-downloads.json',
    ]
    sources = [
        'src/im/send-message.ts', 'src/im/forward-batch.ts', 'src/im/ImFacade.ts',
        'src/im/message-history.ts', 'src/im/attachment-download.ts',
        'src/pages/common/ChooseModal/index.tsx',
        'src/pages/chat/queryChat/ChatFooter/useSendMessage.ts',
        'src/pages/common/GlobalSearchModal/index.tsx',
        'src/pages/common/GlobalSearchModal/index.module.scss',
        'src/pages/common/GlobalSearchModal/DashboardPanel.tsx',
        'src/pages/common/GlobalSearchModal/ChatLogsPanel.tsx',
        'src/components/JumpToMessageWrap/index.tsx',
        'src/pages/chat/queryChat/ChatContent.tsx', 'src/pages/chat/queryChat/index.tsx',
        'src/store/message.ts', 'src/utils/common.ts', 'src/layout/useGlobalEvents.tsx',
        'electron/main/downloadManage.ts', 'electron/main/downloadTarget.ts',
    ]
    result = {
        'createdUtc': datetime.datetime.now(datetime.timezone.utc).isoformat(),
        'status': 'PASS_WINDOWS_387_ISOLATED_DELIVERY',
        'version': current, 'installer': package,
        'fixes': {
            'forwarding': 'The existing client log confirms the Go Quote-send nil dereference. A normal Electron synthetic Quote-forward reproduced it on 3.8.6; the fixed official sendMessageNotOss route preserves quoted content and keeps subsequent SDK calls usable. Ordinary text forwarding also passed. Batches are awaited, deduplicated and retry confirmed failures only.',
            'search': 'Fix initial empty-result closures, zero-height result lists, stale anchors and explicit Search-view pagination. Actual original Web UI and SDK searches locate messages in the correct conversation.',
            'videoDownload': 'Keep the original signed URL unchanged; pass download metadata separately, await IPC errors and enable retry. Real Electron verifies MP4 bytes, SHA, managed-file range access and playback.',
        },
        'validation': {'adapterTestsPassed': 94, 'adapterTestsFailed': 0, 'typescript': 'PASS',
                       'packagedNativeSmoke': 'PASS', 'packagedSourceAudit': 'PASS', 'normalTLSFiles': verified,
                       'fullInstallerHTTPSVerified': True, 'optionalUpdatePublished': True},
        'localExistingClient': {'inspectedReadOnly': True, 'replacementInstallationVerified': False,
                                'stateAtInspection': '3.8.6 window alive; Go message engine exited after a send panic.',
                                'restart': 'Pending the user response to the installation/restart question; no existing client or data was closed or reset.'},
        'sources': [record('clients/web-desktop/' + name) for name in sources],
        'evidence': [record(name) for name in evidence],
        'previousMetadata': bundle['metadataHistory'],
        'serverBackup': deployed['backup'],
        'limits': [
            'Windows installer is unsigned. No in-place replacement of the existing local user client has been verified.',
            'Quote and ordinary-text forwarding runtime tests used synthetic accounts in normal Electron against the isolated server; no actual-user messages were sent.',
            'The video test used a hidden native harness executing original renderer/main download modules; it is not a claimed final-installed original-chat UI click test.',
            'Two generic Object page exceptions also occurred in the search baseline and are retained in the search report.',
            'Audio/video/meeting work is resumed separately. Android114 source/Java tests do not prove device media teardown, ARM64 hardware acceptance or iOS Xcode build.',
            'No production instance, public mapping, HRM integration or actual business data was changed. The full upgrade is not marked accepted.',
        ],
    }
    output = ROOT / 'tests/delivery/windows-387-delivery.json'
    assert not output.exists(), 'Preserve an existing report before regenerating it.'
    output.write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'report': str(output.relative_to(ROOT)), 'sha256': sha(output), 'windowsVersion': current['version'], 'normalTLSFilesVerified': len(verified)}))


if __name__ == '__main__':
    main()
