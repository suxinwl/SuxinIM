"""Build only the frozen private110 normal-main x64 candidate, preserving history."""
from pathlib import Path
import ctypes
import datetime
import hashlib
import json
import os
import shutil
import subprocess
import sys

root = Path(__file__).absolute().parents[2]
client = root / 'clients/flutter'
directory = root / 'tests/flutter'
for name in ['android-main-x64-110-build-start.json', 'android-main-x64-110-frozen-build.log']:
    if (directory / name).exists():
        raise RuntimeError('Refusing to overwrite candidate build evidence')
subprocess.run([sys.executable, str(directory / 'snapshot_runtime_sources.py'), '--verify'], check=True)
subprocess.run([sys.executable, str(directory / 'audit_path_dependency_coverage.py'), '--build', '110', '--check-only'], check=True)
source = json.loads((directory / 'runtime-source-snapshot-110.json').read_text(encoding='utf-8'))
sdk = json.loads((directory / 'sdk-source-snapshot-110.json').read_text(encoding='utf-8'))


class Memory(ctypes.Structure):
    _fields_ = [('length', ctypes.c_ulong), ('load', ctypes.c_ulong),
                ('totalPhysical', ctypes.c_ulonglong), ('availablePhysical', ctypes.c_ulonglong),
                ('totalPageFile', ctypes.c_ulonglong), ('availablePageFile', ctypes.c_ulonglong),
                ('totalVirtual', ctypes.c_ulonglong), ('availableVirtual', ctypes.c_ulonglong),
                ('availableExtendedVirtual', ctypes.c_ulonglong)]


memory = Memory()
memory.length = ctypes.sizeof(memory)
if not ctypes.windll.kernel32.GlobalMemoryStatusEx(ctypes.byref(memory)):
    raise RuntimeError('Cannot read available build memory')
if memory.availablePhysical < 4 * 1024 ** 3:
    raise RuntimeError('Insufficient free physical memory for this one isolated build')
env = os.environ.copy()
env['JAVA_HOME'] = 'E:/AndroidDev/jdk/jdk-17.0.20.1+1'
env['ANDROID_SDK_ROOT'] = 'E:/AndroidDev/sdk'
env['FLUTTER_PREBUILT_ENGINE_VERSION'] = 'ef0cd000916d64fa0c5d09cc809fa7ad244a5767'
env['GRADLE_USER_HOME'] = str(root / '.cache/gradle')
env['TEMP'] = env['TMP'] = 'D:/OpenIM38-BuildScratch/110-temp-20261101'
Path(env['TEMP']).mkdir(parents=True, exist_ok=True)
command = [str(root / 'tools/flutter-3.32.8/bin/flutter.bat'), 'build', 'apk', '--release',
           '--flavor', 'development', '--target-platform', 'android-x64', '--target', 'lib/main.dart',
           '--no-pub', '--dart-define=OPENIM_DEVELOPMENT=true', '--android-project-arg=openimTestAbi=x86_64', '-v']
record = {'at': datetime.datetime.now(datetime.timezone.utc).isoformat(), 'applicationVersion': '3.8.10+110',
          'sourceSHA256': source['sourceSHA256'], 'sourceFiles': source['fileCount'],
          'sdkSourceSHA256': sdk['sourceSHA256'], 'sdkSourceFiles': sdk['fileCount'],
          'fullScopePreVerified': True, 'entrypoint': 'lib/main.dart', 'abi': 'x86_64',
          'privateCandidate': True, 'published': False, 'validationWrapper': False,
          'reason': 'Actual error-observation integration and observation-only native correlation for originalUI acceptance.',
          'command': command, 'buildProcessTemp': env['TEMP'],
          'resourceBefore': {'totalPhysicalBytes': memory.totalPhysical,
                             'availablePhysicalBytes': memory.availablePhysical,
                             'freeDiskBytes': {drive: shutil.disk_usage(drive + ':/').free for drive in ['C', 'D', 'E']}},
          'note': 'Shared official Flutter/CMake graph may execute both Release flavor kernels; selected development APK and all targets are independently inspected.'}
start = directory / 'android-main-x64-110-build-start.json'
start.write_text(json.dumps(record, indent=2) + '\n', encoding='utf-8')
print(json.dumps({'phase': 'build-start', 'version': record['applicationVersion'],
                  'sourceSHA256': source['sourceSHA256'], 'resources': record['resourceBefore']}), flush=True)
log = directory / 'android-main-x64-110-frozen-build.log'
with log.open('xb') as output:
    result = subprocess.run(command, cwd=client, env=env, stdout=output, stderr=subprocess.STDOUT)
end = {'finishedAt': datetime.datetime.now(datetime.timezone.utc).isoformat(), 'exit': result.returncode,
       'log': log.relative_to(root).as_posix(), 'logSHA256': hashlib.sha256(log.read_bytes()).hexdigest()}
(directory / 'android-main-x64-110-build-end.json').write_text(json.dumps(end, indent=2) + '\n', encoding='utf-8')
print(json.dumps(end), flush=True)
sys.exit(result.returncode)
