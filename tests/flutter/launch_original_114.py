"""Launch only the installed114 normalmain on its dedicated native x64 AVD."""
from pathlib import Path
from datetime import datetime, timezone
import hashlib
import json
import re
import subprocess

ROOT = Path('O:/')
OUT = ROOT / 'tests/flutter/android-114-original-start.json'
ADB = ['E:/AndroidDev/sdk/platform-tools/adb.exe', '-s', 'emulator-5588']
PACKAGE = 'io.openim.upgrade38.test.dev'
if OUT.exists():
    raise RuntimeError('Preserve prior original startup evidence')


def adb(*args):
    return subprocess.check_output(ADB + list(args), timeout=60)


if adb('emu', 'avd', 'name').splitlines()[0] != b'openim38_api30':
    raise RuntimeError('Owned original native x64 AVD required')
if adb('shell', 'getprop', 'sys.boot_completed').strip() != b'1':
    raise RuntimeError('Complete boot required before app launch')
facts = adb('shell', 'dumpsys', 'package', PACKAGE).decode('utf-8', 'replace')
if not re.search(r'versionCode=114\b', facts) or not re.search(r'versionName=3\.8\.14\b', facts):
    raise RuntimeError('Exact114 package required')
if subprocess.run(ADB + ['shell', 'pidof', PACKAGE], capture_output=True, timeout=20).returncode == 0:
    raise RuntimeError('Original own app must not already be running')
metadata = json.loads((ROOT / 'tests/flutter/android-main-x64-114.json').read_text())
before = adb('shell', 'dumpsys', 'activity', 'lastanr')
record = {'startedAt': datetime.now(timezone.utc).isoformat(), 'entrypoint': 'lib/main.dart',
          'applicationVersion': '3.8.14+114', 'ABI': 'x86_64',
          'APK_SHA256': metadata['sha256'], 'sourceSHA256': metadata['sourceSHA256'],
          'clearData': False, 'tokenInjected': False, 'credentialsEnteredByThisTool': False,
          'beforeDisplaySize': adb('shell', 'wm', 'size').decode().strip(),
          'beforeDensity': adb('shell', 'wm', 'density').decode().strip(),
          'lastANRBeforeSHA256': hashlib.sha256(before).hexdigest(),
          'emulatorGraphicsBackend': 'Skia selected by Activity boolean intent only; no manifest change'}
adb('shell', 'wm', 'size', '540x1170')
adb('shell', 'wm', 'density', '240')
result = adb('shell', 'am', 'start', '-W', '-n', PACKAGE + '/cn.rentsoft.flutter.openim.consumer.MainActivity',
             '--ez', 'enable-impeller', 'false')
record['activityStartOutput'] = result.decode('utf-8', 'replace')
record['checkedAt'] = datetime.now(timezone.utc).isoformat()
record['PID'] = int(adb('shell', 'pidof', PACKAGE))
record['UIReadyNotAssumed'] = True
OUT.open('x', encoding='utf-8').write(json.dumps(record, indent=2) + '\n')
print(json.dumps(record))
