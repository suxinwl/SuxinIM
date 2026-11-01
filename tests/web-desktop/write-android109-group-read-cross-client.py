"""Aggregate already captured UI evidence; never operates a client or server."""
from pathlib import Path
from datetime import datetime, timezone
import hashlib
import json

ROOT = Path(__file__).resolve().parents[2]
OUT = ROOT / "tests/web-desktop/android109-group-read"
APK_SHA = "2a02d3a885713b6492796916e981cca3b6b6c49d9ed2f838c0c3c902d9c890c4"
MESSAGE = "93a25062adbd023e30b96338411845e6"


def record(path):
    data = path.read_bytes()
    return {"path": path.relative_to(ROOT).as_posix(), "size": len(data),
            "sha256": hashlib.sha256(data).hexdigest()}


evidence = []
android_steps = []
expected = {
    "0-2-unread": {"read": 0, "unread": 2, "activeTab": "unread", "names": ["升级测试102", "升级测试103"], "empty": False},
    "0-2-read": {"read": 0, "unread": 2, "activeTab": "read", "names": [], "empty": True},
    "1-1-read": {"read": 1, "unread": 1, "activeTab": "read", "names": ["升级测试102"], "empty": False},
    "1-1-unread": {"read": 1, "unread": 1, "activeTab": "unread", "names": ["升级测试103"], "empty": False},
    "2-0-unread": {"read": 2, "unread": 0, "activeTab": "unread", "names": [], "empty": True},
    "2-0-read": {"read": 2, "unread": 0, "activeTab": "read", "names": ["升级测试102", "升级测试103"], "empty": False},
}
for name, state in expected.items():
    path = ROOT / f"tests/flutter/android-109-group-read-{name}.json"
    captured = json.loads(path.read_text(encoding="utf-8"))
    screenshot = ROOT / captured["screenshot"]
    image_ref = record(screenshot)
    assert captured["screenshotSHA256"] == image_ref["sha256"]
    assert captured["originalUI"] and captured["entrypoint"] == "lib/main.dart"
    assert captured["ABI"] == "x86_64" and captured["PID"] == 5130 and captured["APK_SHA256"] == APK_SHA
    assert captured["clientMsgID"] == MESSAGE and captured["groupID"] == "2901057568"
    assert captured["messageText"] == "a108gr1" and captured["historicalSeq"] == 2
    labels = [item["text"] for item in captured["labels"]]
    assert f"Unread ({state['unread']})" in labels and f"Read ({state['read']})" in labels
    assert ("No members" in labels) == state["empty"]
    json_ref = record(path)
    evidence.extend([json_ref, image_ref])
    android_steps.append({"captureAt": captured["at"], "state": state, "metadata": json_ref,
        "screenshot": image_ref, "sixPNGsIndependentlyVisuallyInspected": True,
        "nameEvidence": "Names are read from the actual screenshot; some UI-automation text names contain replacement characters and are not used as a Unicode-name oracle."})

web_steps = []
for peer in ["102", "103"]:
    path = OUT / f"web{peer}-original-ui-open.json"
    captured = json.loads(path.read_text(encoding="utf-8"))
    assert captured["originalUIListClicked"] and captured["peer"] == peer
    assert captured["beforeURL"].endswith("/#/chat")
    assert captured["afterURL"].endswith("/#/chat/sg_2901057568")
    assert captured["rendered"]["id"] == "chat_"+MESSAGE
    assert captured["message"]["clientMsgID"] == MESSAGE and captured["message"]["seq"] == 2
    assert captured["message"]["sendID"] == "upgrade38_u104" and captured["message"]["text"] == "a108gr1"
    receipt = captured["markObservation"]
    assert receipt["observed"] and receipt["status"] == 200 and receipt["errCode"] == 0
    assert receipt["seqs"][MESSAGE] == 2 and receipt["mark"][MESSAGE] == 1 and not captured["errors"]
    image_ref = record(OUT / f"web{peer}-original-ui-open.png")
    json_ref = record(path)
    evidence.extend([json_ref, image_ref])
    web_steps.append({"peer": "upgrade38_u"+peer, "originalConversationListClick": True,
        "clickedText": captured["clickedText"], "beforeURL": captured["beforeURL"], "afterURL": captured["afterURL"],
        "rendered": captured["rendered"], "SDKMessage": captured["message"], "observedProductReceipt": receipt,
        "metadata": json_ref, "screenshot": image_ref, "screenshotIndependentlyVisuallyInspected": True})

other_paths = [
    ROOT / "tests/flutter/android-main-x64-109.json",
    ROOT / "tests/flutter/android-main-x64-109-build-start.json",
    ROOT / "tests/flutter/runtime-source-snapshot-109.json",
    ROOT / "tests/flutter/group-read-adapter-review-109.json",
    ROOT / "tests/flutter/group-read-independent-final-review-109.json",
    OUT / "web-runtime-review.json",
    ROOT / "tests/web-desktop/incident-after-synthetic-rotation/20261101T093224Z/ready-after-rotation.json",
    ROOT / "clients/web-desktop/tests/android-collaboration.mjs",
    ROOT / "clients/web-desktop/tests/video-frame-state.mjs",
    ROOT / "clients/web-desktop/tests/rtc-media-state.mjs",
    ROOT / ".private/android-validation/OpenIM38-3.8.9+109-main-x64-candidate.apk",
]
evidence.extend(record(p) for p in other_paths)
assert evidence[-1]["sha256"] == APK_SHA
apk_report = json.loads(other_paths[0].read_text(encoding="utf-8"))
assert apk_report["sha256"] == APK_SHA and apk_report["abis"] == ["x86_64"]
assert apk_report["entrypoint"] == "lib/main.dart"
assert apk_report["sourceSHA256"] == "7c0c75520658c61bcd6e25a93b8e5d3eb266976a22edb39bfa5557572277d55e"

report = {
    "at": datetime.now(timezone.utc).isoformat(),
    "scope": "109 cross-client originalUI group message read-detail test in one synthetic three-person group",
    "result": "PASS within the explicitly stated x86_64 emulator and synthetic-group scope",
    "fixture": {"groupID": "2901057568", "groupName": "Android104 RTC collaboration 1793518543854",
        "members": ["upgrade38_u102", "upgrade38_u103", "upgrade38_u104"], "sender": "upgrade38_u104",
        "message": "a108gr1", "clientMsgID": MESSAGE, "historicalSeq": 2, "messageCreatedBefore109Fix": True},
    "androidArtifact": {"applicationVersion": "3.8.9+109", "APK_SHA256": APK_SHA,
        "entrypoint": "lib/main.dart", "validationWrapper": False, "ABI": "x86_64",
        "runtimePIDAcrossAllSixCaptures": 5130, "environment": "Dedicated API30 native x86_64 AVD",
        "sourceSHA256": apk_report["sourceSHA256"], "ARM64Deliverable": False,
        "physicalDevice": False, "published": False},
    "observedSequence": [
        "Android original Message Recipient List: read0/unread2, names102and103 in unread, read tab No members.",
        "Only after the Android operator confirmed baseline, Web102 clicked its real original conversation-list entry; same message rendered and the product automatically posted a successful receipt.",
        "Android original details changed to read1/unread1:102 in read and103 in unread. Operator reported no page re-entry or manual refresh.",
        "Only after that Android confirmation, Web103 clicked its real original conversation-list entry; same message rendered and its product-generated receipt succeeded.",
        "Android original details changed to read2/unread0:102and103 in read, unread tab No members."
    ],
    "androidOriginalUI": android_steps, "webOriginalUI": web_steps,
    "evidence": evidence, "allEvidenceSizeAndSHARechecked": True,
    "implementationBoundary": {"APIOnlyNotUsedAsUIProof": True, "noHelperDirectReceiptMutation": True,
        "originalListNavigationNoSDKFallback": True, "AndroidNamesConfirmedFromPNGs": True,
        "eachWebActionWaitedForAndroidOperator": True, "noSourceOrClientUIOperationDuringThisAggregation": True,
        "noSecretOrJWTIncluded": True},
    "limits": {"syntheticMembers": 3, "recipientCount": 2, "messagesUnderTest": 1,
        "largeGroupScaleOrLoadTest": False, "physicalARM64RuntimeValidation": False,
        "ARMTranslationStartupPass": False, "offlineRetryIncluded": False,
        "offlineRetryBoundary": "Operator is separately collecting actual offline/error/retry evidence. No result from that work is claimed or linked here.",
        "fullUpgradeAcceptance": False, "RTCOrMediaValidatedByThisReport": False},
    "previousWebReportBoundary": "The immutable earlier web-runtime-review.json correctly marked final Android2/0 pending at its creation. This new aggregation binds the later six captured Android states; it does not rewrite that history."
}
path = OUT / "cross-client-review-109.json"
assert not path.exists(), "Never overwrite an already-announced evidence report"
path.write_text(json.dumps(report, ensure_ascii=False, indent=2)+"\n", encoding="utf-8")
for ref in evidence:
    assert record(ROOT / ref["path"]) == ref
print(json.dumps(record(path)))
