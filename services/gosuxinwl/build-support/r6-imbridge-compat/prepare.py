#!/usr/bin/env python3
"""Verify frozen R6 sources and generate mappings for this checkout only."""

import argparse
import hashlib
import json
from pathlib import Path, PurePosixPath
import sys


HERE = Path(__file__).resolve().parent
UI_FILES = {
    "plugins/biz-suite/admin/assets/openim.js",
    "plugins/biz-suite/admin/assets/openim-access.js",
}


def sha256(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def under(root, relative):
    name = PurePosixPath(relative)
    if name.is_absolute() or ".." in name.parts or "\\" in relative or ":" in relative:
        raise ValueError("Manifest paths must be relative: " + relative)
    result = (root / relative).resolve()
    if not result.is_relative_to(root.resolve()):
        raise ValueError("Manifest path escapes its root: " + relative)
    return result


def prepare(repo_root, check_only):
    root = repo_root.resolve()
    module = (root / "go.mod").read_text(encoding="utf-8")
    if not module.startswith("module github.com/suxinwl/GoSuxin\n"):
        raise ValueError("Repository root must contain the GoSuxin go.mod")
    manifest = json.loads((HERE / "manifest.json").read_text(encoding="utf-8"))
    if manifest.get("schemaVersion") != 1:
        raise ValueError("Unsupported compatibility manifest version")
    entries = manifest["files"]
    go_files = {name for name, item in entries.items() if item["kind"] == "go"}
    ui_files = {name for name, item in entries.items() if item["kind"] == "admin"}
    if len(go_files) != 33 or ui_files != UI_FILES or set(entries) != go_files | ui_files:
        raise ValueError("Expected exactly 33 frozen Go files and the two reviewed IM UI modules")
    if any(PurePosixPath(name).parent.as_posix() != "internal/imbridge" or not name.endswith(".go") for name in go_files):
        raise ValueError("Go overrides must be direct internal/imbridge Go files")

    actual_go = {path.relative_to(root).as_posix() for path in (root / "internal/imbridge").rglob("*.go")}
    if actual_go != go_files:
        extra = sorted(actual_go - go_files)
        missing = sorted(go_files - actual_go)
        raise ValueError("IM Go coverage changed; review before rebuilding. Extra: " + str(extra) + "; missing: " + str(missing))

    replace, admin = {}, {}
    for name, item in sorted(entries.items()):
        if item["source"] != "_source/" + name:
            raise ValueError("Unexpected frozen source mapping: " + name)
        source = under(HERE, item["source"])
        target = under(root, name)
        if not target.is_file():
            raise ValueError("Missing original source: " + name)
        if sha256(source) != item["sha256"]:
            raise ValueError("Frozen R6 source hash mismatch: " + name)
        if item["kind"] == "go":
            replace[str(target)] = str(source)
        else:
            admin[name.removeprefix("plugins/biz-suite/admin/")] = {
                "path": str(source), "sha256": item["sha256"]
            }

    report = {"release": manifest["release"], "verifiedGoFiles": len(replace),
              "verifiedUIFiles": len(admin), "allMatchFrozenManifest": True}
    if not check_only:
        output = HERE / ".generated"
        output.mkdir(exist_ok=True)
        for filename, contents in (("go-overlay.json", {"Replace": replace}), ("admin-overrides.json", admin)):
            destination = output / filename
            temporary = output / (filename + ".tmp")
            temporary.write_text(json.dumps(contents, ensure_ascii=False, indent=2) + "\n", encoding="utf-8", newline="\n")
            temporary.replace(destination)
        report["goOverlay"] = str(output / "go-overlay.json")
        report["adminOverrides"] = str(output / "admin-overrides.json")
    return report


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--repo-root", type=Path, default=HERE.parents[1], help="Default: repository containing this directory")
    parser.add_argument("--check", action="store_true", help="Verify without writing generated mappings")
    args = parser.parse_args()
    try:
        print(json.dumps(prepare(args.repo_root, args.check), ensure_ascii=False, indent=2))
    except (OSError, ValueError, KeyError, TypeError) as error:
        print("R6 compatibility preparation failed: " + str(error), file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
