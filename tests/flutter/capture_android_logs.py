"""Keep byte-exact AVD evidence private; publish redacted diagnostics only."""
from pathlib import Path
import datetime
import hashlib
import json
import re
import subprocess
import sys

ROOT = Path(__file__).absolute().parents[2]
label = sys.argv[1]
if not re.fullmatch(r"[a-z0-9-]+", label):
    raise ValueError("Invalid evidence label")
adb = ["E:/AndroidDev/sdk/platform-tools/adb.exe", "-s", "emulator-5588"]
avd = subprocess.check_output(adb + ["emu", "avd", "name"], timeout=20)
if avd.splitlines()[0] not in [b'openim38_api30', b'openim38_api30_arm109']:
    raise RuntimeError("This capture is limited to our dedicated API30 AVD")
raw = subprocess.check_output(adb + ["logcat", "-d"], timeout=45)
private = ROOT / ".private/android-raw-evidence" / f"{label}-logcat.txt"
if not private.parent.is_dir():
    raise RuntimeError("Create and restrict the private evidence directory first")
if private.exists() and private.read_bytes() != raw:
    raise RuntimeError("Use a new label; original evidence is immutable")
private.write_bytes(raw)
public, jwt_count = re.subn(
    rb"eyJ[A-Za-z0-9_-]{15,}\.[A-Za-z0-9_-]{20,}\.[A-Za-z0-9_-]{15,}",
    b"[REDACTED_JWT]", raw,
)
public, signature_count = re.subn(
    rb"X-Amz-Signature=[A-Fa-f0-9]+",
    b"X-Amz-Signature=[REDACTED]", public,
)
secrets = set()


def collect(value, key=""):
    if isinstance(value, dict):
        for name, item in value.items():
            collect(item, name)
    elif isinstance(value, list):
        for item in value:
            collect(item, key)
    elif isinstance(value, str) and len(value) > 8 and (
        "token" in key.lower() or "password" in key.lower()
    ):
        secrets.add(value.encode())
        if "password" in key.lower():
            login_digest = hashlib.md5(value.encode()).hexdigest()
            secrets.add(login_digest.encode())
            secrets.add(login_digest.upper().encode())


for name in ["test-accounts.json", "test-sessions.json", "android-ui-session.json", "android-incident-109-redaction-secrets.json"]:
    source = ROOT / ".private" / name
    if source.is_file():
        collect(json.loads(source.read_text(encoding="utf-8-sig")))
exact_count = 0
for value in sorted(secrets, key=len, reverse=True):
    exact_count += public.count(value)
    public = public.replace(value, b"[REDACTED_CREDENTIAL]")
# Never export an unreviewed SDP/ICE secret block. Keep its raw bytes private
# and require an exact, counted redaction rule rather than broad log deletion.
if re.search(rb'(?i)\bsdp\b.{0,80}v=0|a=ice-(?:ufrag|pwd):|a=fingerprint:|m=(?:audio|video) [0-9]', public):
    raise RuntimeError('Raw evidence preserved privately; SDP requires an exact redaction rule before public export')
if re.search(rb'(?i)["\']?(?:credential|icePassword)["\']?\s*[:=]\s*["\']?[^\s,)}\]"\']{4}', public):
    raise RuntimeError('Raw evidence preserved privately; ICE credentials require exact redaction before public export')

destination = ROOT / "tests/flutter" / f"{label}-logcat.txt"
destination.write_bytes(public)
record = {
    "capturedAt": datetime.datetime.now(datetime.timezone.utc).isoformat(),
    "publicFile": destination.relative_to(ROOT).as_posix(),
    "privateOriginal": private.relative_to(ROOT).as_posix(),
    "originalSHA256": hashlib.sha256(raw).hexdigest(),
    "redactedSHA256": hashlib.sha256(public).hexdigest(),
    "jwtOccurrencesRemoved": jwt_count,
    "presignedSignaturesRemoved": signature_count,
    "exactKnownCredentialsRemoved": exact_count,
    "otherBytesPreserved": True,
    "SDPAndICECredentialExportGuardPassed": True,
}
version_label = re.match(r'android-(\d+)-', label)
if version_label is None:
    raise ValueError('Use a label with the actual tested Android build')
manifest = ROOT / f"tests/flutter/raw-log-redaction-{version_label[1]}.json"
data = json.loads(manifest.read_text()) if manifest.exists() else {
    "policy": "Unredacted byte-exact originals under current-user/SYSTEM private ACL; only redacted diagnostics exported",
    "files": [],
}
data["files"] = [x for x in data["files"] if x["publicFile"] != record["publicFile"]]
data["files"].append(record)
manifest.write_text(json.dumps(data, indent=2) + "\n", encoding="utf-8")
print(json.dumps({"file": record["publicFile"], "redactions": jwt_count + signature_count + exact_count}))
