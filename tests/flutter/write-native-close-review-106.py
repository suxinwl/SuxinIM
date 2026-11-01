"""Record the limited Android106 native changes, without building an APK.

105 evidence stays historical. Fake JNI tests and the real Android classpath
compile are recorded separately; neither claims runtime acceptance.
"""
from pathlib import Path
from datetime import datetime, timezone
import hashlib
import json

ROOT = Path(__file__).resolve().parents[2]
NATIVE = Path("clients/flutter/local_plugin/compat/flutter_webrtc-1.6.0/android/src/main/java/com/cloudwebrtc/webrtc")
TESTS = Path("tests/flutter")

def digest(path):
    data = (ROOT / path).read_bytes()
    return {"sha256": hashlib.sha256(data).hexdigest(), "size": len(data)}

sources = [NATIVE / name for name in [
    "RtcOperationGate.java", "NativeMediaLifecycle.java", "NativePeerLifecycle.java",
    "PeerConnectionObserver.java", "MethodCallHandlerImpl.java",
    "NativeRendererBinding.java", "NativeCleanupSequence.java", "NativeLocalMediaRelease.java",
    "FlutterRTCVideoRenderer.java", "SurfaceTextureRenderer.java", "StateProvider.java",
    "GetUserMediaImpl.java", "RemoteTrackRegistry.java", "TrackStatsSelector.java",
]] + [Path("clients/flutter/scripts/test-native-java.ps1")]
tests = [TESTS / name for name in [
    "RtcOperationGateTest.java", "RtcClosingReplyTest.java", "NativeOwnerDiagnosticsTest.java",
    "NativePeerLifecycleIndependentTest.java", "TrackStatsSelectorTest.java",
    "NativeRendererBindingIndependentTest.java", "NativeLocalMediaReleaseTest.java",
    "NativeLocalMediaReleaseIndependentTest.java", "NativeRendererWiringTest.java",
]]
tests += [p.relative_to(ROOT) for p in sorted((ROOT / TESTS / "native-renderer-stubs").rglob("*.java"))]
suite = TESTS / "native-lifecycle-java-106.log"
compile_log = TESTS / "native-production-java-106.log"
suite_text = (ROOT / suite).read_text(encoding="utf-8", errors="replace")
for required in [
    "PASS 5 production beginClosing reply/physical-lifetime cases",
    "PASS 3 production native-owner timing metadata cases",
    "PASS pending close/dispose physical coalescing", "PASS failed disposal latch",
    "PASS 10 independent real Binding/Owner cases", "PASS 11 production local ownership",
    "PASS 4 independent real local-release", "PASS 7 actual production FlutterRTCVideoRenderer wiring cases",
]:
    assert required in suite_text, required
assert "BUILD SUCCESSFUL" in (ROOT / compile_log).read_text(encoding="utf-8", errors="replace")

archive = TESTS / "diagnostics/105/native-before-106"
archive_manifest = json.loads((ROOT / archive / "manifest.json").read_text(encoding="utf-8-sig"))
for name, expected in archive_manifest["files"].items():
    assert digest(archive / name) == expected, name

historical = [
    "native-renderer-review-105.json", "native-final-boundary-review-105.json",
    "native-close-diagnosis-105.json", "native-close-primary-path-review-105.json",
    "begin-closing-pending-stats-105-repro.log",
]
report = {
    "generatedAt": datetime.now(timezone.utc).isoformat(),
    "candidateBuild": 106,
    "scope": "Limited native reply completion, physical close/dispose coalescing, and safe timing diagnostics",
    "status": "production_java_compile_and_fake_boundary_tests_passed_pending_unified_parent_freeze_and_runtime",
    "apkBuiltOrAcceptedByThisReport": False,
    "frozen105Archive": {"path": str(archive), "manifest": digest(archive / "manifest.json"), "filesVerified": len(archive_manifest["files"])},
    "historicalEvidence": {str(TESTS / name).replace("\\", "/"): digest(TESTS / name) for name in historical},
    "diagnosis": {
        "firstCall": "105 original UI continuous video passed, but no-dump hangup cleanup hit the unchanged 12-second caller deadline and produced an unhandled close error; runtime acceptance failed.",
        "secondCall": "Two deliberate own-PID SIGQUIT snapshots show media worker holding nativeCall inside JNI nativeClose while stats workers are parked. These invasive snapshots establish location, not unperturbed cleanup performance or the exact stripped C++ substage.",
        "noFalseCause": "Native coalescing and pending-reply completion are deterministic corrections; they do not by themselves establish a fix for the observed JNI-close delay. Dart Room/Engine ordering is a separately owned change.",
    },
    "changes": {
        "pendingStats": "beginClosing immediately completes an existing reply as RTC_STATS_CLOSED. The real native callback/gate lifetime continues; late success/error and the old deadline cannot answer twice or falsely release native work.",
        "physicalSingleFlight": "NativePeerLifecycle publishes one close future and one dispose future before starting work. Real sink detach precedes close; real close success precedes dispose. Pending, successful, and failed futures are retained for all concurrent/reentrant callers.",
        "failure": "Real detach, dispatch, native close or native dispose failure is latched. Dispose cannot retry an unknown close or bypass its failed barrier. There is no private nativeFree reflection, worker replacement, timeout increase or false native completion.",
        "callerDeadline": "Every MethodChannel caller independently observes the same physical future through the existing exactly-once 12000ms report. A caller timeout only quarantines new work; late physical success can still permit dispose and independent identity-checked onDisposed bookkeeping.",
        "diagnostics": "Peer identity-hash, job counter, fixed operation label, phase, elapsed milliseconds, Java thread name and failure class only. queue/workerStart/ownerAcquired/actionEnd and actual JNI begin/end separate queue, lock and native intervals. Diagnostic sink exceptions are ignored without changing lifetime. No tokens, room/user IDs, URLs or native exception messages are logged.",
    },
    "productionWiring": {
        "peerClose": "MethodCallHandler peerConnectionClose obtains the stored observer directly; Pco.beginClosing -> NativePeerLifecycle.close -> independent reportMedia. It never calls PeerConnection.close on the platform Looper.",
        "peerDispose": "MethodCallHandler peerConnectionDispose -> Pco.beginClosing -> NativePeerLifecycle.dispose. Native close success is required before actual PeerConnection.dispose; observer removal remains identity-checked and only follows physical success, including success after caller timeout.",
        "sharedSources": "No additional native source stop or shared-track ownership change; the existing bounded nativeCall exclusion and sink-detach order remain in force.",
    },
    "sourceFiles": {str(p).replace("\\", "/"): digest(p) for p in sources},
    "testSources": {str(p).replace("\\", "/"): digest(p) for p in tests},
    "verification": {
        "formalJavaSuite": {"path": str(suite).replace("\\", "/"), **digest(suite), "exitCode": 0},
        "productionJava": {"path": str(compile_log).replace("\\", "/"), **digest(compile_log), "exitCode": 0, "jdk": "17.0.20.1+1", "command": "gradlew.bat :flutter_webrtc:compileReleaseJavaWithJavac --offline --no-daemon --max-workers=1", "classpath": "Actual project Android/Flutter/WebRTC dependencies; no test stubs"},
        "fakeBoundaryTests": {"newClosingReplyCases": 5, "newDiagnosticsCases": 3, "newIndependentPeerCases": 8, "existingBindingCases": 10, "existingLocalCases": 11, "existingIndependentLocalCases": 4, "existingActualRendererWiringCases": 7, "existingGateAndSelectorSuites": "Passed, unchanged test source"},
        "testBoundary": "Peer tests execute the actual production NativePeerLifecycle and Owner with fake JNI Runnables; real Pco wiring is separately source reviewed and compiled against the actual Android classpath. Renderer wiring executes the actual production renderer with explicit boundary stubs. These are not Android/JNI or hardware runtime results.",
        "independentPeerReview": {"path": "tests/flutter/native-peer-independent-review-106.json", **digest(TESTS / "native-peer-independent-review-106.json")},
        "knownCompileWarnings": "Existing app SDK35/plugin SDK36 and app NDK26/plugin NDK27 metadata warnings remain in the compile log. Native scope did not change build/runtime versions.",
    },
    "requiredRuntime": [
        "Unified106 original Android UI two long calls with fresh decoded frames and no deliberate thread dumps in the performance window",
        "Hangup immediately followed by composer input and a delivered message, no input ANR or unhandled cleanup/socket/typing/startup Future",
        "Camera/media genuinely released, bounded worker counts and successful second call reuse",
        "Preserve failed105 history; do not promote106 until original-UI and actual JNI regression passes",
    ],
    "notTouchedByThisNativeWork": ["Dart product source", "AVD", "Web101 helper", "server processes", "application package/version manifests", "12-second deadline", "vendor C++ WebRTC binary", "shared capture source shutdown policy"],
}
target = TESTS / "native-close-review-106.json"
(ROOT / target).write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
print(json.dumps({"path": str(target), **digest(target)}))
