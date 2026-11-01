"""Build independent official modules, without changing their module files."""
import concurrent.futures
import hashlib
import json
import os
import pathlib
import subprocess
import sys

ROOT = pathlib.Path(__file__).resolve().parents[1]
GO = ROOT / "tools/go1.25.14/bin/go.exe"

def build(component):
    directory = ROOT / "services" / component
    output = ROOT / "artifacts/linux-amd64" / component
    output.mkdir(parents=True, exist_ok=True)
    logs = ROOT / "artifacts/logs"
    logs.mkdir(parents=True, exist_ok=True)
    environment = os.environ.copy()
    environment.update(GOOS="linux", GOARCH="amd64", CGO_ENABLED="0", GOTOOLCHAIN="local",
        GOROOT=str(GO.parents[1]), GOMODCACHE=str(ROOT / ".cache/go-mod"),
        GOCACHE=str(ROOT / ".cache/go-build"), GOTMPDIR=str(ROOT / ".cache/go-tmp"),
        GOMAXPROCS=os.environ.get("GOMAXPROCS","2"), GOMEMLIMIT=os.environ.get("GOMEMLIMIT","768MiB"),
        GOPROXY="https://goproxy.cn,https://proxy.golang.org,direct")
    (ROOT / ".cache/go-tmp").mkdir(parents=True, exist_ok=True)
    args = [str(GO), "build", "-mod=readonly", "-trimpath", "-buildvcs=false", "-p="+os.environ.get("OPENIM38_BUILD_JOBS","2"), "-o", str(output)+os.sep, "./cmd/..."]
    log_path = logs / (component+"-build.log")
    with log_path.open("w",encoding="utf-8") as log:
        status = subprocess.run(args,cwd=directory,env=environment,stdout=log,stderr=subprocess.STDOUT).returncode
    result = {"component":component,"exit":status,"log":str(log_path.relative_to(ROOT)),"files":[]}
    if status == 0:
        for path in sorted(output.iterdir()):
            if path.is_file():
                with path.open("rb") as stream:
                    digest = hashlib.file_digest(stream,"sha256").hexdigest()
                result["files"].append({"path":str(path.relative_to(ROOT)),"size":path.stat().st_size,"sha256":digest})
    else:
        result["errorTail"] = log_path.read_text(encoding="utf-8",errors="replace")[-8000:]
    print(json.dumps(result,ensure_ascii=False),flush=True)
    return result

if __name__ == "__main__":
    components = sys.argv[1:] or ["openim-server","chat"]
    with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
        values = list(pool.map(build,components))
    manifest = ROOT/"artifacts/server-builds.json"
    previous = json.loads(manifest.read_text(encoding="utf-8")) if manifest.exists() else []
    merged = {item["component"]:item for item in previous}
    merged.update({item["component"]:item for item in values})
    manifest.write_text(json.dumps(list(merged.values()),indent=2)+"\n",encoding="utf-8")
    sys.exit(any(value["exit"] != 0 for value in values))
