"""Read-only 108 call1 timing review; writes evidence, never changes product/runtime."""
from __future__ import annotations

import base64
import hashlib
import json
import re
import urllib.request
from datetime import datetime, timezone
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
OUT = ROOT / 'tests/flutter/native-detach-timing-review-108.json'
PRIMARY_DIR = ROOT / 'tests/flutter/diagnostics/108/detach-primary'
NATIVE = Path('clients/flutter/local_plugin/compat/flutter_webrtc-1.6.0/android/src/main/java/com/cloudwebrtc/webrtc')
COMMIT = '2ee3d0ca17c93bc2e8519efb6cb27625cf3efefd'
PRIMARY_FILES = [
    'sdk/android/api/org/webrtc/VideoTrack.java',
    'sdk/android/src/jni/video_track.cc',
    'pc/media_stream_track_proxy.h',
    'pc/proxy.h',
    'pc/video_track.cc',
    'pc/peer_connection_factory.cc',
    'pc/video_rtp_receiver.cc',
    'media/base/video_broadcaster.cc',
    'sdk/android/api/org/webrtc/EglRenderer.java',
]


def describe(path: Path) -> dict:
    absolute = path if path.is_absolute() else ROOT / path
    data = absolute.read_bytes()
    return {'path': absolute.relative_to(ROOT).as_posix(), 'sha256': hashlib.sha256(data).hexdigest(), 'bytes': len(data)}


def main() -> None:
    if OUT.exists() or PRIMARY_DIR.exists():
        raise SystemExit('Refuse to overwrite historical review/evidence')
    log_path = Path('tests/flutter/android-108-call1-complete-logcat.txt')
    log_rows = (ROOT / log_path).read_text(encoding='utf-8', errors='replace').splitlines()
    evidence = []
    for number, line in enumerate(log_rows, 1):
        if re.match(r'11-01 07:17:(?:3[1-9]|4[0-9]|5[0-3])\.', line) and ' 4997 ' in line:
            native = '[OpenIMRtcNative]' in line
            gc = ' GC ' in line
            camera = 'CameraEventsHandler.' in line
            connection = 'FlutterWebRTCPlugin: onConnectionChange' in line
            if native or gc or camera or connection:
                evidence.append({'line': number, 'text': line})
    monitor = [{'line': n, 'text': line} for n, line in enumerate(log_rows, 1) if '[OpenIMRtcReceiveState]' in line]
    inputs = [log_path, Path('tests/flutter/android-108-call1-log-event-review.json'),
              Path('tests/flutter/review_rtc_log.py'), Path('tests/flutter/rtc-log-independent-review-108.json')]
    inputs += [NATIVE / name for name in ['NativeMediaLifecycle.java', 'NativeRendererBinding.java',
        'FlutterRTCVideoRenderer.java', 'MethodCallHandlerImpl.java', 'RtcOperationGate.java',
        'PeerConnectionObserver.java', 'SurfaceTextureRenderer.java', 'GetUserMediaImpl.java',
        'NativeLocalMediaRelease.java']]
    inputs += [Path('clients/flutter/local_plugin/compat/flutter_webrtc-1.6.0/android/build.gradle')]
    source_locks = [describe(p) for p in inputs]
    PRIMARY_DIR.mkdir(parents=True)
    primary = []
    for name in PRIMARY_FILES:
        url = f'https://webrtc.googlesource.com/src/+/{COMMIT}/{name}'
        with urllib.request.urlopen(url + '?format=TEXT', timeout=30) as response:
            content = base64.b64decode(response.read())
        local = PRIMARY_DIR / name.replace('/', '__')
        local.write_bytes(content)
        primary.append({**describe(local), 'upstreamPath': name, 'url': url, 'commit': COMMIT})
    license_path = PRIMARY_DIR / 'LICENSE'
    license_path.write_bytes((ROOT / 'tests/flutter/primary-webrtc-7559/LICENSE').read_bytes())
    primary.append({**describe(license_path), 'url': f'https://webrtc.googlesource.com/src/+/{COMMIT}/LICENSE', 'commit': COMMIT})
    report = {
        'schema': 1, 'generatedAtUTC': datetime.now(timezone.utc).isoformat(),
        'kind': 'read-only source/log timing review; no runtime intervention or product edit',
        'version': 'Android 3.8.8+108 private candidate', 'guestPID': 4997,
        'status': 'caller cleanup failed; physical operations eventually succeeded; internal native wait cause not yet proven',
        'sourceAndEvidence': source_locks,
        'timings': [
            {'job': 3, 'owner': 'peer#e341945', 'operation': 'renderer.detach', 'thread': 'OpenIMRtcMedia-1',
             'tid': 5325, 'queued': '07:17:31.863', 'workerStart': '07:17:31.865', 'ownerAcquired': '07:17:31.866',
             'actionEnd': '07:17:44.895', 'actionMs': 13029, 'physicalComplete': '07:17:44.906'},
            {'job': 4, 'owner': 'local', 'operation': 'renderer.detach', 'thread': 'OpenIMRtcMedia-2',
             'tid': 5375, 'queued': '07:17:31.945', 'workerStart': '07:17:31.947', 'ownerAcquired': '07:17:31.947',
             'actionEnd': '07:17:44.909', 'actionMs': 12961, 'physicalComplete': '07:17:44.910'},
            {'job': 8, 'operation': 'renderer.output.release', 'actionMs': 200, 'actionEnd': '07:17:45.107'},
            {'job': 10, 'operation': 'renderer.output.release', 'actionMs': 95, 'actionEnd': '07:17:45.007'},
            {'job': 9, 'operation': 'local.track.release', 'queued': '07:17:44.909',
             'workerStart': '07:17:45.007', 'ownerAcquired': '07:17:45.008', 'actionEnd': '07:17:50.962', 'actionMs': 5950,
             'cameraWaitLog': '07:17:50.436', 'cameraServiceDisconnected': '07:17:50.911'},
            {'job': 14, 'operation': 'publisher peer.close', 'jniMs': 509},
            {'job': 15, 'operation': 'publisher peer.dispose', 'jniMs': 13},
            {'job': 16, 'operation': 'subscriber peer.close', 'jniMs': 348},
            {'job': 17, 'operation': 'subscriber peer.dispose', 'jniMs': 45},
        ],
        'proven': [
            'Both detach jobs enter their action within 1-2ms of enqueue, with owner acquisition reported as 0ms. The observed 13 seconds is inside the action, not the project media queue or acquisition of its owner lock.',
            'Remote owner uses PeerConnectionObserver.nativeWork.exclusiveNative; local owner uses Runnable::run. They are different Java ownership domains, not one shared project nativeCall lock.',
            'TrackLease detach action calls the production VideoTrack.removeSink and then removes lease bookkeeping. It does not wait for output release. Output release is queued only after the detach action finishes.',
            'The two actionEnd timestamps are 14ms apart. This is correlation, not evidence identifying a shared lock or thread.',
            'The local track operation begins its real release only after physical detach and output work make an executor available. The later repeated Dart stop joins the existing physical result; it is not a repeated JNI release.',
            'The 12s callers fail before late physical completion. Later physicalOutcome success clears each exact deadline, with no permanentFailure in these records; that recovery does not turn the failed caller into a passed cleanup.',
        ],
        'excludedOrLimited': [
            'Post-detach EGL/output release takes 95/200ms and begins after both 13s actions. It is not itself the initial 13s action. An in-flight frame/EGL dependency inside removeSink is still possible.',
            'Only app GC in the detach interval is logged at 07:17:32.963: pause 9.956ms, total 228.137ms. This record cannot explain the full 13s. It does not exclude other unlogged native scheduling, memory, or host pressure.',
            'Logcat contains no sampled stack during the two blocked actions. It does not distinguish VideoTrack.nativeRemoveSink from nativeFreeSink or a paused worker, nor identify a C++ mutex owner.',
            'Local release takes 5.95s across multiple JNI/capturer/helper/source steps. waitForCameraClosed is not logged until 07:17:50.436, after about 5.43s of that action; the whole 5.95s cannot be assigned to that final camera-close wait.',
            'Receive monitoring already reports PENDING_RTC_STATS_BUSY then STOP_RTC_STATS_UNAVAILABLE during initial media. No current publisher/receiver stats stack was captured during hangup; stats contention as the detach cause is unproven.',
            'No after-the-fact host RAM sample establishes resource conditions during the stall. No simulator-only explanation is accepted as a finding.',
        ],
        'upstreamMechanism': {
            'dependency': 'io.github.webrtc-sdk:android:144.7559.09 (current build.gradle)',
            'boundary': 'Fixed M144 primary-source semantics, not disassembly or a runtime trace of the vendor binary.',
            'chain': [
                'VideoTrack.java:49-54 removes sink map entry, calls nativeRemoveSink, then nativeFreeSink.',
                'JNI_VideoTrack_RemoveSink calls VideoTrackInterface::RemoveSink.',
                'media_stream_track_proxy.h:55 uses PROXY_SECONDARY_METHOD1(RemoveSink); proxy.h:109-117 posts to secondary thread and synchronously waits Event::kForever.',
                'PeerConnectionFactory creates local VideoTrackProxy on its worker_thread; VideoRtpReceiver creates its remote track proxy using its supplied worker_thread. The application uses the same mFactory for local tracks and its peer connections. A common native worker is therefore a concrete candidate despite separate Java owners.',
                'VideoTrack::RemoveSink calls source RemoveSink; VideoBroadcaster::RemoveSink locks sinks_and_wants_lock, and OnFrame holds that same mutex while invoking sinks. An in-flight Java SurfaceTextureRenderer.onFrame includes producer surface sizing/getSurface and EglRenderer delivery.',
            ], 'evidence': primary,
        },
        'nextMinimumDiagnostic': {
            'trigger': 'Only in a separately labelled diagnostic call, if detach is still pending after 3s, collect one SIGQUIT of the exact own PID. Do not perform it in a normal latency acceptance run.',
            'threads': ['OpenIMRtcMedia-1', 'OpenIMRtcMedia-2', 'WebRTC worker_thread', 'WebRTC signaling_thread', 'EglThread', 'main_texture_camera_thread', 'main'],
            'questions': [
                'Are both media workers in nativeRemoveSink proxy Event::Wait, nativeFreeSink, or elsewhere?',
                'What is the target native worker executing, and is it waiting on a VideoBroadcaster/frame/surface/GL lock or being scheduled out?',
                'Is a frame callback holding the source mutex while waiting on SurfaceTextureRenderer.surfaceLock, producer.getSurface, EglRenderer, or a camera/GL owner?',
            ],
            'fallback': 'If Java/SIGQUIT native frames are unsymbolized, retain the raw own-PID trace and exact native library BuildId, then obtain an explicitly coordinated native backtrace/profile with matching symbols. No timeout increase, unknown-JNI retry, or product change follows from timing correlation alone.',
            'intrusiveness': 'SIGQUIT may perturb the process; any diagnostic timing is not normal-call performance evidence.',
        },
        'runtimeEvidence': evidence, 'receiveMonitorEvidence': monitor,
        'unchanged': ['product/SDK source', '12s caller budget', 'worker/queue bounds', 'AVD', '17787 Web101', '17788 Web102/103', 'server'],
    }
    for item in source_locks:
        if describe(Path(item['path'])) != item:
            raise SystemExit('Input changed during read-only review: ' + item['path'])
    OUT.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps(describe(OUT), ensure_ascii=False))


if __name__ == '__main__':
    main()
