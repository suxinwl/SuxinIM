"""Patch only the verified public R6 bundle's nullable scope selection.

The Linux executable and all unrelated assets are retained byte-for-byte.
No deployment, service restart, configuration change, or database access occurs.
"""
from __future__ import annotations

import argparse
import copy
import hashlib
import json
import pathlib
import zipfile

ROOT = pathlib.Path(__file__).resolve().parent.parent
BASE = pathlib.Path(r"E:\PUTY_AI\OpenIM\implementation\artifacts\public-access-r6\hrm\biz-suite-linux-0.2.6-openim-public-hrm.1-555d92b87c0d.zip")
BASE_ZIP_HASH = "0dc2c6d6ca8426d8b4ab5f1564f443a1f13f6e44a90b49fca9f91bccf0ada69a"
BASE_MANIFEST_HASH = "555d92b87c0d8385820b64c4fe3090ae9a9ecdef5bc9e19735555c22ed5d14af"
OLD_ASSET = "admin/assets/app-2RTXU3GO.js"
OLD_FRAGMENT = b"i=new Set(r.map(String))"
NEW_FRAGMENT = b"i=new Set(b(r).map(String))"
VERSION = "0.2.6-openim-public-hrm.2"


def digest(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def load_verified(package: pathlib.Path):
    with zipfile.ZipFile(package) as archive:
        infos = [info for info in archive.infolist() if not info.is_dir()]
        assert len({info.filename.casefold() for info in infos}) == len(infos), "Duplicate ZIP entry"
        contents = {info.filename: archive.read(info) for info in infos}
        manifest = json.loads(contents["plugin.json"])
        assert manifest["format"] == "suxin-runtime-v1"
        assert manifest["name"] == "biz-suite" and manifest["protocol"] == 1
        assert manifest["os"] == "linux" and manifest["arch"] == "amd64"
        assert set(contents) == set(manifest["files"]) | {"plugin.json"}, "Unlisted or missing file"
        for name, expected in manifest["files"].items():
            assert digest(contents[name]) == expected, "Invalid manifest hash: " + name
        assert "bin/biz-suite" in contents
    return manifest, contents, infos


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base", type=pathlib.Path, default=BASE)
    parser.add_argument("--output", type=pathlib.Path, default=ROOT / "dist/biz-suite-linux-0.2.6-openim-public-hrm.2-null-scope.zip")
    parser.add_argument("--report", type=pathlib.Path, default=ROOT / "runtime/public-hrm-r6-null-hotfix-build.json")
    args = parser.parse_args()
    assert args.base.resolve() != args.output.resolve(), "The original package must never be overwritten"
    base_bytes = args.base.read_bytes()
    assert digest(base_bytes) == BASE_ZIP_HASH, "This patch is restricted to the audited R6 ZIP"
    manifest, contents, infos = load_verified(args.base)
    assert manifest["version"] == "0.2.6-openim-public-hrm.1"
    assert digest(contents["plugin.json"]) == BASE_MANIFEST_HASH
    original_bundle = contents[OLD_ASSET]
    assert original_bundle.count(OLD_FRAGMENT) == 1, "The pickMany replacement must match exactly once"
    assert b'b=e=>Array.isArray(e)?e:e?.items||e?.list||e?.rows||[]' in original_bundle, "asRows alias must be verified"
    patched_bundle = original_bundle.replace(OLD_FRAGMENT, NEW_FRAGMENT)
    assert patched_bundle.count(NEW_FRAGMENT) == 1
    new_asset = "admin/assets/app-hrm-null-scope-" + digest(patched_bundle)[:12] + ".js"
    old_url = ("/plugins/biz-suite/" + OLD_ASSET).encode()
    new_url = ("/plugins/biz-suite/" + new_asset).encode()
    assert contents["admin/index.html"].count(old_url) == 1, "Exactly one entry bundle reference is required"
    patched_html = contents["admin/index.html"].replace(old_url, new_url)
    updated = copy.deepcopy(manifest)
    updated["version"] = VERSION
    del updated["files"][OLD_ASSET]
    updated["files"][new_asset] = digest(patched_bundle)
    updated["files"]["admin/index.html"] = digest(patched_html)
    manifest_bytes = (json.dumps(updated, ensure_ascii=False, indent=2) + "\n").encode()
    output_contents = {**contents, "plugin.json": manifest_bytes, "admin/index.html": patched_html}
    del output_contents[OLD_ASSET]
    output_contents[new_asset] = patched_bundle
    args.output.parent.mkdir(parents=True, exist_ok=True)
    staging = args.output.with_name(args.output.name + ".tmp")
    with zipfile.ZipFile(staging, "w", compression=zipfile.ZIP_DEFLATED, compresslevel=9) as archive:
        for original_info in infos:
            info = copy.copy(original_info)
            if info.filename == OLD_ASSET:
                info.filename = new_asset
            archive.writestr(info, output_contents[info.filename])
    installed_manifest, verified, _ = load_verified(staging)
    assert installed_manifest == updated
    untouched = sorted(set(contents) - {"plugin.json", "admin/index.html", OLD_ASSET})
    assert all(verified[name] == contents[name] for name in untouched), "Unrelated bytes changed"
    assert verified["bin/biz-suite"] == contents["bin/biz-suite"], "Linux backend must be unchanged"
    assert digest(args.base.read_bytes()) == BASE_ZIP_HASH, "Original ZIP was modified"
    staging.replace(args.output)
    report = {
        "base": str(args.base.resolve()), "output": str(args.output.resolve()),
        "base_zip_sha256": BASE_ZIP_HASH, "output_zip_sha256": digest(args.output.read_bytes()),
        "before_manifest_sha256": BASE_MANIFEST_HASH, "after_manifest_sha256": digest(manifest_bytes),
        "before_manifest": manifest, "after_manifest": updated,
        "before_backend_sha256": digest(contents["bin/biz-suite"]), "after_backend_sha256": digest(verified["bin/biz-suite"]),
        "old_asset": OLD_ASSET, "new_asset": new_asset,
        "replacement_count": 1, "old_fragment": OLD_FRAGMENT.decode(), "new_fragment": NEW_FRAGMENT.decode(),
        "byte_identical_files": untouched, "all_output_hashes_verified": True,
        "original_package_unchanged": True, "deployment_performed": False,
    }
    args.report.parent.mkdir(parents=True, exist_ok=True)
    args.report.write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding="utf8")
    print(json.dumps({key: report[key] for key in ["output", "output_zip_sha256", "after_manifest_sha256", "after_backend_sha256", "new_asset", "all_output_hashes_verified", "original_package_unchanged"]}, indent=2))


if __name__ == "__main__":
    main()
