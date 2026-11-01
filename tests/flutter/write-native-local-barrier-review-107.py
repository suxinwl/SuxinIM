"""Read-only evidence index for the 107 second-call local-capture failure."""
import hashlib
import json
from datetime import datetime, timezone
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
NATIVE = "clients/flutter/local_plugin/compat/flutter_webrtc-1.6.0/android/src/main/java/com/cloudwebrtc/webrtc/"
FILES = [
    NATIVE + "MethodCallHandlerImpl.java",
    NATIVE + "GetUserMediaImpl.java",
    NATIVE + "NativeMediaLifecycle.java",
    NATIVE + "NativeCleanupSequence.java",
    NATIVE + "NativeRendererBinding.java",
    "tests/flutter/android-107-call1-complete-logcat.txt",
    "tests/flutter/android-107-call2-complete-logcat.txt",
    "tests/flutter/android-107-camera-diagnostic-active-logcat.txt",
]

def fingerprint(name):
    data = (ROOT / name).read_bytes()
    return {"path": name, "bytes": len(data), "sha256": hashlib.sha256(data).hexdigest()}

report = {
    "schema": 1,
    "createdAt": datetime.now(timezone.utc).isoformat(),
    "scope": "Read-only source and public redacted logs; no product/runtime/server changes",
    "application": "Android private 3.8.7+107, API30 x64 emulator, PID4810",
    "conclusion": "The second-call capture attempt is actually rejected by the native local-cleanup guard. The first call contains caught renderer failures and a 12-second native-track cleanup failure; it must not be labelled entirely passed solely because ANR/Unhandled counts were zero.",
    "confirmedRuntime": [
        {"log": FILES[5], "line": 228, "time": "05:46:59.899", "event": "First call reaches getUserMedia(video) creation logging"},
        {"log": FILES[5], "line": 464, "time": "05:47:02.912", "event": "First call reaches getUserMedia(audio) creation logging"},
        {"log": FILES[5], "line": 1677, "time": "05:48:09.601", "event": "First caught RTCVideoRenderer source change PlatformException; several later repeats lack error code"},
        {"log": FILES[5], "line": 4246, "time": "05:50:06.487", "event": "Track.stop/nativeTrack failed after 12313ms with PlatformException"},
        {"log": FILES[5], "line": 4286, "time": "05:50:06.891", "event": "A subsequent LocalTrack.stop/nativeTrack request completes after 396ms; nativeStream then completes in44ms"},
        {"log": FILES[5], "line": 4382, "time": "05:50:07.627", "event": "LocalParticipant/tracks failed after 13455ms; error is retained by higher cleanup layers"},
        {"log": FILES[6], "line": 250, "time": "05:53:19.289", "event": "Camera service logs Closed Camera1. This line alone does not prove a newly-created second-call capture existed; no second-call getUserMedia(audio/video) creation log occurs"},
        {"log": FILES[6], "line": 2027, "time": "05:57:16.754", "event": "Second-call LocalParticipant/tracks cleanup completes in0ms"},
        {"log": FILES[7], "line": 457, "time": "06:03:08.709", "event": "Original camera-On diagnostic receives Unable to getUserMedia: Local media cleanup is still pending; stack reaches original ControlsView._enableVideo"},
    ],
    "sourceFacts": [
        {"path": FILES[0], "line": 1737, "fact": "getUserMedia returns RTC_MEDIA_BUSY before allocating a stream or invoking GetUserMediaImpl when isLocalMediaCleanupPending is true. This early result.error has no log statement."},
        {"path": FILES[0], "line": 1866, "fact": "Guard includes sequence pending/failed tail, owner closing/quarantined, and closing track/stream sets."},
        {"path": FILES[0], "line": 1862, "fact": "reportLocal passes the shared plugin-local owner quarantine callback to the caller deadline reporter."},
        {"path": FILES[2], "line": 144, "fact": "At12s caller deadline, reporter invokes onTimeout and returns RTC_MEDIA_TIMEOUT while leaving physical work pending."},
        {"path": FILES[2], "line": 55, "fact": "Owner.quarantine permanently sets a boolean. No success/unquarantine method exists."},
        {"path": FILES[0], "line": 1914, "fact": "Genuine track cleanup success removes local track, its matching future and closing marker; it does not clear owner quarantine."},
        {"path": FILES[0], "line": 1954, "fact": "Genuine stream cleanup success removes stream, matching future and closing markers; it does not clear owner quarantine."},
        {"path": FILES[0], "line": 210, "fact": "Permanent closeForNewWork/detachAll is reserved to whole plugin disposal. Ordinary successful track/stream cleanup does not globally close the owner."},
        {"path": FILES[3], "line": 14, "fact": "Cleanup sequence isPending is also true for an exceptionally completed physical tail. Successful physical tail decrements pending normally."},
        {"path": FILES[1], "line": 736, "fact": "getUserMedia(video) log precedes fresh/shared camera selection, so absence covers both paths after the early guards, but cannot on its own prove no MethodChannel invocation."},
    ],
    "causalAssessment": {
        "directlyProven": "Native capture guard rejects the current camera attempt; second-call outbound publication was not healthy.",
        "supportedSourceChain": "A caller-timeout on first-call local track disposal sets shared owner quarantine, which genuine late physical success cannot clear. That state alone is sufficient to reject all later audio/video capture on the same plugin instance.",
        "notInstrumented": "The exact guard subcondition and earliest owner-quarantine transition were not logged in107. Earlier renderer failures may have additional causes. Do not claim the first quarantine timestamp is proven or infer device/HAL root cause from the close line.",
        "physicalCompletionBoundary": "Later successful duplicate-track and stream cleanup calls support late physical completion. No runtime owner-state query or full resource reference proof was performed.",
    },
    "nextImplementationConstraints": [
        "Keep caller deadline separate from actual physical failure and physical in-flight state; do not equate late known success with unknown JNI failure.",
        "Do not blindly clear all owner quarantine, re-run an unknown failed JNI operation, increase the12s budget, or stop a source still owned by another track.",
        "Before a new capture, true queue/native failures and physical pending work must remain blocked. A bounded per-operation deadline barrier could be released only after that exact operation genuinely succeeds and every relevant owner constraint is healthy.",
        "Narrow diagnostics should retain PlatformException.code plus each guard reason and physical completion timing, without token/SDP/media URLs.",
        "Add controlled blocked-cleanup then late-success capture-recovery tests and real-failure/no-retry tests before changing the frozen source; actual two-call original UI/media/camera release remains device-runtime acceptance.",
    ],
    "boundaries": [
        "No source patch, AVD action, helper restart, new call, server action, or private-log read was performed by this review.",
        "This report is diagnosis of failed107 runtime, not upgrade completion or Android delivery approval.",
    ],
    "inputs": [fingerprint(name) for name in FILES],
}
path = ROOT / "tests/flutter/native-local-capture-barrier-review-107.json"
path.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
print(json.dumps({"path": str(path), "sha256": hashlib.sha256(path.read_bytes()).hexdigest()}, ensure_ascii=False))
