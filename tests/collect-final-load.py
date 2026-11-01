"""Collect only the fixed final-run evidence after it finishes. Linux host only.

No recursion, fixture/private-directory reads, symlinks, extraction or source
changes. Publishes a suxin-readable sanitized archive only after all checks pass.
"""
from __future__ import annotations

import gzip
import hashlib
import io
import json
import os
import pathlib
import re
import stat
import tarfile
import tempfile

ROOT = pathlib.Path('/srv/openim38-upgrade')
RUN = 'final-100-20261101a'
SOURCE = ROOT / 'tests/load-final' / RUN
OUTPUT = pathlib.Path('/tmp/openim38-final-load-results.tar.gz')
MEMBERS = (
    'run.json', 'summary.json', 'barrier.json', 'resources.jsonl',
    'running-binaries-before.json', 'running-binaries-after.json',
    'static-server.log', 'linux50a.log', 'linux50b.log',
    'linux50a/progress.json', 'linux50a/summary.json',
    'linux50b/progress.json', 'linux50b/summary.json',
)
MAX_FILE_BYTES = 32 * 1024**2
MAX_TOTAL_BYTES = 128 * 1024**2
SENSITIVE_KEYS = {
    'token', 'tokens', 'imtoken', 'chattoken', 'admintoken', 'accesstoken', 'refreshtoken',
    'idtoken', 'password', 'passwd', 'pwd', 'secret', 'clientsecret', 'privatekey',
    'credential', 'credentials', 'apikey', 'accesskey', 'secretkey',
}
PRIVATE_PEM = re.compile(
    rb'-----BEGIN ((?:[A-Z0-9]+ )*PRIVATE KEY(?: BLOCK)?)-----'
    rb'(?:[ \t\r\n]|\\[rn])+[A-Za-z0-9+/=][A-Za-z0-9+/= \t\r\n\\rn]{30,}?'
    rb'-----END \1-----'
)
PRIVATE_PEM_HEADER = re.compile(rb'-----BEGIN (?:[A-Z0-9]+ )*PRIVATE KEY(?: BLOCK)?-----')
JWT = re.compile(rb'(?<![A-Za-z0-9_-])eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}(?![A-Za-z0-9_-])')
JSON_FIELD = re.compile(r'(?:\\?["\'])([A-Za-z_][A-Za-z0-9_-]*)(?:\\?["\'])\s*:', re.ASCII)


def sensitive_key(value):
    return re.sub(r'[^a-z0-9]', '', value.lower()) in SENSITIVE_KEYS


def scan_json(value):
    if isinstance(value, dict):
        for key, child in value.items():
            if sensitive_key(key):
                raise ValueError('Sensitive credential field')
            scan_json(child)
    elif isinstance(value, list):
        for child in value:
            scan_json(child)
    elif isinstance(value, str):
        # Catch JSON serialized again inside an error/log string, without ever
        # printing a key value or the content of the string.
        if any(sensitive_key(match.group(1)) for match in JSON_FIELD.finditer(value)):
            raise ValueError('Serialized sensitive credential field')


def scan_payload(name, data):
    if PRIVATE_PEM_HEADER.search(data) or PRIVATE_PEM.search(data) or JWT.search(data):
        raise ValueError('Private key or signed-token material')
    text = data.decode('utf-8')
    if any(sensitive_key(match.group(1)) for match in JSON_FIELD.finditer(text)):
        raise ValueError('Sensitive credential field in text')
    if name.endswith('.json'):
        scan_json(json.loads(text))
    elif name.endswith('.jsonl'):
        for line in text.splitlines():
            if line.strip():
                scan_json(json.loads(line))
    else:
        for line in text.splitlines():
            if line.lstrip().startswith(('{', '[')):
                try:
                    value = json.loads(line)
                except json.JSONDecodeError:
                    continue
                scan_json(value)


def identity(value):
    return value.st_dev, value.st_ino, value.st_size, value.st_mtime_ns


def read_member(base, relative):
    if relative not in MEMBERS:
        raise ValueError('Member is outside the explicit result allowlist')
    path = base / pathlib.PurePosixPath(relative)
    if path.resolve() != path or not path.resolve().is_relative_to(base):
        raise ValueError('Symlink or path escape refused')
    initial = path.lstat()
    if not stat.S_ISREG(initial.st_mode) or initial.st_size > MAX_FILE_BYTES:
        raise ValueError('Only bounded ordinary evidence files are accepted')
    descriptor = os.open(path, os.O_RDONLY | getattr(os, 'O_NOFOLLOW', 0))
    with os.fdopen(descriptor, 'rb') as stream:
        opened = os.fstat(stream.fileno())
        if identity(opened) != identity(initial):
            raise ValueError('Evidence identity changed before reading')
        data = stream.read(MAX_FILE_BYTES + 1)
        after = os.fstat(stream.fileno())
    if len(data) > MAX_FILE_BYTES or identity(after) != identity(initial) or identity(path.lstat()) != identity(initial):
        raise ValueError('Evidence changed while reading')
    scan_payload(relative, data)
    return data, initial


def collect_members(base):
    base = pathlib.Path(base)
    if base.resolve() != base or not base.is_dir():
        raise ValueError('Final evidence root must be a real directory')
    captured, total = {}, 0
    for name in MEMBERS:
        data, metadata = read_member(base, name)
        total += len(data)
        if total > MAX_TOTAL_BYTES:
            raise ValueError('Evidence exceeds the bounded result size')
        captured[name] = (data, metadata)
    run = json.loads(captured['run.json'][0])
    if run.get('runID') != RUN or run.get('phase') not in ('complete', 'failed'):
        raise ValueError('Final coordinator has not reached a terminal state')
    return captured


def publish(captured, output):
    if output.parent.resolve() != pathlib.Path('/tmp') or output.name != OUTPUT.name:
        raise ValueError('Unexpected archive destination')
    if output.exists() or output.is_symlink():
        raise ValueError('An archive already exists; preserve it rather than overwrite it')
    descriptor, temporary_name = tempfile.mkstemp(prefix='openim38-final-load-results-', suffix='.partial', dir='/tmp')
    temporary = pathlib.Path(temporary_name)
    try:
        with os.fdopen(descriptor, 'wb') as raw:
            with gzip.GzipFile(filename='', mode='wb', fileobj=raw, mtime=0) as compressed:
                with tarfile.open(fileobj=compressed, mode='w') as archive:
                    for name, (data, metadata) in captured.items():
                        info = tarfile.TarInfo(RUN + '/' + name)
                        info.size = len(data); info.mode = 0o644
                        info.uid = info.gid = 0; info.mtime = int(metadata.st_mtime)
                        archive.addfile(info, io.BytesIO(data))
            raw.flush(); os.fsync(raw.fileno())
        # Atomic no-overwrite publication; a competing path or symlink causes
        # refusal. The inspected bytes are already in memory, not reread here.
        os.link(temporary, output)
        os.chmod(output, 0o644)
    finally:
        temporary.unlink(missing_ok=True)


def main():
    if ROOT.resolve() != ROOT or SOURCE.resolve() != SOURCE:
        raise RuntimeError('Expected the real isolated final-run directory')
    captured = collect_members(SOURCE)
    publish(captured, OUTPUT)
    with OUTPUT.open('rb') as stream:
        digest = hashlib.file_digest(stream, 'sha256').hexdigest()
    # No report bodies, token/password values, private directories or Docker env.
    print(json.dumps({'sha256': digest, 'bytes': OUTPUT.stat().st_size,
                      'members': [{'name': RUN + '/' + name, 'bytes': len(data),
                                   'sha256': hashlib.sha256(data).hexdigest()}
                                  for name, (data, _) in captured.items()]}, indent=2))


if __name__ == '__main__':
    main()
