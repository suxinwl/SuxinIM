"""One guarded original-login UI action at a time on the owned API30 AVD.

Never prints a credential, never presses Back unless the IME is actually shown,
and refuses the incident's old synthetic password. This is test tooling only.
"""
from pathlib import Path
import argparse
import hashlib
import json
import re
import shlex
import subprocess
import xml.etree.ElementTree as XML

ROOT = Path('O:/')
PACKAGE = 'io.openim.upgrade38.test.dev'
ADB = ['E:/AndroidDev/sdk/platform-tools/adb.exe', '-s', 'emulator-5588']
TEMP_XML = '/data/local/tmp/openim38-guarded-login111.xml'


def adb(*args):
    try:
        result = subprocess.run(ADB + list(args), capture_output=True, timeout=35)
    except BaseException:
        raise RuntimeError('Owned UI command did not complete; arguments suppressed') from None
    if result.returncode:
        raise RuntimeError('Owned UI command failed; arguments suppressed')
    return result.stdout


def foreground():
    windows = adb('shell', 'dumpsys', 'window').decode('utf-8', 'replace')
    focus = next((line for line in windows.splitlines() if 'mCurrentFocus=' in line), '')
    if not re.search(r'\b' + re.escape(PACKAGE) + r'/', focus):
        raise RuntimeError('Foreground is not the owned OpenIM activity; refusing input')


def shape():
    foreground()
    if adb('emu', 'avd', 'name').splitlines()[0] != b'openim38_api30':
        raise RuntimeError('Only the original owned API30 AVD is allowed')
    if b'Override size: 540x1170' not in adb('shell', 'wm', 'size'):
        raise RuntimeError('Restore the explicit phone display setup first')
    try:
        adb('shell', 'uiautomator', 'dump', TEMP_XML)
        root = XML.fromstring(adb('exec-out', 'cat', TEMP_XML))
    finally:
        adb('shell', 'rm', '-f', TEMP_XML)
    foreground()
    nodes = list(root.iter('node'))
    texts = {node.get('text') or node.get('content-desc') or '' for node in nodes
             if node.get('class') != 'android.widget.EditText'}
    if 'Welcome to OpenIM' not in texts or 'IM-FCER 3.8.11+111 SDK: 3.8.3+hotfix.15' not in texts:
        raise RuntimeError('Expected exact111 original login form; refusing input')
    fields = [node for node in nodes if node.get('class') == 'android.widget.EditText']
    if len(fields) != 2 or fields[0].get('password') != 'false' or fields[1].get('password') != 'true':
        raise RuntimeError('Original email/password field identity not established')
    if any(node.get('package') != PACKAGE for node in fields):
        raise RuntimeError('A field belongs to another package')
    return nodes, fields


def measured_bounds(node):
    match = re.fullmatch(r'\[(-?\d+),(-?\d+)\]\[(-?\d+),(-?\d+)\]', node.get('bounds', ''))
    if not match:
        raise RuntimeError('Invalid field bounds')
    x1, y1, x2, y2 = map(int, match.groups())
    if not (0 <= x1 < x2 <= 540 and 0 <= y1 < y2 <= 1170):
        raise RuntimeError('Field outside the declared test display')
    return x1, y1, x2, y2


def center(node):
    x1, y1, x2, y2 = measured_bounds(node)
    return (x1 + x2) // 2, (y1 + y2) // 2


def tap(x, y):
    if not (isinstance(x, int) and isinstance(y, int) and 0 <= x < 540 and 0 <= y < 1170):
        raise RuntimeError('Tap outside the declared test display')
    foreground()
    adb('shell', 'input', 'tap', str(x), str(y))


def input_private_text(value):
    # Keep the secret out of the Windows process command line. Non-PTY shell
    # stdin is captured only in memory, never printed or written to public logs.
    foreground()
    script = ('input text ' + shlex.quote(value.replace(' ', '%s')) + '\n').encode()
    try:
        result = subprocess.run(ADB + ['shell', '-T'], input=script,
                                capture_output=True, timeout=35)
    except BaseException:
        raise RuntimeError('Guarded private input did not complete; contents suppressed') from None
    if result.returncode:
        raise RuntimeError('Guarded private input failed; contents suppressed')


def fixture():
    data = json.loads((ROOT / '.private/test-accounts.json').read_text(encoding='utf-8-sig'))
    users = [user for user in data['users'] if user['userID'] == 'upgrade38_u104']
    if len(users) != 1:
        raise RuntimeError('Exactly the assigned synthetic104 fixture is required')
    user = users[0]
    password = user.get('password') or data.get('password')
    if not isinstance(password, str) or not password:
        raise RuntimeError('Assigned synthetic fixture has no usable rotated password')
    old = json.loads((ROOT / 'tests/flutter/android-109-credential-incident-scope.json').read_text())['oldSecretSHA256']
    if hashlib.sha256(password.encode()).hexdigest() == old:
        raise RuntimeError('Incident credential is not yet rotated; refusing credential input')
    return user['email'], password


parser = argparse.ArgumentParser()
parser.add_argument('action', choices=['status', 'clear', 'fill-email', 'fill-password', 'submit'])
args = parser.parse_args()
nodes, fields = shape()
if args.action == 'clear':
    # These are the original visible clear icons, positioned relative to each
    # freshly measured field, not arbitrary launcher coordinates.
    for index in range(2):
        _, fields = shape()
        if fields[index].get('text', ''):
            if index == 1:
                raise RuntimeError('Nonempty password needs a separately verified clear control; no suffix-coordinate guess allowed')
            bounds = measured_bounds(fields[index])
            if bounds != (65, 353, 446, 392):
                raise RuntimeError('Email clear control was only verified for this exact phone layout')
            _, y = center(fields[index])
            # This email-only clear coordinate was actually verified against
            # the original visible icon. No password suffix offset is inferred.
            tap(bounds[2] + 18, y)
            _, after = shape()
            if after[index].get('text', ''):
                raise RuntimeError('Original clear control did not empty its field; refusing further input')
elif args.action.startswith('fill-'):
    email, password = fixture()
    index = 0 if args.action == 'fill-email' else 1
    if fields[index].get('text', ''):
        raise RuntimeError('Field is not empty; use and verify the clear action first')
    tap(*center(fields[index]))
    _, focused = shape()
    if focused[index].get('focused') != 'true':
        raise RuntimeError('Expected field has not gained focus; refusing credential input')
    foreground()
    value = email if index == 0 else password
    input_private_text(value)
    _, after = shape()
    if after[index].get('focused') != 'true' or not after[index].get('text', ''):
        raise RuntimeError('Field/focus confirmation failed after input; stop and inspect privately')
elif args.action == 'submit':
    email, _ = fixture()
    if fields[0].get('text') != email or not fields[1].get('text', ''):
        raise RuntimeError('Expected complete original form not established')
    ime = adb('shell', 'dumpsys', 'input_method').decode('utf-8', 'replace')
    if re.search(r'\bmInputShown=true\b', ime):
        foreground()
        adb('shell', 'input', 'keyevent', '4')
    nodes, fields = shape()
    buttons = [node for node in nodes if (node.get('text') or node.get('content-desc')) == 'Login']
    if len(buttons) != 1:
        raise RuntimeError('Exact original Login button not established')
    tap(*center(buttons[0]))

if args.action != 'submit':
    _, fields = shape()
    print(json.dumps({'action': args.action, 'foregroundVerified': True,
                      'originalFormVerified': True,
                      'fields': [{'passwordField': node.get('password') == 'true',
                                  'empty': not bool(node.get('text', '')),
                                  'focused': node.get('focused') == 'true'} for node in fields],
                      'secretValuePrinted': False}))
else:
    print(json.dumps({'action': 'submit', 'originalFormAndForegroundVerifiedBeforeClick': True,
                      'loginSuccessNotAssumed': True, 'secretValuePrinted': False}))
