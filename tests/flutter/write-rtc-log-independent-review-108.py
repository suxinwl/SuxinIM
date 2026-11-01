"""Independent read-only source/fixture audit; does not rerun root's tests."""
import hashlib
import json
import re
from datetime import datetime, timezone
from pathlib import Path

ROOT=Path(__file__).resolve().parents[2]
NATIVE="clients/flutter/local_plugin/compat/flutter_webrtc-1.6.0/android/src/main/java/com/cloudwebrtc/webrtc/"
def file(name):
    data=(ROOT/name).read_bytes()
    return {"path":name,"bytes":len(data),"sha256":hashlib.sha256(data).hexdigest()}

root_review=json.loads((ROOT/"tests/flutter/rtc-log-tool-final-review-108.json").read_text())
for row in root_review["inputs"]:
    assert file(row["path"])["sha256"]==row["sha256"],row["path"]
assert root_review["casesActuallyPassed"]==9 and not root_review["runtimeAcceptanceGranted"]

fixtures=[]
for name in ("android-107-call1-complete-logcat.txt","android-107-call2-complete-logcat.txt","android-107-camera-diagnostic-active-logcat.txt"):
    path="tests/flutter/"+name
    own=[]
    for number,line in enumerate((ROOT/path).read_bytes().decode("utf-8",errors="replace").splitlines(),1):
        fields=line.split()
        if len(fields)>3 and fields[2]=="4810":own.append((number,line))
    # Independent literal checks, not importing/reexecuting the reviewed classifier.
    renderer=[n for n,line in own if "RTCVideoRenderer source change failed:" in line]
    cleanup=[n for n,line in own if "[OpenIMRtcCleanup]" in line and re.search(r"\sfailed\s+\d+ms\b",line)]
    unhandled=[n for n,line in own if "Unhandled Exception:" in line]
    guard=[n for n,line in own if "Unable to getUserMedia: Local media cleanup is still pending" in line]
    fixtures.append({"log":file(path),"pid":4810,"independentCounts":{"caughtRendererFailure":len(renderer),"caughtCleanupFailure":len(cleanup),"unhandledException":len(unhandled),"captureCleanupGuard":len(guard)},"matchedLines":{"renderer":renderer,"cleanup":cleanup,"unhandled":unhandled,"guard":guard}})
assert fixtures[0]["independentCounts"]=={"caughtRendererFailure":9,"caughtCleanupFailure":16,"unhandledException":0,"captureCleanupGuard":0}
assert fixtures[1]["independentCounts"]=={"caughtRendererFailure":0,"caughtCleanupFailure":0,"unhandledException":0,"captureCleanupGuard":0}
assert fixtures[2]["independentCounts"]=={"caughtRendererFailure":0,"caughtCleanupFailure":0,"unhandledException":1,"captureCleanupGuard":1}
assert fixtures[2]["matchedLines"]["unhandled"]==fixtures[2]["matchedLines"]["guard"]==[457]

sources=[
    "tests/flutter/review_rtc_log.py","tests/flutter/test_review_rtc_log.py",
    "tests/flutter/rtc-log-tool-final-review-108.json","tests/flutter/rtc-log-review-tool-108-final.log",
    NATIVE+"NativeMediaLifecycle.java",NATIVE+"NativeRendererBinding.java",NATIVE+"FlutterRTCVideoRenderer.java",
    NATIVE+"MethodCallHandlerImpl.java",NATIVE+"SurfaceTextureRenderer.java",
    "clients/flutter/openim_live/lib/src/utils/call_capture.dart",
    "clients/flutter/local_plugin/compat/livekit_client-2.11.0/lib/src/widgets/video_track_renderer.dart",
    "clients/flutter/local_plugin/compat/livekit_client-2.11.0/lib/src/support/cleanup_sequence.dart",
    "clients/flutter/local_plugin/compat/flutter_webrtc-1.6.0/lib/src/native/rtc_video_renderer_impl.dart",
]
report={
    "schema":1,"createdAt":datetime.now(timezone.utc).isoformat(),"status":"NO_CURRENT_FIXED_TAG_OR_PHASE_COVERAGE_BLOCKER_FOUND",
    "scope":"Independent read-only review of root's classifier/test source and current108 native/Dart log producers. No product/tool edits, tests/compilation rerun, device/helper/server action.",
    "rootTests":{"actuallyRunBy":"root","cases":9,"log":file("tests/flutter/rtc-log-review-tool-108-final.log"),"rerunByThisReview":False},
    "sourceChecks":[
        "Caught cleanup failed-ms and legacy RTCVideoRenderer source-change failures are independently classified even without Unhandled Exception.",
        "Current OpenIMCallCapture and OpenIMRendererAttachment fixed fields stage/current/error match both current=true and current=false; failures in retired work remain reviewable rather than discarded.",
        "Native actionEnd/physicalComplete/physicalOutcome failure= fields, callerTimeout/callerError, rejected/attachRejected, quarantine/deadlineOverflow and local.admission blocked are matched.",
        "Native successful actionEnd/physicalOutcome does not become a failure from names such as permanentFailure=false. Normal attachRetired owner_closing is not a failure marker.",
        "retiredSocketFailure complete is a valid cleanup stage name and does not satisfy the failed-ms pattern or SocketException: literal.",
        "The tool copies only line/time/PID/categories/safe fields and codes, not arbitrary exception text, media URLs, SDP or JWTs. Original bytes are SHA256-bound and decoded with replacement for invalid UTF8.",
        "Own PID filtering is intentional; other-PID classified events are counted separately. Event totals include propagated failures and are not distinct causes.",
    ],
    "independentFixtureCounts":fixtures,
    "fixtureInterpretation":[
        "107call1 has25 caught events:9 renderer+16 cleanup. Zero Unhandled/ANR is insufficient to call it clean.",
        "107call2 classified-event count0 is not a pass; actual outbound video/capture failure was established separately and must remain failed/pending.",
        "107camera diagnostic has one physical log line457 with both unhandledException and captureCleanupGuard categories; it is one event, not two root causes.",
    ],
    "boundaries":[
        "Not complete exception detection, ANR trace analysis, media freshness/audio/screen evidence, or automatic acceptance.",
        "Caught cleanup/capture/new renderer safe tags require OPENIM_DEVELOPMENT=true; absence when diagnostics are disabled cannot prove absence of failures.",
        "Patterns are bound to current fixed producers and threadtime format. Future tag/format changes require review; current matching is not a universal parser guarantee.",
        "No current concrete missed fixed failure tag or normal attachRetired/retiredSocketFailure false-positive was found in the reviewed scope.",
    ],
    "inputs":[file(name) for name in sources],
}
target=ROOT/"tests/flutter/rtc-log-independent-review-108.json"
assert not target.exists()
target.write_text(json.dumps(report,ensure_ascii=False,indent=2)+"\n",encoding="utf-8")
print(json.dumps(file(target.relative_to(ROOT).as_posix())))
