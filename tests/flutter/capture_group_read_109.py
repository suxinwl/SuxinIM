"""Capture only this task's original Android group-reader UI; never input text."""
import hashlib
import json
import re
import subprocess
import sys
import xml.etree.ElementTree as ET
from datetime import datetime, timezone
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
ADB = ['E:/AndroidDev/sdk/platform-tools/adb.exe', '-s', 'emulator-5588']
PACKAGE = 'io.openim.upgrade38.test.dev'
label = sys.argv[1]
if not re.fullmatch(r'android-109-group-read-[a-z0-9-]+', label):
    raise SystemExit('Unexpected evidence label')


def adb(*args):
    return subprocess.check_output(ADB + list(args), timeout=60)


focus = adb('shell', 'dumpsys', 'window').decode(errors='replace')
current = next((line.strip() for line in focus.splitlines() if 'mCurrentFocus=' in line), '')
if PACKAGE not in current:
    raise SystemExit('Own application is not foreground')
adb('shell', 'uiautomator', 'dump', '/sdcard/openim-group-reader-109.xml')
try:
    xml = ET.fromstring(adb('shell', 'cat', '/sdcard/openim-group-reader-109.xml'))
    labels = []
    for node in xml.iter('node'):
        if node.get('class') == 'android.widget.EditText':
            continue
        value = node.get('text') or node.get('content-desc') or ''
        if value:
            labels.append({'text': value, 'bounds': node.get('bounds'),
                           'clickable': node.get('clickable')})
    if not any(item['text'] == 'Message Recipient List' for item in labels):
        raise SystemExit('Expected original message-recipient page')
    png = adb('exec-out', 'screencap', '-p')
    if not png.startswith(b'\x89PNG\r\n\x1a\n'):
        raise SystemExit('Invalid screenshot bytes')
    image = ROOT / 'tests/flutter' / (label + '.png')
    record = image.with_suffix('.json')
    if image.exists() or record.exists():
        raise SystemExit('Refusing to overwrite evidence')
    image.write_bytes(png)
    result = {
        'at': datetime.now(timezone.utc).isoformat(),
        'package': PACKAGE, 'PID': int(adb('shell', 'pidof', PACKAGE)),
        'APK_SHA256': '2a02d3a885713b6492796916e981cca3b6b6c49d9ed2f838c0c3c902d9c890c4',
        'entrypoint': 'lib/main.dart', 'ABI': 'x86_64',
        'groupID': '2901057568',
        'clientMsgID': '93a25062adbd023e30b96338411845e6',
        'messageText': 'a108gr1', 'historicalSeq': 2,
        'labels': labels, 'screenshot': image.relative_to(ROOT).as_posix(),
        'screenshotSHA256': hashlib.sha256(png).hexdigest(),
        'originalUI': True,
    }
    record.write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps(result, ensure_ascii=False))
finally:
    adb('shell', 'rm', '/sdcard/openim-group-reader-109.xml')
