"""Record the IO/Signal106 source and its targeted QA without device claims."""
from pathlib import Path
from datetime import datetime, timezone
import hashlib
import json

ROOT = Path(__file__).resolve().parents[2]
BASE = Path("clients/flutter/local_plugin/compat/livekit_client-2.11.0/lib/src")

def digest(path):
    data = (ROOT / path).read_bytes()
    return {"sha256": hashlib.sha256(data).hexdigest(), "size": len(data)}

sources = [BASE / "support/websocket/io.dart", BASE / "core/signal_client.dart"]
before = Path("tests/flutter/source-history/105-a7b18546")
archived = [before / p.relative_to("clients/flutter") for p in sources]
assert all((ROOT / p).is_file() for p in archived)
final_log = Path("tests/flutter/io-signal-integration-106-final.log")
assert "+14: All tests passed!" in (ROOT / final_log).read_text(encoding="utf-8", errors="replace")
source_lock = Path("tests/flutter/io-signal-integration-106-final-source-lock.json")
tested_sources = json.loads((ROOT / source_lock).read_text(encoding="utf-8-sig"))
for path, expected in tested_sources.items():
    assert digest(Path(path))["sha256"] == expected, path
analysis = Path("tests/flutter/io-signal-analysis-106.log")
assert "No issues found!" in (ROOT / analysis).read_text(encoding="utf-8", errors="replace")
report = {
    "generatedAt": datetime.now(timezone.utc).isoformat(),
    "candidateBuild": 106,
    "scope": "Only IO websocket connection/physical cleanup and SignalClient socket epochs",
    "status": "targeted14_passed_and_analyzer_clean_pending_unified_full_suite_and_original_android_runtime",
    "sourceFiles": {str(p).replace("\\", "/"): digest(p) for p in sources},
    "frozen105BeforeBytes": {str(p).replace("\\", "/"): digest(p) for p in archived},
    "physicalOwnership": {
        "ioZone": "Each IO WebSocket owns an isolated guarded error zone and an independent HttpClient. The caller-zone connect completer manually bridges success/error; all real WebSocket futures are awaited within the IO zone.",
        "dispose": "A canonical disposal completer is created in the connection caller's zone. Physical super.dispose runs once in the owning IO zone and explicitly settles that public result. Only after settling does the caller zone invoke onDispose. A callback awaiting the same dispose cannot precede its own physical result.",
        "cleanupOrder": "Attempt WebSocket.close, WebSocket.done, subscription.cancel and owned HttpClient cleanup as distinct stages, preserving first cleanup failure. readyState alone is never substituted for outgoing close completion. The client closes only after done actually settles with closed state; unknown physical state retains the client.",
        "callbacks": "onData/onDispose synchronous and returned-Future failures are observed in the caller zone. Original error identity reaches onError. A failing onError is diagnosed once without recursive error delivery. Notification failure after actual close does not retroactively change physical completion.",
        "unexpectedTransport": "The error stays visible and initiates cleanup of its own connection. SignalClient emits one disconnect for the matching active epoch; an old epoch cannot close or mutate another active socket.",
        "expectedClosedRead": "Only within this exact connection's zone, while closing and after physical close is established: SocketException message exactly Reading from a closed socket, with first two frames _RawSecureSocket.read in dart:io/secure_socket.dart then _Socket._onData in dart:io-patch/socket_patch.dart. It is diagnosed as expected SDK late read. Business errors, other stacks, other connections, and unknown physical state do not match.",
    },
    "epochOwnership": {
        "generation": "Connect and public cleanup invalidate prior epochs synchronously. Connector handlers, post-await assignments, validation result state changes and connectivity updates recheck ownership.",
        "lateConnect": "A connector returning after cleanup/timeout/supersession must actually dispose its own socket before its old future rejects. It cannot assign _ws or emit into a newer epoch.",
        "boundedOwnerTable": 8,
        "capacity": "Pending, active and physically unknown epochs share the same bounded table. Ninth attempt is rejected before changing the active generation. Genuine successful disposal releases its slot and healthy reconnect remains usable.",
        "lateFailure": "A real failed late-socket disposal remains strongly owned. Its first failed barrier rejects subsequent connection attempts, while the currently active newer socket continues. Explicit cleanup still attempts the active socket/queue/timers before reporting the old failure; unknown failed native/IO disposal is never retried.",
        "preConnectorFailure": "An old active socket's cleanup failure rethrows before connector startup or HTTP validation. Original physical error identity/stack and failed barrier are preserved.",
    },
    "diagnostics": "Fixed role/epoch/phase/elapsed time/error type only. WebSocket/Signal URI logger reconstructs scheme/host/port/path, dropping userinfo/query/fragment. No JWT, SDP or signed query is recorded by this instrumentation.",
    "targetedVerification": {
        "finalSuite": {"path": str(final_log), **digest(final_log), "exitCode": 0, "realLoopbackTlsCases": 8, "actualSignalClientFakeBoundaryCases": 6},
        "finalTestedSourceLock": {"path": str(source_lock), **digest(source_lock), "currentInputsMatched": len(tested_sources)},
        "analysis": {"path": str(analysis), **digest(analysis), "result": "No issues found", "exitCode": 0},
        "initialFailureLog": {"path": "tests/flutter/rtc-io-integration-106-first.log", **digest(Path("tests/flutter/rtc-io-integration-106-first.log"))},
        "initialPhaseLog": {"path": "tests/flutter/io-reentry-106-before-phase.log", **digest(Path("tests/flutter/io-reentry-106-before-phase.log"))},
        "initialFailureBoundary": "First candidate 16/17 passed, server-triggered close never reached onDispose and timed out before notify. Original failure logs and input SHA locks remain; initial candidate bytes were not reconstructed after editing. Frozen105 original source bytes are independently retained.",
        "firstBridgePass": {"path": "tests/flutter/io-signal-integration-106-first.log", **digest(Path("tests/flutter/io-signal-integration-106-first.log")), "cases": 11},
        "testBoundary": "Loopback tests use actual IO WebSocket/TLS and ephemeral trusted CA plus certificate pinning, without production endpoint or TLS bypass. Signal tests use the actual SignalClient with explicit connector/socket doubles. They do not reproduce the Android raw secure-socket race or prove Android/JNI runtime acceptance.",
    },
    "requiredRuntime": [
        "Unified106 APK original UI normal two long calls without manual SIGQUIT during the timing window",
        "Fresh decoded frames, hangup immediately followed by delivered text, no input ANR or unhandled socket/cleanup Future",
        "Real camera/media/socket release and second-call healthy reconnect",
    ],
    "notEdited": ["Room", "Engine", "event manager", "native106 source", "AVD", "Web101 helper", "application manifests", "server processes"],
}
target = Path("tests/flutter/io-signal-source-review-106.json")
(ROOT / target).write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
print(json.dumps({"path": str(target), **digest(target)}))
