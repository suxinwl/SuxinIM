#!/usr/bin/env python3
"""Build the pinned iOS13 SQLCipher core on macOS, into a NEW candidate folder.

No download, signing, app identity change, pod installation, or active-framework
replacement occurs. The approved SQLCipher XCFramework is supplied out of band;
its complete tree SHA is mandatory. Windows can run the pure input helpers/tests.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import plistlib
import shutil
import subprocess
import sys
import tempfile

SUITE = Path(__file__).resolve().parents[3]
CORE = SUITE / '.cache/sdk-core-patch15-reference'
MOBILE = 'v0.0.0-20261011034528-197770444867'
CORE_COMMIT = 'd6e0b549db904d0327d76ef7c9c283203879a095'
REQUIRED_SYMBOLS = (
    'ConfigureNativeLocalStore', 'RecoverNativeStoreRoot', 'PrepareNativeRestore',
    'ReadNativeSourceRange', 'ReadNativeAttachmentRange', 'StageNativeBytes',
    'ListNativeAttachments', 'NativeAttachmentInfo', 'GetNativeLocalArchivePage',
    'SetNativeLegacyRoots', 'ConfigureNativeLegacyQuarantine', 'FinalizeNativeLegacyFiles',
    'ImportNativeLocalAttachment', 'PreviewNativeClearRange',
    'PreviewNativeClearScopes', 'ClearNativeLocalScopes',
    'prepareCommit:',
)

def canonical(path):
    path = Path(path).absolute()
    for part in [path, *path.parents]:
        if part.is_symlink():
            raise ValueError('Linked input is not permitted')
    return path.resolve(strict=True)

def inventory(directory):
    directory = canonical(directory)
    result = {}
    for folder, directories, files in os.walk(directory, followlinks=False):
        for name in directories + files:
            item = Path(folder) / name
            if item.is_symlink():
                raise ValueError('Linked framework member is not permitted')
        for name in files:
            item = Path(folder) / name
            if not item.is_file():
                raise ValueError('Framework member is not a regular file')
            result[item.relative_to(directory).as_posix()] = hashlib.sha256(item.read_bytes()).hexdigest()
    if not result or len(result) > 20000:
        raise ValueError('Invalid framework tree')
    return dict(sorted(result.items()))

def tree_sha(directory):
    return hashlib.sha256(json.dumps(inventory(directory), sort_keys=True, separators=(',', ':')).encode()).hexdigest()

def slices(directory):
    directory = canonical(directory)
    data = plistlib.loads((directory / 'Info.plist').read_bytes())
    found = {}
    for row in data.get('AvailableLibraries', []):
        if row.get('SupportedPlatform') != 'ios':
            continue
        variant = row.get('SupportedPlatformVariant', '')
        key = 'simulator' if variant == 'simulator' else ('device' if not variant else None)
        if key is None:
            continue
        identifier, library = row.get('LibraryIdentifier', ''), row.get('LibraryPath', '')
        if '/' in identifier or '\\' in identifier or identifier in ('', '.', '..') or library != 'SQLCipher.framework':
            raise ValueError('Invalid SQLCipher slice identity')
        target = canonical(directory / identifier / library)
        if not target.is_relative_to(directory) or key in found:
            raise ValueError('Ambiguous SQLCipher slice')
        architectures = set(row.get('SupportedArchitectures', []))
        expected = {'arm64', 'x86_64'} if key == 'simulator' else {'arm64'}
        if not expected.issubset(architectures) or not (target / 'Headers/sqlite3.h').is_file():
            raise ValueError('SQLCipher architecture/header is missing')
        found[key] = target
    if set(found) != {'device', 'simulator'}:
        raise ValueError('Device and simulator SQLCipher slices are required')
    return found

def source_files(source):
    source = canonical(source)
    for folder, directories, files in os.walk(source, followlinks=False):
        directories[:] = [name for name in directories if not name.startswith('.') and name not in ('Framework', 'build', 'node_modules')]
        for name in directories + files:
            if (Path(folder) / name).is_symlink():
                raise ValueError('Linked source is not permitted')
        for name in files:
            item = Path(folder) / name
            # Do not copy SDK test databases, developer logs or arbitrary blobs.
            if item.suffix not in ('.go', '.c', '.h', '.s', '.mod', '.sum', '.crt') and item.relative_to(source).as_posix() not in ('version/version', 'LICENSE'):
                continue
            yield item

def copy_source(source, target):
    source = canonical(source)
    target.mkdir(parents=True, exist_ok=False)
    for item in source_files(source):
            out = target / item.relative_to(source)
            out.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(item, out)

def source_fingerprint():
    result = {}
    for root in (CORE, SUITE / 'sdks/native-storage/go-sqlite3', SUITE / 'sdks/native-storage/store', SUITE / 'sdks/local-store-native'):
        for item in source_files(root):
            result[item.relative_to(SUITE).as_posix()] = hashlib.sha256(item.read_bytes()).hexdigest()
    return dict(sorted(result.items()))

def add_cipher_link(directory, framework):
    framework = canonical(framework)
    if any(c in framework.as_posix() for c in ('"', '\\', '\n', '\r', '\0')):
        raise ValueError('Unsupported framework path')
    (directory / 'puty_ios_framework_link.go').write_text(
        '//go:build ios && puty_sqlcipher\n\npackage sqlite3\n\n/*\n'
        f'#cgo CFLAGS: "-I{(framework / "Headers").as_posix()}"\n'
        f'#cgo LDFLAGS: "-F{framework.parent.as_posix()}" -framework SQLCipher\n'
        '*/\nimport "C"\n', encoding='utf-8')

def run(args, cwd=None, env=None):
    # Build inputs contain public paths only; no token/passphrase/master is read.
    subprocess.run([str(value) for value in args], cwd=cwd, env=env, check=True)

def one_framework(directory):
    rows = plistlib.loads((directory / 'Info.plist').read_bytes()).get('AvailableLibraries', [])
    if len(rows) != 1:
        raise ValueError('Unexpected generated framework slices')
    row = rows[0]
    path = canonical(directory / row['LibraryIdentifier'] / row['LibraryPath'])
    if not path.is_relative_to(directory.resolve()) or path.name != 'OpenIMCore.framework':
        raise ValueError('Unexpected generated framework path')
    text = '\n'.join(value.read_text(errors='strict') for value in (path / 'Headers').glob('*.h'))
    if any('Open_im_sdk' + symbol not in text for symbol in REQUIRED_SYMBOLS):
        raise ValueError('New native-store binding is incomplete')
    return path

def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--sqlcipher-xcframework', required=True)
    parser.add_argument('--sqlcipher-tree-sha256')
    parser.add_argument('--describe-input', action='store_true')
    parser.add_argument('--output')
    parser.add_argument('--go', default='go')
    parser.add_argument('--gomobile', default='gomobile')
    args = parser.parse_args(argv)
    cipher = canonical(args.sqlcipher_xcframework)
    inputs = slices(cipher)
    digest = tree_sha(cipher)
    if args.describe_input:
        print(json.dumps({'sqlcipherTreeSHA256': digest, 'sliceCount': len(inputs)}))
        return
    if sys.platform != 'darwin' or args.sqlcipher_tree_sha256 != digest or not args.output:
        raise ValueError('macOS, approved framework SHA and new output folder are required')
    out = Path(args.output).absolute()
    if out.exists() or not out.parent.is_dir() or canonical(out.parent) != out.parent or out.is_relative_to(CORE):
        raise ValueError('Output must be a new canonical candidate directory')
    if subprocess.check_output([args.go, 'version'], text=True).split()[2] != 'go1.26.9':
        raise ValueError('Pinned Go1.26.9 is required')
    if MOBILE not in subprocess.check_output([args.go, 'version', '-m', shutil.which(args.gomobile) or args.gomobile], text=True):
        raise ValueError('Pinned gomobile is required')
    if subprocess.check_output(['git', '-C', str(CORE), 'rev-parse', 'HEAD'], text=True).strip() != CORE_COMMIT:
        raise ValueError('Pinned SDK base differs')
    trust = CORE / 'pkg/enterprise_trust/puty_enterprise_root_ca.crt'
    if hashlib.sha256(trust.read_bytes()).hexdigest() != '9a135d11160e2179c6d396088b765143b60b9ed82c44a8acc2cd8735e6d3f183':
        raise ValueError('Pinned enterprise public CA differs')
    env = dict(os.environ, GOTOOLCHAIN='local', GOPROXY='off', GOSUMDB='off', GOMAXPROCS='2', GOMEMLIMIT='2GiB')
    sources = source_fingerprint()
    out.mkdir(mode=0o700)
    with tempfile.TemporaryDirectory(prefix='puty-ios-build-', dir=out) as temporary:
        stage = Path(temporary)
        generated = []
        for platform, target in [('device', 'ios/arm64'), ('simulator', 'iossimulator/arm64,iossimulator/amd64')]:
            core = stage / ('core-' + platform)
            sqlite = stage / ('sqlite-' + platform)
            copy_source(CORE, core)
            copy_source(SUITE / 'sdks/native-storage/go-sqlite3', sqlite)
            add_cipher_link(sqlite, inputs[platform])
            for module, replacement in [('github.com/mattn/go-sqlite3', sqlite), ('puty.local/nativestore', SUITE / 'sdks/native-storage/store'), ('puty.local/localstore', SUITE / 'sdks/local-store-native')]:
                run([args.go, 'mod', 'edit', '-replace=' + module + '=' + str(replacement)], cwd=core, env=env)
            generated_dir = stage / platform
            generated_dir.mkdir()
            artifact = generated_dir / 'OpenIMCore.xcframework'
            run([args.gomobile, 'bind', '-target=' + target, '-iosversion=13.0', '-tags=puty_sqlcipher,sqlite_omit_load_extension', '-trimpath', '-ldflags=-s -w', '-o', artifact, './open_im_sdk/', './open_im_sdk_callback/'], cwd=core, env=env)
            generated.append(one_framework(artifact))
        if tree_sha(cipher) != digest:
            raise ValueError('SQLCipher input changed during build')
        if source_fingerprint() != sources:
            raise ValueError('Native source changed during build')
        run(['xcodebuild', '-create-xcframework', '-framework', generated[0], '-framework', generated[1], '-output', out / 'OpenIMCore.xcframework'])
        shutil.copytree(cipher, out / 'SQLCipher.xcframework')
    manifest = {'format':'puty-ios-core-candidate-v1', 'go':'1.26.9', 'gomobile':MOBILE, 'iosMinimum':'13.0', 'coreBase':CORE_COMMIT, 'sourceFiles':sources, 'sqlcipherTreeSHA256':digest, 'openIMCoreFiles':inventory(out / 'OpenIMCore.xcframework'), 'macCompileValidated':False, 'deviceMediaValidated':False}
    (out / 'manifest.json').write_text(json.dumps(manifest, sort_keys=True, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'candidateCreated':True, 'manifestSHA256':hashlib.sha256((out / 'manifest.json').read_bytes()).hexdigest(), 'signingPerformed':False}))

if __name__ == '__main__':
    main()
