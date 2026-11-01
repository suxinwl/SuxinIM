"""Record exact just_audio107 changes and actual-class test boundaries."""
from pathlib import Path
from datetime import datetime, timezone
import hashlib
import json

ROOT = Path(__file__).resolve().parents[2]
SOURCE = Path("clients/flutter/local_plugin/compat/just_audio-0.9.46/lib/just_audio.dart")
TEST = Path("clients/flutter/test/just_audio_platform_future_test.dart")
TESTS = Path("tests/flutter")
ARCHIVE = TESTS / "source-history/106-0d9cf0c7"

def digest(path):
    data = (ROOT / path).read_bytes()
    return {"sha256": hashlib.sha256(data).hexdigest(), "size": len(data)}

def evidence(path):
    return {"path": str(path).replace("\\", "/"), **digest(path)}

snapshot = json.loads((ROOT / TESTS / "runtime-source-snapshot-106.json").read_text(encoding="utf-8-sig"))
archived_source = ARCHIVE / SOURCE.relative_to("clients/flutter")
assert digest(archived_source)["sha256"] == snapshot["files"][str(SOURCE).replace("\\", "/")]
source_check = TESTS / "just-audio-final-source-check-107.json"
checks = json.loads((ROOT / source_check).read_text(encoding="utf-8-sig"))
for item in checks["checks"]:
    assert digest(Path(item["path"]))["sha256"] == item["sha256"]
final_log = TESTS / "just-audio-platform-final-107.log"
assert "+9: All tests passed!" in (ROOT / final_log).read_text(encoding="utf-8", errors="replace")
before_log = TESTS / "just-audio-platform-before-107-expanded.log"
before_text = (ROOT / before_log).read_text(encoding="utf-8", errors="replace")
assert "+1 -5: Some tests failed." in before_text
late_log = TESTS / "just-audio-late-epoch-before-107.log"
assert "Actual: <1>" in (ROOT / late_log).read_text(encoding="utf-8", errors="replace")
analysis = TESTS / "just-audio-platform-analysis-final-107.log"
analysis_text = (ROOT / analysis).read_text(encoding="utf-8", errors="replace")
assert "warning -" not in analysis_text and "error -" not in analysis_text
runtime_log = TESTS / "android-106-call1-after-hangup-logcat.txt"
runtime_text = (ROOT / runtime_log).read_text(encoding="utf-8", errors="replace")
assert "AudioPlayer._setPlatformActive.checkInterruption" in runtime_text

report = {
    "generatedAt": datetime.now(timezone.utc).isoformat(),
    "scope": "just_audio0.9.46 internal activation Future propagation and old-activation ownership only",
    "candidateBuild": 107,
    "status": "actual_plugin_class9_and_independent_source_review_passed_pending_unified_suite_and_android107_runtime",
    "onlyProductionFileEditedByThisSubtask": evidence(SOURCE),
    "testSource": evidence(TEST),
    "frozen106BeforeBytes": {
        "sourceSnapshotSHA256": snapshot["sourceSHA256"],
        "archiveManifest": evidence(ARCHIVE / "just-audio-before.json"),
        "originalSource": evidence(archived_source),
        "originalPubspec": evidence(ARCHIVE / "local_plugin/compat/just_audio-0.9.46/pubspec.yaml"),
        "historicalSnapshotOverwritten": False,
    },
    "observed106Failure": {
        "publicLog": evidence(runtime_log),
        "guestTime": "2026-11-01 05:02:11.865",
        "error": "PlatformException(abort, Loading interrupted, null, null)",
        "stack": "AudioPlayer._setPlatformActive.checkInterruption:1331 -> setPlatform:1488",
        "diagnosis": "_setPlatformActive returns durationCompleter.future to load while separately caching setPlatform() as _platform. checkInterruption previously completed duration with an error then threw into the unobserved cached sibling. SafeRingtone observing its public load Future cannot observe this separate cached Future. Initialization/configuration failure before the _load catch also failed the cached Future without settling duration; deferred play has its own playCompleter and could likewise remain pending.",
        "noPhysicalCauseClaim": "This is a confirmed Dart Future propagation defect matching the runtime stack. It does not attribute the error to ExoPlayer, emulator resources, native media JNI or another connection.",
    },
    "changes": {
        "cachedFuture": "Observe the exact original platformFuture with then<void> error handling. The observer bridges original error and stack to still-pending duration/play completers. _platform retains the original failed Future for genuine awaiters; it is not replaced by null or a success fallback.",
        "singleCompletion": "completeActivationError checks each public completer before completion. checkInterruption throws its original abort into that bridge. _sendPlayRequest checks its captured play completer before late native success/error replies, avoiding repeated completion after an activation error.",
        "oldEpoch": "The _load catch only deactivates native state when its captured activationNumber remains current. A superseded load reports its original interruption but cannot call _setPlatformActive(false) against a newer active player.",
        "currentFailure": "An actual current-activation load failure still follows the existing deactivation cleanup. Cancellation is not changed into success, native work is not canceled or declared completed, and platform error identity/stack is retained.",
        "unchanged": "No global error handler, broad PlatformException suppression, timeout extension, platform/backend replacement, or changes to existing setPitch/setSkipSilence fallback branches.",
    },
    "beforeEvidence": {
        "expanded6Cases": {**evidence(before_log), "passed": 1, "failed": 5},
        "exactBeforeTestBytes": evidence(TESTS / "diagnostics/107/just-audio-platform-before-test.dart"),
        "lateNewEpochFailure": {**evidence(late_log), "passed": 0, "failed": 1, "proven": "With old load held, stop then newer same-player load/play succeeded. Releasing the old load physically disposed the newer fake native platform once."},
        "beforeEpochGuardProductionBytes": evidence(TESTS / "diagnostics/107/just-audio-before-epoch-guard.dart"),
    },
    "verification": {
        "finalTargetedPlugin": {**evidence(final_log), "passed": 9, "failed": 0, "exitCode": 0},
        "finalInputs": evidence(source_check),
        "independentFinalReview": evidence(TESTS / "just-audio-independent-final-review-107.json"),
        "allFinalInputsUnchanged": True,
        "analysis": {**evidence(analysis), "errors": 0, "warnings": 0, "existingInfos": 7, "infoCode": "use_super_parameters; existing upstream style, unrelated code intentionally unchanged"},
        "firstFixCombinedHistory": {**evidence(TESTS / "just-audio-ringtone-final-107.log"), "pluginCases": 8, "safeRingtoneCases": 10, "passed": 18, "boundary": "Historical intermediate source before the final old-activation guard. Final plugin9 rerun follows below; the unified Flutter suite must rebind all current sources."},
        "coverage": [
            "Actual AudioPlayer interrupted during held native configuration: public abort remains an error, no cached-sibling unhandled error; same player can subsequently load and stop normally",
            "Native init and pre-load configuration errors settle public load with original identity; configuration error stack retains _NativePlayer.setVolume",
            "Deferred-play activation failure settles its own public Future",
            "A real caller awaiting the cached platform through setSpeed still receives the original configuration error",
            "After actual interruption, late native play success and late native failure each leave the first play result unchanged and cause no duplicate completer/unobserved error",
            "Late old load cannot dispose or finish a newer healthy same-player playback",
            "Healthy actual-class load/play/stop/reload remains usable",
        ],
        "boundary": "Tests execute the real vendored AudioPlayer with a controlled JustAudioPlatform/AudioPlayerPlatform boundary. They are not an ExoPlayer, physical audio, emulator or speaker playback result. No copied activation implementation is tested.",
    },
    "diff": evidence(TESTS / "just-audio-future-107.diff"),
    "requiredRuntime": "Unified107 APK original UI call/ringtone start-stop and load interruption, no unhandled abort or hang, plus the complete Android media/cleanup regression; this subtask did not build or install an APK.",
    "notOperated": ["AVD", "Web101/102/103 helpers", "server", "SafeRingtone/App/Room/Engine/IO/native production source", "dependency/version/signing manifests"],
}
target = TESTS / "just-audio-source-review-107.json"
(ROOT / target).write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
print(json.dumps(evidence(target)))
