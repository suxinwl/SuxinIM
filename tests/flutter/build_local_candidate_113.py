"""Build one source-gated normal-main local candidate; no test or publication action."""
from pathlib import Path
import ctypes
import datetime
import hashlib
import json
import os
import shutil
import subprocess
import sys

ROOT = Path('O:/')
OUT = ROOT / 'tests/flutter'
CLIENT = ROOT / 'clients/flutter'
kind = sys.argv[1]
assert kind in ['x64', 'arm64']
abi = {'x64': 'x86_64', 'arm64': 'arm64-v8a'}[kind]
prefix = f'android-main-{kind}-113'
assert not (OUT / (prefix + '-build-start.json')).exists()
assert not (ROOT / 'artifacts/android/OpenIM38-3.8.13+113-development-arm64.apk').exists()
gate = ROOT / 'tests/delivery/android-113-source-boundary-review.json'
assert gate.is_file(), 'Parent source gate must exist before building'
for args in [['snapshot_runtime_sources.py', '--verify'],
             ['audit_path_dependency_coverage.py', '--build', '113', '--check-only']]:
    subprocess.run([sys.executable, str(OUT / args[0]), *args[1:]], cwd=ROOT, check=True)
source = json.loads((OUT / 'runtime-source-snapshot-113.json').read_text())
sdk = json.loads((OUT / 'sdk-source-snapshot-113.json').read_text())
inputs = json.loads((OUT / 'nonmedia-113-inputs-after.json').read_text())['files']
native = json.loads((OUT / 'native-source-check-111.json').read_text())['before']


def sha(path):
    return hashlib.file_digest(path.open('rb'), 'sha256').hexdigest()


for path, expected in {**source['files'], **sdk['files'], **inputs}.items():
    assert sha(ROOT / path) == expected, path
for item in native:
    assert sha(ROOT / item['path']) == item['sha256'], item['path']


class Memory(ctypes.Structure):
    _fields_ = [('length', ctypes.c_ulong), ('load', ctypes.c_ulong),
                ('totalPhysical', ctypes.c_ulonglong), ('availablePhysical', ctypes.c_ulonglong),
                ('totalPageFile', ctypes.c_ulonglong), ('availablePageFile', ctypes.c_ulonglong),
                ('totalVirtual', ctypes.c_ulonglong), ('availableVirtual', ctypes.c_ulonglong),
                ('availableExtendedVirtual', ctypes.c_ulonglong)]


memory = Memory()
memory.length = ctypes.sizeof(memory)
assert ctypes.windll.kernel32.GlobalMemoryStatusEx(ctypes.byref(memory))
assert memory.availablePhysical >= 4 * 1024 ** 3
env = os.environ.copy()
env.update({
    'JAVA_HOME': 'E:/AndroidDev/jdk/jdk-17.0.20.1+1',
    'ANDROID_SDK_ROOT': 'E:/AndroidDev/sdk',
    'FLUTTER_PREBUILT_ENGINE_VERSION': 'ef0cd000916d64fa0c5d09cc809fa7ad244a5767',
    'GRADLE_USER_HOME': 'O:/.cache/gradle',
    'TEMP': 'D:/OpenIM38-BuildScratch/113-temp-20261101',
    'TMP': 'D:/OpenIM38-BuildScratch/113-temp-20261101',
    'PUB_CACHE': 'C:/Users/Administrator/AppData/Local/Pub/Cache',
})
Path(env['TEMP']).mkdir(parents=True, exist_ok=True)
command = [str(ROOT / 'tools/flutter-3.32.8/bin/flutter.bat'), 'build', 'apk', '--release',
           '--flavor', 'development', '--target-platform', 'android-' + kind,
           '--target', 'lib/main.dart', '--no-pub', '--dart-define=OPENIM_DEVELOPMENT=true',
           '--android-project-arg=openimTestAbi=' + abi, '-v']
record = {
    'at': datetime.datetime.now(datetime.timezone.utc).isoformat(), 'applicationVersion': '3.8.13+113',
    'sourceSHA256': source['sourceSHA256'], 'sourceFiles': source['fileCount'],
    'sdkSourceSHA256': sdk['sourceSHA256'], 'sdkSourceFiles': sdk['fileCount'],
    'sourceGate': gate.relative_to(ROOT).as_posix(), 'sourceGateSHA256': sha(gate),
    'buildRunnerSHA256': sha(Path(__file__)), 'fullScopePreVerified': True,
    'entrypoint': 'lib/main.dart', 'abi': abi, 'localCandidateOnly': True,
    'published': False, 'validationWrapper': False, 'command': command,
    'noTestSuiteInvokedByThisRunner': True, 'userRequestedMediaSkip': True,
    'reason': 'Version-only113 from exact restored111;112 unfinished source recoverably archived.',
    'resourceBefore': {'totalPhysicalBytes': memory.totalPhysical,
                       'availablePhysicalBytes': memory.availablePhysical,
                       'memoryLoadPercent': memory.load,
                       'freeDiskBytes': {drive: shutil.disk_usage(drive + ':/').free for drive in ['C', 'D', 'E']}},
    'environment': {k: env[k] for k in ['JAVA_HOME', 'ANDROID_SDK_ROOT', 'GRADLE_USER_HOME', 'TEMP', 'PUB_CACHE']},
    'note': 'Official shared Gradle/CMake graph can execute both release flavors. Selected development APK is independently inspected.',
}
(OUT / (prefix + '-build-start.json')).write_text(json.dumps(record, indent=2) + '\n', encoding='utf-8')
print(json.dumps({'phase': 'build-start', 'abi': abi, 'resources': record['resourceBefore']}), flush=True)
log = OUT / (prefix + '-frozen-build.log')
with log.open('xb') as stream:
    result = subprocess.run(command, cwd=CLIENT, env=env, stdout=stream, stderr=subprocess.STDOUT)
end = {'finishedAt': datetime.datetime.now(datetime.timezone.utc).isoformat(), 'exitCode': result.returncode,
       'log': log.relative_to(ROOT).as_posix(), 'logSHA256': sha(log)}
(OUT / (prefix + '-build-process-result.json')).open('x', encoding='utf-8').write(json.dumps(end, indent=2) + '\n')
print(json.dumps(end), flush=True)
raise SystemExit(result.returncode)
