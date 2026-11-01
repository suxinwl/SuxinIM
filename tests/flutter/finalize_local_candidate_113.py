"""Inspect and copy one113 local candidate, preserving every preceding artifact."""
from pathlib import Path
from datetime import datetime, timezone
import hashlib
import json
import re
import shutil
import subprocess
import sys
import zipfile

ROOT = Path('O:/')
OUT = ROOT / 'tests/flutter'
APK = ROOT / 'clients/flutter/build/app/outputs/flutter-apk/app-development-release.apk'
CERT = '028253da3ebcb424a5e2e7ae73dddc88fc508a2ce3ccccedd1193dd4f65de8c1'
kind = sys.argv[1]
assert kind in ['x64', 'arm64']
abi = {'x64': 'x86_64', 'arm64': 'arm64-v8a'}[kind]
prefix = f'android-main-{kind}-113'


def sha(path):
    return hashlib.file_digest(path.open('rb'), 'sha256').hexdigest()


def read(path):
    return json.loads(path.read_text(encoding='utf-8-sig'))


def ref(path):
    return {'path': path.relative_to(ROOT).as_posix(), 'sha256': sha(path), 'bytes': path.stat().st_size}


assert not (OUT / (prefix + '.json')).exists()
assert read(OUT / (prefix + '-build-process-result.json'))['exitCode'] == 0
subprocess.run([sys.executable, str(OUT / 'snapshot_runtime_sources.py'), '--verify'], cwd=ROOT, check=True)
subprocess.run([sys.executable, str(OUT / 'audit_path_dependency_coverage.py'), '--build', '113', '--check-only'], cwd=ROOT, check=True)
source = read(OUT / 'runtime-source-snapshot-113.json')
sdk = read(OUT / 'sdk-source-snapshot-113.json')
inputs = read(OUT / 'nonmedia-113-inputs-after.json')['files']
native = read(OUT / 'native-source-check-111.json')['before']
for name, expected in {**source['files'], **sdk['files'], **inputs}.items():
    assert sha(ROOT / name) == expected, name
for item in native:
    assert sha(ROOT / item['path']) == item['sha256'], item['path']

badging = subprocess.check_output(['E:/AndroidDev/sdk/build-tools/34.0.0/aapt.exe', 'dump', 'badging', str(APK)])
text = badging.decode('utf-8', errors='replace')
assert re.search(r"package: name='io\.openim\.upgrade38\.test\.dev' versionCode='113' versionName='3\.8\.13'", text)
assert "sdkVersion:'24'" in text and "targetSdkVersion:'35'" in text
native_line = next(line for line in text.splitlines() if line.startswith('native-code:'))
assert native_line == "native-code: '" + abi + "'"
sign = subprocess.check_output(['E:/AndroidDev/jdk/jdk-17.0.20.1+1/bin/java.exe', '-jar',
                               'E:/AndroidDev/sdk/build-tools/34.0.0/lib/apksigner.jar',
                               'verify', '--verbose', '--print-certs', str(APK)])
sig = sign.decode('utf-8', errors='replace')
assert 'Verified using v2 scheme (APK Signature Scheme v2): true' in sig and CERT in sig.lower()
(OUT / (prefix + '-badging.txt')).open('xb').write(badging)
(OUT / (prefix + '-signature.log')).open('xb').write(sign)
previous_path = OUT / ('android-main-x64-111.json' if kind == 'x64' else 'android-main-arm64-109.json')
previous = read(previous_path)
with zipfile.ZipFile(APK) as archive:
    names = sorted(name for name in archive.namelist() if name.startswith('lib/') and name.endswith('.so'))
    assert len(names) == (11 if kind == 'x64' else 13)
    assert all(name.startswith('lib/' + abi + '/') for name in names)
    libraries = {name: {'size': archive.getinfo(name).file_size,
                        'sha256': hashlib.sha256(archive.read(name)).hexdigest()} for name in names}
    dependencies = [name for name in names if not name.endswith('/libapp.so')]
    assert all(libraries[name] == previous['nativeLibraries'][name] for name in dependencies)
    dex = b''.join(archive.read(name) for name in archive.namelist() if re.fullmatch(r'classes\d*\.dex', name))
    markers = {name: name.encode() in dex for name in ['javaRemoveSinkBegin', 'javaRemoveSinkEnd', 'leaseAssociation', 'camera.waitForClosed', 'substepBegin', 'wrapperJava#']}
    assert all(markers.values())

digest = sha(APK)
candidate = ROOT / f'artifacts/android-candidates/OpenIM38-3.8.13+113-development-{kind}-candidate.apk'
assert not candidate.exists()
assert not (ROOT / 'artifacts/android/OpenIM38-3.8.13+113-development-arm64.apk').exists()
candidate.parent.mkdir(parents=True, exist_ok=True)
shutil.copy2(APK, candidate)
assert sha(candidate) == digest
maps = ROOT / f'.private/android-build-maps/113-{kind}-{digest[:8]}'
maps.mkdir(parents=True, exist_ok=False)
mapping_source = ROOT / 'clients/flutter/build/app/outputs/mapping/developmentRelease'
mapping_files = []
for original in sorted(mapping_source.rglob('*')):
    if original.is_file():
        target = maps / original.relative_to(mapping_source)
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(original, target)
        assert sha(original) == sha(target)
        mapping_files.append(ref(target))
assert mapping_files
(maps / 'manifest.json').write_text(json.dumps({'APK_SHA256': digest, 'sourceSHA256': source['sourceSHA256'],
                                               'files': mapping_files}, indent=2) + '\n', encoding='utf-8')
metadata = {
    'recordedAt': datetime.now(timezone.utc).isoformat(),
    'applicationVersion': '3.8.13', 'applicationBuild': 113,
    'applicationID': 'io.openim.upgrade38.test.dev',
    'artifact': candidate.relative_to(ROOT).as_posix(), 'sha256': digest, 'size': candidate.stat().st_size,
    'normalOutputAtInspection': APK.relative_to(ROOT).as_posix(), 'normalOutputSHA256AtInspection': digest,
    'abis': [abi], 'minSDK': 24, 'targetSDK': 35,
    'signatureVerified': True, 'signatureScheme': 'v2', 'signingCertificateSHA256': CERT,
    'entrypoint': 'lib/main.dart', 'flavor': 'development', 'validationWrapper': False,
    'entrypointEvidence': ref(OUT / (prefix + '-build-start.json')),
    'buildLog': ref(OUT / (prefix + '-frozen-build.log')),
    'sourceSHA256': source['sourceSHA256'], 'sourceFiles': source['fileCount'],
    'sourceManifest': ref(OUT / 'runtime-source-snapshot-113.json'),
    'SDKSourceManifest': ref(OUT / 'sdk-source-snapshot-113.json'),
    'SDKFilesVerifiedBeforeAndAfterBuild': 194, 'DartTestInputsVerifiedBeforeAndAfterBuild': len(inputs),
    'NativeInputsVerifiedBeforeAndAfterBuild': len(native), 'sourceInputsChangedByBuild': [],
    'nativeLibraries': libraries,
    'dependencyComparison': {'previous': ref(previous_path), 'unchangedPayloads': len(dependencies),
                             'libappComparedSeparately': True},
    'retainedNativeObservationMarkers': markers, 'markersAreNotExecutionEvidence': True,
    'privateMappingArchive': ref(maps / 'manifest.json'),
    'nonmediaTestsPassedThisRun': 42, 'nonmediaSelection': ref(OUT / 'nonmedia-selection-113.json'),
    'userRequestedMediaSkip': True,
    'skippedValidation': ['Video', 'Voice', 'Calls', 'Meetings', 'Audio/ringtone', 'RTC/renderer/IO-Signal test suites',
                          'Native Java tests (unchanged111 history only)', 'ARM translation runtime retry', 'Physical ARM64 runtime'],
    'knownLimits': [
        'Unresolved110 group cleanup72.313s failure and first meeting interruption remain historical failures.',
        '111 meeting detail baseline has no authoritative Ended branch; unfinished112 changes are archived.',
        'Ordinary physical-phone Go SDK development-CA trust remains unverified.',
        'No physical ARM64 device available; ARM translation SIGILL is not a physical-device result.',
    ],
    'localCandidateOnly': True, 'published': False, 'releaseAccepted': False,
    'runtimeAccepted': False, 'runtimeRTCAccepted': False, 'physicalARM64DeviceValidated': False,
    'formalArtifactPathAbsent': True,
    'noUploadNoChatPolicyMutation': True,
    'trustBoundary': 'Development CA configuration retained; no TLS verification bypass.',
}
sidecar = candidate.with_suffix('.json')
sidecar.open('x', encoding='utf-8').write(json.dumps(metadata, indent=2) + '\n')
(OUT / (prefix + '.json')).open('x', encoding='utf-8').write(json.dumps({**metadata, 'candidateSidecar': ref(sidecar)}, indent=2) + '\n')
(OUT / (prefix + '-source-postverification.json')).open('x', encoding='utf-8').write(json.dumps({
    'verifiedAt': datetime.now(timezone.utc).isoformat(), 'sourceSHA256': source['sourceSHA256'],
    'sourceFiles': source['fileCount'], 'SDKFiles': 194, 'DartInputs': len(inputs), 'nativeInputs': len(native),
    'allNamedSourceBytesUnchanged': True, 'APK_SHA256': digest, 'candidateSHA256': sha(candidate),
    'normalOutputSHA256AtInspection': digest,
}, indent=2) + '\n')
print(json.dumps({'candidate': candidate.relative_to(ROOT).as_posix(), 'sha256': digest,
                  'bytes': candidate.stat().st_size, 'ABI': abi, 'unchangedDependencyPayloads': len(dependencies),
                  'source': source['sourceSHA256'], 'mappingFiles': len(mapping_files), 'sidecar': ref(sidecar)}))
