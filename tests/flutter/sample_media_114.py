"""Observe actual114 native/Web media without changing a track or connection."""
from pathlib import Path
from datetime import datetime, timezone
import argparse
import hashlib
import json
import re
import subprocess
import time
import urllib.request

ROOT = Path('O:/')
ADB = ['E:/AndroidDev/sdk/platform-tools/adb.exe', '-s', 'emulator-5588']
PACKAGE = 'io.openim.upgrade38.test.dev'
parser = argparse.ArgumentParser()
parser.add_argument('label')
parser.add_argument('--duration', type=int, default=45)
parser.add_argument('--interval', type=int, default=10)
args = parser.parse_args()
if not re.fullmatch(r'android-114-[a-z0-9-]+', args.label):
    raise RuntimeError('An explicit own114 evidence label is required')
if not (0 <= args.duration <= 180 and 5 <= args.interval <= 30):
    raise RuntimeError('Bounded observation window required')
target = ROOT / 'tests/delivery/media-resume-20261102' / (args.label + '-samples.json')
if target.exists():
    raise RuntimeError('Preserve previous observation bytes')


def adb(*parts):
    return subprocess.check_output(ADB + list(parts), timeout=35)


if adb('emu', 'avd', 'name').splitlines()[0] != b'openim38_api30':
    raise RuntimeError('Only the original owned native x64 AVD is allowed')
facts = adb('shell', 'dumpsys', 'package', PACKAGE).decode('utf-8', 'replace')
if not re.search(r'versionCode=114\b', facts) or not re.search(r'versionName=3\.8\.14\b', facts):
    raise RuntimeError('Installed114 must match the observation')
metadata = json.loads((ROOT / 'tests/flutter/android-main-x64-114.json').read_text())
pid = int(adb('shell', 'pidof', PACKAGE))
device_start = int(adb('shell', 'date', '+%s'))
anr_before = adb('shell', 'dumpsys', 'activity', 'lastanr')
started = datetime.now(timezone.utc).isoformat()
clock = time.monotonic()
samples = []
record = {
    'startedAt': started, 'deviceEpochStart': device_start, 'PID': pid,
    'applicationVersion': '3.8.14+114', 'APK_SHA256': metadata['sha256'],
    'sourceSHA256': metadata['sourceSHA256'], 'originalMain': True, 'ABI': 'x86_64',
    'durationRequested': args.duration, 'intervalRequested': args.interval,
    'observerOnly': True, 'webPeerSyntheticSources': True, 'physicalAudibilityClaimed': False,
    'lastANRBeforeSHA256': hashlib.sha256(anr_before).hexdigest(), 'samples': samples,
}
target.open('x', encoding='utf-8').write(json.dumps(record, indent=2) + '\n')
while True:
    if int(adb('shell', 'pidof', PACKAGE)) != pid:
        raise RuntimeError('Original app process changed; preserve partial observation')
    with urllib.request.urlopen('http://127.0.0.1:17794/state', timeout=30) as response:
        web = json.load(response)
    camera = adb('shell', 'dumpsys', 'media.camera').decode('utf-8', 'replace')
    active = re.search(r'Active Camera Clients:\s*(\[[\s\S]*?\])', camera)
    clients = active.group(1).strip() if active else None
    if clients and not clients[1:-1].strip():
        clients = '[]'
    tasks = subprocess.run(ADB + ['shell', f'cat /proc/{pid}/task/*/comm 2>/dev/null'],
                           capture_output=True, timeout=30)
    names = tasks.stdout.decode('utf-8', 'replace').splitlines()
    # Read only exact known development-counter grammar. The raw log never goes
    # to stdout; the separate private-original/redacted-export workflow owns it.
    log = adb('logcat', '-d', '-v', 'epoch', '-s', 'flutter:I').decode('utf-8', 'replace')
    counters, states = {}, {}
    for line in log.splitlines():
        match = re.fullmatch(r'\s*(\d+\.\d+)\s+(\d+)\s+\d+\s+I\s+flutter\s*:\s*\[OpenIMRtcReceive\] track=([A-Za-z0-9_.:/-]{1,160}) framesDecoded=(\d+(?:\.\d+)?|null) bytesReceived=(\d+(?:\.\d+)?|null)', line)
        if match and int(match[2]) == pid and float(match[1]) >= device_start:
            counters[match[3]] = {'deviceEpoch': float(match[1]),
                                  'framesDecoded': None if match[4] == 'null' else float(match[4]),
                                  'bytesReceived': None if match[5] == 'null' else float(match[5])}
        match = re.fullmatch(r'\s*(\d+\.\d+)\s+(\d+)\s+\d+\s+I\s+flutter\s*:\s*\[OpenIMRtcReceiveState\] track=([A-Za-z0-9_.:/-]{1,160}) state=([A-Za-z0-9_]{1,100})', line)
        if match and int(match[2]) == pid and float(match[1]) >= device_start:
            states[match[3]] = {'deviceEpoch': float(match[1]), 'state': match[4]}
    item = {'elapsedSeconds': round(time.monotonic() - clock, 3),
            'at': datetime.now(timezone.utc).isoformat(), 'activeCameraClients': clients,
            'nativeStatsWorkers': sum(n.startswith('OpenIMRtcStats') for n in names),
            'nativeCleanupWorkers': sum(n.startswith('OpenIMRtcCleanu') for n in names),
            'nativeMediaWorkers': sum(n.startswith('OpenIMRtcMedia-') for n in names),
            'nativeMediaDeadlineThreads': sum(n.startswith('OpenIMRtcMediaD') for n in names),
            'nativeFrameWorkers': sum(n.startswith('NativeFrameMail') for n in names),
            'totalThreads': len(names), 'threadSampleReturnCode': tasks.returncode,
            'nativeRemoteReceiveCounters': counters, 'nativeRemoteReceiveStates': states,
            'web': web}
    samples.append(item)
    record['lastSampleAt'] = item['at']
    target.write_text(json.dumps(record, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'label': args.label, 'elapsed': item['elapsedSeconds'],
                      'nativeFrameWorkers': item['nativeFrameWorkers'],
                      'remoteCounterTracks': len(counters), 'webPeers': len(web.get('peers', []))}), flush=True)
    remaining = args.duration - (time.monotonic() - clock)
    if remaining <= 0:
        break
    time.sleep(min(args.interval, remaining))
anr_after = adb('shell', 'dumpsys', 'activity', 'lastanr')
record.update({'finishedAt': datetime.now(timezone.utc).isoformat(),
               'lastANRAfterSHA256': hashlib.sha256(anr_after).hexdigest(),
               'globalLastANRUnchanged': anr_before == anr_after,
               'unchangedLastANRDoesNotProveNoCleanupErrors': True,
               'toolSHA256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest()})
target.write_text(json.dumps(record, indent=2) + '\n', encoding='utf-8')
