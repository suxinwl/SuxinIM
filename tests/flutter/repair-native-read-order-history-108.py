"""Record the failed archive attempt honestly; verify recoverable old bytes."""
import hashlib
import json
from pathlib import Path

ROOT=Path(__file__).resolve().parents[2]
ARCHIVE=ROOT/"tests/flutter/diagnostics/108/before-read-order"
report_path=ROOT/"tests/flutter/native-deadline-review-108.json"
report_data=report_path.read_bytes()
assert hashlib.sha256(report_data).hexdigest()=="6d4bdebb4bbb8f529ca002e0aea2ee2f0e2d8dc936a01c21323386a26900015d"
report=json.loads(report_data)
expected={row["path"]:row for row in report["modifiedProductionFiles"]}
base="clients/flutter/local_plugin/compat/flutter_webrtc-1.6.0/android/src/main/java/com/cloudwebrtc/webrtc/"
replacements={
    "NativeMediaLifecycle.java":(
        "    // Failure is published before its pending entry is removed. Read in the\n    // matching order so false(old hard) + zero(new pending) cannot admit work.\n    boolean isQuarantined() { return deadlines.pendingCount() != 0 || quarantined; }",
        "    boolean isQuarantined() { return quarantined || deadlines.pendingCount() != 0; }"),
    "NativeRendererBinding.java":(
        "      final boolean pendingDeadline = deadlines.pendingCount() != 0;\n",
        ""),
}
items=[]
for name,(new,old) in replacements.items():
    path=base+name
    data=(ROOT/path).read_bytes()
    newline=b"\r\n" if b"\r\n" in data else b"\n"
    def encoded(text):return text.encode().replace(b"\n",newline)
    assert data.count(encoded(new))==1
    restored=data.replace(encoded(new),encoded(old))
    if name=="NativeRendererBinding.java":
        newer=b"if (track != null && pendingDeadline)"
        older=b"if (track != null && deadlines.pendingCount() != 0)"
        assert restored.count(newer)==1
        restored=restored.replace(newer,older)
    assert hashlib.sha256(restored).hexdigest()==expected[path]["sha256"],path
    dst=ARCHIVE/path
    dst.parent.mkdir(parents=True,exist_ok=True)
    assert not dst.exists()
    dst.write_bytes(restored)
    items.append({**expected[path],"method":"Exact inverse of two last read-order edits, verified against already-recorded prior88 source SHA; not falsely labelled captured before edit"})
for name,sha in (
    ("tests/flutter/native-deadline-review-108.json","6d4bdebb4bbb8f529ca002e0aea2ee2f0e2d8dc936a01c21323386a26900015d"),
    (report["diff"]["path"],report["diff"]["sha256"]),
):
    data=(ROOT/name).read_bytes();assert hashlib.sha256(data).hexdigest()==sha
    dst=ARCHIVE/name;dst.parent.mkdir(parents=True,exist_ok=True);assert not dst.exists();dst.write_bytes(data)
    items.append({"path":name,"sha256":sha,"bytes":len(data),"method":"Still-available original bytes copied after detected archive attempt failure"})
source=ARCHIVE/"original-native-source-check-108.json"
assert hashlib.sha256(source.read_bytes()).hexdigest()==report["sourceCheck"]["sha256"]
items.append({**report["sourceCheck"],"archivedAs":"original-native-source-check-108.json","method":"Original file actually moved by the earlier command"})
manifest=ARCHIVE/"manifest.json"
assert manifest.exists()
failed=ARCHIVE/"manifest-archive-attempt-failed.json"
assert not failed.exists();manifest.rename(failed)
history={
    "schema":1,"scope":"Historical88 before failure-read-order correction; partial log preservation explicitly disclosed",
    "archiveMistake":"PowerShell array concatenation produced one invalid combined source path. Copy loop failed; the old source-check Move succeeded. Old88 validation logs were then overwritten by the final run.",
    "preservedOrVerifiedRestored":items,
    "originalLogBytesUnavailable":[{"path":row["log"]["path"],"previouslyRecordedSHA256":row["log"]["sha256"],"evidence":"Prior tool execution completed exit0 and prior report recorded this SHA; no log bytes reconstructed or falsely claimed preserved"} for row in report["actuallyExecuted"].values() if isinstance(row,dict) and "log" in row],
    "unaffected":"107 frozen byte archives and86 historical full validation artifacts remain available. Final post-read-order88 logs will be separately real executions, not replacements claimed as old bytes.",
    "failedAttemptManifestSHA256":hashlib.sha256(failed.read_bytes()).hexdigest(),
}
manifest.write_text(json.dumps(history,ensure_ascii=False,indent=2)+"\n",encoding="utf-8")
print(json.dumps({"path":str(manifest),"sha256":hashlib.sha256(manifest.read_bytes()).hexdigest(),"missingHistoricalLogs":len(history["originalLogBytesUnavailable"])}))
