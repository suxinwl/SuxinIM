"""Install project-local, checksum-verified build runtimes."""
import concurrent.futures
import hashlib
import json
import pathlib
import shutil
import urllib.request
import zipfile

ROOT = pathlib.Path(__file__).resolve().parents[1]

def get(url):
    return urllib.request.urlopen(urllib.request.Request(url, headers={"User-Agent":"OpenIM38-build-bootstrap"}), timeout=90)

def install_go():
    with get("https://go.dev/dl/?mode=json&include=all") as stream:
        release = next(item for item in json.load(stream) if item["version"] == "go1.25.14")
    asset = next(item for item in release["files"] if item["os"] == "windows" and item["arch"] == "amd64" and item["kind"] == "archive")
    return download_extract("https://go.dev/dl/"+asset["filename"], asset["sha256"], "go1.25.14", "go")

def install_node():
    filename = "node-v22.23.4-win-x64.zip"
    with get("https://nodejs.org/dist/v22.23.4/SHASUMS256.txt") as stream:
        sha = next(line.split()[0] for line in stream.read().decode().splitlines() if line.endswith(filename))
    return download_extract("https://nodejs.org/dist/v22.23.4/"+filename, sha, "node-v22.23.4-win-x64", "node-v22.23.4-win-x64")

def download_extract(url, sha, destination, prefix):
    target = ROOT / "tools" / destination
    marker = target / "runtime.lock.json"
    if marker.exists():
        return json.loads(marker.read_text())
    archive = ROOT / ".cache" / "downloads" / url.rsplit("/",1)[1]
    archive.parent.mkdir(parents=True, exist_ok=True)
    if not archive.exists():
        with get(url) as source, archive.with_suffix(".partial").open("wb") as output:
            shutil.copyfileobj(source, output)
        archive.with_suffix(".partial").replace(archive)
    with archive.open("rb") as stream:
        digest = hashlib.file_digest(stream, "sha256").hexdigest()
    if digest != sha:
        raise RuntimeError(f"Runtime checksum mismatch: {archive}")
    target.mkdir(parents=True, exist_ok=True)
    with zipfile.ZipFile(archive) as package:
        for item in package.infolist():
            parts = pathlib.PurePosixPath(item.filename).parts
            if not parts or parts[0] != prefix:
                raise RuntimeError("Unexpected runtime archive layout")
            relative = parts[1:]
            if not relative:
                continue
            path = target.joinpath(*relative).resolve()
            if not path.is_relative_to(target.resolve()):
                raise RuntimeError("Runtime archive path escape")
            if item.is_dir():
                path.mkdir(parents=True, exist_ok=True)
            else:
                path.parent.mkdir(parents=True, exist_ok=True)
                with package.open(item) as source, path.open("wb") as output:
                    shutil.copyfileobj(source, output)
    value = {"url":url,"sha256":sha,"path":str(target.relative_to(ROOT))}
    marker.write_text(json.dumps(value, indent=2)+"\n")
    print(json.dumps({"installed":destination}), flush=True)
    return value

if __name__ == "__main__":
    with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
        futures = [pool.submit(install_go), pool.submit(install_node)]
        values = [future.result() for future in futures]
    (ROOT / "runtimes.lock.json").write_text(json.dumps(values, indent=2)+"\n")
