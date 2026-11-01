"""Read or operate one exact original114 label on the dedicated owned AVD.

This helper never supplies credentials, arbitrary input, SDK calls, or signaling.
Every mutation requires freshly observed original-page labels and own focus.
"""
from pathlib import Path
import argparse
import json
import re
import subprocess
import xml.etree.ElementTree as ET

ADB = ['E:/AndroidDev/sdk/platform-tools/adb.exe', '-s', 'emulator-5588']
PACKAGE = 'io.openim.upgrade38.test.dev'
TEMP = '/data/local/tmp/openim114-media-ui.xml'


def adb(*args):
    return subprocess.check_output(ADB + list(args), timeout=40)


def focus():
    text = adb('shell', 'dumpsys', 'window').decode('utf-8', 'replace')
    line = next((s for s in text.splitlines() if 'mCurrentFocus=' in s), '')
    if PACKAGE + '/' not in line:
        raise RuntimeError('Own main activity is not focused; refusing UI operation')


def guard():
    if adb('emu', 'avd', 'name').splitlines()[0] != b'openim38_api30':
        raise RuntimeError('Only the original owned native x64 AVD is allowed')
    facts = adb('shell', 'dumpsys', 'package', PACKAGE).decode('utf-8', 'replace')
    if not re.search(r'versionCode=114\b', facts) or not re.search(r'versionName=3\.8\.14\b', facts):
        raise RuntimeError('Exact installed114 package required')
    if b'Override size: 540x1170' not in adb('shell', 'wm', 'size'):
        raise RuntimeError('Only the declared540x1170 phone layout is allowed')
    focus()


def hierarchy():
    guard()
    try:
        adb('shell', 'uiautomator', 'dump', TEMP)
        nodes = list(ET.fromstring(adb('exec-out', 'cat', TEMP)).iter('node'))
    finally:
        adb('shell', 'rm', '-f', TEMP)
    focus()
    # The login helper owns secret entry; do not export or operate its form.
    if any(n.get('class') == 'android.widget.EditText' and n.get('password') == 'true'
           for n in nodes):
        raise RuntimeError('Password form belongs to the separate guarded login helper')
    return nodes


def label(node):
    return node.get('text') or node.get('content-desc') or ''


parser = argparse.ArgumentParser()
parser.add_argument('action', choices=['status', 'tap-label', 'back', 'tap-old-meeting-enter', 'home-plus', 'group-call-menu'])
parser.add_argument('--label')
parser.add_argument('--requires', action='append', default=[])
args = parser.parse_args()
nodes = hierarchy()
texts = {label(n) for n in nodes}
for expected in args.requires:
    if expected not in texts:
        raise RuntimeError('Fresh original-page guard label is absent')
if args.action == 'group-call-menu':
    required = {'Android114 media synthetic 1793559206654', '(3)'}
    if not required.issubset(texts) or {'Hang Up', 'End'} & texts:
        raise RuntimeError('Only the inspected own1739269662 three-person chat is allowed')
    if any(n.get('class') == 'android.widget.EditText' and n.get('text') for n in nodes):
        raise RuntimeError('Composer must be empty before opening original call menu')
    focus()
    adb('shell', 'input', 'tap', '432', '76')
    print(json.dumps({'action': args.action, 'verifiedCheckpoint': 'android-114-group-first-chat.png',
                      'ownFocusAndOriginalPageVerified': True, 'resultNotAssumed': True}))
elif args.action == 'home-plus':
    # Unlabelled plus was visually verified at this exact phone checkpoint.
    required = {'升级测试104', 'OpenIM', 'Contacts', 'Discover', 'Mine'}
    if not required.issubset(texts) or any(n.get('class') == 'android.widget.EditText' for n in nodes):
        raise RuntimeError('Exact original synthetic104 Home is required')
    focus()
    adb('shell', 'input', 'tap', '497', '87')
    print(json.dumps({'action': 'home-plus', 'verifiedCheckpoint': 'android-114-home-before-media.png',
                      'ownFocusAndOriginalPageVerified': True, 'resultNotAssumed': True}))
elif args.action == 'back':
    if not args.requires:
        raise RuntimeError('An exact current-page label is required before Back')
    if re.search(r'\bmInputShown=true\b', adb('shell', 'dumpsys', 'input_method').decode('utf-8', 'replace')):
        raise RuntimeError('Back with an active keyboard is outside this navigation helper')
    focus()
    adb('shell', 'input', 'keyevent', '4')
    print(json.dumps({'action': 'back', 'ownFocusAndOriginalPageVerified': True,
                      'resultNotAssumed': True}))
elif args.action in ['tap-label', 'tap-old-meeting-enter']:
    if not args.requires or not args.label:
        raise RuntimeError('A declared original-page guard and exact label are required')
    matches = [n for n in nodes if label(n) == args.label and n.get('package') == PACKAGE]
    if len(matches) != 1:
        raise RuntimeError('Exactly one own-package original label is required')
    match = re.fullmatch(r'\[(\d+),(\d+)\]\[(\d+),(\d+)\]', matches[0].get('bounds', ''))
    if match is None:
        raise RuntimeError('Measured original-label bounds required')
    x1, y1, x2, y2 = map(int, match.groups())
    if not (0 <= x1 < x2 <= 540 and 0 <= y1 < y2 <= 1170):
        raise RuntimeError('Original control is outside the phone display')
    x, y = (x1 + x2) // 2, (y1 + y2) // 2
    if args.action == 'tap-old-meeting-enter':
        # This old synthetic card merges its button into one semantics node.
        # Its lower Enter row was visually inspected in the exact114 PNG;
        # never infer a new room/card geometry or tap its non-action body again.
        if (x1, y1, x2, y2) != (96, 313, 452, 578) or not args.label.startswith('Video meeting initiated by '):
            raise RuntimeError('Only the inspected old synthetic card geometry is allowed')
        if 'Number: 3f4012792cdae08f4168f9b6a2ccea45\nEnter Meeting' not in args.label:
            raise RuntimeError('Only the preserved synthetic110 meeting card is allowed')
        y = 544
    focus()
    adb('shell', 'input', 'tap', str(x), str(y))
    print(json.dumps({'action': args.action, 'label': args.label,
                      'ownFocusAndOriginalPageVerified': True, 'resultNotAssumed': True}))
else:
    print(json.dumps({'action': 'status', 'labels': [
        {'text': label(n), 'bounds': n.get('bounds'), 'clickable': n.get('clickable') == 'true'}
        for n in nodes if label(n) and n.get('class') != 'android.widget.EditText'
    ], 'inputFieldValuesOmitted': True}, ensure_ascii=False))
