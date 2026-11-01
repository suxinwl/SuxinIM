"""Capture this fixed, input-free 110 meeting on the physical display only."""
from pathlib import Path
import datetime
import hashlib
import json
import re
import subprocess
import sys
import xml.etree.ElementTree as ET

ROOT = Path(__file__).resolve().parents[2]
PACKAGE = 'io.openim.upgrade38.test.dev'
ROOM = '3f4012792cdae08f4168f9b6a2ccea45'
ADB = ['E:/AndroidDev/sdk/platform-tools/adb.exe', '-s', 'emulator-5588']
label = sys.argv[1]
if not re.fullmatch(r'android-110-[a-z0-9-]+', label):
    raise RuntimeError('Explicit own110 meeting label required')
output = ROOT / 'tests/flutter' / (label + '.json')
image = output.with_suffix('.png')
if output.exists() or image.exists():
    raise RuntimeError('Preserve existing evidence')


def adb(*args):
    return subprocess.check_output(ADB + list(args), timeout=45)


if adb('emu', 'avd', 'name').splitlines()[0] != b'openim38_api30':
    raise RuntimeError('Wrong owned AVD')
if int(adb('shell', 'pidof', PACKAGE)) != 7093:
    raise RuntimeError('Expected existing110 PID')
package = adb('shell', 'dumpsys', 'package', PACKAGE).decode(errors='replace')
if not re.search(r'versionCode=110\b', package) or 'versionName=3.8.10' not in package:
    raise RuntimeError('Expected installed110')
display_text = adb('shell', 'dumpsys', 'window', 'displays').decode(errors='replace')
sections = re.split(r'(?m)^\s*Display: mDisplayId=', display_text)
physical = [s for s in sections[1:] if re.match(r'0\s+stacks=', s)]
if len(physical) != 1:
    raise RuntimeError('Exactly one physical display0 required')
focus = re.findall(r'(?m)^\s*mCurrentFocus=(.*)$', physical[0])
if len(focus) != 1 or PACKAGE + '/' not in focus[0]:
    raise RuntimeError('Own app must focus physical display0')
xml = '/data/local/tmp/openim110-meeting-checkpoint.xml'
try:
    nodes = []
    for attempt in range(3):
        adb('shell', 'uiautomator', 'dump', xml)
        try:
            nodes = list(ET.fromstring(adb('exec-out', 'cat', xml)).iter('node'))
        except ET.ParseError:
            continue
        if nodes:
            break
    labels = [n.get('text') or n.get('content-desc') or '' for n in nodes]
    if not any(ROOM in s for s in labels) or 'End' not in labels:
        raise RuntimeError('Expected fixed original meeting overlay and detail')
    if any(n.get('class') == 'android.widget.EditText' for n in nodes):
        raise RuntimeError('No editable field allowed in this meeting capture')
    png = adb('exec-out', 'screencap', '-p')
    if not png.startswith(b'\x89PNG\r\n\x1a\n'):
        raise RuntimeError('Invalid PNG')
    artifact = json.loads((ROOT / 'tests/flutter/android-main-x64-110.json').read_text(encoding='utf-8'))
    image.open('xb').write(png)
    record = {'capturedAt': datetime.datetime.now(datetime.timezone.utc).isoformat(),
              'PID': 7093, 'roomID': ROOM, 'package': PACKAGE, 'physicalDisplay': 0,
              'physicalDisplayForegroundVerified': True, 'noEditableFields': True,
              'APK_SHA256': artifact['sha256'], 'sourceSHA256': artifact['sourceSHA256'],
              'entrypoint': 'lib/main.dart', 'ABI': 'x86_64',
              'labels': [{'text': n.get('text') or n.get('content-desc'), 'bounds': n.get('bounds')}
                         for n in nodes if n.get('text') or n.get('content-desc')],
              'screenshot': image.relative_to(ROOT).as_posix(),
              'screenshotSHA256': hashlib.sha256(png).hexdigest(),
              'checkpointOnlyNotAutomaticPass': True}
    output.open('x', encoding='utf-8').write(json.dumps(record, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps({'file': output.relative_to(ROOT).as_posix(), 'labels': labels}, ensure_ascii=False))
finally:
    adb('shell', 'rm', '-f', xml)
