"""One original host end, with one conditional diagnostic SIGQUIT after physical detach exceeds12s at most."""
from pathlib import Path
import datetime
import hashlib
import json
import re
import subprocess
import time
import xml.etree.ElementTree as ET

ROOT = Path(__file__).resolve().parents[2]
ADB = ['E:/AndroidDev/sdk/platform-tools/adb.exe', '-s', 'emulator-5588']
PACKAGE = 'io.openim.upgrade38.test.dev'
ROOM = '3f4012792cdae08f4168f9b6a2ccea45'
PID = 7093
OUTPUT = ROOT / 'tests/flutter/android-110-meeting-original-host-confirmed-end.json'
if OUTPUT.exists():
    raise RuntimeError('Preserve original end evidence')


def adb(*args, timeout=45):
    return subprocess.check_output(ADB + list(args), timeout=timeout)


def assert_identity():
    if int(adb('shell', 'pidof', PACKAGE)) != PID:
        raise RuntimeError('Original own app PID changed')


assert_identity()
if adb('emu', 'avd', 'name').splitlines()[0] != b'openim38_api30':
    raise RuntimeError('Wrong owned AVD')
xml = '/data/local/tmp/openim110-end-meeting.xml'
adb('shell', 'uiautomator', 'dump', xml)
nodes = list(ET.fromstring(adb('exec-out', 'cat', xml)).iter('node'))
labels = [n.get('text') or n.get('content-desc') or '' for n in nodes]
if not any(ROOM in s for s in labels) or not any('Are you sure you want to end the meeting?' in s for s in labels):
    raise RuntimeError('Expected fixed original host closing sheet')
targets = [n for n in nodes if (n.get('text') or n.get('content-desc')) == 'OK']
if len(targets) != 1 or any(n.get('class') == 'android.widget.EditText' for n in nodes):
    raise RuntimeError('Original end action not uniquely available')
bounds = list(map(int, re.fullmatch(r'\[(-?\d+),(-?\d+)\]\[(-?\d+),(-?\d+)\]', targets[0].get('bounds')).groups()))
x, y = (bounds[0] + bounds[2]) // 2, (bounds[1] + bounds[3]) // 2
if not 0 <= x < 540 or not 0 <= y < 1170:
    raise RuntimeError('Outside original test display')
before_anr = adb('shell', 'dumpsys', 'activity', 'lastanr')
before_files = adb('shell', 'ls', '-1', '/data/anr').decode(errors='replace').splitlines()
artifact = json.loads((ROOT / 'tests/flutter/android-main-x64-110.json').read_text(encoding='utf-8'))
started_epoch = time.time()
started = time.monotonic()
record = {'startedAt': datetime.datetime.now(datetime.timezone.utc).isoformat(),
          'PID': PID, 'roomID': ROOM, 'APK_SHA256': artifact['sha256'],
          'sourceSHA256': artifact['sourceSHA256'], 'originalEndMeetingFinalConfirmTapped': False,
          'diagnosticWindow': True, 'unperturbedPerformanceAcceptance': False,
          'nativeProcSamplesWereUnavailable': True,
          'conditionalSIGQUITCount': 0, 'traceFileNamesBefore': before_files,
          'lastANRSHA256Before': hashlib.sha256(before_anr).hexdigest(),
          'observedNativeAndCleanupStages': []}
try:
    adb('shell', 'input', 'tap', str(x), str(y))
    record['originalEndMeetingFinalConfirmTapped'] = True
    record['tapCompletedSeconds'] = time.monotonic() - started
    seen = set()
    pending = {}
    finished_at = None
    while time.monotonic() - started < 120:
        assert_identity()
        lines = adb('logcat', '-d', '--pid=' + str(PID), '-v', 'epoch').decode(errors='replace').splitlines()
        for line in lines:
            match = re.match(r'\s*(\d+\.\d+)\s+', line)
            if not match or float(match[1]) < started_epoch - 0.5:
                continue
            if '[OpenIMRtcNative]' not in line and '[OpenIMRtcCleanup]' not in line:
                continue
            # These fixed native/stage prefixes contain no URI, SDP or credentials.
            if line not in seen:
                seen.add(line)
                record['observedNativeAndCleanupStages'].append(line)
            begin = re.search(r'phase=javaRemoveSinkBegin rendererJava#([0-9a-f]+)', line)
            end = re.search(r'phase=javaRemoveSinkEnd rendererJava#([0-9a-f]+)', line)
            if begin:
                pending.setdefault(begin[1], float(match[1]))
            if end:
                pending.pop(end[1], None)
            if re.search(r'Room.shutdown/ownedResources (?:complete|failed) ', line):
                finished_at = finished_at or time.monotonic()
        stalled = {key: time.time() - at for key, at in pending.items() if time.time() - at > 12}
        if stalled and record['conditionalSIGQUITCount'] == 0:
            assert_identity()
            record['SIGQUITAt'] = datetime.datetime.now(datetime.timezone.utc).isoformat()
            record['pendingJavaRemoveSinkAgeSecondsAtSignal'] = stalled
            adb('shell', 'kill', '-3', str(PID))
            record['conditionalSIGQUITCount'] = 1
        if finished_at is not None and time.monotonic() - finished_at > 3:
            record['stopReason'] = 'observed_owned_resource_cleanup_terminal_stage'
            break
        time.sleep(2)
    else:
        record['stopReason'] = 'bounded120second_observation_completed'
    record['pendingJavaRemoveSinkAtStop'] = list(pending)
    record['lastANRSHA256After'] = hashlib.sha256(adb('shell', 'dumpsys', 'activity', 'lastanr')).hexdigest()
    record['traceFileNamesAfter'] = adb('shell', 'ls', '-1', '/data/anr').decode(errors='replace').splitlines()
    record['newTraceFiles'] = [n for n in record['traceFileNamesAfter'] if n not in before_files]
finally:
    record['hostElapsedSeconds'] = time.monotonic() - started
    OUTPUT.open('x', encoding='utf-8').write(json.dumps(record, indent=2) + '\n')
    adb('shell', 'rm', '-f', xml)
print(json.dumps({'file': OUTPUT.relative_to(ROOT).as_posix(),
                  'signalCount': record['conditionalSIGQUITCount'],
                  'stopReason': record.get('stopReason'),
                  'elapsedSeconds': record['hostElapsedSeconds']}))
