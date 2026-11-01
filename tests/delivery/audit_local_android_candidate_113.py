"""Static inspection of one explicitly named113 local candidate. Never installs/builds/publishes."""
from pathlib import Path
import argparse
import datetime as dt
import hashlib
import json
import re
import struct
import subprocess
import sys
import zipfile

ROOT = Path('O:/')
OUT = ROOT / 'tests/delivery'
parser = argparse.ArgumentParser()
parser.add_argument('--kind', required=True, choices=['x64', 'arm64'])
kind = parser.parse_args().kind
ABI = {'x64': 'x86_64', 'arm64': 'arm64-v8a'}[kind]
MACHINE = {'x64': 62, 'arm64': 183}[kind]
COUNT = {'x64': 11, 'arm64': 13}[kind]
EXPECTED_X64 = '347b2eaa592576fdbf59b47cea52eb730efe8595c375baecdbb053c08edb0ed4'
CERT = '028253da3ebcb424a5e2e7ae73dddc88fc508a2ce3ccccedd1193dd4f65de8c1'
APK = ROOT / f'artifacts/android-candidates/OpenIM38-3.8.13+113-development-{kind}-candidate.apk'
SIDECAR = APK.with_suffix('.json')
PREFIX = f'android-113-{kind}'
REVIEW = OUT / (PREFIX + '-artifact-provenance-review.json')
BOUNDARY = OUT / (PREFIX + '-build-output-boundary.json')
assert not REVIEW.exists() and not BOUNDARY.exists(), 'preserve original evidence'
assert APK.is_file() and SIDECAR.is_file(), 'candidate is not ready'
assert '.private' not in APK.parts

def digest(path):
    with path.open('rb') as source:
        return hashlib.file_digest(source, 'sha256').hexdigest()

def record(path):
    return {'path': path.relative_to(ROOT).as_posix(), 'bytes': path.stat().st_size, 'sha256': digest(path)}

def read(path):
    return json.loads(path.read_text(encoding='utf-8'))

commands = []

def run(name, command):
    log = OUT / (PREFIX + '-' + name + '-independent.log')
    assert not log.exists()
    started = dt.datetime.now(dt.timezone.utc).isoformat()
    result = subprocess.run([str(item) for item in command], cwd=ROOT, stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT, timeout=60)
    with log.open('xb') as stream:
        stream.write(result.stdout)
    commands.append({'startedAt': started, 'finishedAt': dt.datetime.now(dt.timezone.utc).isoformat(),
        'command': [str(item) for item in command], 'exitCode': result.returncode, 'log': record(log)})
    assert result.returncode == 0, f'{name} failed; raw output retained'
    return result.stdout.decode('utf-8', errors='replace')

before = record(APK)
sidecar = read(SIDECAR)
owner_path = ROOT / f'tests/flutter/android-main-{kind}-113.json'
owner = read(owner_path)
assert before['sha256'] == sidecar['sha256'] == owner['sha256']
if kind == 'x64': assert before['sha256'] == EXPECTED_X64
assert before['bytes'] == sidecar['size'] == owner['size']
assert {key: value for key, value in owner.items() if key != 'candidateSidecar'} == sidecar
assert record(SIDECAR) == owner['candidateSidecar']
assert sidecar['entrypoint'] == 'lib/main.dart' and not sidecar['validationWrapper']
assert sidecar['localCandidateOnly'] and not sidecar['published'] and not sidecar['releaseAccepted']
formal = ROOT / 'artifacts/android/OpenIM38-3.8.13+113-development-arm64.apk'
assert not formal.exists(), 'formal artifact path must remain absent'

badging = run('badging', ['E:/AndroidDev/sdk/build-tools/34.0.0/aapt.exe', 'dump', 'badging', APK])
assert re.search(r"package: name='io\.openim\.upgrade38\.test\.dev' versionCode='113' versionName='3\.8\.13'", badging)
assert "sdkVersion:'24'" in badging and "targetSdkVersion:'35'" in badging
native_line = next(line for line in badging.splitlines() if line.startswith('native-code:'))
assert native_line == "native-code: '" + ABI + "'"
signature = run('signature', ['E:/AndroidDev/jdk/jdk-17.0.20.1+1/bin/java.exe', '-Xmx128m',
    '-XX:ActiveProcessorCount=2', '-jar', 'E:/AndroidDev/sdk/build-tools/34.0.0/lib/apksigner.jar',
    'verify', '--verbose', '--print-certs', APK])
assert 'Verified using v2 scheme (APK Signature Scheme v2): true' in signature
certificates = re.findall(r'Signer #\d+ certificate SHA-256 digest: ([a-fA-F0-9]{64})', signature)
assert [value.lower() for value in certificates] == [CERT]

previous_path = ROOT / ('tests/flutter/android-main-x64-111.json' if kind == 'x64' else 'tests/flutter/android-main-arm64-109.json')
previous = read(previous_path)
libraries = {}
elf = []
with zipfile.ZipFile(APK) as archive:
    assert len(archive.namelist()) == len(set(archive.namelist())), 'duplicate ZIP names'
    names = sorted(name for name in archive.namelist() if name.startswith('lib/') and name.endswith('.so'))
    assert len(names) == COUNT and all(name.startswith('lib/' + ABI + '/') for name in names)
    for name in names:
        payload = archive.read(name)
        libraries[name] = {'size': len(payload), 'sha256': hashlib.sha256(payload).hexdigest()}
        assert payload[:4] == b'\x7fELF' and payload[4] == 2 and payload[5] == 1
        machine = struct.unpack_from('<H', payload, 18)[0]
        assert machine == MACHINE
        elf.append({'path': name, 'ELFClass': 64, 'endianness': 'little', 'eMachine': machine})
assert libraries == sidecar['nativeLibraries'] == owner['nativeLibraries']
dependencies = [name for name in names if not name.endswith('/libapp.so')]
assert len(dependencies) == COUNT - 1
assert all(libraries[name] == previous['nativeLibraries'][name] for name in dependencies)

run('readable-resource-audit', [sys.executable, ROOT / 'tools/audit_apk_resources.py', '--apk', APK,
    '--expected-sha256', before['sha256'], '--output', BOUNDARY])
boundary = read(BOUNDARY)
assert boundary['zipAndReadableResourcesPassed']
assert record(APK) == before, 'candidate changed during independent inspection'

references = [record(SIDECAR), record(owner_path), record(previous_path), record(BOUNDARY),
    record(ROOT / 'tools/audit_apk_resources.py'), record(Path(__file__)),
    record(ROOT / 'tests/delivery/android-113-nonmedia-source-independent-final-review.json')]
for key in ['entrypointEvidence', 'buildLog', 'sourceManifest', 'SDKSourceManifest']:
    expected = sidecar[key]
    assert record(ROOT / expected['path']) == expected
    references.append(expected)
start = read(ROOT / sidecar['entrypointEvidence']['path'])
assert start['entrypoint'] == 'lib/main.dart' and not start['validationWrapper'] and start['localCandidateOnly']
assert start['command'][start['command'].index('--target') + 1] == 'lib/main.dart'
post_path = ROOT / f'tests/flutter/android-main-{kind}-113-source-postverification.json'
post = read(post_path)
assert post['sourceSHA256'] == sidecar['sourceSHA256'] == '81398ce26d6dd35345f7f22ae8d6047e307efc244db93f0d75ce5afb14511432'
assert [post['sourceFiles'], post['SDKFiles'], post['DartInputs'], post['nativeInputs']] == [3229, 194, 1041, 99]
assert post['allNamedSourceBytesUnchanged'] and post['candidateSHA256'] == before['sha256']
references.append(record(post_path))
if kind == 'arm64':
    provenance = ROOT / 'tests/flutter/android-main-arm64-109-payload-verification.json'
    if provenance.is_file(): references.append(record(provenance))
review = {
    'createdAt': dt.datetime.now(dt.timezone.utc).isoformat(),
    'status': 'STATIC_LOCAL_CANDIDATE_CONSISTENT_NOT_RELEASE_ACCEPTANCE',
    'applicationVersion': '3.8.13+113', 'candidate': before, 'candidateSidecar': record(SIDECAR),
    'independentRead': 'Actual public candidate APK bytes, not the private validation path or potentially mutating normal build output.',
    'signature': {'verified': True, 'schemeV2': True, 'signingCertificateSHA256': CERT,
        'tool': 'JDK17.0.20.1+1 + Android build-tools34.0.0 apksigner'},
    'package': {'id': 'io.openim.upgrade38.test.dev', 'versionCode': 113, 'versionName': '3.8.13',
        'minSDK': 24, 'targetSDK': 35, 'ABI': ABI, 'debuggableMarkerPresent': 'application-debuggable' in badging},
    'nativeLibraries': libraries, 'nativeLibraryCount': len(libraries), 'ELFHeaders': elf,
    'allNativeHashesAndSizesMatchSidecar': True,
    'dependencyComparison': {'previousPublicRecord': record(previous_path), 'count': len(dependencies),
        'allDependencyHashesAndSizesMatch': True, 'libappComparedSeparately': True,
        'libappMatchesPrevious': libraries['lib/' + ABI + '/libapp.so'] == previous['nativeLibraries']['lib/' + ABI + '/libapp.so'],
        'scope': 'Direct comparison to the previous proven payload records; prior private APK is not opened. ARM109 record supplies the fixed AAR/Go strip provenance, not a new strip operation.'},
    'normalOutputCopyEvidence': {'source': 'owner recorded build output inspection/copy before next ABI build',
        'normalOutputAtOwnerInspection': sidecar['normalOutputAtInspection'],
        'normalOutputSHA256AtOwnerInspection': sidecar['normalOutputSHA256AtInspection'],
        'ownerRecord': record(owner_path), 'independentlyReadNormalOutputNow': False,
        'reason': 'Shared normal output may be overwritten by sequential ARM/x64 build. This audit directly reads the fixed candidate.'},
    'entrypointBoundary': 'Owner build-start command targets actual lib/main.dart, development release; sidecar says validationWrapper=false. This is build provenance, not decompilation or runtime proof.',
    'sourceBoundary': {'digest': post['sourceSHA256'], 'sourceFiles': 3229, 'sdkFiles': 194,
        'testInputs': 1041, 'nativeInputs': 99, 'prePostNoChanges': True,
        'basis': 'Owner build pre/post records plus the independent113 source gate. No new test/build/source-hash loop performed during this static APK audit.'},
    'readableResourceBoundary': record(BOUNDARY),
    'formalArtifactPathAbsent': not formal.exists(), 'formalArtifactPath': formal.relative_to(ROOT).as_posix(),
    'localCandidateOnly': True, 'publishedByThisTask': False, 'installedByThisTask': False,
    'runtimeAccepted': False, 'runtimeRTCAccepted': False, 'releaseAccepted': False,
    'userRequestedMediaSkip': True, 'noNetworkNoDeviceNoPrivateReads': True,
    'commands': commands, 'references': references,
    'limits': ['No APK installation, execution, device/AVD, media/voice/call/meeting tests, rebuild, upload or Chat policy mutation.',
        'Development signing is project test signing, not Play/production release identity.',
        'Finite resource scan covers ZIP metadata and ordinary UTF-8 resources only, not DEX/native/AOT/binaryXML secrets or full secret-free proof.',
        'Normal-main native_probe channel identifier is not a shipped probe fixture; binary code was not generically string-scanned.',
        'ARM64 static build is not an emulator translation or physical-phone runtime result. iOS Xcode not built.',
        'Unresolved historical media failures and archived112 Ended UI remain deferred, not converted to passed.'],
}
with REVIEW.open('x', encoding='utf-8', newline='\n') as stream:
    json.dump(review, stream, indent=2)
    stream.write('\n')
print(json.dumps({'review': record(REVIEW), 'boundary': record(BOUNDARY), 'APK': before,
    'nativeLibraries': len(libraries), 'dependencyPayloads': len(dependencies)}))
