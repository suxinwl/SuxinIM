"""Compare a fresh checkout with this repository, including every file's bytes.

Does not read ignored working data, install dependencies, or run applications.
Clone with core.autocrlf=false and core.longpaths=true before using this tool.
The default report is local evidence and is excluded from Git.
"""
import argparse
import datetime
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import subprocess

ROOT = Path(__file__).resolve().parents[1]


def git(root, *args):
    return subprocess.check_output(['git', '-C', str(root), *args])


def inventory(root):
    result = {}
    for record in git(root, 'ls-tree', '-r', '-z', 'HEAD').split(b'\0'):
        if not record:
            continue
        metadata, raw_path = record.split(b'\t', 1)
        mode, kind, oid = metadata.decode('ascii').split()
        name = raw_path.decode('utf-8')
        relative = PurePosixPath(name)
        if relative.is_absolute() or '..' in relative.parts or kind != 'blob':
            raise RuntimeError('Unsupported path or unpopulated submodule: ' + name)
        result[name] = (mode, oid)
    return result


def native(path):
    return Path('\\\\?\\' + str(path.absolute())) if os.name == 'nt' else path


def file_hashes(root, name, mode):
    path = native(root / name)
    if mode == '120000':
        if path.is_symlink():
            target = os.readlink(path)
            raw = (target.replace('\\', '/') if os.name == 'nt' else target).encode('utf-8')
        elif os.name == 'nt' and path.is_file():
            # Git core.symlinks=false materializes the committed target text.
            raw = path.read_bytes()
        else:
            raise RuntimeError('Tracked link is not a link: ' + name)
        return hashlib.sha1(b'blob ' + str(len(raw)).encode() + b'\0' + raw).hexdigest(), hashlib.sha256(raw).hexdigest(), len(raw), False
    if not path.is_file() or path.is_symlink():
        raise RuntimeError('Tracked file is missing or replaced: ' + name)
    size = path.stat().st_size
    blob = hashlib.sha1(b'blob ' + str(size).encode() + b'\0')
    sha = hashlib.sha256()
    with path.open('rb') as stream:
        first = stream.read(1024 * 1024)
        lfs = first.startswith(b'version https://git-lfs.github.com/spec/v1\n')
        blob.update(first)
        sha.update(first)
        while block := stream.read(1024 * 1024):
            blob.update(block)
            sha.update(block)
    return blob.hexdigest(), sha.hexdigest(), size, lfs


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--checkout', type=Path, required=True)
    parser.add_argument('--report', type=Path, default=ROOT / '.cache/source-verification/checkout.json')
    args = parser.parse_args()
    checkout = args.checkout.resolve()
    if checkout == ROOT:
        raise RuntimeError('A separate fresh checkout is required')
    reference_head = git(ROOT, 'rev-parse', 'HEAD').decode().strip()
    if git(checkout, 'rev-parse', 'HEAD').decode().strip() != reference_head:
        raise RuntimeError('Checkout and reference commits differ')
    expected = inventory(ROOT)
    actual = inventory(checkout)
    if expected != actual:
        raise RuntimeError('Tracked paths, modes or Git blobs differ')
    rows = []
    modules = {}
    for name, (mode, oid) in sorted(expected.items()):
        original = file_hashes(ROOT, name, mode)
        copied = file_hashes(checkout, name, mode)
        if original != copied or copied[0] != oid:
            raise RuntimeError('Working bytes differ from committed source: ' + name)
        if copied[3]:
            raise RuntimeError('LFS pointer has no source payload: ' + name)
        module = '/'.join(name.split('/')[:2]) if '/' in name else '(root)'
        modules[module] = modules.get(module, 0) + 1
        rows.append({'path': name, 'mode': mode, 'gitBlob': oid, 'bytes': copied[2], 'sha256': copied[1]})
    if git(checkout, 'status', '--porcelain'):
        raise RuntimeError('Fresh checkout has uncommitted changes')
    subprocess.run(['git', '-C', str(checkout), 'fsck', '--full'], check=True, capture_output=True)
    report = {'createdUTC': datetime.datetime.now(datetime.timezone.utc).isoformat(),
              'commit': reference_head, 'passed': True,
              'trackedFiles': len(rows), 'bytes': sum(x['bytes'] for x in rows),
              'gitlinkCount': 0, 'LFSPointerCount': 0,
              'symbolicLinkEntries': [name for name, (mode, _) in expected.items() if mode == '120000'],
              'windowsGitSymlinkStubsSupported': True,
              'filesMatchCommittedBlobs': True, 'checkoutClean': True, 'gitFsckPassed': True,
              'modules': modules, 'files': rows,
              'scope': 'Every committed source file was compared against both the original working bytes and a separate checkout. This does not claim dependency installation or new platform build acceptance.'}
    args.report.parent.mkdir(parents=True, exist_ok=True)
    args.report.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({key: report[key] for key in ('commit', 'passed', 'trackedFiles', 'bytes', 'gitFsckPassed')}, ensure_ascii=False))


if __name__ == '__main__':
    main()
