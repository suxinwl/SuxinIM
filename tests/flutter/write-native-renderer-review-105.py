"""Record source/QA boundaries for the Android105 renderer candidate.

This records evidence; it neither builds nor changes a client/server/session.
Run after production Java compile and the formal Java suite pass.
"""
from pathlib import Path
import hashlib
import json
from datetime import datetime, timezone

ROOT = Path(__file__).resolve().parents[2]
NATIVE = Path("clients/flutter/local_plugin/compat/flutter_webrtc-1.6.0/android/src/main/java/com/cloudwebrtc/webrtc")

def digest(path):
    path = ROOT / path
    return {"sha256": hashlib.sha256(path.read_bytes()).hexdigest(), "size": path.stat().st_size}

sources = [NATIVE / name for name in [
    "NativeMediaLifecycle.java", "NativeRendererBinding.java", "NativeCleanupSequence.java",
    "NativeLocalMediaRelease.java", "FlutterRTCVideoRenderer.java", "SurfaceTextureRenderer.java",
    "RtcOperationGate.java", "PeerConnectionObserver.java", "MethodCallHandlerImpl.java",
    "StateProvider.java", "GetUserMediaImpl.java", "RemoteTrackRegistry.java",
]] + [Path("clients/flutter/scripts/test-native-java.ps1")]
tests = [Path("tests/flutter") / name for name in [
    "NativeRendererBindingIndependentTest.java", "NativeRendererWiringTest.java",
    "NativeLocalMediaReleaseTest.java", "NativeLocalMediaReleaseIndependentTest.java",
    "RtcOperationGateTest.java", "TrackStatsSelectorTest.java",
]]
tests += sorted((ROOT / "tests/flutter/native-renderer-stubs").rglob("*.java"))
tests = [p.relative_to(ROOT) if p.is_absolute() else p for p in tests]
suite = Path("tests/flutter/native-lifecycle-java-105-final.log")
compile_log = Path("tests/flutter/native-production-java-105-ownership-final.log")
suite_text = (ROOT / suite).read_text(encoding="utf-8", errors="replace")
compile_text = (ROOT / compile_log).read_text(encoding="utf-8", errors="replace")
assert "PASS 7 actual production FlutterRTCVideoRenderer wiring cases" in suite_text
assert "PASS 4 independent real local-release" in suite_text
assert "PASS 11 production local ownership" in suite_text
assert "BUILD SUCCESSFUL" in compile_text

report = {
    "generatedAt": datetime.now(timezone.utc).isoformat(),
    "scope": "Android105 candidate native renderer/track/stream lifecycle adaptation; production source review and fake-boundary Java tests",
    "status": "java_compile_and_boundary_tests_passed_awaiting_parent_review_and_android_runtime",
    "candidateBuild": 105,
    "apkBuiltOrAcceptedByThisReport": False,
    "historicalFailure": {
        "build": 104,
        "trace": "tests/flutter/android-104-call2-native-anr.txt",
        **digest(Path("tests/flutter/android-104-call2-native-anr.txt")),
        "mainThread": "VideoTrack.nativeRemoveSink -> VideoTrack.removeSink -> FlutterRTCVideoRenderer.removeRendererFromVideoTrack -> setVideoTrack -> setStream -> MethodCallHandlerImpl -> platform Looper",
        "proven": "Hangup followed immediately by composer input blocked Android main in JNI sink removal; this trace is not stats/Camera.stop.",
        "notProven": "Stripped native frames and a concurrent camera/GLES snapshot do not establish the underlying native deadlock/GPU cause.",
        "runtimeAcceptance": "failed; preserved historical evidence, never promoted by these Java tests",
    },
    "ownership": {
        "nativeOwners": "One serial owner per remote peer using its existing nativeCall exclusion; one shared reusable local owner for cloned-capturer tracks; one serial output owner per renderer.",
        "physicalOrder": "Renderer old lease detach really completes before new owner attach. PC close/dispose waits physical detach continuation before acquiring native exclusion. Local last-owner cleanup waits all matching sink detaches before stop/source/track disposal.",
        "noWaitUnderOwnerLock": "CompletableFuture continuations run after native exclusion exits; no worker waits synchronously for another queued owner job.",
        "main": "Main updates generation/metadata and handles MethodChannel/EventChannel/final producer release; potentially blocking sink/EGL/peer/local cleanup runs on the bounded media pool.",
        "bounds": {"globalMediaWorkers": 2, "globalWorkerQueue": 32, "perOwnerOutstanding": 8, "perRendererPending": 8, "localCleanupRequests": 8, "reservedSingleShutdownRequest": 1, "callerDeadlineMillis": 12000},
        "deadlines": "Timeout reports once and quarantines new attachment. It does not cancel, complete, retry, or replace a physical JNI operation. Late real success still performs identity-checked bookkeeping/final output release.",
        "failure": "Unknown native failure keeps strong tombstones/leases/references, prevents retry via another cleanup API, and does not claim native completion. Successful history uses weak identity keys and successful request mappings are removed.",
        "localStreams": "Shared owning streams retain the common track/capturer until the last owner exits. Preserved tracks are borrowed: removing a borrowed stream does not dispose the track. Explicit track disposal first removes all stream memberships. Empty MediaStream wrapper and each owned track wrapper are disposed once.",
        "sources": "AudioSource Java factory ref is returned after createAudioTrack retains its native source. Shared camera source survives primary promotion and is disposed once at last capturer owner. Screen capture now registers its source and texture helper for the same last-owner release.",
        "captureRace": "New local capture/metadata edits/lookups reject while cleanup is pending/closing/quarantined. Async permission callbacks also compare the captured generation. A never-published stream verified to have no owned/borrowed tracks can finish via the same bounded native owner cleanup=true after the ordinary sequence closes; it touches no registered/shared objects, and released/failed identity guards still apply. Nonempty partial creation does not bypass the ordinary ownership sequence. Strong rejected refs are removed only after real success.",
        "remoteWrappers": "Renderer resolves remote stream/event-owned stable wrappers; fallback getReceivers/getTransceivers cache enumeration was removed from renderer lookup. Stock M144 JNI observer retains callback receiver GlobalOwners and its PC-dispose destruction calls receiver.dispose, which releases the cached track. This patch physically detaches sinks first and does not duplicate that JNI ownership release. Existing RemoteTrackRegistry is unchanged from build104 and only provides identity/thread-safe metadata. Primary-source semantics are distinct from vendor binary reverse engineering.",
    },
    "sourceFiles": {str(p).replace("\\", "/"): digest(p) for p in sources},
    "testSources": {str(p).replace("\\", "/"): digest(p) for p in tests},
    "verification": {
        "formalSuite": {"path": str(suite).replace("\\", "/"), **digest(suite), "exitCode": 0},
        "productionJava": {"path": str(compile_log).replace("\\", "/"), **digest(compile_log), "exitCode": 0, "command": "gradlew.bat :flutter_webrtc:compileReleaseJavaWithJavac --offline --no-daemon --max-workers=1", "jdk": "17.0.20.1+1", "classpath": "Actual project Android/Flutter/WebRTC dependencies, no test stubs"},
        "fakeBoundaryCoverage": [
            "Actual Binding/Owner: native detach blocking, physical barriers, deadlines/late completion, unknown failure, full queue/executor, stale generation, reentry, concurrent dispose and rejected main dispatch",
            "Actual Renderer: track/stream selection, real production setTrack -> setStream(null) -> Dispose sequencing, missing owner, known/fresh EGL failure, 12-second timeout then late success, shared-stream sink isolation",
            "Actual local ownership/sequence: shared streams, explicit track then stream, borrowed tracks, sibling cleanup after failure, no cross-API unknown retry, cloned-source boundary, bounded requests, concurrent last owner, weak identity history/drain",
            "Actual sequence final shutdown: eight accepted ordinary requests plus one shared final slot, real physical completion before final snapshot, no new ordinary work, failed tail never advances JNI; late unpublished empty stream after shutdown and snapshot in either order disposes exactly once, nonempty and previously-failed native objects never gain a retry bypass",
        ],
        "primaryAudioSourceEvidence": "tests/flutter/audio-source-ownership-review-105.json",
        "independentReviews": {name: digest(Path("tests/flutter") / name) for name in ["remote-event-ownership-review-105.json", "audio-source-ownership-review-105.json", "native-local-independent-review-105.json", "native-renderer-wiring-review-105.json", "native-renderer-binding-independent-review-105.json"]},
        "knownBuildWarnings": "Existing SDK36 plugin/SDK35 app and NDK27 plugin/NDK26 app metadata warnings remain in compile log. This report does not change project runtime/build versions or claim those warnings disappeared.",
    },
    "requiredRuntime": [
        "Final candidate APK original Android UI two long calls with continuously increasing decoded video frames",
        "Hang up then immediately enter/send text; no main input ANR or unhandled renderer/typing/startup Future errors",
        "Real camera/media release and bounded native thread counts; second call reconnect/reuse",
        "Android real JNI/hardware remains distinct from these fake JNI/EGL/producer tests",
    ],
    "failureRecoveryBoundary": "Caller deadlines quarantine new work; late real completion still releases resources/bookkeeping, but this patch does not automatically rehabilitate a quarantined owner. Unknown native failure/stall retains references and rejects new capture/attachment until process restart. No replacement/unbounded native worker is created and no cancellation is falsely reported as completion.",
    "notTouched": ["AVD", "Web101 helper", "production services", "Dart product source", "PUB/version manifest", "Windows/Web application packages"],
}
target = ROOT / "tests/flutter/native-renderer-review-105.json"
target.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
print(json.dumps({"path": str(target), **digest(target.relative_to(ROOT))}))
