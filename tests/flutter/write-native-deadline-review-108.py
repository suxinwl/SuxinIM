"""Bind native108 changes to preserved107 bytes and current validation evidence."""
import difflib
import hashlib
import json
from datetime import datetime, timezone
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
BASE = "clients/flutter/local_plugin/compat/flutter_webrtc-1.6.0/android/src/main/java/com/cloudwebrtc/webrtc/"
ARCHIVE = "tests/flutter/source-history/107-79430315/native-before-108/"
PRODUCT = [BASE + name for name in ("NativeMediaLifecycle.java", "NativeRendererBinding.java", "FlutterRTCVideoRenderer.java", "MethodCallHandlerImpl.java", "NativeCleanupSequence.java", "SurfaceTextureRenderer.java")]

def file(name):
    data = (ROOT / name).read_bytes()
    return {"path": name, "bytes": len(data), "sha256": hashlib.sha256(data).hexdigest()}

source_check = json.loads((ROOT / "tests/flutter/native-source-check-108.json").read_text())
assert source_check["identical"] and source_check["before"] == source_check["after"]
assert file(ARCHIVE + "manifest.json")["sha256"] == "f3bbe8562d28929c8f3e663a783596e6283b7c3ae9a564be12c703c5939c6d39"
java_log = (ROOT / "tests/flutter/native-java-formal-108.log").read_text(encoding="utf-8-sig", errors="replace")
javac_log = (ROOT / "tests/flutter/native-android-javac-108.log").read_text(encoding="utf-8-sig", errors="replace")
for marker in ("PASS 11 independent production", "PASS 5 production binding/actual-renderer", "PASS 3 actual production SurfaceTextureRenderer"):
    assert marker in java_log, marker
assert "BUILD SUCCESSFUL" in javac_log and ":flutter_webrtc:compileReleaseJavaWithJavac" in javac_log
diff = []
for name in PRODUCT:
    before = (ROOT / ARCHIVE / name).read_text(encoding="utf-8-sig").splitlines(True)
    after = (ROOT / name).read_text(encoding="utf-8-sig").splitlines(True)
    diff.extend(difflib.unified_diff(before, after, fromfile="107/" + name, tofile="108/" + name))
diff_path = "tests/flutter/native-deadline-108.diff"
(ROOT / diff_path).write_text("".join(diff), encoding="utf-8")
report = {
    "schema": 1,
    "createdAt": datetime.now(timezone.utc).isoformat(),
    "status": "NATIVE_SOURCE_AND_BOUNDARY_VALIDATION_PASSED_ANDROID_RUNTIME_PENDING",
    "application": "3.8.8+108 candidate native subset; final application freeze owned by Flutter/root",
    "problem": "107 original UI first-call renderer/track cleanup errors were caught; a12s caller timeout permanently quarantined shared local/output ownership even after exact physical cleanup genuinely succeeded. Subsequent native getUserMedia was actually rejected by the local cleanup guard.",
    "historicalDiagnosis": file("tests/flutter/native-local-capture-barrier-review-107.json"),
    "changes": [
        "Identity-keyed PhysicalDeadlines retains at most32 physical Futures per owner/binding. Caller timeout does not cancel, replace, retry or mark the physical operation successful.",
        "Soft barrier is removed only by that exact physical Future completion. Failure publishes permanent quarantine before removing soft entry; other pending deadlines, hard failure and closing remain blocking.",
        "Admission reads pending deadline state before volatile hard/failed state, matching failure-publication then pending-removal order, so it cannot combine an old false hard flag with a newer empty pending map.",
        "Local report binds the complete done Future including main-thread map/closing-marker bookkeeping. Existing sequence/closing/hard-failure guards remain effective.",
        "Renderer deadline does not mutate current/generation. It rejects newly-admitted non-null sets while null cleanup/disposal may continue. Previously admitted current work ignores newly-added soft deadline and really attaches after safe serial predecessors; explicit retired generations retain old semantics.",
        "SetRequest.accepted prevents a rejected renderer request overwriting accepted stream/owner/track metadata. Physical callbacks do not rewrite newer metadata.",
        "Already-admitted current renderer work encountering a permanent owner failure returns a real RTC_MEDIA_UNAVAILABLE without invoking attach. Normal owner shutdown retires unstarted attach with a fixed reason and permits lease release/physical peer close; it does not erase a hard failure.",
        "Surface cleanup caller timeout uses the same exact-Future soft barrier. Actual physical error remains hard with original error and handler; pendingSurfaceCleanup still prevents premature EGL recreation. Failure-handler registration consults permanent failures only.",
        "Added fixed metadata-only owner/job/operation, caller deadline, physical outcome, admission reason and error code logs. No media URL, token, SDP, media content or native error message is emitted by new diagnostics.",
    ],
    "preservedConstraints": {
        "defaultCallerDeadlineMs": 12000,
        "nativeMediaWorkers": 2,
        "globalNativeQueue": 32,
        "ownerNativeQueue": 8,
        "rendererPendingQueue": 8,
        "newThreads": 0,
        "jniRetryAdded": False,
        "hardFailureResetAdded": False,
        "closeReversalAdded": False,
        "sdkAarOrBinaryChanged": False,
    },
    "sourceCheck": {**file("tests/flutter/native-source-check-108.json"), "inputCount": len(source_check["before"]), "beforeAfterIdentical": True, "scope": "Whole vendored native Java module, formal test Java/stubs, standard native test script"},
    "modifiedProductionFiles": [file(name) for name in PRODUCT],
    "deliveryIntegration": [file("clients/flutter/scripts/test-native-java.ps1")],
    "newTests": [file("tests/flutter/NativeDeadlineRecoveryIndependentTest.java"), file("tests/flutter/NativeDeadlineRendererTest.java"), file("tests/flutter/NativeSurfaceDeadlineTest.java")],
    "newSurfaceBoundaryStubs": [file(path.relative_to(ROOT).as_posix()) for path in sorted((ROOT / "tests/flutter/native-surface-stubs").rglob("*.java"))],
    "updatedRendererBoundaryStub": file("tests/flutter/native-renderer-stubs/android/util/Log.java"),
    "actuallyExecuted": {
        "standardJavaSuite": {"exitCode": 0, "executableGroups": 12, "scenarioCount": 88, "legacyScenariosReexecuted": 69, "newNamedCases": {"independentOwnerSequenceBinding": 11, "bindingAndActualRenderer": 5, "actualSurface": 3}, "log": file("tests/flutter/native-java-formal-108.log"), "boundary": "Actual production Java lifecycle classes; controlled JNI/EGL/Android/Flutter stubs, not device JNI execution"},
        "androidProductionJavac": {"exitCode": 0, "task": ":flutter_webrtc:compileReleaseJavaWithJavac --offline --no-daemon --max-workers=1", "jdk": "17.0.20.1+1", "log": file("tests/flutter/native-android-javac-108.log"), "boundary": "Real project Android/AAR classpath compilation, not an APK build or runtime acceptance"},
        "reusedTestsWithoutExecution": [],
    },
    "historicalProgressOnly": [file("tests/flutter/native-java-108-before-surface-83.log"), file("tests/flutter/native-android-javac-108-before-surface.log"), file("tests/flutter/native-deadline-renderer-108-first.log"), file("tests/flutter/native-surface-deadline-108-first.log"), file("tests/flutter/native-deadline-independent-108-candidate.log")],
    "preserved86CandidateBeforeHardCurrentCorrection": file("tests/flutter/diagnostics/108/before-hard-current/manifest.json"),
    "partialHistorical88BeforeFailureReadOrderCorrection": {**file("tests/flutter/diagnostics/108/before-read-order/manifest.json"), "originalLogBytesUnavailable": True, "note": "Old source-check/report/diff and hash-verified reconstructed Java bytes retained; prior log SHAs retained but log bytes lost due disclosed archive command error. Current final validation logs below are real new executions."},
    "historicalArchiveErrorNote": file("tests/flutter/diagnostics/108/before-read-order/history-note.json"),
    "preservedOriginalBytes": {
        "unchangedOriginalManifest": file(ARCHIVE + "manifest.json"),
        "surfaceAppendix": file(ARCHIVE + "surface-before.json"),
        "stubAppendix": file(ARCHIVE + "stub-before.json"),
        "pathLayout": "Each original file is retained byte-for-byte below the archive directory using its original project-relative path",
    },
    "diff": file(diff_path),
    "runtimeLogPhases": {
        "ownerJob": ["queued", "workerStart", "ownerAcquired", "actionEnd", "physicalComplete"],
        "hardOwner": ["quarantine", "rejected", "closing"],
        "deadline": ["callerTimeout", "physicalOutcome", "deadlineOverflow"],
        "localGuard": "operation=local.admission phase=blocked; explicit owner/sequence/closing counts",
        "rpcFailure": "phase=callerError code=RTC_*",
        "attachRetirement": "phase=attachRetired reason=owner_closing",
        "hardAttachFailure": "phase=attachRejected reason=owner_hard_failure",
    },
    "limits": [
        "107 earliest renderer failure code/guard transition was not instrumented; do not retrospectively assert a precise earlier JNI cause.",
        "No AVD, active Web helper, production service, server configuration, native AAR, user data or media source was operated/changed by this subtask.",
        "Any12s timeout remains a real caller error. Recovery tests do not convert an errored long-call run into acceptance success.",
        "Two normal original-UI Android video calls, post-hangup immediate message, full camera/native resource release, capture republish and Surface lifecycle still require108 runtime evidence.",
    ],
}
path = ROOT / "tests/flutter/native-deadline-review-108.json"
path.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
print(json.dumps(file(path.relative_to(ROOT).as_posix())))
