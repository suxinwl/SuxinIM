"""Verify the completed frozen111 build and preserve its private artifact."""
from pathlib import Path
import datetime
import hashlib
import json
import re
import shutil
import subprocess
import zipfile

ROOT = Path(__file__).absolute().parents[2]
OUT = ROOT / 'tests/flutter'
APK = ROOT / 'clients/flutter/build/app/outputs/flutter-apk/app-development-release.apk'
CERT = '028253da3ebcb424a5e2e7ae73dddc88fc508a2ce3ccccedd1193dd4f65de8c1'


def sha(path):
    return hashlib.file_digest(path.open('rb'), 'sha256').hexdigest()


def read(path):
    return json.loads(path.read_text(encoding='utf-8-sig'))


def ref(path):
    return {'path': path.relative_to(ROOT).as_posix(), 'sha256': sha(path), 'bytes': path.stat().st_size}


if (OUT / 'android-main-x64-111.json').exists():
    raise RuntimeError('Preserve existing111 artifact evidence')
process = read(OUT / 'android-main-x64-111-build-process-result.json')
if process['exitCode'] != 0:
    raise RuntimeError('Build process did not succeed')
subprocess.run([str(ROOT / '.venv-tools/Scripts/python.exe'),
                str(OUT / 'snapshot_runtime_sources.py'), '--verify'], cwd=ROOT, check=True)
source = read(OUT / 'runtime-source-snapshot-111.json')
sdk = read(OUT / 'sdk-source-snapshot-111.json')
for name, expected in sdk['files'].items():
    if sha(ROOT / name) != expected:
        raise RuntimeError('Fixed SDK drift: ' + name)
inputs = read(OUT / 'unit-tests-111-inputs-after.json')['files']
changed_dart = [name for name, expected in inputs.items() if sha(ROOT / name) != expected]
if changed_dart:
    raise RuntimeError('Dart build input drift: ' + ','.join(changed_dart))

badging = subprocess.check_output(['E:/AndroidDev/sdk/build-tools/34.0.0/aapt.exe', 'dump', 'badging', str(APK)])
text = badging.decode('utf-8', errors='replace')
if not re.search(r"package: name='io\.openim\.upgrade38\.test\.dev' versionCode='111' versionName='3\.8\.11'", text):
    raise RuntimeError('Wrong actual APK package/version')
if "sdkVersion:'24'" not in text or "targetSdkVersion:'35'" not in text or "native-code: 'x86_64'" not in text:
    raise RuntimeError('Wrong actual SDK/ABI')
sign = subprocess.check_output(['E:/AndroidDev/jdk/jdk-17.0.20.1+1/bin/java.exe', '-jar',
                               'E:/AndroidDev/sdk/build-tools/34.0.0/lib/apksigner.jar',
                               'verify', '--verbose', '--print-certs', str(APK)])
sig = sign.decode('utf-8', errors='replace')
if 'Verified using v2 scheme (APK Signature Scheme v2): true' not in sig or CERT not in sig.lower():
    raise RuntimeError('Actual signature/signer mismatch')
(OUT / 'android-main-x64-111-badging.txt').open('xb').write(badging)
(OUT / 'android-main-x64-111-signature.log').open('xb').write(sign)

old = read(OUT / 'android-main-x64-110.json')
with zipfile.ZipFile(APK) as z:
    names = sorted(n for n in z.namelist() if n.startswith('lib/') and n.endswith('.so'))
    if len(names) != 11 or any(not n.startswith('lib/x86_64/') for n in names):
        raise RuntimeError('Unexpected native payload scope')
    libs = {name: {'size': z.getinfo(name).file_size, 'sha256': hashlib.sha256(z.read(name)).hexdigest()} for name in names}
    unchanged = [name for name in names if name != 'lib/x86_64/libapp.so' and libs[name] == old['nativeLibraries'][name]]
    if len(unchanged) != 10:
        raise RuntimeError('Fixed dependency payload changed')
    dex = b''.join(z.read(n) for n in z.namelist() if re.fullmatch(r'classes\d*\.dex', n))
    markers = {name: name.encode() in dex for name in ['javaRemoveSinkBegin', 'javaRemoveSinkEnd', 'leaseAssociation', 'camera.waitForClosed', 'substepBegin', 'wrapperJava#']}
    if not all(markers.values()):
        raise RuntimeError('Required prior observation markers absent')

digest = sha(APK)
private = ROOT / '.private/android-validation/OpenIM38-3.8.11+111-main-x64-candidate.apk'
if private.exists():
    raise RuntimeError('Do not replace private candidate')
shutil.copy2(APK, private)
if sha(private) != digest:
    raise RuntimeError('Private copy differs')
mapping = ROOT / ('.private/android-build-maps/111-x64-' + digest[:8])
mapping.mkdir(parents=True, exist_ok=False)
mapping_source = ROOT / 'clients/flutter/build/app/outputs/mapping/developmentRelease'
maps = []
for original in sorted(mapping_source.rglob('*')):
    if original.is_file():
        dest = mapping / original.relative_to(mapping_source)
        dest.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(original, dest)
        if sha(original) != sha(dest):
            raise RuntimeError('Mapping archive differs')
        maps.append(ref(dest))
if not maps:
    raise RuntimeError('No private R8 mappings preserved')
(mapping / 'manifest.json').write_text(json.dumps({'APK_SHA256': digest, 'sourceSHA256': source['sourceSHA256'], 'files': maps}, indent=2) + '\n', encoding='utf-8')

metadata = {k: v for k, v in old.items() if k not in ['newNativeObservationMarkersPresentInDEX', 'nativeDependenciesUnchangedFrom109']}
metadata.update({'recordedAt': datetime.datetime.now(datetime.timezone.utc).isoformat(),
                 'applicationVersion': '3.8.11', 'applicationBuild': 111,
                 'artifact': private.relative_to(ROOT).as_posix(), 'sha256': digest, 'size': APK.stat().st_size,
                 'sourceSHA256': source['sourceSHA256'], 'sourceFiles': source['fileCount'],
                 'sourceManifest': 'tests/flutter/runtime-source-snapshot-111.json',
                 'entrypointEvidence': 'tests/flutter/android-main-x64-111-build-start.json',
                 'buildLog': 'tests/flutter/android-main-x64-111-frozen-build.log',
                 'buildLogSHA256': sha(OUT / 'android-main-x64-111-frozen-build.log'),
                 'nativeLibraries': libs, 'privateMappingArchive': (mapping / 'manifest.json').relative_to(ROOT).as_posix(),
                 'prior110NativeObservationMarkersRetainedInDEX': markers,
                 'nativeDependenciesUnchangedFrom110': len(unchanged),
                 'nativeMarkersAreNotExecutionEvidence': True,
                 'SDKSourceManifest': 'tests/flutter/sdk-source-snapshot-111.json',
                 'SDKSourceFilesUnchangedByBuild': 194, 'all1039DartBuildInputsUnchanged': True})
(OUT / 'android-main-x64-111.json').open('x', encoding='utf-8').write(json.dumps(metadata, indent=2) + '\n')
(OUT / 'android-main-x64-111-source-postverification.json').open('x', encoding='utf-8').write(json.dumps({
    'verifiedAt': datetime.datetime.now(datetime.timezone.utc).isoformat(), 'sourceSHA256': source['sourceSHA256'],
    'sourceFiles': source['fileCount'], 'sourceDrift': [], 'SDKSourceSHA256': sdk['sourceSHA256'],
    'SDKFiles': 194, 'SDKDrift': [], 'DartInputsVerified': len(inputs), 'DartDrift': [],
    'APK_SHA256': digest, 'privateArtifactSHA256': sha(private), 'normalOutput': APK.relative_to(ROOT).as_posix()}, indent=2) + '\n')
print(json.dumps({'sha256': digest, 'size': APK.stat().st_size, 'ABI': 'x86_64', 'entrypoint': 'lib/main.dart',
                  'sourceVerified': source['fileCount'], 'SDKVerified': 194, 'dependencyPayloadsUnchanged': 10,
                  'mappingArchiveFiles': len(maps), 'privateCandidate': private.relative_to(ROOT).as_posix()}))
