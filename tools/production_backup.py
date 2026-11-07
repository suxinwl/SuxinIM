#!/usr/bin/env python3
"""Pinned-SSH production backups with local, authenticated streaming encryption.

Local dependencies: cryptography and paramiko. No server connection occurs for
keygen, inspect or the test suite. Recovery keys never leave the local machine.
"""
import argparse
import base64
import contextlib
import datetime
import getpass
import hashlib
import io
import json
import os
import pathlib
import secrets
import shlex
import struct
import subprocess
import sys
import tarfile
import tempfile
import threading

from cryptography.hazmat.primitives.ciphers.aead import AESGCM
from cryptography.hazmat.primitives.kdf.hkdf import HKDF
from cryptography.hazmat.primitives import hashes

ROOT = pathlib.Path(__file__).resolve().parents[1]
PRIVATE = ROOT / '.private'
DEFAULT_KEY = PRIVATE / 'production-backup-keys/recovery.key'
BACKUPS = PRIVATE / 'production-backups'
WORKER = ROOT / 'deploy/production/backup_remote.py'
MAGIC = b'PUTYBK02'
FORMAT = 'puty-production-backup-v2'
CHUNK_BYTES = 4 * 1024 * 1024
FINGERPRINT = 'SHA256:dK01/2cnU2tHyQtdLV2/fsz/Xmf2T1QlUPbAhS3Bwg8'
SCRATCH_MARKER = '.puty-backup-scratch-owner.json'


def require(condition, message):
    if not condition:
        raise RuntimeError(message)


def canonical(value):
    return json.dumps(value, sort_keys=True, separators=(',', ':'), ensure_ascii=False).encode()


def utc():
    return datetime.datetime.now(datetime.timezone.utc).isoformat()


def protect(path):
    """Restrict actual key/plaintext directories on both supported operator OSes."""
    if os.name == 'nt':
        sid_result = subprocess.run(['whoami', '/user', '/fo', 'csv', '/nh'], capture_output=True, check=True)
        import csv
        sid = next(csv.reader([sid_result.stdout.decode().strip()]))[1]
        rights = '(OI)(CI)F' if path.is_dir() else 'F'
        subprocess.run(['icacls', str(path), '/inheritance:r', '/grant:r', '*' + sid + ':' + rights],
                       check=True, stdout=subprocess.DEVNULL, stderr=subprocess.PIPE)
    else:
        path.chmod(0o700 if path.is_dir() else 0o600)


def private_path(path, *, make_parent=True):
    path = pathlib.Path(path).absolute()
    require(path.resolve().is_relative_to(PRIVATE.resolve()), 'Backup material must stay inside this repository .private directory')
    for parent in [path, *path.parents]:
        require(not parent.is_symlink(), 'Linked local backup path refused')
        if parent == ROOT:
            break
    if make_parent:
        path.parent.mkdir(parents=True, exist_ok=True)
        protect(path.parent)
    return path


def assert_scratch_protected(path):
    """Verify no non-operator allow ACE survived an explicit Windows ACL."""
    if os.name == 'nt':
        import csv
        result = subprocess.run(['whoami', '/user', '/fo', 'csv', '/nh'], capture_output=True, check=True)
        sid = next(csv.reader([result.stdout.decode().strip()]))[1]
        script = r'''
try {
 $v=ConvertFrom-Json -InputObject ([Console]::In.ReadToEnd())
 if([System.IO.Directory]::Exists($v.path)){$acl=[System.IO.Directory]::GetAccessControl($v.path)}
 else{$acl=[System.IO.File]::GetAccessControl($v.path)}
 $allow=@($v.sid,'S-1-5-18','S-1-5-32-544')
 if(-not $acl.AreAccessRulesProtected){exit 2}
 if($allow -notcontains $acl.GetOwner([System.Security.Principal.SecurityIdentifier]).Value){exit 2}
 # Python 3.13's private Windows mkdtemp also grants OWNER RIGHTS. It only
 # resolves to the already-validated owner, never another account or group.
 $allow+=@('S-1-3-4')
 foreach($entry in $acl.Access){
  if($entry.AccessControlType -eq [System.Security.AccessControl.AccessControlType]::Allow){
   if($allow -notcontains $entry.IdentityReference.Translate([System.Security.Principal.SecurityIdentifier]).Value){exit 2}
  }
 }
 [Console]::Out.Write('protected')
} catch {exit 2}
'''
        checked = subprocess.run(['powershell.exe', '-NoProfile', '-NonInteractive', '-Command', script],
            input=canonical({'path': str(path), 'sid': sid}), stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, timeout=20)
        require(checked.returncode == 0 and checked.stdout == b'protected', 'Scratch ACL is not private to this operator')
    else:
        info = path.stat()
        require(info.st_uid == os.getuid() and not info.st_mode & 0o077, 'Scratch permissions are not private to this operator')


def owned_scratch_directory(value):
    """Only the explicitly selected empty/owned leaf may hold temporary plaintext."""
    path = pathlib.Path(value).absolute()
    require(path.parent != path and path != pathlib.Path(path.anchor)
            and not ROOT.resolve().is_relative_to(path.resolve())
            and not path.resolve().is_relative_to(DEFAULT_KEY.parent.resolve())
            and not path.resolve().is_relative_to(BACKUPS.resolve())
            and not BACKUPS.resolve().is_relative_to(path.resolve()), 'Scratch must be a dedicated non-root directory')
    for item in (path, *path.parents):
        require(not item.is_symlink() and not getattr(item, 'is_junction', lambda: False)(), 'Linked scratch path refused')
    if path.exists():
        require(path.is_dir() and path.resolve() == path, 'Scratch directory is not canonical')
        entries = list(path.iterdir())
        require(not entries or [entry.name for entry in entries] == [SCRATCH_MARKER],
                'Scratch must be initially empty or contain only its owned marker')
    else:
        path.mkdir(parents=True)
    require(path.resolve() == path, 'Scratch directory is not canonical')
    marker = path / SCRATCH_MARKER
    expected = {'format': 'puty-backup-scratch-1', 'repositorySHA256': hashlib.sha256(str(ROOT.resolve()).encode()).hexdigest(),
                'purpose': 'temporary-authenticated-backup-verification-only'}
    if marker.exists():
        require(marker.is_file() and not marker.is_symlink() and marker.stat().st_nlink == 1
                and marker.stat().st_size <= 4096 and json.loads(marker.read_bytes()) == expected,
                'Foreign scratch ownership marker refused')
    protect(path)
    assert_scratch_protected(path)
    if not marker.exists():
        with marker.open('xb') as output:
            protect(marker)
            output.write(canonical(expected))
            output.flush()
            os.fsync(output.fileno())
    protect(marker)
    assert_scratch_protected(marker)
    return path


def generate_key(path):
    path = private_path(path)
    with path.open('xb') as output:
        output.write(secrets.token_bytes(32))
    protect(path)
    return hashlib.sha256(path.read_bytes()).hexdigest()[:16]


def load_key(path, archive=None):
    path = private_path(path, make_parent=False)
    require(path.is_file() and path.stat().st_size == 32, 'Recovery key must be the original 32-byte private key file')
    if archive:
        require(path.parent != pathlib.Path(archive).resolve().parent, 'Recovery key and backup archive must be stored in separate directories')
    protect(path)
    return path.read_bytes()


def exact(source, size):
    parts, remaining = [], size
    while remaining:
        block = source.read(remaining)
        require(block, 'Truncated authenticated backup')
        parts.append(block)
        remaining -= len(block)
    return b''.join(parts)


def derive(key, salt):
    return HKDF(algorithm=hashes.SHA256(), length=32, salt=salt,
                info=b'puty-production-backup-v2').derive(key)


def encrypt_stream(source, output, key, archive_id=None, chunk_bytes=CHUNK_BYTES):
    require(len(key) == 32 and 1 <= chunk_bytes <= CHUNK_BYTES, 'Invalid encryption key/chunk size')
    salt, prefix = secrets.token_bytes(32), secrets.token_bytes(8)
    header = {'format': FORMAT, 'cipher': 'AES-256-GCM-chunked-HKDF-SHA256', 'chunkBytes': chunk_bytes,
              'archiveID': archive_id or secrets.token_hex(16), 'created': utc(),
              'salt': base64.b64encode(salt).decode(), 'noncePrefix': base64.b64encode(prefix).decode(),
              'keyID': hashlib.sha256(key).hexdigest()[:16]}
    raw_header = canonical(header)
    output.write(MAGIC + struct.pack('>I', len(raw_header)) + raw_header)
    aad = hashlib.sha256(MAGIC + raw_header).digest()
    cipher, digest, total, index = AESGCM(derive(key, salt)), hashlib.sha256(), 0, 0
    while True:
        block = source.read(chunk_bytes)
        if not block:
            break
        require(index < 0xFFFFFFFE, 'Backup exceeds safe chunk sequence')
        record = struct.pack('>BII', 0, index, len(block))
        output.write(record + cipher.encrypt(prefix + struct.pack('>I', index), block, aad + record))
        digest.update(block)
        total += len(block)
        index += 1
    ending = canonical({'bytes': total, 'sha256': digest.hexdigest(), 'chunks': index})
    record = struct.pack('>BII', 1, index, len(ending))
    output.write(record + cipher.encrypt(prefix + struct.pack('>I', index), ending, aad + record))
    return {**header, 'payloadBytes': total, 'payloadSha256': digest.hexdigest(), 'chunks': index}


def decrypt_stream(source, output, key):
    require(exact(source, len(MAGIC)) == MAGIC, 'Unknown backup envelope')
    length = struct.unpack('>I', exact(source, 4))[0]
    require(1 <= length <= 16384, 'Invalid envelope header length')
    raw_header = exact(source, length)
    header = json.loads(raw_header)
    require(header.get('format') == FORMAT and header.get('cipher') == 'AES-256-GCM-chunked-HKDF-SHA256'
            and 1 <= header.get('chunkBytes', 0) <= CHUNK_BYTES, 'Unsupported encryption format')
    salt = base64.b64decode(header['salt'], validate=True)
    prefix = base64.b64decode(header['noncePrefix'], validate=True)
    require(len(salt) == 32 and len(prefix) == 8, 'Invalid salt or nonce prefix')
    cipher, digest, total, index = AESGCM(derive(key, salt)), hashlib.sha256(), 0, 0
    aad = hashlib.sha256(MAGIC + raw_header).digest()
    while True:
        record = exact(source, 9)
        final, sequence, size = struct.unpack('>BII', record)
        require(final in (0, 1) and sequence == index and index < 0xFFFFFFFF
                and 0 < size <= (1024 if final else header['chunkBytes']), 'Invalid encryption chunk sequence')
        block = cipher.decrypt(prefix + struct.pack('>I', index), exact(source, size + 16), aad + record)
        if final:
            ending = json.loads(block)
            require(ending == {'bytes': total, 'sha256': digest.hexdigest(), 'chunks': index}
                    and source.read(1) == b'', 'Invalid final authentication record or trailing data')
            return {**header, 'payloadBytes': total, 'payloadSha256': digest.hexdigest(), 'chunks': index}
        output.write(block)
        digest.update(block)
        total += len(block)
        index += 1


def file_sha(path):
    with path.open('rb') as source:
        return hashlib.file_digest(source, 'sha256').hexdigest()


def validate_symlink_graph(links):
    """Resolve the complete archive graph before creating any real links.

    A lexical normpath is insufficient: an earlier link changes the meaning of
    later '..' components. Keep the original components until links expand.
    """
    resolved = {}

    def resolve(parts, active, budget):
        current = []
        for part in parts:
            if part in ('', '.'):
                continue
            if part == '..':
                require(len(current) > 1, 'Symlink graph escapes archive namespace')
                current.pop()
                continue
            current.append(part)
            name = pathlib.PurePosixPath(*current).as_posix()
            if name in links:
                require(name not in active, 'Symlink graph loop refused')
                budget[0] += 1
                require(budget[0] <= 40, 'Symlink graph exceeds resolution limit')
                target = links[name]
                require(target and not pathlib.PurePosixPath(target).is_absolute()
                        and '\\' not in target and '\x00' not in target,
                        'Absolute or malformed symlink refused')
                current = list(resolve((*current[:-1], *pathlib.PurePosixPath(target).parts),
                                       active | {name}, budget))
        return tuple(current)

    for name in links:
        destination = resolve(pathlib.PurePosixPath(name).parts, set(), [0])
        require(destination and destination[0] == pathlib.PurePosixPath(name).parts[0],
                'Symlink graph escapes archive namespace')
        resolved[name] = destination
    return {name: pathlib.PurePosixPath(*parts) for name, parts in resolved.items()}


def verify_archive(path):
    """Validate the decrypted tar and its manifest before uploading for restore."""
    inventory, seen, links, manifest = [], set(), {}, None
    activation_files = {}
    activation_paths = {'runtime/activation.json', 'runtime/gosuxin/runtime-environment.json', 'runtime/openim/instance.json'}
    with tarfile.open(path, 'r:gz') as archive:
        for member in archive:
            name = pathlib.PurePosixPath(member.name)
            require(not name.is_absolute() and '..' not in name.parts and '\\' not in member.name
                    and '\x00' not in member.name and name.parts and member.name not in seen
                    and manifest is None, 'Unsafe or duplicate tar member')
            seen.add(member.name)
            if member.name == 'manifest.json':
                require(member.isfile() and member.size <= 128 * 1024 * 1024, 'Invalid manifest')
                manifest = json.load(archive.extractfile(member))
                continue
            require(member.name.startswith(('runtime/', 'host-config/', 'volumes/', 'logical/')),
                    'Unknown archive namespace')
            require(member.isdir() or member.isfile() or member.issym(), 'Special tar member refused')
            row = {'path': member.name, 'mode': member.mode, 'uid': member.uid, 'gid': member.gid,
                   'type': 'directory' if member.isdir() else 'symlink' if member.issym() else 'file'}
            if member.isfile():
                if member.name in activation_paths:
                    require(member.size <= 1024 * 1024, 'Oversized activation identity file')
                    raw = archive.extractfile(member).read()
                    activation_files[member.name] = json.loads(raw)
                    row.update(bytes=member.size, sha256=hashlib.sha256(raw).hexdigest())
                else:
                    row.update(bytes=member.size, sha256=hashlib.file_digest(archive.extractfile(member), 'sha256').hexdigest())
            elif member.issym():
                target = pathlib.PurePosixPath(member.linkname)
                require(member.linkname and not target.is_absolute() and '\\' not in member.linkname and '\x00' not in member.linkname,
                        'Absolute or malformed symlink refused')
                parts = list(name.parent.parts)
                for part in target.parts:
                    if part == '..':
                        require(len(parts) > 1, 'Escaping archive symlink refused')
                        parts.pop()
                    elif part != '.':
                        parts.append(part)
                links[name] = member.linkname
                row['target'] = member.linkname
            inventory.append(row)
    require(manifest and manifest.get('format') == FORMAT and manifest.get('files') == inventory,
            'Archive manifest inventory mismatch')
    require(not any(pathlib.PurePosixPath(n) != link and pathlib.PurePosixPath(n).is_relative_to(link)
                    for n in seen for link in links), 'Archive contains files below a symlink')
    validate_symlink_graph({name.as_posix(): target for name, target in links.items()})
    if 'instanceID' in manifest:
        require(activation_files.keys() == activation_paths, 'Required activation identity files missing')
        activation = activation_files['runtime/activation.json']
        runtime = activation_files['runtime/gosuxin/runtime-environment.json']
        owner = activation_files['runtime/openim/instance.json']
        require(activation == manifest.get('activation') and activation.get('instanceID') == manifest['instanceID']
                and owner.get('instanceID') == manifest['instanceID'] and owner.get('production') is True
                and owner.get('testOnly') is False and owner.get('stackName') == 'puty-suite-prod'
                and owner.get('root') == '/srv/puty-suite/openim'
                and activation.get('dataPolicy') == 'fresh-business-data'
                and isinstance(activation.get('startAt'), str) and activation['startAt']
                and runtime.get('GOSUXIN_SYNC_START_AT') == activation['startAt']
                and runtime.get('GOSUXIN_REHEARSAL') == 'false', 'Backup activation ownership or runtime cutoff mismatch')
        try:
            stamp = datetime.datetime.fromisoformat(activation['startAt'].replace('Z', '+00:00'))
        except ValueError:
            raise RuntimeError('Invalid backup activation timestamp') from None
        require(stamp.tzinfo is not None and stamp.utcoffset() is not None, 'Backup activation timezone missing')
    return manifest


@contextlib.contextmanager
def decrypted_archive(archive, key, scratch_directory=None):
    scratch = (owned_scratch_directory(scratch_directory) if scratch_directory is not None else
               private_path(PRIVATE / 'production-backup-scratch/placeholder').parent)
    with tempfile.TemporaryDirectory(prefix='verify-', dir=scratch) as temporary:
        folder = pathlib.Path(temporary)
        protect(folder)
        if scratch_directory is not None:
            assert_scratch_protected(folder)
        plaintext = folder / 'payload.tar.gz'
        with archive.open('rb') as source, plaintext.open('xb') as output:
            protect(plaintext)
            metadata = decrypt_stream(source, output, key)
        manifest = verify_archive(plaintext)
        yield plaintext, metadata, manifest


def connect(args):
    import paramiko
    class PinnedHost(paramiko.MissingHostKeyPolicy):
        def missing_host_key(self, client, hostname, key):
            actual = 'SHA256:' + base64.b64encode(hashlib.sha256(key.asbytes()).digest()).decode().rstrip('=')
            require(secrets.compare_digest(actual, args.fingerprint), 'SSH server host fingerprint mismatch')
    client = paramiko.SSHClient()
    client.set_missing_host_key_policy(PinnedHost())
    credential = {'key_filename': str(args.ssh_key)} if args.ssh_key else {'password': getpass.getpass('SSH password (memory only): ')}
    client.connect(args.host, port=args.port, username=args.user, allow_agent=False, look_for_keys=False,
                   timeout=15, banner_timeout=20, auth_timeout=30, **credential)
    client.get_transport().set_keepalive(20)
    return client


def remote_command(action, *arguments):
    source = base64.b64encode(WORKER.read_bytes()).decode()
    script = 'import base64;exec(compile(base64.b64decode(' + repr(source) + '),"backup_remote.py","exec"))'
    return 'python3 -c ' + shlex.quote(script) + ' ' + ' '.join(shlex.quote(x) for x in (action, *arguments))


def stderr_reader(stream, events):
    for line in stream:
        try:
            item = json.loads(line)
            events.append(item)
            print(json.dumps({'remote': item}), file=sys.stderr, flush=True)
        except (ValueError, TypeError):
            print('Remote diagnostic withheld (may contain private paths).', file=sys.stderr)


def create(args):
    target = private_path(args.output or BACKUPS / ('puty-' + datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ') + '-' + secrets.token_hex(4) + '.putybak'))
    require(not target.exists(), 'Backup target already exists')
    key = load_key(args.key, target)
    partial = target.with_name(target.name + '.partial')
    client, events = connect(args), []
    try:
        stdin, stdout, stderr = client.exec_command(remote_command('create', '--max-stop-seconds', str(args.max_stop_seconds)), timeout=args.max_stop_seconds + 300)
        stdin.channel.shutdown_write()
        reader = threading.Thread(target=stderr_reader, args=(stderr, events), daemon=True)
        reader.start()
        with partial.open('xb') as output:
            protect(partial)
            metadata = encrypt_stream(stdout, output, key)
            output.flush()
            os.fsync(output.fileno())
        code = stdout.channel.recv_exit_status()
        reader.join(30)
        require(code == 0 and any(x.get('event') == 'services-resumed' for x in events),
                'Remote backup failed or service resumption was not confirmed; archive not committed')
        with decrypted_archive(partial, key, scratch_directory=getattr(args, 'scratch_directory', None)) as (_payload, verified, manifest):
            require(verified['payloadSha256'] == metadata['payloadSha256'], 'Local roundtrip payload mismatch')
            metadata.update(sourceInstanceID=manifest['instanceID'], sourceArchiveID=manifest['archiveID'],
                            manifestSha256=hashlib.sha256(canonical(manifest)).hexdigest(),
                            consistency=manifest['consistency'], files=len(manifest['files']),
                            volumes=manifest['volumes'], mysqlDatabase=manifest['mysqlDatabase'], mongoDatabases=manifest['mongoDatabases'])
        partial.replace(target)
        metadata.update(archive=str(target), ciphertextBytes=target.stat().st_size, ciphertextSha256=file_sha(target),
                        recoveryKeyStoredOnServer=False, recoveryKeyIncludedInArchive=False)
        target.with_suffix(target.suffix + '.manifest.json').write_bytes(canonical(metadata) + b'\n')
        target.with_suffix(target.suffix + '.sha256').write_text(metadata['ciphertextSha256'] + '  ' + target.name + '\n')
        print(json.dumps(metadata))
    finally:
        client.close()
        if partial.exists():
            partial.unlink()


def restore_check(args):
    archive = private_path(args.archive, make_parent=False)
    key = load_key(args.key, archive)
    with decrypted_archive(archive, key, scratch_directory=getattr(args, 'scratch_directory', None)) as (payload, metadata, manifest):
        client, events = connect(args), []
        try:
            stdin, stdout, stderr = client.exec_command(remote_command('restore-check', '--payload-sha256', metadata['payloadSha256']), timeout=3600)
            reader = threading.Thread(target=stderr_reader, args=(stderr, events), daemon=True)
            reader.start()
            with payload.open('rb') as source:
                while block := source.read(CHUNK_BYTES):
                    stdin.write(block)
            stdin.flush()
            stdin.channel.shutdown_write()
            raw = stdout.read()
            code = stdout.channel.recv_exit_status()
            reader.join(30)
            require(len(raw) <= 1024 * 1024, 'Unexpected restore report size')
            report = json.loads(raw)
            report.update(ciphertextSha256=file_sha(archive), sourceArchiveID=manifest['archiveID'])
            report_path = private_path(args.report or archive.with_suffix('.restore-check.json'))
            report_path.write_bytes(canonical(report) + b'\n')
            require(code == 0 and report.get('passed') and report.get('cleanupVerified'),
                    'Isolated restore validation failed; inspect its private report')
            print(json.dumps({'report': str(report_path), **report}))
        finally:
            client.close()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--key', type=pathlib.Path, default=DEFAULT_KEY)
    parser.add_argument('--host', default='192.168.10.11')
    parser.add_argument('--port', type=int, default=2223)
    parser.add_argument('--user', default='root')
    parser.add_argument('--fingerprint', default=FINGERPRINT)
    parser.add_argument('--ssh-key', type=pathlib.Path)
    parser.add_argument('--scratch-directory', type=pathlib.Path,
                        help='Explicit dedicated private directory for temporary decrypted verification only; archives and keys stay in .private')
    commands = parser.add_subparsers(dest='command', required=True)
    commands.add_parser('keygen')
    backup = commands.add_parser('create')
    backup.add_argument('--output', type=pathlib.Path)
    backup.add_argument('--max-stop-seconds', type=int, default=1800)
    restore = commands.add_parser('restore-check')
    restore.add_argument('--archive', type=pathlib.Path, required=True)
    restore.add_argument('--report', type=pathlib.Path)
    inspect = commands.add_parser('inspect')
    inspect.add_argument('--archive', type=pathlib.Path, required=True)
    args = parser.parse_args()
    if args.scratch_directory is not None:
        args.scratch_directory = owned_scratch_directory(args.scratch_directory)
    if args.command == 'keygen':
        print(json.dumps({'key': str(args.key), 'keyID': generate_key(args.key), 'keepSeparately': True}))
    elif args.command == 'create':
        require(120 <= args.max_stop_seconds <= 7200, 'Stop interval must be 120..7200 seconds')
        create(args)
    elif args.command == 'restore-check':
        restore_check(args)
    else:
        archive = private_path(args.archive, make_parent=False)
        with decrypted_archive(archive, load_key(args.key, archive), scratch_directory=args.scratch_directory) as (_payload, metadata, manifest):
            print(json.dumps({**metadata, 'ciphertextSha256': file_sha(archive), 'sourceArchiveID': manifest['archiveID'],
                              'sourceInstanceID': manifest['instanceID'], 'files': len(manifest['files']),
                              'consistency': manifest['consistency'], 'authenticated': True}))


if __name__ == '__main__':
    try:
        main()
    except Exception as error:
        print(json.dumps({'failed': True, 'category': type(error).__name__, 'message': str(error)}), file=sys.stderr)
        sys.exit(1)
