"""Fetch immutable upstream sources; never alters the original 3.4/3.5 trees."""
from __future__ import annotations
import concurrent.futures
import hashlib
import json
import os
import pathlib
import shutil
import tarfile
import time
import urllib.request

ROOT = pathlib.Path(__file__).resolve().parents[1]
SOURCES = [
    ("openimsdk/open-im-server", "v3.8.3-patch.18", "services/openim-server", "120cee6d5783959a02ea1acfa414589fe5aae90f"),
    ("openimsdk/chat", "v1.8.4-patch.5", "services/chat", "22dc25e296aef327fb6779079c2a8b5b8fce83db"),
    ("openimsdk/openim-sdk-js-wasm", "v3.8.3-patch.15.1", "sdks/wasm", None),
    ("openimsdk/open-im-sdk-android", "v3.8.3.5", "sdks/android-java", None),
    ("openimsdk/openim-sdk-core-ios", "3.8.3-hotfix.15", "sdks/ios-core", None),
    ("openimsdk/open-im-sdk-flutter", "3.8.3+hotfix.15", "sdks/flutter", "1b64cba2bdeace5e858b236b5e1b3deedd2760a0"),
]

def request(url: str):
    return urllib.request.urlopen(urllib.request.Request(url, headers={"User-Agent": "OpenIM38-source-bootstrap", "Accept": "application/vnd.github+json"}), timeout=90)

def fetch(item):
    repo, tag, destination, expected = item
    target = ROOT / destination
    marker = target / ".upstream.json"
    if marker.exists():
        value = json.loads(marker.read_text(encoding="utf-8"))
        if value["tag"] != tag or (expected and value["commit"] != expected):
            raise RuntimeError(f"Existing source identity differs: {target}")
        return value
    if target.exists() and any(target.iterdir()):
        raise RuntimeError(f"Refusing to overwrite populated target: {target}")
    with request(f"https://api.github.com/repos/{repo}/commits/{tag}") as response:
        commit = json.load(response)["sha"]
    if expected and expected != commit:
        raise RuntimeError(f"Tag changed for {repo}: {commit}")
    downloads = ROOT / ".cache" / "downloads"
    downloads.mkdir(parents=True, exist_ok=True)
    archive = downloads / (repo.rsplit("/", 1)[1] + "-" + commit + ".tar.gz")
    url = f"https://codeload.github.com/{repo}/tar.gz/{commit}"
    if not archive.exists():
        temporary = archive.with_suffix(".partial")
        for attempt in range(4):
            try:
                with request(url) as response, temporary.open("wb") as output:
                    shutil.copyfileobj(response, output)
                temporary.replace(archive)
                break
            except Exception:
                if attempt == 3:
                    raise
                time.sleep(attempt + 1)
    digest = hashlib.file_digest(archive.open("rb"), "sha256").hexdigest()
    stage = ROOT / ".cache" / "source-stage" / repo.rsplit("/", 1)[1]
    stage.mkdir(parents=True, exist_ok=True)
    def native(path):
        return pathlib.Path("\\\\?\\" + str(path.resolve())) if os.name == "nt" else path
    links = []
    with tarfile.open(archive, "r:gz") as tar:
        for member in tar.getmembers():
            parts = pathlib.PurePosixPath(member.name).parts[1:]
            if not parts:
                continue
            resolved = stage.joinpath(*parts).resolve()
            if not resolved.is_relative_to(stage.resolve()):
                raise RuntimeError("Archive path escapes staging directory")
            destination_path = native(resolved)
            if member.isdir():
                destination_path.mkdir(parents=True, exist_ok=True)
            elif member.isfile():
                destination_path.parent.mkdir(parents=True, exist_ok=True)
                with tar.extractfile(member) as source, destination_path.open("wb") as output:
                    shutil.copyfileobj(source, output)
                destination_path.chmod(member.mode)
            elif member.issym():
                source = (resolved.parent / member.linkname).resolve()
                if not source.is_relative_to(stage.resolve()):
                    raise RuntimeError("Archive symlink escapes staging directory")
                links.append((source, resolved))
            else:
                raise RuntimeError("Unsupported archive member")
    for source, destination_path in links:
        shutil.copy2(native(source), native(destination_path))
    target.parent.mkdir(parents=True, exist_ok=True)
    if target.exists():
        target.rmdir()  # only the previously checked empty target
    shutil.move(str(stage), str(target))
    value = {"repository": f"https://github.com/{repo}", "tag": tag, "commit": commit,
             "path": destination, "archiveUrl": url, "archiveSha256": digest}
    marker.write_text(json.dumps(value, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({"imported": destination, "commit": commit}), flush=True)
    return value

if __name__ == "__main__":
    results = []
    with concurrent.futures.ThreadPoolExecutor(max_workers=5) as pool:
        futures = {pool.submit(fetch, item): item for item in SOURCES}
        for future in concurrent.futures.as_completed(futures):
            results.append(future.result())
    (ROOT / "upstream-sources.lock.json").write_text(json.dumps(sorted(results, key=lambda item: item["path"]), indent=2) + "\n", encoding="utf-8")
