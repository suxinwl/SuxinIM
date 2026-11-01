"""Read-only audit of the isolated stack's mounted and running executables.

Run as root on the test host; never prints container environment or credentials.
The JSON output can be compared with local artifacts by binary basename.
"""
import datetime
import hashlib
import json
import pathlib
import subprocess

ROOT = pathlib.Path("/srv/openim38-upgrade")
PROJECT = "openim38-upgrade"


def sha(path):
    with path.open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


ids = subprocess.check_output(
    ["docker", "ps", "-aq", "--filter", f"label=com.docker.compose.project={PROJECT}"],
    text=True,
).split()
if not ids:
    raise RuntimeError("The isolated Compose project is absent")
containers = json.loads(subprocess.check_output(["docker", "inspect", *ids], text=True))
records = []
for container in containers:
    labels = container["Config"].get("Labels", {})
    if labels.get("com.docker.compose.project") != PROJECT:
        raise RuntimeError("Unexpected Compose project")
    service = labels.get("com.docker.compose.service")
    for mount in container.get("Mounts", []):
        if not mount["Destination"].startswith("/app/bin/"):
            continue
        source = pathlib.Path(mount["Source"]).resolve()
        if source.parent != ROOT / "bin" or mount.get("RW"):
            raise RuntimeError("Unexpected executable mount")
        pid = container["State"].get("Pid", 0)
        running = container["State"].get("Running", False)
        records.append({
            "service": service,
            "binary": source.name,
            "running": running,
            "image": container["Config"]["Image"],
            "mountedSHA256": sha(source),
            "processSHA256": sha(pathlib.Path(f"/proc/{pid}/exe")) if running and pid else None,
        })
result = {
    "recordedAt": datetime.datetime.now(datetime.timezone.utc).isoformat(),
    "composeProject": PROJECT,
    "containers": len(containers),
    "binaries": sorted(records, key=lambda record: record["service"]),
    "stackModified": False,
}
print(json.dumps(result, indent=2))
