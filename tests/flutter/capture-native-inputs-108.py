"""Hash the exact current native module and formal test inputs around validation."""
import hashlib
import json
import re
import sys
from datetime import datetime, timezone
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
NATIVE = ROOT / "clients/flutter/local_plugin/compat/flutter_webrtc-1.6.0/android/src/main/java/com/cloudwebrtc/webrtc"
SCRIPT = ROOT / "clients/flutter/scripts/test-native-java.ps1"
CHECK = ROOT / "tests/flutter/native-source-check-108.json"
paths = set(NATIVE.rglob("*.java"))
for name in re.findall(r"Join-Path \$tests '([^']+\.java)'", SCRIPT.read_text()):
    paths.add(ROOT / "tests/flutter" / name)
for directory in ("native-renderer-stubs", "native-surface-stubs"):
    paths.update((ROOT / "tests/flutter" / directory).rglob("*.java"))
paths.add(SCRIPT)
rows = []
for path in sorted(paths):
    data = path.read_bytes()
    rows.append({"path": path.relative_to(ROOT).as_posix(), "bytes": len(data), "sha256": hashlib.sha256(data).hexdigest()})
phase = sys.argv[1]
if phase == "before":
    if CHECK.exists():
        raise SystemExit("Existing current check must be preserved")
    report = {"schema": 1, "beforeAt": datetime.now(timezone.utc).isoformat(), "before": rows}
elif phase == "after":
    report = json.loads(CHECK.read_text())
    if "after" in report:
        raise SystemExit("Existing final check must be preserved")
    report.update(afterAt=datetime.now(timezone.utc).isoformat(), after=rows, identical=report["before"] == rows)
else:
    raise SystemExit("Use before or after")
CHECK.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
print(json.dumps({"phase": phase, "inputs": len(rows), "identical": report.get("identical"), "sha256": hashlib.sha256(CHECK.read_bytes()).hexdigest()}))
