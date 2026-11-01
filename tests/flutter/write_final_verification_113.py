"""Record the explicit nonmedia scope without promoting historical media results."""
from pathlib import Path
from datetime import datetime, timezone
import hashlib
import json

ROOT = Path('O:/')
OUT = ROOT / 'tests/flutter'


def read(path):
    return json.loads((ROOT / path).read_text(encoding='utf-8'))


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def ref(path):
    p = ROOT / path
    return {'path': path, 'sha256': sha(p), 'bytes': p.stat().st_size}


result = read('tests/flutter/nonmedia-113-inputs-after.json')
assert result['allSelectedTestsPassed'] and result['allInputsUnchanged']
assert sum(x['passed'] for x in result['results']) == 42
analysis = read('tests/flutter/analysis-scope-113.json')
assert analysis['counts']['error'] == 0 and len(analysis['analysisTargets']) == 19
source = read('tests/flutter/runtime-source-snapshot-113.json')
sdk = read('tests/flutter/sdk-source-snapshot-113.json')
native = read('tests/flutter/native-source-check-111.json')
for item in native['before']:
    p = ROOT / item['path']
    assert p.stat().st_size == item['bytes'] and sha(p) == item['sha256'], item['path']
reuse = {
    'applicationVersion': '3.8.13+113',
    'createdAt': datetime.now(timezone.utc).isoformat(),
    'testsExecutedThisRun': False,
    'reason': 'User explicitly skips video, voice, meeting and call verification, including native lifecycle tests.',
    'unchangedInputCount': len(native['before']),
    'all99CurrentHashesAndSizesMatch111': True,
    'sourceCheck': ref('tests/flutter/native-source-check-111.json'),
    'historicalJavaCasesPassed': 94,
    'historicalJavaLog': ref('tests/flutter/native-java-formal-111.log'),
    'historicalAndroidClasspathJavacLog': ref('tests/flutter/native-android-javac-111.log'),
    'limits': 'Unchanged-input historical reuse only. No113 Java rerun or media/device acceptance claim.',
}
p = OUT / 'native-verification-reuse-113.json'
assert not p.exists()
p.write_text(json.dumps(reuse, indent=2) + '\n', encoding='utf-8')
compat = read('clients/flutter/local_plugin/compat/PATCH-MANIFEST.json')
verification = {
    'applicationVersion': '3.8.13+113',
    'generatedAt': datetime.now(timezone.utc).isoformat(),
    'status': 'nonmedia_source_checks_passed_local_candidates_pending_not_published',
    'userRequestedMediaSkip': True,
    'unfinished112Archive': ref('tests/flutter/source-history/111-d4420474/112-unfinished-at-user-skip-20261101T144042Z/manifest.json'),
    'restored111Proof': ref('tests/flutter/112-restored-to-111-proof.json'),
    'restored111Verification': ref('tests/flutter/112-restored-111-verification.json'),
    'flutterTests': {
        'passedThisRun': 42, 'scope': 'Explicit nonmedia selection; never the full test directory.',
        'selection': ref('tests/flutter/nonmedia-selection-113.json'),
        'logs': [ref('tests/flutter/nonmedia-113-part1.log'), ref('tests/flutter/nonmedia-113-part2.log')],
        'before': ref('tests/flutter/nonmedia-113-inputs-before.json'),
        'after': ref('tests/flutter/nonmedia-113-inputs-after.json'),
        'inputCount': result['fileCount'], 'allInputsUnchanged': True,
        'fullSuiteExecuted': False,
        'skipped': 'All remaining signaling, RTC, renderer, audio, meeting, call and wakelock tests; unfinished112 tests archived.',
    },
    'analyzer': {
        **analysis['counts'], 'analysisExit': analysis['analysisExit'], 'cleanLint': False,
        'targets': analysis['analysisTargets'], 'scope': ref('tests/flutter/analysis-scope-113.json'),
        'log': ref('tests/flutter/analyze-113-final.log'),
        'limits': 'Static same19scope analysis, including existing media source leaves; no media test execution.',
    },
    'javaTestsExecutedThisRun': 0,
    'unchangedNativeHistoricalReuse': ref('tests/flutter/native-verification-reuse-113.json'),
    'sourceManifest': ref('tests/flutter/runtime-source-snapshot-113.json'),
    'sourceSHA256': source['sourceSHA256'], 'sourceFiles': source['fileCount'],
    'runtimeSourceDelta': ref('tests/flutter/runtime-source-delta-111-113.json'),
    'independentSdkSource': ref('tests/flutter/sdk-source-snapshot-113.json'),
    'localPathDependencyCoverage': ref('tests/flutter/path-dependency-coverage-113.json'),
    'compatibilityPackages': len(compat['packages']),
    'compatibilityModifiedFiles': sum(len(x['modifiedFiles']) for x in compat['packages']),
    'compatibilityManifestUnchangedFrom111': ref('clients/flutter/local_plugin/compat/PATCH-MANIFEST.json'),
    'aarManifestUnchanged': ref('tests/flutter/resolved-aar-hashes.json'),
    'candidateTargets': {
        'entrypoint': 'lib/main.dart', 'flavor': 'development', 'sequentialABIs': ['x86_64', 'arm64-v8a'],
        'x64': 'artifacts/android-candidates/OpenIM38-3.8.13+113-development-x64-candidate.apk',
        'arm64': 'artifacts/android-candidates/OpenIM38-3.8.13+113-development-arm64-candidate.apk',
        'formalPathMustRemainAbsent': 'artifacts/android/OpenIM38-3.8.13+113-development-arm64.apk',
        'published': False, 'httpDownloadOffered': False,
    },
    'knownLimits': [
        '110 group72.313s cleanup failure and first meeting interruption remain unresolved historical evidence.',
        'Meeting detail111 baseline has no authoritative Ended branch; unfinished112 fix is archived.',
        'No113 media runtime acceptance; user asked to skip video, voice, meetings and calls.',
        'No physical ARM64 phone available; no repeated ARM translation SIGILL test.',
        'Development Go AAR trust on ordinary physical phones remains unverified.',
        'iOS source version synchronized; no Xcode build on Windows.',
    ],
    'runtimeAccepted': False, 'runtimeRTCAccepted': False, 'releaseAccepted': False,
    'physicalARM64DeviceValidated': False, 'iOSXcodeBuilt': False,
}
p = OUT / 'final-verification-113.json'
assert not p.exists()
p.write_text(json.dumps(verification, indent=2) + '\n', encoding='utf-8')
provenance = read('tests/flutter/build-provenance.json')
provenance.setdefault('verificationHistory', {})['111'] = provenance['verification']
provenance['verificationHistory']['112'] = {
    'status': 'unfinished_archived_at_user_media_skip_not_built',
    'archive': verification['unfinished112Archive'], 'sourceRestoredTo111Before113': True,
}
provenance['verification'] = verification
provenance['verificationStatus'] = '113 version-only from exact restored111;42 explicit nonmedia tests passed,19scope analyzer0errors. Media/native tests skipped by user; local normal-main x64/ARM64 builds pending; not published or fully accepted.'
provenance['runtimeSourceManifest'] = 'tests/flutter/runtime-source-snapshot-113.json'
provenance['localPathDependencyCoverage'] = 'tests/flutter/path-dependency-coverage-113.json'
provenance['flutterOpenimSDK']['sourceManifest'] = 'tests/flutter/sdk-source-snapshot-113.json'
provenance['android'].update({
    'applicationVersion': '3.8.13', 'applicationBuild': 113,
    'privateCandidateArtifactManifest': 'tests/flutter/android-main-x64-113.json',
    'localArm64CandidateArtifactManifest': 'tests/flutter/android-main-arm64-113.json',
    'currentCandidateBuilt': False, 'currentCandidatePublished': False,
    'deliveryEntrypoint': 'lib/main.dart',
})
(OUT / 'build-provenance.json').write_text(json.dumps(provenance, indent=2) + '\n', encoding='utf-8')
print(json.dumps({'sourceFiles': source['fileCount'], 'sourceSHA256': source['sourceSHA256'],
                  'sdkFiles': sdk['fileCount'], 'testsThisRun': 42, 'javaThisRun': 0,
                  'compatPackages': len(compat['packages']),
                  'modifiedFiles': verification['compatibilityModifiedFiles']}))
