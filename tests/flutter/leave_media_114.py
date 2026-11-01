"""One original114 hangup and bounded observation of the full native barrier.

No timeout is changed, no SIGQUIT, forced close, thread replacement, or direct
SDK call occurs. A later physical completion cannot erase a12s failure.
"""
from pathlib import Path
from datetime import datetime, timezone
import argparse
import hashlib
import json
import re
import subprocess
import time
import xml.etree.ElementTree as ET

ROOT = Path('O:/')
ADB = ['E:/AndroidDev/sdk/platform-tools/adb.exe', '-s', 'emulator-5588']
PACKAGE = 'io.openim.upgrade38.test.dev'
parser = argparse.ArgumentParser()
parser.add_argument('label')
parser.add_argument('--expected', required=True)
parser.add_argument('--control', default='Hang Up')
args = parser.parse_args()
if not re.fullmatch(r'android-114-[a-z0-9-]+', args.label):
    raise RuntimeError('Exact114 evidence label required')
target = ROOT / 'tests/delivery/media-resume-20261102' / (args.label + '-leave.json')
if target.exists():
    raise RuntimeError('Preserve previous leave evidence')


def adb(*parts):
    return subprocess.check_output(ADB + list(parts), timeout=35)


if adb('emu', 'avd', 'name').splitlines()[0] != b'openim38_api30':
    raise RuntimeError('Only the original owned native x64 AVD is allowed')
facts = adb('shell', 'dumpsys', 'package', PACKAGE).decode('utf-8', 'replace')
if not re.search(r'versionCode=114\b', facts) or not re.search(r'versionName=3\.8\.14\b', facts):
    raise RuntimeError('Installed114 required')
focus = adb('shell', 'dumpsys', 'window').decode('utf-8', 'replace')
if PACKAGE + '/' not in next((s for s in focus.splitlines() if 'mCurrentFocus=' in s), ''):
    raise RuntimeError('Own original call must be foreground')
temp = '/data/local/tmp/openim114-owned-hangup.xml'
try:
    adb('shell', 'uiautomator', 'dump', temp)
    nodes = list(ET.fromstring(adb('exec-out', 'cat', temp)).iter('node'))
finally:
    adb('shell', 'rm', '-f', temp)
text = lambda node: node.get('text') or node.get('content-desc') or ''
if args.expected not in {text(n) for n in nodes}:
    raise RuntimeError('Original fixed test-call title is absent')
controls = [n for n in nodes if text(n) == args.control and n.get('package') == PACKAGE]
fields = [n for n in nodes if n.get('class') == 'android.widget.EditText']
empty_underlying_group_composer = (
    args.control == 'Hang Up' and args.expected == 'Android114 media synthetic 1793559206654'
    and len(fields) == 1 and fields[0].get('package') == PACKAGE
    and fields[0].get('password') == 'false' and not fields[0].get('text', ''))
if len(controls) != 1 or (fields and not empty_underlying_group_composer):
    raise RuntimeError('One original hangup and no private editable fields required')
match = re.fullmatch(r'\[(\d+),(\d+)\]\[(\d+),(\d+)\]', controls[0].get('bounds', ''))
if match is None:
    raise RuntimeError('Measured original control bounds required')
x1, y1, x2, y2 = map(int, match.groups())
if not (0 <= x1 < x2 <= 540 and 0 <= y1 < y2 <= 1170):
    raise RuntimeError('Original control is outside declared display')
x, y = (x1 + x2) // 2, (y1 + y2) // 2
if args.control == 'Hang Up':
    # Actual LiveButton binds onTap to its circle image, not its text. This
    # exact geometry was inspected in the owned114 group PNG and source.
    if (x1, y1, x2, y2) != (241, 1025, 324, 1055) or args.expected != 'Android114 media synthetic 1793559206654':
        raise RuntimeError('Only the inspected own group hangup-circle layout is allowed')
    x, y = 282, 970
pid = int(adb('shell', 'pidof', PACKAGE))
device_start = int(adb('shell', 'date', '+%s'))
anr_before = adb('shell', 'dumpsys', 'activity', 'lastanr')
metadata = json.loads((ROOT / 'tests/flutter/android-main-x64-114.json').read_text())
record = {'startedAt': datetime.now(timezone.utc).isoformat(), 'deviceEpochStart': device_start,
          'PID': pid, 'expectedOriginalTitle': args.expected, 'control': args.control,
          'applicationVersion': '3.8.14+114', 'APK_SHA256': metadata['sha256'],
          'sourceSHA256': metadata['sourceSHA256'], 'unperturbedByDiagnosticSignals': True,
          'callerBudgetMillisecondsUnchanged': 12000, 'boundedObservationSeconds': 40,
          'lastANRBeforeSHA256': hashlib.sha256(anr_before).hexdigest(),
          'safeNativeAndCleanupLines': [], 'cameraSamples': [],
          'physicalAcceptanceNotInferredFromFastRemoveSink': True}
record['originalControlClickPoint'] = {'x': x, 'y': y,
    'sourceVerifiedCircleAboveSemanticsText': args.control == 'Hang Up'}
record['emptyUnderlyingOwnGroupComposerAllowed'] = empty_underlying_group_composer
clock = time.monotonic()
seen = set()
terminal_observed = None
try:
    adb('shell', 'input', 'tap', str(x), str(y))
    record['originalHangupTapCompletedSeconds'] = round(time.monotonic() - clock, 3)
    while time.monotonic() - clock < 40:
        if int(adb('shell', 'pidof', PACKAGE)) != pid:
            raise RuntimeError('Original app process changed during leave')
        log = adb('logcat', '-d', '--pid=' + str(pid), '-v', 'epoch').decode('utf-8', 'replace')
        for line in log.splitlines():
            stamp = re.match(r'\s*(\d+\.\d+)\s+', line)
            if not stamp or float(stamp[1]) < device_start:
                continue
            # Only the already-reviewed fixed observation prefixes are exported.
            if '[OpenIMRtcNative]' not in line and '[OpenIMRtcCleanup]' not in line:
                continue
            if line not in seen:
                if re.search(r'eyJ[A-Za-z0-9_-]{15,}\.|https?://|a=ice-|\b(?:token|password)=', line, re.I):
                    raise RuntimeError('Unexpected credential grammar in supposedly safe stage line')
                seen.add(line)
                record['safeNativeAndCleanupLines'].append(line)
            if 'Room.dispose/ownership complete ' in line:
                terminal_observed = terminal_observed or time.monotonic()
        camera = adb('shell', 'dumpsys', 'media.camera').decode('utf-8', 'replace')
        clients = re.search(r'Active Camera Clients:\s*(\[[\s\S]*?\])', camera)
        active = clients.group(1).strip() if clients else None
        if active and not active[1:-1].strip():
            active = '[]'
        record['cameraSamples'].append({'elapsedSeconds': round(time.monotonic() - clock, 3),
                                        'activeCameraClients': active})
        if terminal_observed is not None and active == '[]' and time.monotonic() - terminal_observed >= 2:
            record['stopReason'] = 'observed_full_room_dispose_and_camera_released'
            break
        time.sleep(1)
    else:
        record['stopReason'] = 'bounded40second_observation_expired'
finally:
    record['elapsedSeconds'] = round(time.monotonic() - clock, 3)
    record['finishedAt'] = datetime.now(timezone.utc).isoformat()
    record['lastANRAfterSHA256'] = hashlib.sha256(adb('shell', 'dumpsys', 'activity', 'lastanr')).hexdigest()
    record['toolSHA256'] = hashlib.sha256(Path(__file__).read_bytes()).hexdigest()
    lines = record['safeNativeAndCleanupLines']
    record['failedCleanupLines'] = [s for s in lines if '[OpenIMRtcCleanup]' in s and ' failed ' in s]
    record['ownedResourceCompletionMilliseconds'] = [int(m[1]) for s in lines
        if (m := re.search(r'Room.shutdown/ownedResources complete (\d+)ms', s))]
    record['fullRoomDisposeObserved'] = any('Room.dispose/ownership complete ' in s for s in lines)
    record['cameraReleasedObserved'] = any(s['activeCameraClients'] == '[]' for s in record['cameraSamples'])
    record['manualReviewRequired'] = True
    record['noAutomaticReleaseAcceptance'] = True
    target.open('x', encoding='utf-8').write(json.dumps(record, indent=2) + '\n')
print(json.dumps({'file': target.relative_to(ROOT).as_posix(), 'elapsed': record['elapsedSeconds'],
                  'fullRoomDisposeObserved': record['fullRoomDisposeObserved'],
                  'failedCleanupStages': len(record['failedCleanupLines']),
                  'ownedResourceCompletionMilliseconds': record['ownedResourceCompletionMilliseconds'],
                  'cameraReleasedObserved': record['cameraReleasedObserved']}))
