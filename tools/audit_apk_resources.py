"""Read-only APK ZIP/resource boundary check; no extraction or binary strings scan.

Example:
  python tools/audit_apk_resources.py --apk artifacts/android/FINAL.apk \
    --expected-sha256 <64 hex digits> --output tests/delivery/android-103-boundary.json

Only explicitly supplied APK bytes, ZIP metadata and ordinary UTF-8 resources
are inspected. .private paths are rejected, never searched or opened.
"""
from __future__ import annotations

import argparse
import datetime
import hashlib
import json
import os
import pathlib
import re
import stat
import sys
import zipfile

ROOT = pathlib.Path(__file__).resolve().parents[1]
MAX_MEMBERS = 20000
MAX_TEXT_BYTES = 8 * 1024 * 1024
MAX_TOTAL_TEXT_BYTES = 64 * 1024 * 1024
TEXT_SUFFIXES = {
    ".json", ".txt", ".xml", ".yaml", ".yml", ".html", ".htm", ".js",
    ".css", ".md", ".csv", ".properties", ".conf", ".config", ".crt",
    ".pem", ".pub", ".cer", ".dart", ".java", ".kt", ".ini",
}
PRIVATE_NAMES = re.compile(
    r"(^|/)(?:\.private|secrets\.json|(?:test[-_](?:accounts|sessions)|"
    r"final[-_]load[-_]sessions)\.json|key\.properties)(?:/|$)|"
    r"\.(?:key|jks|keystore|p12|pfx)$", re.I,
)
FIXTURE_NAMES = re.compile(
    r"(^|/)(?:fixtures?|validation|probes?)(/|$)|"
    r"(^|/)(?:validation_main|native_probe)(?:\.[^/]+)?$|"
    r"(^|/)(?:test[-_](?:accounts|sessions)|final[-_]load[-_]sessions)\.json$",
    re.I,
)
# A header literal in source is not a private key. Require a matching footer
# and an armored body, including JSON/JS escaped newlines.
PRIVATE_PEM = re.compile(
    r"-----BEGIN ((?:[A-Z0-9]+ )*PRIVATE KEY(?: BLOCK)?)-----"
    r"(?:[ \t\r\n]|\\[rn])+[A-Za-z0-9+/=]"
    r"[A-Za-z0-9+/= \t\r\n\\rn]{30,}?-----END \1-----"
)
IDENTITY_FIELDS = {"userid", "username", "account", "email", "phone", "login", "user"}
PASSWORD_FIELDS = {"password", "passwd", "pwd", "userpassword"}
TOKEN_FIELDS = {"token", "imtoken", "chattoken", "accesstoken", "refreshtoken", "idtoken"}
SECRET_FIELDS = {"apikey", "clientsecret", "secretkey", "privatekey", "signingkey"}
PLACEHOLDERS = {"", "null", "none", "undefined", "password", "token", "secret", "string", "example"}


def normalize_field(value: str) -> str:
    return re.sub(r"[^a-z0-9]", "", value.lower())


def concrete(value) -> bool:
    if not isinstance(value, str) or value.strip().lower() in PLACEHOLDERS:
        return False
    value = value.strip()
    return not (value.startswith(("${", "{{", "<")) or value.endswith(">") or set(value) <= {"*", "x", "X"})


def credential_json_fields(value, pointer="$") -> list[str]:
    """Flag credential-shaped data, not SDK method names or token label strings.

    Passwords require an identity in the same record (or a numeric secret).
    Token/secret values require concrete opaque material. Values never appear
    in the report; this is a shape check, not a private-registry comparison.
    """
    found = []
    if isinstance(value, dict):
        normalized = {normalize_field(str(key)): item for key, item in value.items()}
        identified = any(concrete(normalized.get(key)) for key in IDENTITY_FIELDS)
        for key, item in value.items():
            field = normalize_field(str(key))
            location = pointer + "/" + str(key).replace("~", "~0").replace("/", "~1")
            if concrete(item):
                secret = item.strip()
                if field in PASSWORD_FIELDS and (identified or secret.isdecimal()):
                    found.append(location)
                elif field in TOKEN_FIELDS and (
                    re.fullmatch(r"[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+", secret)
                    or len(secret) >= 24 and bool(re.search(r"[0-9_+/=-]", secret))
                ):
                    found.append(location)
                elif field in SECRET_FIELDS and len(secret) >= 16:
                    found.append(location)
            found.extend(credential_json_fields(item, location))
    elif isinstance(value, list):
        for index, item in enumerate(value):
            found.extend(credential_json_fields(item, f"{pointer}/{index}"))
    return found


def checked_path(path: pathlib.Path) -> pathlib.Path:
    # Resolve metadata only; refuse both a literal .private path and a symlink
    # resolving into one before any file contents are opened.
    absolute = path.absolute()
    if any(part.lower() == ".private" for part in absolute.parts):
        raise ValueError("Private directory paths are outside audit scope")
    resolved = absolute.resolve()
    if any(part.lower() == ".private" for part in resolved.parts):
        raise ValueError("Resolved private directory paths are outside audit scope")
    if absolute.is_symlink() or not stat.S_ISREG(absolute.lstat().st_mode):
        raise ValueError("APK must be a regular non-symlink file")
    return resolved


def identity(value) -> tuple:
    return value.st_dev, value.st_ino, value.st_size, value.st_mtime_ns


def scan_apk(path: pathlib.Path, expected_sha256: str) -> dict:
    if not re.fullmatch(r"[0-9a-fA-F]{64}", expected_sha256):
        raise ValueError("Expected SHA-256 must contain exactly 64 hex digits")
    path = checked_path(path)
    if path.suffix.lower() != ".apk":
        raise ValueError("Only an explicitly named .apk is supported")
    result = {
        "artifact": str(path.relative_to(ROOT)).replace("\\", "/") if path.is_relative_to(ROOT) else str(path),
        "expectedSha256": expected_sha256.lower(), "sha256": None, "sha256Matches": False,
        "bytes": 0, "zipEntries": 0, "readableTextResourcesChecked": 0,
        "binaryXmlOrNonUtf8ResourcesSkipped": 0, "compiledOrNonTextEntriesSkipped": 0,
        "readableTextBytesChecked": 0, "privateMaterialFlags": [],
        "validationOrProbeFixtureEntryNames": [], "errors": [],
    }
    initial = path.stat()
    with path.open("rb") as source:
        if identity(initial) != identity(os.fstat(source.fileno())):
            raise OSError("APK changed before audit")
        result["bytes"] = initial.st_size
        result["sha256"] = hashlib.file_digest(source, "sha256").hexdigest()
        result["sha256Matches"] = result["sha256"] == expected_sha256.lower()
        if not result["sha256Matches"]:
            result["errors"].append({"rule": "expected SHA-256 mismatch; resources not inspected"})
        else:
            source.seek(0)
            with zipfile.ZipFile(source) as archive:
                entries = archive.infolist()
                result["zipEntries"] = len(entries)
                if len(entries) > MAX_MEMBERS:
                    raise ValueError("APK ZIP entry limit exceeded")
                seen = set()
                for entry in entries:
                    name = entry.filename.replace("\\", "/")
                    parts = pathlib.PurePosixPath(name).parts
                    mode = entry.external_attr >> 16
                    flags = result["privateMaterialFlags"]
                    if name in seen:
                        result["errors"].append({"path": name, "rule": "duplicate ZIP entry"})
                    seen.add(name)
                    if name.startswith("/") or ".." in parts or "\x00" in name or re.match(r"^[A-Za-z]:", name):
                        flags.append({"path": name, "rule": "unsafe ZIP entry path"})
                        continue
                    if stat.S_ISLNK(mode) or stat.S_IFMT(mode) not in (0, stat.S_IFREG, stat.S_IFDIR):
                        flags.append({"path": name, "rule": "non-regular ZIP entry"})
                        continue
                    if PRIVATE_NAMES.search(name):
                        flags.append({"path": name, "rule": "private material entry name; contents not read"})
                        continue
                    if FIXTURE_NAMES.search(name):
                        result["validationOrProbeFixtureEntryNames"].append(name)
                        continue
                    if entry.is_dir():
                        continue
                    suffix = pathlib.PurePosixPath(name).suffix.lower()
                    basename = pathlib.PurePosixPath(name).name.upper()
                    if suffix not in TEXT_SUFFIXES and basename not in ("LICENSE", "NOTICE", "README"):
                        result["compiledOrNonTextEntriesSkipped"] += 1
                        continue
                    if entry.flag_bits & 1:
                        result["errors"].append({"path": name, "rule": "encrypted readable resource not inspected"})
                        continue
                    if entry.file_size > MAX_TEXT_BYTES or result["readableTextBytesChecked"] + entry.file_size > MAX_TOTAL_TEXT_BYTES:
                        result["errors"].append({"path": name, "rule": "readable resource size limit exceeded"})
                        continue
                    with archive.open(entry) as member:
                        raw = member.read(MAX_TEXT_BYTES + 1)
                    if len(raw) != entry.file_size:
                        raise ValueError("ZIP resource size did not match metadata")
                    try:
                        text = raw.decode("utf-8-sig")
                        if re.search(r"[\x00-\x08\x0b\x0c\x0e-\x1f]", text):
                            raise UnicodeError("binary resource")
                    except UnicodeError:
                        result["binaryXmlOrNonUtf8ResourcesSkipped"] += 1
                        continue
                    result["readableTextResourcesChecked"] += 1
                    result["readableTextBytesChecked"] += len(raw)
                    if PRIVATE_PEM.search(text):
                        flags.append({"path": name, "rule": "armored private key body"})
                    if suffix == ".json":
                        try:
                            payload = json.loads(text)
                        except ValueError:
                            result["errors"].append({"path": name, "rule": "unparseable JSON resource"})
                        else:
                            for pointer in credential_json_fields(payload):
                                flags.append({"path": name, "rule": "credential-shaped JSON value", "field": pointer})
        if identity(initial) != identity(os.fstat(source.fileno())) or identity(initial) != identity(path.stat()):
            raise OSError("APK changed during audit; rerun after packaging finishes")
    result["zipAndReadableResourcesPassed"] = bool(result["sha256Matches"] and not result["privateMaterialFlags"]
        and not result["validationOrProbeFixtureEntryNames"] and not result["errors"])
    return result


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--apk", required=True, type=pathlib.Path)
    parser.add_argument("--expected-sha256", required=True)
    parser.add_argument("--output", type=pathlib.Path, help="Optional new machine-readable report; existing reports are never overwritten")
    args = parser.parse_args()
    report = {
        "recordedAt": datetime.datetime.now(datetime.timezone.utc).isoformat(),
        "tool": "tools/audit_apk_resources.py", "privateDirectoryRead": False,
        "archiveModifiedOrExtracted": False,
        "limits": [
            "Only the explicitly named APK ZIP metadata and ordinary UTF-8 resources are inspected; no .private directory reads.",
            "DEX, libapp.so/native libraries, binary XML, nested archives and arbitrary binary strings are not scanned.",
            "Credential-shaped JSON checks use record/field/value structure, not token words in source, schemas or labels; no comparison with private credentials.",
            "The development openim38/native_probe runtime channel identifier alone is not a probe fixture finding; compiled code is outside this audit.",
            "No whole-NSIS, device runtime, update-install, signing identity or full secret-free conclusion is made.",
        ],
    }
    try:
        report.update(scan_apk(args.apk, args.expected_sha256))
        if args.output is not None:
            output = args.output.absolute()
            if any(part.lower() == ".private" for part in output.parts + output.resolve().parts):
                raise ValueError("Report cannot be written into a private directory")
            if output.resolve() == args.apk.resolve():
                raise ValueError("Report cannot replace the APK")
            output.parent.mkdir(parents=True, exist_ok=True)
            # Exclusive creation preserves historical reports and package bytes.
            with output.open("x", encoding="utf-8") as target:
                target.write(json.dumps(report, indent=2) + "\n")
    except (OSError, ValueError, zipfile.BadZipFile, RuntimeError):
        # Avoid echoing exception strings which can include member contents.
        print(json.dumps({"tool": report["tool"], "passed": False,
                          "error": "Audit or exclusive report write failed; verify APK, SHA, path and new output name"}))
        return 2
    print(json.dumps(report, indent=2))
    return 0 if report["zipAndReadableResourcesPassed"] else 1


if __name__ == "__main__":
    sys.exit(main())
