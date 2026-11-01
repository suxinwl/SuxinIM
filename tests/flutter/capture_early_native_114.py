"""One bounded native stack at an observed own114 media-blockage trigger.

Preparation/import/self-check never start adb or a call. This observer never
creates, accepts, hangs up, retries, kills an app, changes a timeout, or injects
an SDK request. Its log stream and backtrace originals remain private.
"""
from pathlib import Path
from datetime import datetime, timezone
import argparse
import hashlib
import json
import queue
import re
import subprocess
import threading
import time

ROOT = Path('O:/')
ADB = ['E:/AndroidDev/sdk/platform-tools/adb.exe', '-s', 'emulator-5588']
PACKAGE = 'io.openim.upgrade38.test.dev'
PID = 5016


class Trigger:
    def __init__(self, device_start):
        self.device_start = device_start
        self.pending = {}
        self.observed = []

    def feed(self, line, clock):
        match = re.match(r'^\s*(\d+\.\d+)\s+(\d+)\s+\d+\s+[VDIWEF]\s+[^:]+:\s*(.*)$', line)
        if not match or int(match[2]) != PID or float(match[1]) < self.device_start:
            return None
        text = match[3]
        state = re.fullmatch(r'\[OpenIMRtcReceiveState\] track=([A-Za-z0-9_.:/-]{1,160}) state=(STOP_RTC_STATS_TIMEOUT|STOP_RTC_STATS_UNAVAILABLE)', text)
        if state:
            return {'type': 'first_receive_stats_stop', 'deviceEpoch': float(match[1]),
                    'track': state[1], 'state': state[2],
                    'sourceLineSHA256': hashlib.sha256(line.encode()).hexdigest()}
        native = re.search(r'^\[OpenIMRtcNative\] operation=renderer\.detach phase=(javaRemoveSinkBegin|javaRemoveSinkEnd) rendererJava#([0-9a-f]+)\b', text)
        if native:
            safe = {'phase': native[1], 'rendererJavaID': native[2], 'deviceEpoch': float(match[1]),
                    'observedMonotonic': clock, 'sourceLineSHA256': hashlib.sha256(line.encode()).hexdigest()}
            self.observed.append(safe)
            if native[1] == 'javaRemoveSinkBegin':
                self.pending.setdefault(native[2], safe)
            else:
                self.pending.pop(native[2], None)
        return None

    def due(self, clock):
        ready = [item for item in self.pending.values() if clock - item['observedMonotonic'] >= 3]
        if not ready:
            return None
        first = min(ready, key=lambda item: item['observedMonotonic'])
        return {'type': 'remove_sink_no_end_after_three_seconds', **first,
                'pendingObservedSeconds': round(clock - first['observedMonotonic'], 3)}


def self_check():
    def line(t, msg, pid=PID):
        return f'{t:.3f} {pid} 6993 D FlutterWebRTCPlugin: {msg}'
    begin = '[OpenIMRtcNative] operation=renderer.detach phase=javaRemoveSinkBegin rendererJava#ab123 bindingTrackJava#1 returned=false elapsedMs=0'
    end = begin.replace('javaRemoveSinkBegin', 'javaRemoveSinkEnd')
    a = Trigger(100)
    assert a.feed(line(99, begin), 1) is None and not a.pending
    assert a.feed(line(101, begin, 999), 1) is None and not a.pending
    assert a.feed(line(101, begin), 1) is None and a.due(3.999) is None
    assert a.due(4)['type'] == 'remove_sink_no_end_after_three_seconds'
    assert a.feed(line(102, end), 4.001) is None and not a.pending and a.due(99) is None
    b = Trigger(100)
    for state in ['PENDING_RTC_STATS_BUSY', 'STOP_RTC_STATS_TIMEOUT', 'STOP_RTC_STATS_UNAVAILABLE']:
        got = b.feed(line(102, f'[OpenIMRtcReceiveState] track=TR_synthetic state={state}'), 2)
        assert bool(got) == state.startswith('STOP_')
    assert b.feed(line(103, 'unrelated token=do-not-export'), 3) is None
    print(json.dumps({'parserChecksPassed': True, 'deviceOrMediaOperations': 0,
                      'covers': ['stale_and_other_PID_ignored', 'three_second_boundary',
                                 'physical_end_cancels_trigger', 'only_explicit_stats_stop',
                                 'unrelated_log_not_exported']}))


def main(label):
    if label != 'android-114-directed-group-native':
        raise RuntimeError('Only the single parent-coordinated diagnostic label is allowed')
    here = ROOT / 'tests/delivery/media-resume-20261102'
    target = here / (label + '-result.json')
    private = ROOT / '.private/android-raw-evidence'
    raw_stream = private / (label + '-stream.txt')
    stack_file = private / (label + '-backtrace.txt')
    error_file = private / (label + '-backtrace.stderr.txt')
    if any(path.exists() for path in [target, raw_stream, stack_file, error_file]):
        raise RuntimeError('This diagnostic can run once; preserve preceding originals')

    def adb(*parts, timeout=12):
        return subprocess.check_output(ADB + list(parts), timeout=timeout)

    if adb('emu', 'avd', 'name').splitlines()[0] != b'openim38_api30':
        raise RuntimeError('Owned original AVD required')
    if adb('shell', 'pidof', PACKAGE).strip() != str(PID).encode():
        raise RuntimeError('Original114 process must still match')
    facts = adb('shell', 'dumpsys', 'package', PACKAGE).decode('utf-8', 'replace')
    if not re.search(r'versionCode=114\b', facts) or not re.search(r'versionName=3\.8\.14\b', facts):
        raise RuntimeError('Exact114 installed version required')
    device_start = int(adb('shell', 'date', '+%s'))
    record = {'startedAt': datetime.now(timezone.utc).isoformat(), 'PID': PID,
              'applicationVersion': '3.8.14+114', 'deviceEpochStart': device_start,
              'diagnosticOnlyNotAcceptance': True, 'totalObservationCaptureLimitSeconds': 150,
              'establishedMediaTriggerObservationLimitSeconds': 120,
              'deviceNativeCaptureLimitSeconds': 20, 'maximumNativeCaptures': 1,
              'sourceTimeoutVersionMutation': False, 'SDKOrMediaAction': False,
              'allRawBytesPrivate': True, 'toolSHA256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
              'nativeCapturePerformed': False}
    started = time.monotonic()
    deadline = started + 150
    observe_deadline = started + 120
    detector = Trigger(device_start)
    events = queue.Queue(maxsize=1024)
    overflow = threading.Event()
    reader_errors = []
    process = subprocess.Popen(ADB + ['logcat', '--pid=' + str(PID), '-v', 'epoch', '-T', '1',
                                     'FlutterWebRTCPlugin:D', 'flutter:I', '*:S'],
                               stdout=subprocess.PIPE, stderr=subprocess.DEVNULL)

    def receive():
        try:
            with raw_stream.open('xb') as output:
                for line in process.stdout:
                    output.write(line)
                    try:
                        events.put_nowait(line)
                    except queue.Full:
                        overflow.set()
        except BaseException as error:
            reader_errors.append(type(error).__name__)

    reader = threading.Thread(target=receive, name='owned114-log-reader', daemon=True)
    reader.start()
    print(json.dumps({'phase': 'armed', 'label': label, 'at': record['startedAt'],
                      'PID': PID, 'observationLimitSeconds': 120,
                      'totalLimitSeconds': 150, 'noMediaStartedByObserver': True}), flush=True)
    trigger = None
    try:
        while time.monotonic() < observe_deadline:
            if overflow.is_set() or reader_errors:
                record['stopReason'] = 'raw_preserved_trigger_queue_or_reader_failed'
                break
            if process.poll() is not None:
                record['stopReason'] = 'log_stream_ended'
                break
            try:
                raw = events.get(timeout=.05)
                trigger = detector.feed(raw.decode('utf-8', 'replace').rstrip('\r\n'), time.monotonic())
            except queue.Empty:
                pass
            trigger = trigger or detector.due(time.monotonic())
            if trigger is None:
                continue
            record['trigger'] = trigger
            record['triggerAt'] = datetime.now(timezone.utc).isoformat()
            remaining = deadline - time.monotonic() - 5
            if remaining < 7:
                record['stopReason'] = 'trigger_too_late_for_bounded_capture'
                break
            if adb('shell', 'pidof', PACKAGE, timeout=min(3, remaining)).strip() != str(PID).encode():
                raise RuntimeError('Own process changed before diagnostic')
            limit = max(1, min(20, int(deadline - time.monotonic() - 9)))
            record['nativeCapturePerformed'] = True
            record['actualDeviceNativeTimeoutSeconds'] = limit
            capture_started = time.monotonic()
            try:
                result = subprocess.run(ADB + ['shell', 'timeout', str(limit), 'debuggerd', '-b', str(PID)],
                                        capture_output=True, timeout=min(limit + 4, deadline - time.monotonic() - 5))
                out, err = result.stdout, result.stderr
                record['nativeExitCode'] = result.returncode
            except subprocess.TimeoutExpired as error:
                out, err = error.stdout or b'', error.stderr or b''
                record['nativeHostTimeout'] = True
            stack_file.open('xb').write(out)
            error_file.open('xb').write(err)
            record['nativeCaptureElapsedSeconds'] = round(time.monotonic() - capture_started, 3)
            record['nativeThreadHeaders'] = len(re.findall(rb'(?m)^"[^"\n]+" sysTid=', out))
            record['nativeFrames'] = len(re.findall(rb'(?m)^\s*#\d+\s+pc ', out))
            record['stopReason'] = 'one_triggered_native_capture_completed'
            break
        else:
            record['stopReason'] = 'bounded_window_without_capture'
    finally:
        # Only this read-only logcat client is stopped. Never stop the app,
        # emulator, room, native worker, or a caller's physical cleanup.
        process.terminate()
        try:
            process.wait(timeout=2)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait(timeout=1)
        reader.join(timeout=2)
        record['readerFinished'] = not reader.is_alive()
        record['triggerQueueOverflow'] = overflow.is_set()
        record['readerErrors'] = reader_errors
        record['observedSafeDetachEvents'] = detector.observed
        record['elapsedSeconds'] = round(time.monotonic() - started, 3)
        record['finishedAt'] = datetime.now(timezone.utc).isoformat()
        record['privateFiles'] = []
        for path in [raw_stream, stack_file, error_file]:
            if path.exists() and not reader.is_alive():
                record['privateFiles'].append({'path': path.relative_to(ROOT).as_posix(),
                                                'bytes': path.stat().st_size,
                                                'sha256': hashlib.sha256(path.read_bytes()).hexdigest()})
        target.open('x', encoding='utf-8').write(json.dumps(record, indent=2) + '\n')
    print(json.dumps({'result': target.relative_to(ROOT).as_posix(),
                      'stopReason': record.get('stopReason'), 'elapsedSeconds': record['elapsedSeconds'],
                      'nativeCapturePerformed': record['nativeCapturePerformed'],
                      'nativeFrames': record.get('nativeFrames')}), flush=True)


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('label', nargs='?')
    parser.add_argument('--self-check', action='store_true')
    args = parser.parse_args()
    if args.self_check:
        self_check()
    else:
        main(args.label)
