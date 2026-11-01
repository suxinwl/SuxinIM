"""Build a frozen normal-main114 local candidate after resumed media checks."""
from pathlib import Path
from datetime import datetime, timezone
import ctypes
import hashlib
import json
import os
import shutil
import subprocess
import sys

ROOT = Path('O:/')
OUT = ROOT / 'tests/flutter'
HERE = ROOT / 'tests/delivery/media-resume-20261102'
CLIENT = ROOT / 'clients/flutter'
kind = sys.argv[1]
assert kind in ['x64', 'arm64']
abi = {'x64': 'x86_64', 'arm64': 'arm64-v8a'}[kind]
prefix = f'android-main-{kind}-114'
assert not (OUT / (prefix + '-build-start.json')).exists()
assert not (ROOT / 'artifacts/android/OpenIM38-3.8.14+114-development-arm64.apk').exists()
gate = HERE / 'source-freeze-114.json'
assert gate.is_file(), 'Frozen source/test boundary must exist before building'
for args in [['snapshot_runtime_sources.py', '--verify'], ['audit_path_dependency_coverage.py', '--build', '114', '--check-only']]:
    subprocess.run([sys.executable, str(OUT / args[0]), *args[1:]], cwd=ROOT, check=True)
source = json.loads((OUT / 'runtime-source-snapshot-114.json').read_text())
sdk = json.loads((OUT / 'sdk-source-snapshot-114.json').read_text())
inputs = json.loads((HERE / 'full-114-quote-final.json').read_text())['after']
native = json.loads((HERE / 'native-formal-final.json').read_text())['after']
def sha(path):
    return hashlib.file_digest(path.open('rb'), 'sha256').hexdigest()
for path, expected in {**source['files'], **sdk['files'], **inputs}.items():
    assert sha(ROOT / path) == expected, path
for item in native:
    assert sha(ROOT / item['path']) == item['sha256'], item['path']
class Memory(ctypes.Structure):
    _fields_ = [('length', ctypes.c_ulong), ('load', ctypes.c_ulong), ('totalPhysical', ctypes.c_ulonglong),
                ('availablePhysical', ctypes.c_ulonglong), ('totalPageFile', ctypes.c_ulonglong),
                ('availablePageFile', ctypes.c_ulonglong), ('totalVirtual', ctypes.c_ulonglong),
                ('availableVirtual', ctypes.c_ulonglong), ('availableExtendedVirtual', ctypes.c_ulonglong)]
memory = Memory()
memory.length = ctypes.sizeof(memory)
assert ctypes.windll.kernel32.GlobalMemoryStatusEx(ctypes.byref(memory))
assert memory.availablePhysical >= 4 * 1024 ** 3, 'Do not build under insufficient free host memory'
env = os.environ.copy()
env.update({'JAVA_HOME': 'E:/AndroidDev/jdk/jdk-17.0.20.1+1', 'ANDROID_SDK_ROOT': 'E:/AndroidDev/sdk',
            'FLUTTER_PREBUILT_ENGINE_VERSION': 'ef0cd000916d64fa0c5d09cc809fa7ad244a5767',
            'GRADLE_USER_HOME': 'O:/.cache/gradle', 'TEMP': 'D:/OpenIM38-BuildScratch/114-temp-20261102',
            'TMP': 'D:/OpenIM38-BuildScratch/114-temp-20261102', 'PUB_CACHE': 'C:/Users/Administrator/AppData/Local/Pub/Cache'})
Path(env['TEMP']).mkdir(parents=True, exist_ok=True)
command = [str(ROOT / 'tools/flutter-3.32.8/bin/flutter.bat'), 'build', 'apk', '--release', '--flavor', 'development',
           '--target-platform', 'android-' + kind, '--target', 'lib/main.dart', '--no-pub',
           '--dart-define=OPENIM_DEVELOPMENT=true', '--android-project-arg=openimTestAbi=' + abi]
record = {'at': datetime.now(timezone.utc).isoformat(), 'applicationVersion': '3.8.14+114',
          'sourceSHA256': source['sourceSHA256'], 'sourceFiles': source['fileCount'],
          'sdkSourceSHA256': sdk['sourceSHA256'], 'sdkSourceFiles': sdk['fileCount'],
          'sourceGate': gate.relative_to(ROOT).as_posix(), 'sourceGateSHA256': sha(gate),
          'buildRunnerSHA256': sha(Path(__file__)), 'fullScopePreVerified': True, 'entrypoint': 'lib/main.dart',
          'abi': abi, 'localCandidateOnly': True, 'published': False, 'validationWrapper': False, 'command': command,
          'noTestSuiteInvokedByThisRunner': True, 'mediaDevelopmentResumed': True,
          'reason': 'Reviewed native frame handoff and meeting state/session ownership, plus narrow official Quote send route; real media runtime acceptance pending.',
          'resourceBefore': {'totalPhysicalBytes': memory.totalPhysical, 'availablePhysicalBytes': memory.availablePhysical,
                             'memoryLoadPercent': memory.load,
                             'freeDiskBytes': {drive: shutil.disk_usage(drive + ':/').free for drive in ['C', 'D', 'E']}},
          'environment': {key: env[key] for key in ['JAVA_HOME', 'ANDROID_SDK_ROOT', 'GRADLE_USER_HOME', 'TEMP', 'PUB_CACHE']}}
with (OUT / (prefix + '-build-start.json')).open('x', encoding='utf-8') as output:
    json.dump(record, output, indent=2)
print(json.dumps({'phase': 'build-start', 'abi': abi, 'resources': record['resourceBefore']}), flush=True)
log = OUT / (prefix + '-frozen-build.log')
with log.open('xb') as output:
    result = subprocess.run(command, cwd=CLIENT, env=env, stdout=output, stderr=subprocess.STDOUT)
end = {'finishedAt': datetime.now(timezone.utc).isoformat(), 'exitCode': result.returncode,
       'log': log.relative_to(ROOT).as_posix(), 'logSHA256': sha(log)}
with (OUT / (prefix + '-build-process-result.json')).open('x', encoding='utf-8') as output:
    json.dump(end, output, indent=2)
print(json.dumps(end), flush=True)
raise SystemExit(result.returncode)
