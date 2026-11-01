"""Verify the SSH-collected final load archive, then extract only 13 new reports."""
import hashlib
import importlib.util
import json
import pathlib
import tarfile

ROOT = pathlib.Path(__file__).resolve().parents[1]
RUN = "final-100-20261101a"
spec = importlib.util.spec_from_file_location("final_load_collector", ROOT / "tests/collect-final-load.py")
collector = importlib.util.module_from_spec(spec)
spec.loader.exec_module(collector)
manifest = json.loads((ROOT / "tests/load-final/collection-manifest.json").read_text(encoding="utf-8"))
archive_path = ROOT / ".cache/openim38-final-load-results.tar.gz"
with archive_path.open("rb") as source:
    digest = hashlib.file_digest(source, "sha256").hexdigest()
if digest != manifest["sha256"] or archive_path.stat().st_size != manifest["bytes"]:
    raise RuntimeError("Collected archive SHA-256/size mismatch")
expected = {RUN + "/" + name for name in collector.MEMBERS}
records = {item["name"]: item for item in manifest["members"]}
if len(records) != len(manifest["members"]) or set(records) != expected:
    raise RuntimeError("Collection manifest differs from the fixed allowlist")
captured = {}
total = 0
with tarfile.open(archive_path, "r|gz") as archive:
    for member in archive:
        if member.name not in expected or member.name in captured or not member.isfile():
            raise RuntimeError("Unexpected, duplicate or non-ordinary archive member")
        if member.size < 0 or member.size > collector.MAX_FILE_BYTES:
            raise RuntimeError("Evidence member exceeds its bounded size")
        total += member.size
        if total > collector.MAX_TOTAL_BYTES:
            raise RuntimeError("Evidence archive exceeds its bounded size")
        with archive.extractfile(member) as source:
            data = source.read(collector.MAX_FILE_BYTES + 1)
        record = records[member.name]
        if len(data) != record["bytes"] or hashlib.sha256(data).hexdigest() != record["sha256"]:
            raise RuntimeError("Evidence member SHA-256/size mismatch")
        collector.scan_payload(member.name, data)
        captured[member.name] = data
if set(captured) != expected:
    raise RuntimeError("Evidence archive is incomplete")
parent = ROOT / "tests/load-final"
destination = parent / RUN
if parent.resolve() != parent or destination.exists() or destination.is_symlink():
    raise RuntimeError("Evidence target must be a new directory under the real load-final folder")
destination.mkdir()
for name, data in captured.items():
    target = parent / pathlib.PurePosixPath(name)
    if not target.resolve().is_relative_to(destination):
        raise RuntimeError("Evidence path escaped its new result directory")
    target.parent.mkdir(parents=True, exist_ok=True)
    with target.open("xb") as output:
        output.write(data)
print(json.dumps({"verifiedSHA256": digest, "extractedReports": len(captured),
                  "destination": destination.relative_to(ROOT).as_posix(), "oldEvidenceOverwritten": False}))
