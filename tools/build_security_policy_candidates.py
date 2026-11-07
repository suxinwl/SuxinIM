"""Build the isolated 17-service candidate; never changes production manifests."""
import hashlib
import json
import os
import pathlib
import subprocess
import sys

ROOT = pathlib.Path(__file__).resolve().parents[1]
GO = ROOT / "tools/go1.25.14/bin/go.exe"
OUTPUT = ROOT / "artifacts/security-policy-store-fix"
PACKAGES = {
    "openim-server": ["./cmd/" + name for name in ["openim-api", "openim-crontask", "openim-msggateway", "openim-msgtransfer", "openim-push"]] + ["./cmd/openim-rpc/" + name for name in ["openim-rpc-auth", "openim-rpc-conversation", "openim-rpc-friend", "openim-rpc-group", "openim-rpc-msg", "openim-rpc-third", "openim-rpc-user"]],
    "chat": ["./cmd/api/admin-api", "./cmd/api/chat-api", "./cmd/rpc/admin-rpc", "./cmd/rpc/chat-rpc"],
    "enterprise-service": ["./cmd/enterprise-service"],
}


def sources():
    values = []
    for component in ["identity", *PACKAGES]:
        directory = ROOT / "services" / component
        for path in sorted(directory.rglob("*")):
            if path.is_file() and (path.suffix in {".go", ".mod", ".sum", ".yml", ".yaml"}):
                if path.is_symlink():
                    raise RuntimeError("Candidate source contains a link")
                values.append({"path": path.relative_to(ROOT).as_posix(), "sha256": hashlib.sha256(path.read_bytes()).hexdigest()})
    return values


def main():
    environment = os.environ.copy()
    environment.update(GOOS="linux", GOARCH="amd64", CGO_ENABLED="0", GOTOOLCHAIN="local", GOWORK="off", GOROOT=str(GO.parents[1]), GOMODCACHE=str(ROOT / ".cache/go-mod"), GOCACHE=str(ROOT / ".cache/go-build"), GOTMPDIR=str(ROOT / ".cache/go-tmp"), GOMAXPROCS="2", GOMEMLIMIT="1GiB", GOPROXY="off", GOSUMDB="off")
    (ROOT / ".cache/go-tmp").mkdir(parents=True, exist_ok=True)
    OUTPUT.mkdir(parents=True, exist_ok=True)
    before = sources()
    report = {"productionChanged": False, "sourceFiles": before, "toolchain": "go1.25.14", "platform": "linux/amd64", "components": [], "passed": False}
    for component, packages in PACKAGES.items():
        directory = OUTPUT / "linux-amd64" / component
        directory.mkdir(parents=True, exist_ok=True)
        command = [str(GO), "build", "-mod=readonly", "-trimpath", "-buildvcs=false", "-p=1", "-ldflags=-s -w", "-o", str(directory) + os.sep, *packages]
        log = OUTPUT / (component + "-build.log")
        with log.open("w", encoding="utf-8") as stream:
            code = subprocess.run(command, cwd=ROOT / "services" / component, env=environment, stdout=stream, stderr=subprocess.STDOUT).returncode
        row = {"component": component, "exitCode": code, "log": log.relative_to(ROOT).as_posix(), "files": []}
        if code == 0:
            for package in packages:
                path = directory / package.rsplit("/", 1)[1]
                with path.open("rb") as stream:
                    digest = hashlib.file_digest(stream, "sha256").hexdigest()
                row["files"].append({"path": path.relative_to(ROOT).as_posix(), "sha256": digest, "size": path.stat().st_size})
        report["components"].append(row)
        print(json.dumps({"component": component, "exitCode": code, "binaryCount": len(row["files"])}), flush=True)
        if code:
            break
    report["sourceUnchangedDuringBuild"] = sources() == before
    report["passed"] = report["sourceUnchangedDuringBuild"] and len(report["components"]) == 3 and all(row["exitCode"] == 0 for row in report["components"]) and sum(len(row["files"]) for row in report["components"]) == 17
    (OUTPUT / "build-manifest.json").write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")
    return 0 if report["passed"] else 1


if __name__ == "__main__":
    sys.exit(main())
