"""Recheck all native boundaries and real Android classpath after110 logs only."""
from pathlib import Path
import datetime
import hashlib
import json
import os
import subprocess
import sys

root = Path(__file__).absolute().parents[2]
directory = root / 'tests/flutter'
prior = json.loads((directory / 'native-verification-reuse-109.json').read_text(encoding='utf-8'))
names = [row['path'] for row in prior['checks']]
assert len(names) == 95


def snapshot():
    result = []
    for name in names:
        path = root / name
        result.append({'path': name, 'bytes': path.stat().st_size,
                       'sha256': hashlib.sha256(path.read_bytes()).hexdigest()})
    return result


record = directory / 'native-source-check-110.json'
if record.exists():
    raise RuntimeError('Preserve existing verification record')
before = snapshot()
data = {'createdAt': datetime.datetime.now(datetime.timezone.utc).isoformat(),
        'scope': 'Same95 native production Java, actual renderer/platform stubs, helper tests and formal script inputs as109; four production files add safe observations only.',
        'inputCount': len(before), 'before': before, 'commands': []}
record.write_text(json.dumps(data, indent=2) + '\n', encoding='utf-8')
env = os.environ.copy()
env['JAVA_HOME'] = 'E:/AndroidDev/jdk/jdk-17.0.20.1+1'
env['ANDROID_SDK_ROOT'] = 'E:/AndroidDev/sdk'
env['FLUTTER_PREBUILT_ENGINE_VERSION'] = 'ef0cd000916d64fa0c5d09cc809fa7ad244a5767'
env['GRADLE_USER_HOME'] = str(root / '.cache/gradle')
env['TEMP'] = env['TMP'] = 'D:/OpenIM38-BuildScratch/110-temp-20261101'
commands = [
    (['powershell.exe', '-NoProfile', '-File', str(root / 'clients/flutter/scripts/test-native-java.ps1')],
     root, 'native-java-formal-110.log'),
    ([str(root / 'clients/flutter/android/gradlew.bat'), ':flutter_webrtc:compileReleaseJavaWithJavac',
      '--offline', '--no-daemon', '--max-workers=1', '-Ptarget-platform=android-x64',
      '-PopenimTestAbi=x86_64', '-Ptarget=lib/main.dart'],
     root / 'clients/flutter/android', 'native-android-javac-110.log'),
]
for command, cwd, filename in commands:
    log = directory / filename
    if log.exists():
        raise RuntimeError('Do not overwrite native execution evidence')
    print(json.dumps({'phase': 'started', 'log': filename}), flush=True)
    started = datetime.datetime.now(datetime.timezone.utc).isoformat()
    with log.open('xb') as output:
        result = subprocess.run(command, cwd=cwd, env=env, stdout=output, stderr=subprocess.STDOUT)
    item = {'command': command, 'cwd': str(cwd), 'startedAt': started,
            'finishedAt': datetime.datetime.now(datetime.timezone.utc).isoformat(),
            'log': log.relative_to(root).as_posix(), 'exit': result.returncode,
            'sha256': hashlib.sha256(log.read_bytes()).hexdigest()}
    data['commands'].append(item)
    record.write_text(json.dumps(data, indent=2) + '\n', encoding='utf-8')
    print(json.dumps(item), flush=True)
    if result.returncode:
        sys.exit(result.returncode)
data['after'] = snapshot()
data['beforeAfterIdentical'] = data['before'] == data['after']
data['finishedAt'] = datetime.datetime.now(datetime.timezone.utc).isoformat()
record.write_text(json.dumps(data, indent=2) + '\n', encoding='utf-8')
if not data['beforeAfterIdentical']:
    raise RuntimeError('Native verification input changed during execution')
print(json.dumps({'all95InputsUnchanged': True, 'formalAndActualJavacPassed': True}), flush=True)
