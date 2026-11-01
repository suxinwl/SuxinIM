"""Run one complete110 Dart suite with byte-preserved logs; no device actions."""
from pathlib import Path
import hashlib
import json
import os
import re
import subprocess
import sys
from datetime import datetime, timezone

root = Path(__file__).absolute().parents[2]
client = root / 'clients/flutter'
assert re.search(r'^version: 3\.8\.10\+110$', (client / 'pubspec.yaml').read_text(encoding='utf-8'), re.M)
env = os.environ.copy()
env['FLUTTER_PREBUILT_ENGINE_VERSION'] = 'ef0cd000916d64fa0c5d09cc809fa7ad244a5767'
env['JAVA_HOME'] = 'E:/AndroidDev/jdk/jdk-17.0.20.1+1'
env['ANDROID_SDK_ROOT'] = 'E:/AndroidDev/sdk'
env['GRADLE_USER_HOME'] = str(root / '.cache/gradle')
env['TEMP'] = env['TMP'] = 'D:/OpenIM38-BuildScratch/110-temp-20261101'
Path(env['TEMP']).mkdir(parents=True, exist_ok=True)
flutter = str(root / 'tools/flutter-3.32.8/bin/flutter.bat')
records = []
for args, name in [(['pub', 'get'], 'pub-get-110.log'),
                   (['test', '--no-pub', '--concurrency=1', '--reporter', 'expanded', 'test'], 'unit-tests-110-final.log')]:
    target = root / 'tests/flutter' / name
    if target.exists():
        raise RuntimeError('Preserve prior verification logs')
    started = datetime.now(timezone.utc).isoformat()
    with target.open('xb') as log:
        result = subprocess.run([flutter, *args], cwd=client, env=env,
                                stdout=log, stderr=subprocess.STDOUT)
    text = target.read_text(encoding='utf-8', errors='replace')
    marker = re.findall(r'\+(\+?\d+)(?:\s+-\d+)?:\s+All tests passed!', text)
    records.append({'command': [flutter, *args], 'startedAt': started,
                    'finishedAt': datetime.now(timezone.utc).isoformat(),
                    'exit': result.returncode, 'log': target.relative_to(root).as_posix(),
                    'SHA256': hashlib.sha256(target.read_bytes()).hexdigest(),
                    'allTestsPassedMarker': marker[-1] if marker else None})
    print(json.dumps(records[-1]), flush=True)
    if result.returncode:
        sys.exit(result.returncode)
(root / 'tests/flutter/dart-final-run-110.json').write_text(json.dumps(records, indent=2) + '\n', encoding='utf-8')
