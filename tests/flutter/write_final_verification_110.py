"""Bind the authorized110 source change, complete tests and private build target."""
from pathlib import Path
import datetime
import hashlib
import json
import re

root = Path(__file__).absolute().parents[2]
directory = root / 'tests/flutter'
client = root / 'clients/flutter'


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def reference(name):
    path = root / name
    return {'path': name, 'sha256': sha(path), 'bytes': path.stat().st_size}


destination = directory / 'final-verification-110.json'
if destination.exists():
    raise RuntimeError('Preserve final verification evidence')
old = json.loads((directory / 'runtime-source-snapshot-109.json').read_text(encoding='utf-8'))
new = json.loads((directory / 'runtime-source-snapshot-110.json').read_text(encoding='utf-8'))
analysis = json.loads((directory / 'analysis-scope-110.json').read_text(encoding='utf-8'))
native = json.loads((directory / 'native-source-check-110.json').read_text(encoding='utf-8'))
compat = json.loads((client / 'local_plugin/compat/PATCH-MANIFEST.json').read_text(encoding='utf-8'))
log = (directory / 'unit-tests-110-final.log').read_text(encoding='utf-8', errors='replace')
marker = re.search(r'\+(\d+):\s+All tests passed!', log)
assert marker and int(marker[1]) == 134
assert analysis['counts']['error'] == 0 and len(analysis['analysisTargets']) == 19
assert native['inputCount'] == 95 and native['beforeAfterIdentical']
assert len(native['commands']) == 2 and all(row['exit'] == 0 for row in native['commands'])
assert '> Task :flutter_webrtc:compileReleaseJavaWithJavac\n' in (directory / 'native-android-javac-110.log').read_text(encoding='utf-8', errors='replace')
changed = sorted(key for key in new['files'].keys() & old['files'].keys() if new['files'][key] != old['files'][key])
added = sorted(new['files'].keys() - old['files'].keys())
removed = sorted(old['files'].keys() - new['files'].keys())
assert len(changed) == 8 and not added and not removed
delta = {'priorManifest': 'tests/flutter/runtime-source-snapshot-109.json',
         'newManifest': 'tests/flutter/runtime-source-snapshot-110.json',
         'priorSourceSHA256': old['sourceSHA256'], 'newSourceSHA256': new['sourceSHA256'],
         'changedInputs': changed, 'addedInputs': added, 'removedInputs': removed,
         'scopeCorrection': False,
         'scope': 'Same full runtime3229 scope. Bugly implementation, four Java observations, analysis script target list and app/iOS version metadata only. No SDK/AAR/media ordering or timeout change.'}
(directory / 'runtime-source-delta-109-110.json').write_text(json.dumps(delta, indent=2) + '\n', encoding='utf-8')
checks = [reference('tests/flutter/' + name) for name in [
    'unit-tests-110-final.log', 'analyze-110-final.log', 'pub-get-110.log',
    'native-java-formal-110.log', 'native-android-javac-110.log', 'native-source-check-110.json']]
verification = {
    'applicationVersion': '3.8.10+110', 'generatedAt': datetime.datetime.now(datetime.timezone.utc).isoformat(),
    'status': 'source_checks_passed_pending_independent_freeze_and_private_x64_build',
    'flutterTests': {'passed': 134, 'log': 'tests/flutter/unit-tests-110-final.log',
                     'scope': 'Complete test directory; includes11 actual flutter_bugly package tests counted once.'},
    'analyzer': {'errors': analysis['counts']['error'], 'warnings': analysis['counts']['warning'],
                 'info': analysis['counts']['info'], 'analysisExit': analysis['analysisExit'], 'cleanLint': False,
                 'targets': analysis['analysisTargets'], 'analysisScope': 'tests/flutter/analysis-scope-110.json',
                 'log': 'tests/flutter/analyze-110-final.log',
                 'scopeNote': '19targets; actual Bugly implementation leaf added to10918targets.'},
    'javaBoundaryCases': 88,
    'javaVerification': {'rerunPerformed': True, 'sourceCheck': 'tests/flutter/native-source-check-110.json',
                         'actualAndroidClasspathJavacPassed': True, 'inputCount': 95,
                         'allBeforeAfterInputsUnchanged': True,
                         'boundary': 'Actual production Renderer/Binding/Owner with controlled Android/JNI/EGL boundaries; original shared-track case additionally checks Java object-hash correlation fields and no extra track.id() queries. Not device acceptance.'},
    'nativeObservation': {
        'productionFilesChanged': 4, 'testOrStubFilesChanged': 3,
        'newMediaJniCalls': 0, 'newThreads': 0, 'schedulingChanged': False, 'timeoutBudgetChanged': False,
        'identityHashBoundary': 'Correlation hints can collide; equal hashes alone do not prove one Java wrapper or one C++ track. No global identity registry or retained history was added.',
        'lockBoundary': 'No new lock and no I/O in lease/renderer metadata monitors. Exact remote Java removeSink wrapper begin/end are an explicitly accepted exception inside the pre-existing exclusiveNative/nativeCall physical serial region, where existing owner action timing already runs. No callback or scheduling change was added to move logs.',
        'timingBoundary': 'Java removeSink includes its nativeRemoveSink/nativeFreeSink path and diagnostic overhead. Elapsed is not pure JNI timing or a zero-perturbation measurement.',
        'cameraStateRisk': 'Pre-existing CameraEventsHandler nonvolatile polling is recorded as a separate visibility risk; unchanged in110 and not established as the removeSink wait cause.'},
    'sourceReviews': [reference('tests/flutter/bugly-product-integration-review-110.json')],
    'sourceManifest': 'tests/flutter/runtime-source-snapshot-110.json', 'sourceSHA256': new['sourceSHA256'],
    'sourceFiles': new['fileCount'], 'runtimeSourceDelta': reference('tests/flutter/runtime-source-delta-109-110.json'),
    'independentSdkSource': reference('tests/flutter/sdk-source-snapshot-110.json'),
    'localPathDependencyCoverage': reference('tests/flutter/path-dependency-coverage-110.json'),
    'compatibilityPackages': len(compat['packages']),
    'compatibilityModifiedFiles': sum(len(item['modifiedFiles']) for item in compat['packages']),
    'priorPassedPartialUI': 'tests/flutter/original-ui-group-read-report-109.json',
    'priorRuntimeFailure': 'tests/flutter/original-ui-rtc-report-108.json',
    'candidateBuildTarget': {'abi': 'x86_64', 'entrypoint': 'lib/main.dart', 'privateOnly': True, 'published': False},
    'runtimeAccepted': False, 'physicalARM64DeviceValidated': False, 'iOSXcodeBuilt': False, 'logs': checks,
}
destination.write_text(json.dumps(verification, indent=2) + '\n', encoding='utf-8')
path = directory / 'build-provenance.json'
provenance = json.loads(path.read_text(encoding='utf-8-sig'))
provenance['android'].update({'applicationVersion': '3.8.10', 'applicationBuild': 110,
                              'privateCandidateArtifactManifest': 'tests/flutter/android-main-x64-110.json',
                              'currentCandidateBuilt': False, 'currentCandidatePublished': False})
provenance['verificationStatus'] = '110134Flutter/19scope0error and88native tests plus real Android-classpathJavac passed. Four Java changes add observations only; private x64 normal-main build/runtime pending. No Android release published.'
provenance['verification'] = verification
provenance['runtimeSourceManifest'] = verification['sourceManifest']
provenance['flutterOpenimSDK']['sourceManifest'] = 'tests/flutter/sdk-source-snapshot-110.json'
provenance['localPathDependencyCoverage'] = 'tests/flutter/path-dependency-coverage-110.json'
provenance['runtimeEvidence'] = {'latestAcceptedScopedOriginalUI': 'tests/flutter/original-ui-group-read-report-109.json',
                                 'latestOriginalFormLogin': 'tests/flutter/android-109-x64-original-login-passed.json',
                                 'latestFailedRTC': 'tests/flutter/original-ui-rtc-report-108.json',
                                 'new110Accepted': False}
path.write_text(json.dumps(provenance, indent=2) + '\n', encoding='utf-8')
print(json.dumps({'version': '3.8.10+110', 'sourceFiles': new['fileCount'], 'sourceSHA256': new['sourceSHA256'],
                  'changed': len(changed), 'new': len(added), 'removed': len(removed), 'flutterTests': 134,
                  'analyzer': analysis['counts'], 'nativeRerun': 88, 'compatModified': verification['compatibilityModifiedFiles'],
                  'privateBuildPending': True}))
