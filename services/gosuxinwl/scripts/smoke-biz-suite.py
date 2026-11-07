"""Run the actual worker against a disposable SQLite host, without tenant secrets.

Usage: python scripts/smoke-biz-suite.py [--package dist/biz-suite.zip]
The runtime installer can optionally be supplied with --installer PATH.
"""
import argparse
from contextlib import ExitStack
import json
import os
from pathlib import Path
import secrets
import socket
import sqlite3
import subprocess
import tempfile
import time
import urllib.error
import urllib.request
import zipfile

ROOT = Path(__file__).resolve().parents[1]


def free_port():
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]


def run(package, installer):
    with tempfile.TemporaryDirectory(prefix="biz-suite-smoke-") as directory, ExitStack() as cleanup:
        host = Path(directory)
        db_path = host / "host.db"
        db = sqlite3.connect(db_path)
        cleanup.callback(db.close)
        db.executescript((ROOT / "devsource/developer/install/sqlite.sql").read_text(encoding="utf-8-sig"))
        db.execute("INSERT OR IGNORE INTO gf_auth_role_access (uid,role_id) VALUES(1,1)")
        db.commit()
        before = {table: db.execute(f"SELECT * FROM {table}").fetchall() for table in ("gf_admin", "gf_auth_dept", "gf_auth_role", "gf_auth_role_access")}
        config = host / "manifest/config"
        config.mkdir(parents=True)
        (config / "config.yaml").write_text(
            f"server:\n  address: '127.0.0.1:{free_port()}'\ndatabase:\n  default:\n    type: sqlite\n    name: '{db_path.as_posix()}'\n    prefix: gf_\n    createdAt: createtime\n    updatedAt: updatetime\n    deletedAt: deletetime\n    debug: false\nlogger:\n  stdout: false\n",
            encoding="utf-8",
        )
        (config / "app.yaml").write_text("app:\n  superRoleAuth: false\n", encoding="utf-8")
        env = {key: value for key, value in os.environ.items() if not key.startswith(("WECOM_", "LIVEKIT_", "SUXIN_PLUGIN_", "GF_GCFG_"))}
        env["GF_GCFG_PATH"] = str(config)
        if installer:
            lock = host / "devsource/developer/install/install.lock"
            lock.parent.mkdir(parents=True)
            lock.touch()
            result = subprocess.run([str(installer), str(package)], cwd=host, env=env, capture_output=True, timeout=60)
            if result.returncode:
                log = host / "runtime/plugins/biz-suite.log"
                raise AssertionError(result.stderr.decode(errors="replace") + (log.read_text(encoding="utf-8", errors="replace") if log.exists() else ""))
            print("PASS runtime installer and managed worker health check")
        unpacked = host / "package"
        with zipfile.ZipFile(package) as archive:
            archive.extractall(unpacked)
        addr = f"127.0.0.1:{free_port()}"
        secret = secrets.token_hex(24)
        env.update(SUXIN_PLUGIN_NAME="biz-suite", SUXIN_PLUGIN_ADDR=addr, SUXIN_PLUGIN_SECRET=secret, SUXIN_PLUGIN_PACKAGE=str(unpacked))
        exe = unpacked / "bin/biz-suite.exe"

        def start():
            return subprocess.Popen([str(exe)], cwd=host, env=env, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)

        def request(path, data=None, identity=True, trusted=True):
            headers = {}
            if trusted:
                headers["X-Suxin-Plugin-Secret"] = secret
            if identity:
                headers["X-Suxin-Plugin-User"] = '{"uid":1}'
            body = None
            if data is not None:
                body = json.dumps(data).encode()
                headers["Content-Type"] = "application/json"
            req = urllib.request.Request(f"http://{addr}{path}", body, headers)
            try:
                response = urllib.request.urlopen(req, timeout=3)
            except urllib.error.HTTPError as error:
                response = error
            with response:
                return response.status, response.read()

        def api(path, data=None):
            status, body = request("/api/admin/" + path, data)
            assert status == 200, (path, status, body)
            result = json.loads(body)
            assert result["code"] == 0, (path, result)
            return result["data"]

        worker = start()
        try:
            deadline = time.monotonic() + 20
            while True:
                if worker.poll() is not None:
                    raise AssertionError(worker.stdout.read().decode(errors="replace"))
                try:
                    if request("/__plugin_health", identity=False)[0] == 200:
                        break
                except (OSError, urllib.error.URLError):
                    pass
                assert time.monotonic() < deadline, "worker did not become healthy"
                time.sleep(0.1)
            assert request("/__plugin_health", trusted=False)[0] == 403
            assert request("/api/admin/hrm/employees", identity=False)[0] == 401
            access = api("access")
            assert len(access["permissions"]) == 9
            assert [item["title"] for item in access["menus"]] == ["智慧HRM", "企业即时通讯"], access["menus"]
            for title, route in (("智慧HRM", "/runtime-plugins/biz-suite/hrm"), ("企业即时通讯", "/runtime-plugins/biz-suite/im")):
                menu = db.execute("SELECT title,onlypage,hideinmenu FROM gf_auth_rule WHERE type=1 AND routepath=?", (route,)).fetchall()
                assert menu == [(title, 1, 0)], (route, menu)
            employee = api("hrm/employees", {"name": "Smoke employee", "user_id": 1, "dept_id": 1})["id"]
            api("hrm/attendance/groups", {"name": "Day", "status": 1, "employee_ids": json.dumps([employee]), "dept_ids": "[]", "shifts": '[{"start":"09:00","end":"18:00","weekdays":[1],"break_start":"12:00","break_end":"13:00"}]', "rule_json": "{}"})
            for kind, stamp in (("in", "09:00:00"), ("out", "18:00:00")):
                api("hrm/attendance/clocks", {"employee_id": employee, "clock_type": kind, "clock_time": "2026-01-05 " + stamp})
            monthly = api("hrm/attendance/month?month=2026-01")["items"][0]
            assert monthly["worked_days"] == 1 and monthly["late_count"] == 0, monthly
            assert api("wecom/status")["configured"] is False
            assert len(api("wecom/departments/options")["departments"]) == 1
            api("im/conversations")
            for path in ("/admin/hrm/employees", "/admin/im/groups", "/admin/wecom", "/admin/assets/app.js"):
                assert request(path)[0] == 200, path
            tables = [row[0] for row in db.execute("SELECT name FROM sqlite_master WHERE type='table'")]
            assert not any(name.startswith(("biz_oa", "oa_", "act_")) for name in tables)
            assert db.execute("SELECT COUNT(*) FROM gf_auth_rule WHERE permission LIKE 'biz-suite.%'").fetchone()[0] == 11
            for table, records in before.items():
                after = db.execute(f"SELECT * FROM {table}").fetchall()
                if table == "gf_auth_role":
                    assert all(record in after for record in records), "existing roles changed"
                    assert len(after) == len(records) + 4, "staff role templates missing"
                else:
                    assert after == records, table + " changed"
            assert db.execute("SELECT COUNT(*) FROM biz_org_role").fetchone()[0] == 4
            assert api("wecom/organization/sync") == {}
            print("PASS real worker startup, host identity, RBAC/4 staff role seeds, HRM writes/month, SPA deep links, no OA, existing host identities/grants preserved")
        finally:
            worker.stdin.close()
            try:
                worker.wait(timeout=10)
            except subprocess.TimeoutExpired:
                worker.kill()
                worker.wait()
            worker.stdout.close()
        # Corrupt migration metadata and ensure there is no half-initialized service.
        db.execute("UPDATE biz_schema_version SET checksum='invalid'")
        db.commit()
        failed = start()
        try:
            failed.wait(timeout=10)
            assert failed.returncode != 0, "checksum mismatch started a healthy worker"
            assert b"schema checksum changed" in failed.stdout.read(), "unexpected startup failure"
            print("PASS schema mismatch fails before serving requests")
        finally:
            if failed.poll() is None:
                failed.kill()
                failed.wait()
            failed.stdin.close()
            failed.stdout.close()
            db.close()


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--package", type=Path, default=ROOT / "dist/biz-suite.zip")
    parser.add_argument("--installer", type=Path)
    args = parser.parse_args()
    run(args.package.resolve(), args.installer.resolve() if args.installer else None)
