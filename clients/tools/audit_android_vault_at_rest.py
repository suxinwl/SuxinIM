"""Read-only synthetic-marker audit of the controlled API30 test emulator.

Never emits paths, source content, account IDs, or search markers. Run only
after the parent-controlled normal Main acceptance has authorized its fixture.
ADB root must already have been enabled separately for emulator-5582.
"""
from __future__ import annotations

import argparse
import hashlib
import json
import pathlib
import re
import shlex
import subprocess
import sys
import threading

SERIAL = "emulator-5582"
PACKAGE = "/data/user/0/com.szputy.im"
MAX_FILES = 30000
MAX_BYTES = 2 * 1024 * 1024 * 1024


def adb_run(adb: str, command: str) -> bytes:
    # Shell v2 carries the remote exit status. exec-out may report local exit 0
    # for a remote failure and must not turn a failed read into an absence claim.
    completed = subprocess.run([adb, "-s", SERIAL, "shell", "-T", "sh", "-c", shlex.quote(command)], capture_output=True, timeout=15)
    if completed.returncode:
        raise RuntimeError("Controlled app read failed")
    return completed.stdout


def audit(args: argparse.Namespace) -> dict:
    fixture = json.loads(pathlib.Path(args.input).read_text(encoding="utf-8-sig"))
    approved_sha = hashlib.sha256(pathlib.Path(args.approved_fixture).read_bytes()).hexdigest()
    if approved_sha != args.expected_fixture_sha256 or fixture.get("sourceFixtureSHA256") != approved_sha:
        raise ValueError("Approved fixture bytes did not match the parent pin")
    account = fixture["userID"]
    if not isinstance(account, str) or not re.fullmatch(r"emp_[0-9a-f]{32}", account):
        raise ValueError("Exact synthetic account is required")
    if len(set(args.allowed_user_id)) != 2 or account not in args.allowed_user_id or any(not re.fullmatch(r"emp_[0-9a-f]{32}", item) for item in args.allowed_user_id):
        raise ValueError("Account is outside the parent's exact I/J allowlist")
    if fixture.get("namespace") != "local-store-117-main":
        raise ValueError("Wrong isolated acceptance namespace")
    markers = fixture["markers"]
    if not isinstance(markers, list) or not markers or len(markers) > 20:
        raise ValueError("Bounded synthetic marker list required")
    if any(not isinstance(item, str) or len(item) < 24 or len(item.encode("utf-8")) > 8192 for item in markers):
        raise ValueError("Invalid synthetic marker")
    needles = [item.encode("utf-8") for item in markers]
    overlap = max(map(len, needles)) - 1
    expected_plain_sha = fixture.get("plaintextFileSHA256", [])
    if any(not isinstance(item, str) or not re.fullmatch(r"[0-9a-f]{64}", item) for item in expected_plain_sha):
        raise ValueError("Invalid synthetic file digest")
    if adb_run(args.adb, "id -u").strip() != b"0":
        raise RuntimeError("Read-only audit requires separately authorized root on the fixed emulator")
    def assert_stopped() -> None:
        processes = adb_run(args.adb, "ps -A -o NAME").decode("utf-8", errors="strict").splitlines()
        if any(item.strip() == "com.szputy.im" or item.strip().startswith("com.szputy.im:") for item in processes):
            raise RuntimeError("Normal app must be stopped separately before this read-only snapshot")
    assert_stopped()
    account_root = PACKAGE + "/no_backup/puty-local/accounts/" + hashlib.sha256(account.encode()).hexdigest()
    roots = [account_root, PACKAGE + "/app_flutter", PACKAGE + "/shared_prefs", PACKAGE + "/cache",
             "/storage/emulated/0/Android/data/com.szputy.im/files",
             "/storage/emulated/0/Android/data/com.szputy.im/cache"]
    files: set[str] = set()
    absent_roots = 0
    for root in roots:
        quoted = shlex.quote(root)
        listing = adb_run(args.adb, f"if [ -L {quoted} ]; then exit 98; fi; if [ -d {quoted} ]; then find {quoted} -type f -print0; else printf MISSING; fi")
        if listing == b"MISSING":
            absent_roots += 1
            continue
        for raw in listing.split(b"\0"):
            if not raw:
                continue
            path = raw.decode("utf-8", errors="strict")
            if not path.startswith(root + "/") or "\x00" in path:
                raise RuntimeError("Managed path boundary rejected")
            files.add(path)
    if account_root not in roots or not any(path.startswith(account_root + "/") for path in files):
        raise RuntimeError("Selected account vault has no auditable files")
    if len(files) > MAX_FILES:
        raise RuntimeError("Managed audit file bound exceeded")
    marker_file_matches = 0
    plaintext_file_matches = 0
    sqlite_headers = 0
    encrypted_objects = 0
    total = 0
    for path in sorted(files):
        # No source names enter the report or subprocess output. Paths remain
        # shell-quoted, and find does not follow directory symlinks.
        command = "test ! -L " + shlex.quote(path) + " && cat " + shlex.quote(path)
        identity_command = "test ! -L " + shlex.quote(path) + " && stat -c '%d|%i|%s|%y|%z' " + shlex.quote(path)
        before = adb_run(args.adb, identity_command)
        process = subprocess.Popen([args.adb, "-s", SERIAL, "exec-out", "sh", "-c", command], stdout=subprocess.PIPE, stderr=subprocess.DEVNULL)
        def kill_if_live() -> None:
            try:
                if process.poll() is None:
                    process.kill()
            except OSError:
                pass
        timeout = threading.Timer(60, kill_if_live)
        timeout.daemon = True
        timeout.start()
        assert process.stdout is not None
        digest = hashlib.sha256()
        tail = b""
        matched = False
        first = True
        try:
            while chunk := process.stdout.read(65536):
                total += len(chunk)
                if total > MAX_BYTES:
                    raise RuntimeError("Managed audit byte bound exceeded")
                if first:
                    sqlite_headers += int(chunk.startswith(b"SQLite format 3\0"))
                    encrypted_objects += int(chunk.startswith(b"PUTYLS1\n"))
                    first = False
                digest.update(chunk)
                window = tail + chunk
                matched = matched or any(needle in window for needle in needles)
                tail = window[-overlap:] if overlap else b""
            if process.wait() != 0:
                raise RuntimeError("Managed file changed or became unreadable during audit")
            if adb_run(args.adb, identity_command) != before:
                raise RuntimeError("Managed file identity changed during audit")
            marker_file_matches += int(matched)
            plaintext_file_matches += int(digest.hexdigest() in expected_plain_sha)
        finally:
            timeout.cancel()
            process.stdout.close()
            if process.poll() is None:
                process.kill()
                process.wait()
    assert_stopped()
    return {"format": "PUTY-ANDROID-AT-REST-SYNTHETIC-1", "device": SERIAL,
            "deviceKind": "API30-x86_64-emulator", "readOnly": True,
            "sourceFixtureSHA256": approved_sha,
            "appStoppedBeforeAndAfter": True, "perFileIdentityStable": True,
            "otherAccountVaultTreesRead": False, "systemTrustStoreModified": False,
            "filesRead": len(files), "bytesRead": total, "absentOptionalRoots": absent_roots,
            "syntheticMarkerFileMatches": marker_file_matches,
            "exactSyntheticPlaintextFileMatches": plaintext_file_matches,
            "plaintextSQLiteHeadersObserved": sqlite_headers,
            "authenticatedFormatMagicObjectsObserved": encrypted_objects,
            "syntheticPlaintextAbsent": marker_file_matches == 0 and plaintext_file_matches == 0,
            "scope": "Synthetic markers and exact synthetic file hashes in one account vault and app-managed shared/cache roots only; no broad secret-free or hardware claim"}


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--adb", default="E:/AndroidDev/sdk/platform-tools/adb.exe")
    parser.add_argument("--input", required=True)
    parser.add_argument("--output", required=True)
    parser.add_argument("--approved-fixture", required=True)
    parser.add_argument("--expected-fixture-sha256", required=True)
    parser.add_argument("--allowed-user-id", action="append", required=True)
    arguments = parser.parse_args()
    try:
        result = audit(arguments)
        pathlib.Path(arguments.output).write_text(json.dumps(result, indent=2) + "\n", encoding="utf-8")
        print(json.dumps(result))
    except Exception:
        print(json.dumps({"error": "ANDROID_AT_REST_AUDIT_FAILED", "passed": False}))
        sys.exit(1)
