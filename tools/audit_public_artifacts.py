"""Audit only named delivery archives; never reads .private or extracts archives.

Run with project Python: python tools/audit_public_artifacts.py
The report records the exact inspected archives, not a claim about future builds.
"""
from __future__ import annotations

import argparse
import datetime
import hashlib
import json
import pathlib
import re
import tarfile
import zipfile

ROOT = pathlib.Path(__file__).resolve().parents[1]
APPROVED_ARCHIVES = (
    "admin-web-final.tar.gz",
    "admin-web.tar.gz",
    "load-bundle.tar.gz",
    "OpenIM38-Development-CA.zip",
    "server-binaries.tar.gz",
    "web-admin.tar.gz",
    "web-client-final.tar.gz",
)
PRIVATE_NAMES = re.compile(
    r"(^|/)(?:\.private|secrets\.json|test-accounts\.json|test-sessions\.json|"
    r"key\.properties)(?:/|$)|\.(?:key|jks|keystore|p12|pfx)$",
    re.IGNORECASE,
)
# Require an actual armored body, so library code mentioning a PEM header is
# not misreported as an embedded private key. Accept JSON/JS escaped newlines.
PRIVATE_PEM = re.compile(
    rb"-----BEGIN ((?:[A-Z0-9]+ )*PRIVATE KEY(?: BLOCK)?)-----"
    rb"(?:[ \t\r\n]|\\[rn])+[A-Za-z0-9+/=]"
    rb"[A-Za-z0-9+/= \t\r\n\\rn]{30,}?"
    rb"-----END \1-----"
)


def private_path(name: str) -> bool:
    return bool(PRIVATE_NAMES.search(name.replace("\\", "/")))


def contains_private_key(stream) -> bool:
    tail = b""
    while chunk := stream.read(1024 * 1024):
        data = tail + chunk
        if PRIVATE_PEM.search(data):
            return True
        # Ordinary RSA/OpenSSH/EC private keys fit within this overlap, including
        # escaped line breaks. Streaming avoids loading application binaries.
        tail = data[-65536:]
    return False


def scan_archive(path: pathlib.Path) -> dict:
    if path.name not in APPROVED_ARCHIVES:
        raise ValueError("Archive name is outside the explicit delivery allowlist")
    initial = path.stat()
    with path.open("rb") as stream:
        digest = hashlib.file_digest(stream, "sha256").hexdigest()
    flags = []
    names = []
    files_checked = 0

    def inspect(name, stream=None, target=None):
        nonlocal files_checked
        names.append(name)
        if private_path(name) or target is not None and private_path(target):
            flags.append({"path": name, "rule": "private filename or link target"})
            return
        if stream is not None:
            files_checked += 1
            if contains_private_key(stream):
                flags.append({"path": name, "rule": "armored private key body"})

    if path.suffix.lower() == ".zip":
        with zipfile.ZipFile(path) as archive:
            for member in archive.infolist():
                if member.is_dir():
                    inspect(member.filename)
                else:
                    with archive.open(member) as stream:
                        inspect(member.filename, stream)
    else:
        with tarfile.open(path, "r:gz") as archive:
            for member in archive:
                if member.isfile():
                    with archive.extractfile(member) as stream:
                        inspect(member.name, stream)
                else:
                    inspect(member.name, target=member.linkname if member.issym() or member.islnk() else None)
    final = path.stat()
    identity = lambda value: (value.st_dev, value.st_ino, value.st_size, value.st_mtime_ns)
    if identity(initial) != identity(final):
        raise OSError("Archive changed during audit; rerun after packaging finishes")
    result = {
        "archive": path.name,
        "bytes": final.st_size,
        "sha256": digest,
        "members": len(names),
        "memberFilesChecked": files_checked,
        "privateMaterialFlags": flags,
        "passed": not flags,
    }
    if path.name == "OpenIM38-Development-CA.zip":
        result["names"] = names
    return result


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--archive", choices=APPROVED_ARCHIVES, action="append",
                        help="Check selected approved names; default checks all present approved archives")
    args = parser.parse_args()
    selected = args.archive or APPROVED_ARCHIVES
    artifact_root = ROOT / "artifacts"
    results, absent, errors = [], [], []
    for name in dict.fromkeys(selected):
        path = artifact_root / name
        if not path.exists():
            absent.append(name)
            continue
        # Do not follow an artifact symlink into .private or any external path.
        if path.resolve().parent != artifact_root.resolve():
            errors.append({"archive": name, "error": "Archive resolves outside artifacts directory"})
            continue
        try:
            results.append(scan_archive(path))
        except (OSError, ValueError, zipfile.BadZipFile, tarfile.TarError) as error:
            errors.append({"archive": name, "error": type(error).__name__})
    report = {
        "recordedAt": datetime.datetime.now(datetime.timezone.utc).isoformat(),
        "tool": "tools/audit_public_artifacts.py",
        "approvedArchiveNames": list(APPROVED_ARCHIVES),
        "publicCandidateArchives": results,
        "absentApprovedArchives": absent,
        "errors": errors,
        "passed": bool(results) and not errors and all(item["passed"] for item in results),
        "privateDirectoryRead": False,
        "archivesExtractedOrModified": False,
        "limits": [
            "Only the listed archive bytes and members were inspected; rerun after rebuilding a package.",
            "Checks private filenames and armored private-key bodies; does not compare contents with any private credential registry.",
            "This is not a scan of APK/EXE packages, source folders, arbitrary secrets, or unlisted archives.",
        ],
    }
    output = ROOT / "tests/delivery/archive-privacy.json"
    output.parent.mkdir(parents=True, exist_ok=True)
    temporary = output.with_suffix(".json.tmp")
    temporary.write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")
    temporary.replace(output)
    print(json.dumps({"report": output.relative_to(ROOT).as_posix(), "archives": len(results),
                      "passed": report["passed"], "errors": len(errors),
                      "privateMaterialFlags": sum(len(item["privateMaterialFlags"]) for item in results)}))
    return 0 if report["passed"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
