"""Install a verified immutable runtime ZIP through the normal HRM admin API."""
import argparse
import base64
import hashlib
import json
import pathlib
import re
import stat
import time
import urllib.error
import urllib.request
import uuid
import zipfile


def hrm_client():
    # Validation and offline tests must not load any production session/config.
    import production_hrm_acceptance
    return production_hrm_acceptance


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        raise RuntimeError('Unexpected redirect during authenticated plugin installation')


def request(path, data, content_type):
    hrm = hrm_client()
    timestamp = str(int(time.time()))
    token = json.loads((hrm.PRIVATE / 'hrm-session.json').read_text())['token']
    verify = hashlib.md5((hrm.VERIFY + timestamp).encode()).hexdigest()
    headers = {
        'apiverify': base64.b64encode((verify + '#' + timestamp).encode()).decode(),
        'Authorization': hrm.crypt(token + '#' + timestamp),
        'locale': 'zh-CN',
        'Content-Type': content_type,
    }
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}),
        urllib.request.HTTPSHandler(context=hrm.context), NoRedirect())
    req = urllib.request.Request(hrm.ORIGIN + path, data=data, headers=headers, method='POST')
    try:
        with opener.open(req, timeout=300) as response:
            return response.status, json.loads(response.read(2 << 20))
    except urllib.error.HTTPError as error:
        return error.code, json.loads(error.read(2 << 20))


def validate_package(raw, expected, identity):
    import io
    def require(ok, message):
        if not ok:
            raise RuntimeError(message)
    require(isinstance(expected, str) and re.fullmatch(r'[0-9a-f]{64}', expected)
            and isinstance(identity, str) and re.fullmatch(r'[0-9a-f]{64}', identity), 'Exact package digests required')
    require(len(raw) < 160 << 20 and hashlib.sha256(raw).hexdigest() == expected, 'Archive digest or size mismatch')
    with zipfile.ZipFile(io.BytesIO(raw)) as package:
        rows, seen = {}, set()
        require(1 < len(package.infolist()) <= 3000
                and sum(row.file_size for row in package.infolist()) <= 320 << 20, 'Runtime ZIP limits exceeded')
        for item in package.infolist():
            name = item.filename[:-1] if item.is_dir() else item.filename
            path = pathlib.PurePosixPath(name)
            require(name and path.as_posix() == name and not path.is_absolute() and '\\' not in name
                    and all(part and not part.startswith('.') and part.rstrip('. ') == part
                            and ':' not in part and not any(ord(c) < 32 for c in part) for part in path.parts)
                    and name.casefold() not in seen, 'Unsafe or duplicate runtime path')
            seen.add(name.casefold())
            require(stat.S_IFMT(item.external_attr >> 16) in (0, stat.S_IFDIR, stat.S_IFREG)
                    and not item.flag_bits & 1, 'Special or encrypted runtime entry')
            if not item.is_dir():
                require(name == 'plugin.json' or name.startswith(('bin/', 'admin/', 'public/'))
                        or name in ('README.md', 'LICENSE.txt'), 'Unexpected runtime file')
                rows[name] = item
        require('plugin.json' in rows and rows['plugin.json'].file_size <= 1 << 20, 'Runtime manifest missing')
        manifest = package.read(rows['plugin.json'])
        require(hashlib.sha256(manifest).hexdigest() == identity, 'Manifest identity mismatch')
        metadata = json.loads(manifest)
        files = metadata.get('files')
        require(metadata.get('name') == 'biz-suite' and metadata.get('format') == 'suxin-runtime-v1'
                and metadata.get('protocol') == 1 and metadata.get('os') == 'linux' and metadata.get('arch') == 'amd64'
                and isinstance(metadata.get('version'), str) and metadata['version']
                and isinstance(files, dict) and 'bin/biz-suite' in files
                and set(rows) == set(files) | {'plugin.json'}, 'Runtime manifest or exact file set invalid')
        require(len({n.casefold() for n in files}) == len(files), 'Casefold duplicate manifest path')
        for name, file_digest in files.items():
            require(isinstance(file_digest, str) and re.fullmatch(r'[0-9a-f]{64}', file_digest)
                    and hashlib.sha256(package.read(rows[name])).hexdigest() == file_digest,
                    'Package file digest mismatch')
        return metadata


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--zip', required=True)
    parser.add_argument('--sha256', required=True)
    parser.add_argument('--package-identity', required=True)
    parser.add_argument('--receipt-prefix', default='plugin-logo')
    args = parser.parse_args()
    assert re.fullmatch(r'[a-z0-9][a-z0-9-]{0,63}', args.receipt_prefix), 'Invalid receipt prefix'
    archive = pathlib.Path(args.zip)
    assert archive.is_file() and not archive.is_symlink() and archive.resolve() == archive.absolute(), 'Archive redirected'
    assert archive.stat().st_size < 160 << 20, 'Archive too large'
    raw = archive.read_bytes()
    validate_package(raw, args.sha256, args.package_identity)
    hrm = hrm_client()
    boundary = 'puty-' + uuid.uuid4().hex
    prefix = (f'--{boundary}\r\nContent-Disposition: form-data; name="file"; '
              f'filename="biz-suite-runtime.zip"\r\nContent-Type: application/zip\r\n\r\n').encode()
    upload = prefix + raw + f'\r\n--{boundary}--\r\n'.encode()
    status, body = request('/admin/developer/packinstall/installLocalCode', upload,
                           'multipart/form-data; boundary=' + boundary)
    (hrm.PRIVATE / (args.receipt_prefix + '-stage-response.json')).write_text(json.dumps(body), encoding='utf-8')
    stage = body.get('data')
    assert status == 200 and body.get('code') == 0, f'Plugin upload rejected: HTTP {status}, code {body.get("code")}'
    assert isinstance(stage, str) and re.fullmatch(r'runtime-[0-9a-f]{48}', stage), 'Invalid staged package identifier'
    print(json.dumps({'uploaded': True, 'archiveVerified': True, 'stagedIdentifierValidated': True}), flush=True)
    status, body = request('/admin/developer/packinstall/installCode',
                           json.dumps({'name': stage}).encode(), 'application/json')
    (hrm.PRIVATE / (args.receipt_prefix + '-install-response.json')).write_text(json.dumps(body), encoding='utf-8')
    assert status == 200 and body.get('code') == 0, f'Plugin activation rejected: HTTP {status}, code {body.get("code")}'
    print(json.dumps({'installed': True, 'status': status, 'code': body.get('code'),
                      'packageIdentity': args.package_identity}), flush=True)


if __name__ == '__main__':
    main()
