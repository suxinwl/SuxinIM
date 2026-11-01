"""Capture a named own114 UI checkpoint, refusing nonempty secret text fields."""
from pathlib import Path
import datetime
import hashlib
import json
import re
import subprocess
import sys
import xml.etree.ElementTree as ET

root = Path(__file__).absolute().parents[2]
adb_args = ['E:/AndroidDev/sdk/platform-tools/adb.exe', '-s', 'emulator-5588']
package = 'io.openim.upgrade38.test.dev'
label = sys.argv[1]
if not re.fullmatch(r'android-114-[a-z0-9-]+', label):
    raise RuntimeError('Explicit114 evidence label required')
image = root / 'tests/flutter' / (label + '.png')
record = image.with_suffix('.json')
if image.exists() or record.exists():
    raise RuntimeError('Preserve existing UI checkpoint')


def adb(*args):
    return subprocess.check_output(adb_args + list(args), timeout=45)


if adb('emu', 'avd', 'name').splitlines()[0] != b'openim38_api30':
    raise RuntimeError('Owned original API30 AVD only')
focus = adb('shell', 'dumpsys', 'window').decode('utf-8', 'replace')
if package not in next((line for line in focus.splitlines() if 'mCurrentFocus=' in line), ''):
    raise RuntimeError('Own app is not foreground; screenshot refused')
facts = adb('shell', 'dumpsys', 'package', package).decode('utf-8', 'replace')
if not re.search(r'versionCode=114\b', facts) or not re.search(r'versionName=3\.8\.14\b', facts):
    raise RuntimeError('Expected installed114 own package')
xml_path = '/data/local/tmp/openim114-ui-checkpoint.xml'
try:
    adb('shell', 'uiautomator', 'dump', xml_path)
    nodes = list(ET.fromstring(adb('exec-out', 'cat', xml_path)).iter('node'))
    fields = [node for node in nodes if node.get('class') == 'android.widget.EditText']
    if any(node.get('text', '') not in ['', 'u104@openim38.test'] for node in fields):
        raise RuntimeError('A nonempty private input may be visible; screenshot refused')
    labels = [{'text': node.get('text') or node.get('content-desc'), 'bounds': node.get('bounds')}
              for node in nodes if node.get('class') != 'android.widget.EditText'
              and (node.get('text') or node.get('content-desc'))]
    png = adb('exec-out', 'screencap', '-p')
    if not png.startswith(b'\x89PNG\r\n\x1a\n'):
        raise RuntimeError('Invalid screenshot')
    image.write_bytes(png)
    artifact = json.loads((root / 'tests/flutter/android-main-x64-114.json').read_text(encoding='utf-8'))
    result = {'capturedAt': datetime.datetime.now(datetime.timezone.utc).isoformat(),
              'package': package, 'PID': int(adb('shell', 'pidof', package)),
              'APK_SHA256': artifact['sha256'], 'sourceSHA256': artifact['sourceSHA256'],
              'entrypoint': 'lib/main.dart', 'ABI': 'x86_64', 'labels': labels,
              'screenshot': image.relative_to(root).as_posix(),
              'screenshotSHA256': hashlib.sha256(png).hexdigest(),
              'privateInputCaptureRefused': True, 'checkpointOnlyNotAutomaticPass': True}
    record.write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps(result, ensure_ascii=False))
finally:
    adb('shell', 'rm', '-f', xml_path)
