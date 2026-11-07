"""Build two audited rule-list UI patches; never deploy or access a database.

Only the entry bundle, its HTML reference and (when shipped) the corresponding
unbundled attendance source change. Workers, IM modules and other assets retain
their exact bytes. Each base manifest is pinned to the verified installed build.
"""
from __future__ import annotations

import argparse
import copy
import hashlib
import json
import pathlib
import zipfile

ROOT = pathlib.Path(__file__).resolve().parent.parent
TARGETS = {
    "local": {
        "manifest": "4adb279694ab46ad990008d99e87125ef4611ab6ff0ff9b7e7c01745aaa2cf1f",
        "base": ROOT / "storage/plugins/biz-suite/packages/4adb279694ab46ad990008d99e87125ef4611ab6ff0ff9b7e7c01745aaa2cf1f",
        "os": "windows", "version": "0.3.1", "bundle": "admin/assets/app-IALJZYBX.js",
        "anchor": b'let[r,n]=await Promise.all([Z("/rules"),pt()]);if(!a())return;',
        "result": "r", "rows": "g",
    },
    "public": {
        "manifest": "711958d503d91b9b1902f7bd1a63acee704488e76fb911df87ef1a6334b058c9",
        "base": ROOT / "dist/biz-suite-linux-0.3.2-openim-r6.2-711958d503d9.zip",
        "os": "linux", "version": "0.3.2-openim-r6.2", "bundle": "admin/assets/app-LV5TANUP.js",
        "anchor": b'let[s,n]=await Promise.all([ae("/rules"),ut()]);if(!a())return;',
        "result": "s", "rows": "v",
    },
}


def digest(data):
    return hashlib.sha256(data).hexdigest()


def load_verified(path):
    if path.is_dir():
        contents = {p.relative_to(path).as_posix(): p.read_bytes() for p in path.rglob("*") if p.is_file()}
        infos = []
        for name in sorted(contents):
            info = zipfile.ZipInfo(name)
            info.create_system = 3
            info.external_attr = (0o100755 if name.startswith("bin/") else 0o100644) << 16
            info.compress_type = zipfile.ZIP_DEFLATED
            infos.append(info)
    else:
        with zipfile.ZipFile(path) as archive:
            infos = [item for item in archive.infolist() if not item.is_dir()]
            assert len({item.filename.casefold() for item in infos}) == len(infos), "Duplicate ZIP entry"
            contents = {item.filename: archive.read(item) for item in infos}
    for name in contents:
        parts = pathlib.PurePosixPath(name)
        assert not parts.is_absolute() and ".." not in parts.parts and "\\" not in name and ":" not in name, "Unsafe entry"
    manifest = json.loads(contents["plugin.json"])
    assert manifest["format"] == "suxin-runtime-v1" and manifest["name"] == "biz-suite" and manifest["protocol"] == 1
    assert set(contents) == set(manifest["files"]) | {"plugin.json"}, "Unlisted/missing files"
    for name, expected in manifest["files"].items():
        assert digest(contents[name]) == expected, "Hash mismatch: " + name
    return manifest, contents, infos


def patch_source(original):
    text = original.decode("utf8")
    newline = "\r\n" if "\r\n" in text else "\n"
    marker = "export async function rules() {"
    assert text.count(marker) == 1 and "function cleanedUpRule(" not in text
    helper = newline.join([
        "function cleanedUpRule(doc) {",
        "  let source = doc.source;",
        '  if (typeof source === "string") { try { source = JSON.parse(source); } catch { return false; } }',
        "  return source?.cleanup?.hidden === true;",
        "}", "",
    ])
    text = text.replace(marker, helper + marker)
    old = "asRows(result).filter(doc =>"
    assert text.count(old) == 2, "Expected only current/history rule selectors"
    return text.replace(old, "asRows(result).filter(doc => !cleanedUpRule(doc)).filter(doc =>").encode("utf8")


def build(target, output_dir, report_dir):
    spec = TARGETS[target]
    base = spec["base"]
    manifest, contents, infos = load_verified(base)
    assert digest(contents["plugin.json"]) == spec["manifest"], "Unreviewed base manifest"
    assert manifest["version"] == spec["version"] and manifest["os"] == spec["os"] and manifest["arch"] == "amd64"
    version = spec["version"] + "-rule-cleanup.1"
    output = output_dir / f"biz-suite-{spec['os']}-{version}.zip"
    assert output.resolve() != base.resolve() and not output.exists(), "Never overwrite input or an existing release"
    old_asset = spec["bundle"]
    original = contents[old_asset]
    anchor = spec["anchor"]
    assert original.count(anchor) == 1, "Rule list anchor must match once"
    # The adapter is local to rules(), keeping the fetched response untouched in
    # every other consumer (historical rosters, details, audit and calculations).
    fragment = (spec["result"] + "=" + spec["rows"] + "(" + spec["result"] + ").filter(function(cleanupRule){let cleanupSource=cleanupRule.source;"
                'if(typeof cleanupSource==="string"){try{cleanupSource=JSON.parse(cleanupSource)}catch{return true}}'
                "return cleanupSource?.cleanup?.hidden!==true});").encode()
    assert b"cleanupSource" not in original
    patched = original.replace(anchor, anchor + fragment)
    assert patched.replace(anchor + fragment, anchor) == original
    new_asset = "admin/assets/app-rule-cleanup-" + digest(patched)[:12] + ".js"
    old_url = ("/plugins/biz-suite/" + old_asset).encode()
    new_url = ("/plugins/biz-suite/" + new_asset).encode()
    assert contents["admin/index.html"].count(old_url) == 1
    changed = {"admin/index.html": contents["admin/index.html"].replace(old_url, new_url)}
    if "admin/assets/attendance-v2.js" in contents:
        changed["admin/assets/attendance-v2.js"] = patch_source(contents["admin/assets/attendance-v2.js"])
    updated = copy.deepcopy(manifest)
    updated["version"] = version
    del updated["files"][old_asset]
    updated["files"][new_asset] = digest(patched)
    for name, data in changed.items():
        updated["files"][name] = digest(data)
    manifest_bytes = (json.dumps(updated, ensure_ascii=False, indent=2) + "\n").encode()
    output_contents = {**contents, **changed, "plugin.json": manifest_bytes}
    del output_contents[old_asset]
    output_contents[new_asset] = patched
    output_dir.mkdir(parents=True, exist_ok=True)
    staging = output.with_suffix(".zip.tmp")
    assert not staging.exists(), "Existing temporary output requires inspection"
    with zipfile.ZipFile(staging, "w", compression=zipfile.ZIP_DEFLATED, compresslevel=9) as archive:
        for original_info in infos:
            info = copy.copy(original_info)
            if info.filename == old_asset:
                info.filename = new_asset
            archive.writestr(info, output_contents[info.filename])
    checked_manifest, checked, _ = load_verified(staging)
    assert checked_manifest == updated
    untouched = sorted(set(contents) - {"plugin.json", old_asset, *changed})
    assert all(contents[name] == checked[name] for name in untouched), "Unrelated file changed"
    backends = {name: digest(data) for name, data in contents.items() if name.startswith("bin/")}
    assert backends and all(checked[name] == contents[name] for name in backends), "Worker bytes changed"
    _, reread, _ = load_verified(base)
    assert reread == contents, "Original package was modified"
    staging.replace(output)
    report = {
        "target": target, "base": str(base.resolve()), "output": str(output.resolve()),
        "before_manifest_sha256": spec["manifest"], "after_manifest_sha256": digest(manifest_bytes),
        "output_zip_sha256": digest(output.read_bytes()), "before_version": manifest["version"], "after_version": version,
        "before_manifest": manifest, "after_manifest": updated,
        "old_asset": old_asset, "new_asset": new_asset,
        "old_bundle_sha256": digest(original), "new_bundle_sha256": digest(patched),
        "bundle_inserted_bytes": len(fragment), "bundle_anchor_count": 1,
        "bundle_anchor": anchor.decode(), "inserted_fragment": fragment.decode(),
        "modified_files": sorted(["plugin.json", *changed]),
        "renamed_files": {old_asset: new_asset}, "byte_identical_files": untouched,
        "before_backend_sha256": backends, "after_backend_sha256": {name: digest(checked[name]) for name in backends},
        "all_output_hashes_verified": True, "original_package_unchanged": True,
        "database_access": False, "deployment_performed": False,
    }
    report_dir.mkdir(parents=True, exist_ok=True)
    report_path = report_dir / f"attendance-rule-cleanup-{target}-build.json"
    report_path.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf8")
    print(json.dumps({key: report[key] for key in ["target", "output", "after_manifest_sha256", "output_zip_sha256", "new_asset", "before_backend_sha256"]}))
    return report


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--target", choices=["local", "public", "all"], default="all")
    parser.add_argument("--output-dir", type=pathlib.Path, default=ROOT / "dist")
    parser.add_argument("--report-dir", type=pathlib.Path, default=ROOT / "runtime")
    args = parser.parse_args()
    for target in TARGETS if args.target == "all" else [args.target]:
        build(target, args.output_dir, args.report_dir)


if __name__ == "__main__":
    main()
