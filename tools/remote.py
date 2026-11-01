"""Persistent SSH/SFTP operator. Credentials remain in process memory only."""
import base64
import getpass
import hashlib
import json
import pathlib
import shlex
import sys

sys.path.insert(0, str(pathlib.Path(__file__).parent / "python-lib"))
import paramiko

EXPECTED = "SHA256:C1k9PwEvAcC8w/eQE86prgm9KCw9S5/VbDqMKlbNOTo"

class VerifyHost(paramiko.MissingHostKeyPolicy):
    def missing_host_key(self, client, hostname, key):
        actual = "SHA256:" + base64.b64encode(hashlib.sha256(key.asbytes()).digest()).decode().rstrip("=")
        if actual != EXPECTED:
            raise RuntimeError("SSH host fingerprint mismatch")

password = getpass.getpass("SSH password (not stored): ")
ssh = paramiko.SSHClient()
ssh.set_missing_host_key_policy(VerifyHost())
ssh.connect("192.168.10.11", port=2223, username="suxin", password=password,
            allow_agent=False, look_for_keys=False, timeout=15)
ssh.get_transport().set_keepalive(30)
sftp = ssh.open_sftp()
print(json.dumps({"ready": True, "hostVerified": True}), flush=True)
for raw in sys.stdin:
    try:
        req = json.loads(raw)
        if req["op"] == "run":
            command = req["command"]
            if req.get("sudo"):
                command = "sudo -S -p '' -- sh -c " + shlex.quote(command)
            stdin, stdout, stderr = ssh.exec_command(command, timeout=req.get("timeout", 120))
            if req.get("sudo"):
                stdin.write(password + "\n")
                stdin.flush()
            stdin.channel.shutdown_write()
            out, err = stdout.read().decode(errors="replace"), stderr.read().decode(errors="replace")
            status = stdout.channel.recv_exit_status()
            result = {"exit": status, "stdout": out[-req.get("limit", 16000):], "stderr": err[-8000:]}
        elif req["op"] == "put":
            sftp.put(req["local"], req["remote"])
            result = {"uploaded": req["remote"]}
        elif req["op"] == "get":
            pathlib.Path(req["local"]).parent.mkdir(parents=True, exist_ok=True)
            sftp.get(req["remote"], req["local"])
            result = {"downloaded": req["local"]}
        elif req["op"] == "quit":
            break
        else:
            raise ValueError("Unknown operation")
        print(json.dumps(result, ensure_ascii=False), flush=True)
    except Exception as exc:
        print(json.dumps({"error": str(exc)}), flush=True)
sftp.close()
ssh.close()
