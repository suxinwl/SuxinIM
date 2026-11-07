"""Pinned-host SSH operator. Password is prompted once and stays in memory."""
import argparse
import base64
import getpass
import hashlib
import json
import pathlib
import sys

import paramiko


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--host", default="192.168.10.11")
    parser.add_argument("--port", type=int, default=2223)
    parser.add_argument("--user", default="root")
    parser.add_argument("--fingerprint", default="SHA256:dK01/2cnU2tHyQtdLV2/fsz/Xmf2T1QlUPbAhS3Bwg8")
    args = parser.parse_args()

    class VerifyHost(paramiko.MissingHostKeyPolicy):
        def missing_host_key(self, client, hostname, key):
            actual = "SHA256:" + base64.b64encode(hashlib.sha256(key.asbytes()).digest()).decode().rstrip("=")
            if actual != args.fingerprint:
                raise RuntimeError("SSH host fingerprint mismatch")

    password = getpass.getpass("SSH password (not stored): ")
    client = paramiko.SSHClient()
    client.set_missing_host_key_policy(VerifyHost())
    client.connect(args.host, port=args.port, username=args.user, password=password,
                   allow_agent=False, look_for_keys=False, timeout=15)
    del password
    client.get_transport().set_keepalive(30)
    sftp = client.open_sftp()
    print(json.dumps({"ready": True, "hostVerified": True}), flush=True)
    try:
        for raw in sys.stdin:
            try:
                request = json.loads(raw)
                if request.get("op") == "request_file":
                    requests = pathlib.Path(request["path"]).read_text(encoding="utf-8-sig").splitlines()
                    index = request["index"]
                    if not isinstance(index, int) or index < 0 or index >= len(requests):
                        raise ValueError("Invalid request file index")
                    request = json.loads(requests[index])
                    if request.get("op") in ["request_file", "quit"]:
                        raise ValueError("Nested request files and indirect quit are not supported")
                operation = request.get("op", "run")
                if operation == "quit":
                    break
                if operation == "run":
                    stdin, stdout, stderr = client.exec_command(request["command"], timeout=request.get("timeout", 120))
                    stdin.channel.shutdown_write()
                    out, err = stdout.read().decode(errors="replace"), stderr.read().decode(errors="replace")
                    result = {"exit": stdout.channel.recv_exit_status(),
                              "stdout": out[-request.get("limit", 16000):], "stderr": err[-4000:]}
                elif operation == "put":
                    sftp.put(request["local"], request["remote"])
                    if "mode" in request:
                        sftp.chmod(request["remote"], int(request["mode"]))
                    result = {"uploaded": request["remote"], "bytes": pathlib.Path(request["local"]).stat().st_size}
                elif operation == "get":
                    destination = pathlib.Path(request["local"])
                    destination.parent.mkdir(parents=True, exist_ok=True)
                    sftp.get(request["remote"], str(destination))
                    result = {"downloaded": str(destination), "bytes": destination.stat().st_size}
                else:
                    raise ValueError("Unsupported operation")
                print(json.dumps(result, ensure_ascii=False), flush=True)
            except Exception as error:
                print(json.dumps({"error": type(error).__name__, "message": str(error)}), flush=True)
    finally:
        sftp.close()
        client.close()


if __name__ == "__main__":
    main()
