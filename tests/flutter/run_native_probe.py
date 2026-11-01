"""Run on this upgrade's own AVD. Synthetic credentials are injected at runtime only."""
import hashlib
import json
import re
from pathlib import Path
import shlex
import subprocess
import sys
import time

ROOT = Path(__file__).absolute().parents[2]
ADB = Path('E:/AndroidDev/sdk/platform-tools/adb.exe')
SERIAL = 'emulator-5588'
PACKAGE = 'io.openim.upgrade38.test.dev'

def adb(*args, timeout=90):
    run = subprocess.run([str(ADB), '-s', SERIAL, *args], capture_output=True,
                         text=True, encoding='utf-8', errors='replace', timeout=timeout)
    if run.returncode:
        raise RuntimeError(f'ADB {args[0]} failed: {run.stderr[:300]}')
    return run.stdout

apk = Path(sys.argv[1])
test_abi = next((x.split('=', 1)[1] for x in sys.argv[2:] if x.startswith('--abi=')), 'arm64-v8a')
rotation_fixture = next((Path(x.split('=', 1)[1]) for x in sys.argv[2:] if x.startswith('--rotation-fixture=')), None)
if test_abi not in ['arm64-v8a', 'x86_64']:
    raise RuntimeError('Unsupported isolated test ABI')
if '--reuse' not in sys.argv[2:]:
    adb('install', '-r', '--abi', test_abi, str(apk), timeout=180)
accounts = json.loads((ROOT / '.private/test-accounts.json').read_text(encoding='utf-8-sig'))
ca_setup = json.loads((ROOT / 'tests/flutter/avd-ca-setup.json').read_text())
zygotes = set(adb('shell', 'pidof', 'zygote64', 'zygote').split())
if set(ca_setup['zygotePIDs']) != zygotes:
    raise RuntimeError('Dedicated AVD rebooted; reapply its runtime CA setup first')
ca_snapshot = ROOT / f'tests/flutter/avd-ca-setup-native-{int(time.time())}.json'
ca_snapshot.write_text(json.dumps(ca_setup, indent=2) + '\n')
fixture_dir = f'/data/user/0/{PACKAGE}/files/fixtures'
adb('shell', 'mkdir', '-p', fixture_dir)
uid = adb('shell', 'stat', '-c', '%u', f'/data/user/0/{PACKAGE}').strip()
context = adb('shell', 'ls', '-Zd', f'/data/user/0/{PACKAGE}').split()[0]
if not uid.isdigit() or uid == '0' or not context.startswith('u:object_r:app_data_file:'):
    raise RuntimeError('Unexpected dedicated app data ownership')
# Fresh installs do not have files/ yet. Root test setup must preserve app UID
# and SELinux labels before Application.attachBaseContext writes its CA file.
adb('shell', f'chown {uid}:{uid} /data/user/0/{PACKAGE}/files; chcon {context} /data/user/0/{PACKAGE}/files')
for filename in ['synthetic.png', 'synthetic.wav', 'synthetic.mp4',
                 'synthetic-rotate90.mp4', 'synthetic-cancel.mp4']:
    fixture = ROOT / '.cache/media-fixtures' / filename
    if filename == 'synthetic-rotate90.mp4' and rotation_fixture is not None:
        fixture = rotation_fixture
    if fixture.is_file():
        adb('push', str(fixture), fixture_dir + '/' + filename)
adb('shell', f'chown -R {uid}:{uid} {fixture_dir}; chmod -R u+rwX,go+rX {fixture_dir}; chcon -R {context} {fixture_dir}')
config = {'baseURL': accounts['baseURL'], 'password': accounts['password'],
          'sender': accounts['users'][103], 'receiver': accounts['users'][104],
          'crossReceiver': accounts['users'][100],
          'fixtureDir': fixture_dir}
adb('shell', 'am', 'force-stop', PACKAGE)
adb('logcat', '-c')
command = ('am start -n ' + PACKAGE + '/cn.rentsoft.flutter.openim.consumer.MainActivity'
           + ' --ez enable-impeller false'
           + ' --es openim38Probe ' + shlex.quote(json.dumps(config, separators=(',', ':'))))
adb('shell', command)
deadline = time.monotonic() + 480
previous = set()
while time.monotonic() < deadline:
    logs = adb('logcat', '-d', '-s', 'flutter:I', timeout=30)
    for line in logs.splitlines():
        if 'OPENIM38_PROBE' not in line or line in previous:
            continue
        previous.add(line)
        if 'OPENIM38_PROBE_RESULT ' in line:
            try:
                report = json.loads(line.split('OPENIM38_PROBE_RESULT ', 1)[1])
            except json.JSONDecodeError:
                # Only this task's rooted AVD can use this long-log fallback.
                report = json.loads(adb('shell', 'cat',
                    f'/data/user/0/{PACKAGE}/files/native-probe-report.json'))
            package_info = adb('shell', 'dumpsys', 'package', PACKAGE)
            report.update({'device': SERIAL, 'physicalDevice': False,
                'applicationVersion': re.search(r'versionName=([^\s]+)', package_info).group(1),
                'applicationBuild': int(re.search(r'versionCode=(\d+)', package_info).group(1)),
                'rootedDedicatedTestAVD': True,
                'systemCATrustSetup': ca_snapshot.relative_to(ROOT).as_posix(),
                'applicationCAOnlySufficient': False,
                'runtimeAPKABI': test_abi,
                'androidAPI': int(adb('shell', 'getprop', 'ro.build.version.sdk').strip()),
                'avd': adb('emu', 'avd', 'name').splitlines()[0].strip(),
                'apkSHA256': hashlib.file_digest(apk.open('rb'), 'sha256').hexdigest(),
                'rotationFixture': str(rotation_fixture or (ROOT / '.cache/media-fixtures/synthetic-rotate90.mp4')),
                'rotationFixtureSHA256': hashlib.file_digest((rotation_fixture or (ROOT / '.cache/media-fixtures/synthetic-rotate90.mp4')).open('rb'), 'sha256').hexdigest(),
                'caValidationBypass': False,
                'rendererOverride': 'Skia via test AVD launch intent enable-impeller=false; phone delivery manifest unchanged',
                'note': ('ARM64 APK on isolated x86_64 AVD with native bridge; not a physical-device result.' if test_abi == 'arm64-v8a' else 'Private validation APK using official native x86_64 AAR on isolated x86_64 AVD; original main UI with optional synthetic probe intent. Not a physical ARM64 result.')})
            (ROOT / 'tests/flutter/native-sdk-report.json').write_text(
                json.dumps(report, indent=2) + '\n', encoding='utf-8')
            print(json.dumps(report))
            sys.exit(0 if report.get('passed') else 1)
        print(line.split('OPENIM38_PROBE ', 1)[-1], flush=True)
    time.sleep(3)
raise TimeoutError('Native probe did not produce a final result; AVD and app left running for diagnosis')
