#!/usr/bin/env python3
"""Scoped runtime-key rotation. Default plan is read-only; never logs key material."""
import argparse
import base64
import contextlib
import copy
import datetime
import hashlib
import hmac
import json
import os
import pathlib
import re
import secrets
import signal
import socket
import ssl
import subprocess
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

BASE = pathlib.Path('/srv/puty-suite')
PROJECT = 'puty-suite-prod'
SUPERVISOR = '/www/server/panel/pyenv/bin/supervisorctl'
NAMES = ('mongo', 'redis', 'minio', 'im', 'livekit', 'webhook', 'session', 'bridge')
KINDS = ('hrm_generation', 'media_grant', 'session_revocation')
STORAGE = {'mysql', 'mongo', 'redis', 'etcd', 'kafka', 'minio'}
CHANGED_STORAGE = ('mongo', 'redis', 'minio')
FILES = ('openim/secrets.json', 'openim/compose.yaml',
         'openim/config/openim-server/mongodb.yml', 'openim/config/openim-server/redis.yml',
         'openim/config/openim-server/share.yml', 'openim/config/openim-server/minio.yml',
         'openim/config/openim-server/webhooks.yml', 'openim/config/chat/mongodb.yml',
         'openim/config/chat/redis.yml', 'openim/config/chat/share.yml',
         'openim/config/chat/chat-rpc-chat.yml', 'openim/config/livekit.yml',
         'gosuxin/runtime-environment.json', 'gosuxin/manifest/config/config.yaml')
AAD = b'openim38-media-session'


def harden_storage_process_arguments(compose):
    """Identical semantic patch to generate_instance; no credential argv."""
    services = compose.get('services', {})
    if not all(isinstance(services.get(name), dict)
               and isinstance(services[name].get('healthcheck'), dict) for name in ('mongo', 'redis')):
        raise ValueError('storage_process_model_invalid')
    redis_script = (
        'umask 077; '
        'case "$$REDIS_PASSWORD" in ""|*[!0-9a-f]*) exit 1;; esac; '
        '[ "$${#REDIS_PASSWORD}" -eq 64 ] || exit 1; '
        'puty_redis_config=$$(mktemp /tmp/puty-redis.XXXXXX.conf); '
        "printf 'requirepass %s\\nappendonly yes\\n' "
        '"$$REDIS_PASSWORD" > "$$puty_redis_config"; '
        'exec redis-server "$$puty_redis_config"'
    )
    services['redis']['command'] = ['sh', '-ec', redis_script]
    services['redis']['healthcheck']['test'] = [
        'CMD-SHELL', 'REDISCLI_AUTH="$$REDIS_PASSWORD" redis-cli --no-auth-warning '
        '-h 127.0.0.1 ping | grep -qx PONG']
    mongo_script = (
        'const c=new Mongo("mongodb://"+'
        'encodeURIComponent(process.env.MONGO_INITDB_ROOT_USERNAME)+":"+'
        'encodeURIComponent(process.env.MONGO_INITDB_ROOT_PASSWORD)+'
        '"@127.0.0.1:27017/admin");'
        'quit(c.getDB("admin").runCommand({ping:1}).ok===1?0:1);'
    )
    services['mongo']['healthcheck']['test'] = [
        'CMD', 'mongosh', '--nodb', '--quiet', '--eval', mongo_script]
    return compose


def require(ok, code):
    if not ok:
        raise RuntimeError(code)


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def canonical(value):
    return json.dumps(value, sort_keys=True, separators=(',', ':')).encode()


def sync_parent_directory(path):
    if os.name == 'posix':
        descriptor = os.open(path.parent, os.O_RDONLY | os.O_DIRECTORY)
        try:
            os.fsync(descriptor)
        finally:
            os.close(descriptor)


def private_write(path, raw):
    require(not path.is_symlink() and path.parent.resolve() == path.parent,
            'linked_private_path_refused')
    temporary = path.with_name(path.name+'.'+secrets.token_hex(6)+'.tmp')
    descriptor = os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    try:
        with os.fdopen(descriptor, 'wb') as output:
            output.write(raw)
            output.flush()
            os.fsync(output.fileno())
        os.replace(temporary, path)
        sync_parent_directory(path)
    finally:
        if temporary.exists():
            temporary.unlink()


def save(path, value):
    private_write(path, json.dumps(value, indent=2).encode())


def execute(args, body=None, timeout=180):
    # argv contains paths and fixed commands only. Sensitive scripts use stdin.
    try:
        result = subprocess.run(args, input=body, stdout=subprocess.PIPE,
                                stderr=subprocess.PIPE, timeout=timeout)
    except Exception:
        raise RuntimeError('required_subprocess_unavailable') from None
    require(result.returncode == 0, 'required_subprocess_failed')
    return result.stdout


def bundle_valid(bundle):
    require(bundle.get('format') == 'puty-runtime-key-rotation-v1'
            and re.fullmatch(r'rotate-[0-9a-f]{24}', bundle.get('rotationID', ''))
            and re.fullmatch(r'[0-9a-f]{32}', bundle.get('instanceID', ''))
            and set(bundle.get('old', {})) == set(bundle.get('new', {})) == set(NAMES),
            'rotation_bundle_schema_mismatch')
    values = [bundle[side][name] for side in ('old', 'new') for name in NAMES]
    require(all(isinstance(value, str) and re.fullmatch(r'[0-9a-f]{64}', value) for value in values)
            and len(set(values)) == len(values), 'rotation_keys_must_be_unique_256_bit_hex')


def render_files(root, bundle, local=False):
    bundle_valid(bundle)
    paths = (*FILES, 'suite-secrets.json') if local else FILES
    rows = []
    counts = {name: 0 for name in NAMES}
    for name in paths:
        path = root / name
        require(path.is_file() and not path.is_symlink() and path.resolve().is_relative_to(root.resolve())
                and path.stat().st_size <= 2 * 1024 * 1024, 'runtime_input_not_regular_owned_file')
        before = path.read_bytes()
        after = before
        changes = {}
        for key in NAMES:
            old, new = bundle['old'][key].encode(), bundle['new'][key].encode()
            number = before.count(old)
            if number:
                after = after.replace(old, new)
                changes[key] = number
                counts[key] += number
        if name in ('openim/secrets.json', 'suite-secrets.json', 'gosuxin/runtime-environment.json'):
            left, right = json.loads(before), json.loads(after)
            if name == 'gosuxin/runtime-environment.json':
                allowed = {'GOSUXIN_IM_HMAC_KEY', 'GOSUXIN_IM_REDIS_URL'}
                require(left.keys() == right.keys() and all(left[key] == right[key] for key in left if key not in allowed),
                        'non_scoped_HRM_environment_changed')
                old_url, new_url = urllib.parse.urlsplit(left['GOSUXIN_IM_REDIS_URL']), urllib.parse.urlsplit(right['GOSUXIN_IM_REDIS_URL'])
                require((old_url.scheme, old_url.hostname, old_url.port, old_url.path, old_url.username, old_url.query, old_url.fragment)
                        == (new_url.scheme, new_url.hostname, new_url.port, new_url.path, new_url.username, new_url.query, new_url.fragment)
                        and old_url.path == '/6' and old_url.password == bundle['old']['redis']
                        and new_url.password == bundle['new']['redis']
                        and right['GOSUXIN_IM_HMAC_KEY'] == bundle['new']['bridge'], 'HRM_Redis_database_or_bridge_contract_changed')
            else:
                allowed = {'bridge'} if name == 'suite-secrets.json' else set(NAMES)-{'bridge'}
                require(left.keys() == right.keys() and all(left[key] == right[key] for key in left if key not in allowed),
                        'non_scoped_generator_material_changed')
        if name == 'openim/compose.yaml':
            import yaml
            baseline = yaml.safe_load(after)
            proposed = harden_storage_process_arguments(copy.deepcopy(baseline))
            unchanged = copy.deepcopy(proposed)
            for service, section, field in (('redis', None, 'command'), ('redis', 'healthcheck', 'test'),
                                             ('mongo', 'healthcheck', 'test')):
                if section is None:
                    unchanged['services'][service][field] = baseline['services'][service][field]
                else:
                    unchanged['services'][service][section][field] = baseline['services'][service][section][field]
            require(unchanged == baseline, 'Compose_delta_exceeds_three_argv_fields')
            after = yaml.safe_dump(proposed, allow_unicode=True, sort_keys=False).encode()
        require(all(bundle['old'][key].encode() not in after for key in NAMES),
                'old_runtime_key_remains_in_rendered_file')
        if before != after:
            stat = path.stat()
            rows.append({'path': name, 'before': base64.b64encode(before).decode(),
                         'after': base64.b64encode(after).decode(), 'beforeSHA256': sha(before),
                         'afterSHA256': sha(after), 'mode': stat.st_mode & 0o777,
                         'uid': stat.st_uid, 'gid': stat.st_gid, 'changes': changes})
    require(all(counts.values()), 'runtime_key_propagation_incomplete')
    return rows


def rewrap_documents(raw_documents, old_key, new_key):
    from cryptography.hazmat.primitives.ciphers.aead import AESGCM
    from cryptography.exceptions import InvalidTag
    old, new = AESGCM(bytes.fromhex(old_key)), AESGCM(bytes.fromhex(new_key))
    rows, nonces = [], set()
    require(len(raw_documents) <= 10000, 'encrypted_document_count_exceeds_review_bound')
    for raw in raw_documents:
        document = json.loads(raw)
        require(document.get('kind') in KINDS and isinstance(document.get('_id'), str),
                'encrypted_document_kind_or_identity_mismatch')
        binary = base64.b64decode(document['data']['$binary']['base64'], validate=True)
        data = json.loads(binary)
        encrypted = base64.b64decode(data['Ciphertext'], validate=True)
        require(len(encrypted) >= 28, 'encrypted_document_ciphertext_invalid')
        clear = old.decrypt(encrypted[:12], encrypted[12:], AAD)
        require(isinstance(json.loads(clear), dict), 'encrypted_actor_not_JSON_object')
        nonce = secrets.token_bytes(12)
        require(nonce not in nonces, 'duplicate_rotation_nonce')
        nonces.add(nonce)
        replacement = nonce + new.encrypt(nonce, clear, AAD)
        require(new.decrypt(replacement[:12], replacement[12:], AAD) == clear,
                'rewrapped_actor_changed')
        try:
            old.decrypt(replacement[:12], replacement[12:], AAD)
        except InvalidTag:
            pass
        else:
            raise RuntimeError('old_AES_key_accepted_rewrapped_document')
        # Preserve every byte outside the unique ciphertext string, including
        # unknown fields, whitespace, and large/scientific numeric literals.
        pattern = re.compile(rb'("Ciphertext"\s*:\s*")'+re.escape(data['Ciphertext'].encode())+rb'(")')
        require(len(pattern.findall(binary)) == 1, 'ciphertext_field_not_unique')
        binary = pattern.sub(lambda match: match[1]+base64.b64encode(replacement)+match[2], binary, count=1)
        proposed = json.loads(raw)
        proposed['data']['$binary']['base64'] = base64.b64encode(binary).decode()
        rows.append({'before': raw, 'after': json.dumps(proposed, separators=(',', ':')),
                     'kind': document['kind']})
    return rows


def mongo(container, password, code):
    config = json.dumps({'password': password})
    script = 'const cfg='+config+'; const conn=new Mongo("mongodb://root:"+encodeURIComponent(cfg.password)+"@127.0.0.1:27017/admin"); const db=conn.getDB("enterprise38");\n'+code
    raw = execute(['docker', 'exec', '-i', container, 'mongosh', '--nodb', '--quiet', '--file', '/dev/stdin'], script.encode(), 180)
    try:
        return json.loads(raw)
    except Exception:
        raise RuntimeError('private_Mongo_result_invalid') from None


def mongo_auth(container, password):
    try:
        return mongo(container, password, 'print(JSON.stringify({ok:conn.getDB("admin").runCommand({ping:1}).ok===1}));') == {'ok': True}
    except RuntimeError:
        return False


def mongo_scan(container, password):
    return mongo(container, password, 'const rows=db.enterprise_documents.find({kind:{$in:'+json.dumps(KINDS)+'}}).sort({_id:1}).limit(10001).toArray(); print(JSON.stringify(rows.map(d=>EJSON.stringify(d,{relaxed:false}))));')


def mongo_password(container, current, proposed):
    mongo(container, current, 'conn.getDB("admin").updateUser("root",{pwd:'+json.dumps(proposed)+'}); print(JSON.stringify({ok:true}));')


def mongo_rewrap(container, password, rows, reverse=False):
    # Full BSON document CAS; only its binary JSON data field is changed.
    pairs = [{'from': row['after'] if reverse else row['before'],
              'to': row['before'] if reverse else row['after']} for row in rows]
    script = 'const pairs='+json.dumps(pairs)+'; let changed=0,unchanged=0; for(const p of pairs){const before=EJSON.parse(p.from,{relaxed:false}),after=EJSON.parse(p.to,{relaxed:false}); if(db.enterprise_documents.countDocuments(after)===1){unchanged++;continue;} const r=db.enterprise_documents.updateOne(before,{$set:{data:after.data}}); if(r.matchedCount!==1){print(JSON.stringify({ok:false,changed,unchanged}));quit(1);} changed++;} print(JSON.stringify({ok:true,changed,unchanged}));'
    result = mongo(container, password, script)
    require(result.get('ok') and result['changed'] + result['unchanged'] == len(rows),
            'encrypted_document_compare_and_swap_failed')


def rollback_documents(container, password, old_key, new_key):
    # Apps may have created/updated records during final readiness. Quiesce
    # first, then rewrap CURRENT records and preserve their latest semantics.
    from cryptography.hazmat.primitives.ciphers.aead import AESGCM
    from cryptography.exceptions import InvalidTag
    old = AESGCM(bytes.fromhex(old_key))
    rows = []
    documents = mongo_scan(container, password)
    require(len(documents) <= 10000, 'rollback_document_count_exceeds_review_bound')
    for raw in documents:
        document = json.loads(raw)
        data = json.loads(base64.b64decode(document['data']['$binary']['base64'], validate=True))
        encrypted = base64.b64decode(data['Ciphertext'], validate=True)
        try:
            old.decrypt(encrypted[:12], encrypted[12:], AAD)
        except InvalidTag:
            rows.extend(rewrap_documents([raw], new_key, old_key))
    mongo_rewrap(container, password, rows)


class Redis:
    def __init__(self, password):
        self.socket = socket.create_connection(('127.0.0.1', 16379), timeout=5)
        self.stream = self.socket.makefile('rb')
        try:
            self.command('AUTH', password)
        except Exception:
            self.close()
            raise

    def command(self, *items):
        parts = [str(item).encode() for item in items]
        self.socket.sendall(b'*'+str(len(parts)).encode()+b'\r\n'+b''.join(b'$'+str(len(p)).encode()+b'\r\n'+p+b'\r\n' for p in parts))
        line = self.stream.readline()
        require(line and line[:1] != b'-', 'private_Redis_command_rejected')
        if line[:1] == b'$':
            size = int(line[1:-2])
            data = self.stream.read(size)
            self.stream.read(2)
            return data
        return line[1:-2]

    def close(self):
        self.stream.close()
        self.socket.close()


def redis_auth(password):
    try:
        client = Redis(password)
        try:
            return client.command('PING') == b'PONG'
        finally:
            client.close()
    except Exception:
        return False


def redis_password(current, proposed):
    client = Redis(current)
    try:
        require(client.command('CONFIG', 'SET', 'requirepass', proposed) == b'OK',
                'Redis_password_update_failed')
    finally:
        client.close()


def compose(*args):
    return execute(['docker', 'compose', '--project-directory', str(BASE/'openim'), '-f',
                    str(BASE/'openim/compose.yaml'), '-p', PROJECT, *args], timeout=300)


def all_containers(owner):
    ids = execute(['docker', 'ps', '-aq', '--filter', 'label=com.docker.compose.project='+PROJECT]).decode().split()
    require(ids, 'production_containers_missing')
    values = json.loads(execute(['docker', 'inspect', *ids]))
    for item in values:
        labels = item['Config'].get('Labels') or {}
        service = labels.get('com.docker.compose.service')
        require(labels.get('com.puty.suite') == owner and labels.get('com.docker.compose.project') == PROJECT and service,
                'foreign_production_container')
    return values


def current_containers(owner):
    result = {}
    for item in all_containers(owner):
        service = item['Config']['Labels']['com.docker.compose.service']
        require(service not in result, 'duplicate_production_container_requires_reconciliation')
        result[service] = item
    return result


def validate_storage_ownership(owner, expected=None, allow_missing=False):
    spec = json.loads(compose('config', '--format', 'json'))
    require(spec.get('name') == PROJECT and set(spec.get('volumes', {})) == {name+'-data' for name in STORAGE},
            'storage_project_or_logical_volumes_mismatch')
    volumes = {}
    for logical, definition in spec['volumes'].items():
        name = definition.get('name', '')
        require(re.fullmatch(r'[A-Za-z0-9_.-]+', name), 'storage_volume_name_invalid')
        value = json.loads(execute(['docker', 'volume', 'inspect', name]))[0]
        mount = pathlib.Path(value.get('Mountpoint', ''))
        require(value.get('Name') == name and value.get('Driver') == 'local'
                and (value.get('Labels') or {}).get('com.puty.suite') == owner
                and mount == pathlib.Path('/var/lib/docker/volumes')/name/'_data'
                and mount.is_dir() and mount.resolve() == mount, 'foreign_or_redirected_storage_volume')
        volumes[logical] = {'name': name, 'mountpoint': str(mount)}
    if expected is not None:
        require(volumes == expected, 'reviewed_storage_volume_identity_changed')
    for service in spec['services'].values():
        for bind in service.get('volumes', []):
            if bind.get('type') == 'bind':
                require(pathlib.Path(bind['source']).resolve().is_relative_to(BASE), 'active_bind_mount_leaves_owned_suite')
    found = set()
    for item in all_containers(owner):
        service = item['Config']['Labels']['com.docker.compose.service']
        require(service in spec['services'], 'unconfigured_owned_container_refused')
        for mount in item.get('Mounts', []):
            if mount.get('Type') == 'bind':
                require(pathlib.Path(mount['Source']).resolve().is_relative_to(BASE), 'container_bind_mount_leaves_owned_suite')
        if service in STORAGE:
            found.add(service)
            actual = volumes[service+'-data']
            declared = [mount for mount in spec['services'][service].get('volumes', [])
                        if mount.get('type') == 'volume' and mount.get('source') == service+'-data']
            require(len(declared) == 1, 'storage_data_mount_not_uniquely_declared')
            require(any(mount.get('Type') == 'volume' and mount.get('Name') == actual['name']
                        and mount.get('Source') == actual['mountpoint'] and mount.get('Destination') == declared[0]['target']
                        and mount.get('RW') is True for mount in item.get('Mounts', [])),
                    'storage_container_uses_foreign_or_wrong_volume')
    require(allow_missing or STORAGE <= found, 'storage_container_missing_before_maintenance')
    return volumes


def wait_containers(owner, services):
    for _ in range(90):
        current = current_containers(owner)
        if all(name in current and current[name]['State']['Running']
               and current[name]['State'].get('Health', {}).get('Status') not in ('starting', 'unhealthy')
               for name in services):
            return current
        time.sleep(2)
    raise RuntimeError('rotated_containers_not_healthy')


def restart_services(plan):
    owner, writers = plan['instanceID'], plan['writers']
    validate_storage_ownership(owner, plan['volumes'], allow_missing=True)
    compose('up', '-d', '--no-build', '--no-deps', '--force-recreate', *CHANGED_STORAGE)
    wait_containers(owner, CHANGED_STORAGE)
    rpc = [name for name in writers if name.startswith('openim-rpc-') or name in ('chat-admin', 'chat-business')]
    remaining = [name for name in writers if name not in rpc and name != 'enterprise-service']
    if rpc:
        compose('up', '-d', '--no-build', '--no-deps', '--force-recreate', *rpc)
        wait_containers(owner, rpc)
        time.sleep(5)
    if remaining:
        compose('up', '-d', '--no-build', '--no-deps', '--force-recreate', *remaining)
        wait_containers(owner, remaining)
    if 'enterprise-service' in writers:
        compose('up', '-d', '--no-build', '--no-deps', '--force-recreate', 'enterprise-service')
        wait_containers(owner, ['enterprise-service'])
    if plan['hrmRunning']:
        requested = False
        for _ in range(90):
            state = hrm_state()
            if state == 'RUNNING':
                break
            if state in ('STOPPED', 'EXITED', 'FATAL') and not requested:
                execute([SUPERVISOR, 'start', 'puty-hrm'], timeout=90)
                requested = True
            time.sleep(1)
        else:
            raise RuntimeError('HRM_supervisor_did_not_reach_RUNNING')


def install_files(rows, reverse=False):
    for row in rows:
        require(row['path'] in FILES, 'runtime_file_outside_rotation_allowlist')
        path = BASE / row['path']
        raw = path.read_bytes()
        expected = (row['beforeSHA256'], row['afterSHA256'])
        require(not path.is_symlink() and path.resolve().is_relative_to(BASE)
                and sha(raw) in expected, 'runtime_file_changed_outside_rotation')
        proposed = base64.b64decode(row['before' if reverse else 'after'])
        private_write(path, proposed)
        os.chmod(path, row['mode'])
        os.chown(path, row['uid'], row['gid'])


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *args, **kwargs):
        return None


def plain_request(url, body=None, headers=None, method=None):
    request = urllib.request.Request(url, data=body, headers={'operationID': secrets.token_hex(16), **(headers or {})}, method=method)
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), NoRedirect())
    try:
        response = opener.open(request, timeout=8)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        return response.status, response.read(2*1024*1024)


def warm_authentication_entry():
    # Traverse the actual TLS/IP Nginx entry and its trusted client-IP headers.
    # Fixed invalid credentials cannot create a session or touch employee data.
    context = ssl.create_default_context()
    context.load_verify_locations(cafile=str(BASE/'openim/certs/puty-root.crt'))
    invalid = 'puty-rotation-fixed-invalid-credential'
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), NoRedirect(), urllib.request.HTTPSHandler(context=context))
    def request(path, value, headers=None, method=None):
        req = urllib.request.Request('https://192.168.10.11:9885'+path, canonical(value) if value is not None else None,
                                     {'Content-Type': 'application/json', 'operationID': secrets.token_hex(16), **(headers or {})}, method=method)
        try:
            response = opener.open(req, timeout=8)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            return response.status, json.loads(response.read(65536))
    code, body = request('/enterprise/v1/signaling/get_signal_invitation_info_start_app', {},
                         {'token': invalid, 'chatToken': invalid, 'platformID': '2'})
    require(code == 401 and body.get('errCode') == 1501
            and body.get('errMsg') == 'Chat session expired or revoked', 'Chat_RPC_TLS_entry_not_ready')
    code, body = request('/api/auth/parse_token', {'token': invalid})
    require(invalid_native_rejected(code, body), 'OpenIM_RPC_TLS_entry_not_ready')
    code, body = request('/sso/captcha', None, method='GET')
    require(code == 200 and body.get('code') == 0, 'HRM_anonymous_TLS_entry_not_ready')
    return True


def invalid_native_rejected(status, body):
    return status == 200 and body.get('errCode') == 1503 and body.get('errMsg') == 'TokenMalformedError'


def old_im_signature_rejected(status, body):
    # Locked tools alpha.122 maps signature ValidationError to code 1505;
    # unlike expired/malformed/transport errors, its fixed message names it.
    return (status == 200 and body.get('errCode') == 1505
            and 'signature is invalid' in str(body.get('errMsg', '')))


def unregistered_im_signature_verified(status, body):
    return status == 200 and body.get('errCode') == 1507 and body.get('errMsg') == 'TokenNotExistError'


def im_key_token(secret):
    encode = lambda value: base64.urlsafe_b64encode(value).rstrip(b'=')
    now = int(time.time())
    claims = {'UserID': 'puty-runtime-key-proof-unregistered', 'PlatformID': 2, 'exp': now+60,
              'iat': now-5, 'jti': secrets.token_hex(16)}
    unsigned = encode(b'{"alg":"HS256","typ":"JWT"}')+b'.'+encode(canonical(claims))
    return (unsigned+b'.'+encode(hmac.new(secret.encode(), unsigned, hashlib.sha256).digest())).decode()


def signed_bridge(key):
    path = '/internal/hrm/rotation-validation-unknown-route'
    raw = b'{}'
    stamp, nonce = str(int(time.time())), secrets.token_hex(32)
    signature = hmac.new(key.encode(), '\n'.join((stamp, nonce, 'POST', path, sha(raw))).encode(), hashlib.sha256).hexdigest()
    return plain_request('http://127.0.0.1:10010'+path, raw,
                         {'Content-Type': 'application/json', 'X-HRM-Timestamp': stamp,
                          'X-HRM-Nonce': nonce, 'X-HRM-Signature': signature})[0]


def minio_head(secret):
    now = datetime.datetime.now(datetime.timezone.utc)
    stamp, day = now.strftime('%Y%m%dT%H%M%SZ'), now.strftime('%Y%m%d')
    empty, host, path = sha(b''), '127.0.0.1:19000', '/openim38'
    canonical_request = '\n'.join(('HEAD', path, '', 'host:'+host+'\nx-amz-content-sha256:'+empty+'\nx-amz-date:'+stamp+'\n', 'host;x-amz-content-sha256;x-amz-date', empty))
    scope, signing = day+'/us-east-1/s3/aws4_request', ('AWS4'+secret).encode()
    for value in (day, 'us-east-1', 's3', 'aws4_request'):
        signing = hmac.new(signing, value.encode(), hashlib.sha256).digest()
    signature = hmac.new(signing, ('AWS4-HMAC-SHA256\n'+stamp+'\n'+scope+'\n'+sha(canonical_request.encode())).encode(), hashlib.sha256).hexdigest()
    return plain_request('http://'+host+path, headers={'X-Amz-Date': stamp,
                         'X-Amz-Content-Sha256': empty, 'Authorization': 'AWS4-HMAC-SHA256 Credential=root/'+scope+', SignedHeaders=host;x-amz-content-sha256;x-amz-date, Signature='+signature}, method='HEAD')[0]


def livekit_list(key_id, secret):
    encode = lambda value: base64.urlsafe_b64encode(value).rstrip(b'=')
    now = int(time.time())
    unsigned = encode(b'{"alg":"HS256","typ":"JWT"}')+b'.'+encode(canonical({'iss': key_id, 'nbf': now-5, 'exp': now+60, 'video': {'roomList': True}}))
    token = unsigned+b'.'+encode(hmac.new(secret.encode(), unsigned, hashlib.sha256).digest())
    return plain_request('http://127.0.0.1:7880/twirp/livekit.RoomService/ListRooms', b'{}',
                         {'Content-Type': 'application/json', 'Authorization': 'Bearer '+token.decode()})[0]


def application_proof(bundle, plan):
    current = current_containers(plan['instanceID'])
    mongo_id = current['mongo']['Id']
    old, new = bundle['old'], bundle['new']
    proofs = {'MongoNewAuthenticates': mongo_auth(mongo_id, new['mongo']),
              'MongoOldRejected': not mongo_auth(mongo_id, old['mongo']),
              'RedisNewAuthenticates': redis_auth(new['redis']),
              'RedisOldRejected': not redis_auth(old['redis']),
              'MinioNewAuthenticates': minio_head(new['minio']) == 200,
              'MinioOldRejected': minio_head(old['minio']) in (401, 403),
              'BridgeNewAuthenticates': signed_bridge(new['bridge']) == 404,
              'BridgeOldRejected': signed_bridge(old['bridge']) == 401}
    key_id = json.loads((BASE/'openim/secrets.json').read_text())['livekit_key']
    proofs['LiveKitNewAuthenticates'] = livekit_list(key_id, new['livekit']) == 200
    proofs['LiveKitOldRejected'] = livekit_list(key_id, old['livekit']) in (401, 403)
    for side in ('old', 'new'):
        code, raw = plain_request('http://127.0.0.1:18090/enterprise/v1/internal/openim/webhook/'+bundle[side]['webhook'], method='GET')
        body = json.loads(raw)
        proofs['WebhookNewAuthenticates' if side == 'new' else 'WebhookOldRejected'] = (
            code == 400 and body.get('errMsg') == 'POST required') if side == 'new' else (
            code == 401 and body.get('errMsg') == 'invalid webhook credential')
    for side in ('new', 'old', 'new'):
        # Never submit a raw shared secret to a protobuf DTO: upstream error
        # interceptors log failed requests. A signed short-lived token proves
        # the same key without revealing it. This non-admin identity has no
        # registered token/SID, so both outcomes are unusable as credentials.
        code, raw = plain_request('http://127.0.0.1:10002/auth/parse_token',
                                  canonical({'token': im_key_token(bundle[side]['im'])}),
                                  {'Content-Type': 'application/json'})
        result = json.loads(raw)
        accepted = unregistered_im_signature_verified(code, result)
        if side == 'new':
            require(accepted, 'runtime_proof_failed:OpenIMNewSignatureWithoutSession')
            proofs['OpenIMNewSignatureWithoutSession'] = True
        else:
            proofs['OpenIMOldRejected'] = old_im_signature_rejected(code, result)
    fresh = mongo_scan(mongo_id, new['mongo'])
    # Rewrap back in memory only: proves new-key decrypts every exact kind and
    # the old key cannot decrypt the current ciphertext. No data is changed.
    from cryptography.hazmat.primitives.ciphers.aead import AESGCM
    from cryptography.exceptions import InvalidTag
    old_cipher, new_cipher = AESGCM(bytes.fromhex(old['session'])), AESGCM(bytes.fromhex(new['session']))
    for raw in fresh:
        document = json.loads(raw)
        data = json.loads(base64.b64decode(document['data']['$binary']['base64'], validate=True))
        encrypted = base64.b64decode(data['Ciphertext'], validate=True)
        new_cipher.decrypt(encrypted[:12], encrypted[12:], AAD)
        try:
            old_cipher.decrypt(encrypted[:12], encrypted[12:], AAD)
        except InvalidTag:
            pass
        else:
            raise RuntimeError('old_AES_key_accepted_current_document')
    proofs['AllEncryptedDocumentsNewKeyVerified'] = True
    proofs['AllEncryptedDocumentsOldKeyRejected'] = True
    proofs['EnterpriseMongoReadiness'] = plain_request('http://127.0.0.1:18090/enterprise/v1/readyz')[0] == 200
    proofs['RealTLSEntryRPCWarmup'] = warm_authentication_entry()
    failed = [name for name, passed in proofs.items() if not passed]
    require(not failed, 'runtime_proof_failed:'+','.join(failed))
    return proofs


def validate_ownership(bundle):
    bundle_valid(bundle)
    require(BASE.resolve() == BASE and (BASE/'openim').resolve() == BASE/'openim', 'production_root_redirected')
    require(all((BASE/name).is_file() and not (BASE/name).is_symlink()
                and (BASE/name).resolve().is_relative_to(BASE) for name in (
                    'openim/instance.json', 'activation.json', 'openim/secrets.json', 'gosuxin/runtime-environment.json')),
            'production_identity_files_not_regular')
    owner = json.loads((BASE/'openim/instance.json').read_text())
    activation = json.loads((BASE/'activation.json').read_text())
    keys = json.loads((BASE/'openim/secrets.json').read_text())
    runtime = json.loads((BASE/'gosuxin/runtime-environment.json').read_text())
    require(owner.get('instanceID') == activation.get('instanceID') == bundle['instanceID']
            and owner.get('production') is True and owner.get('testOnly') is False
            and owner.get('stackName') == PROJECT and activation.get('startAt') == bundle['startAt']
            and runtime.get('GOSUXIN_SYNC_START_AT') == bundle['startAt'], 'production_identity_or_cutoff_mismatch')
    require(sha(canonical({k: v for k, v in keys.items() if k not in NAMES})) == bundle['preservedIMSHA256']
            and sha(runtime['WECOM_CREDENTIAL_KEY'].encode()) == bundle['preservedWeComSHA256'],
            'runtime_keys_or_preserved_identity_mismatch')
    return keys, runtime


def runtime_validate(bundle):
    keys, runtime = validate_ownership(bundle)
    require(all(keys.get(name) == bundle['old'][name] for name in NAMES if name != 'bridge')
            and runtime.get('GOSUXIN_IM_HMAC_KEY') == bundle['old']['bridge'], 'old_runtime_key_authority_mismatch')
    return current_containers(bundle['instanceID'])


def create_plan(bundle, directory):
    current = runtime_validate(bundle)
    require(all(item['State']['Running'] for item in current.values()), 'all_original_services_must_be_running')
    writers = sorted(set(current)-STORAGE)
    require('enterprise-service' in writers and all(name in current for name in CHANGED_STORAGE),
            'required_rotation_services_missing')
    status = subprocess.run([SUPERVISOR, 'status', 'puty-hrm'], capture_output=True, timeout=20)
    require(b' RUNNING ' in status.stdout, 'HRM_must_be_running_before_rotation')
    documents = rewrap_documents(mongo_scan(current['mongo']['Id'], bundle['old']['mongo']),
                                 bundle['old']['session'], bundle['new']['session'])
    files = render_files(BASE, bundle)
    proposed_hashes = {row['path']: row['afterSHA256'] for row in files}
    plan = {'rotationID': bundle['rotationID'], 'instanceID': bundle['instanceID'],
            'bundleSHA256': sha(canonical(bundle)), 'createdAt': int(time.time()),
            'files': files, 'documents': documents,
            'writers': writers, 'hrmRunning': True,
            'volumes': validate_storage_ownership(bundle['instanceID']),
            'protectedFiles': {name: proposed_hashes.get(name, sha((BASE/name).read_bytes())) for name in (
                'activation.json', 'openim/instance.json', 'gosuxin/manifest/config/config.yaml',
                'gosuxin/manifest/config/app.yaml')}}
    # Verify actual deployed probe contracts before changing a key. Roles are
    # reversed here: current credentials must work, proposed keys must fail.
    plan['currentCredentialPreflight'] = application_proof(
        {**bundle, 'new': bundle['old'], 'old': bundle['new']}, plan)
    save(directory/'plan.json', plan)
    return plan


def hrm_state():
    status = subprocess.run([SUPERVISOR, 'status', 'puty-hrm'], capture_output=True, timeout=20)
    match = re.search(rb'^puty-hrm\s+(STOPPED|STARTING|RUNNING|BACKOFF|STOPPING|EXITED|FATAL|UNKNOWN)\b', status.stdout)
    require(match is not None and match[1] != b'UNKNOWN', 'owned_HRM_supervisor_program_missing')
    return match[1].decode()


def stop_writers(plan):
    requested = False
    for _ in range(90):
        state = hrm_state()
        if state in ('STOPPED', 'EXITED', 'FATAL'):
            break
        if state in ('RUNNING', 'STARTING', 'BACKOFF') and not requested:
            # STOPPING/already-exited races are handled by the bounded poll;
            # never begin DB changes while HRM still owns a live process.
            result = subprocess.run([SUPERVISOR, 'stop', 'puty-hrm'], capture_output=True, timeout=90)
            require(result.returncode in (0, 3), 'HRM_supervisor_stop_failed')
            requested = True
        time.sleep(1)
    else:
        raise RuntimeError('HRM_supervisor_did_not_stop')
    current = all_containers(plan['instanceID'])
    ids = [item['Id'] for item in current if item['Config']['Labels']['com.docker.compose.service'] in plan['writers']
           and item['State']['Running']]
    if ids:
        execute(['docker', 'stop', '--time', '30', *ids], timeout=180)
    require(all(not item['State']['Running'] for item in all_containers(plan['instanceID'])
                if item['Config']['Labels']['com.docker.compose.service'] in plan['writers']), 'writer_still_running')


def recover_storage_processes(plan):
    # A killed compose invocation may leave a changed storage container stopped
    # or missing. Start it with the reviewed before/after files and owned volumes;
    # its healthcheck may use the other password, so inspect process state only.
    require(all(sha((BASE/row['path']).read_bytes()) in (row['beforeSHA256'], row['afterSHA256'])
                for row in plan['files']), 'recovery_runtime_files_not_reviewed')
    validate_storage_ownership(plan['instanceID'], plan['volumes'], allow_missing=True)
    compose('up', '-d', '--no-build', '--no-deps', *CHANGED_STORAGE)
    for _ in range(90):
        current = current_containers(plan['instanceID'])
        if all(name in current and current[name]['State']['Running'] for name in CHANGED_STORAGE):
            mongo_id = current['mongo']['Id']
            for mongo_key in (plan['_newMongo'], plan['_oldMongo']):
                if mongo_auth(mongo_id, mongo_key):
                    for redis_key in (plan['_newRedis'], plan['_oldRedis']):
                        if redis_auth(redis_key):
                            return mongo_id, mongo_key, redis_key
        time.sleep(2)
    raise RuntimeError('rollback_storage_authentication_not_recovered')


def _rollback(bundle, plan, journal, directory):
    validate_ownership(bundle)
    require(plan['instanceID'] == bundle['instanceID'] and plan['bundleSHA256'] == sha(canonical(bundle)),
            'rollback_owner_or_bundle_mismatch')
    validate_storage_ownership(plan['instanceID'], plan['volumes'], allow_missing=True)
    stop_writers(plan)
    # A SIGKILL can occur between a database change and journal update. Detect
    # which credential is live; never guess from a completion flag.
    candidate_plan = {**plan, '_newMongo': bundle['new']['mongo'], '_oldMongo': bundle['old']['mongo'],
                      '_newRedis': bundle['new']['redis'], '_oldRedis': bundle['old']['redis']}
    mongo_id, mongo_current, redis_current = recover_storage_processes(candidate_plan)
    rollback_documents(mongo_id, mongo_current, bundle['old']['session'], bundle['new']['session'])
    if mongo_current != bundle['old']['mongo']:
        mongo_password(mongo_id, mongo_current, bundle['old']['mongo'])
    if redis_current != bundle['old']['redis']:
        redis_password(redis_current, bundle['old']['redis'])
    install_files(plan['files'], reverse=True)
    restart_services(plan)
    before_hashes = {row['path']: row['beforeSHA256'] for row in plan['files']}
    require(all(sha((BASE/name).read_bytes()) == before_hashes.get(name, expected)
                for name, expected in plan['protectedFiles'].items()), 'rollback_protected_material_mismatch')
    require(mongo_auth(current_containers(plan['instanceID'])['mongo']['Id'], bundle['old']['mongo'])
            and redis_auth(bundle['old']['redis']), 'rollback_credentials_not_live')
    journal['status'] = 'rolledBack'
    save(directory/'journal.json', journal)


def rollback(bundle, plan, journal, directory):
    durable = json.loads((directory/'journal.json').read_text())
    require(durable.get('status') != 'committed', 'durably_committed_rotation_cannot_be_rolled_back')
    if durable.get('status') == 'rolledBack':
        return
    journal = durable
    try:
        _rollback(bundle, plan, journal, directory)
    except Exception:
        with contextlib.suppress(Exception):
            stop_writers(plan)
        journal['status'] = 'rollbackFailed'
        save(directory/'journal.json', journal)
        raise RuntimeError('verified_rollback_incomplete_writers_kept_stopped') from None


def process_start(pid):
    try:
        tail = pathlib.Path('/proc/'+str(pid)+'/stat').read_text().rsplit(') ', 1)[1].split()
        return None if tail[0] == 'Z' else tail[19]
    except (FileNotFoundError, IndexError):
        return None


def watchdog(bundle, plan, directory, parent, maximum):
    require(os.getpgrp() == parent, 'rotation_requires_own_process_group')
    identity = process_start(parent)
    require(identity is not None, 'rotation_parent_identity_unavailable')
    pid = os.fork()
    if pid:
        return pid
    try:
        os.setsid()
        with open(os.devnull, 'r+b', buffering=0) as sink:
            for descriptor in (0, 1, 2):
                os.dup2(sink.fileno(), descriptor)
        deadline = time.monotonic()+maximum
        while True:
            journal = json.loads((directory/'journal.json').read_text())
            if journal.get('status') in ('committed', 'rolledBack'):
                break
            alive = process_start(parent) == identity
            if not alive or time.monotonic() >= deadline:
                if alive:
                    os.killpg(parent, signal.SIGTERM)
                    for _ in range(15):
                        time.sleep(1)
                        if process_start(parent) != identity:
                            break
                    else:
                        os.killpg(parent, signal.SIGKILL)
                # Reap any orphaned docker/mongosh client process from the
                # isolated helper group before starting compensating commands.
                # Linux cannot reuse its PGID while original children remain.
                if process_start(parent) in (None, identity):
                    with contextlib.suppress(ProcessLookupError):
                        os.killpg(parent, signal.SIGKILL)
                # The parent may have crossed its durable commit point after
                # this loop first read 'applying'. Re-read after all possible
                # parent writers are gone; never roll back a committed receipt.
                journal = json.loads((directory/'journal.json').read_text())
                if journal.get('status') in ('committed', 'rolledBack'):
                    break
                for _ in range(3):
                    try:
                        rollback(bundle, plan, journal, directory)
                        break
                    except Exception:
                        time.sleep(5)
                break
            time.sleep(1)
    finally:
        os._exit(0)


def apply(bundle, plan, directory):
    current = runtime_validate(bundle)
    validate_storage_ownership(plan['instanceID'], plan['volumes'])
    require(plan['bundleSHA256'] == sha(canonical(bundle)) and plan['instanceID'] == bundle['instanceID'], 'plan_bundle_mismatch')
    before_hashes = {row['path']: row['beforeSHA256'] for row in plan['files']}
    require(all(sha((BASE/row['path']).read_bytes()) == row['beforeSHA256'] for row in plan['files'])
            and all(sha((BASE/name).read_bytes()) == before_hashes.get(name, expected) for name, expected in plan['protectedFiles'].items()),
            'reviewed_runtime_files_changed')
    require(not (directory/'journal.json').exists() and not (directory/'receipt.json').exists(), 'rotation_already_started_use_explicit_recover')
    journal = {'rotationID': bundle['rotationID'], 'status': 'applying', 'phase': 'prepared'}
    save(directory/'journal.json', journal)
    watcher = watchdog(bundle, plan, directory, os.getpid(), 1800)
    try:
        stop_writers(plan)
        actual = mongo_scan(current['mongo']['Id'], bundle['old']['mongo'])
        require(actual == [row['before'] for row in plan['documents']], 'encrypted_documents_changed_since_review')
        journal['phase'] = 'rewrap-attempted'; save(directory/'journal.json', journal)
        mongo_rewrap(current['mongo']['Id'], bundle['old']['mongo'], plan['documents'])
        journal['phase'] = 'database-passwords-attempted'; save(directory/'journal.json', journal)
        mongo_password(current['mongo']['Id'], bundle['old']['mongo'], bundle['new']['mongo'])
        redis_password(bundle['old']['redis'], bundle['new']['redis'])
        journal['phase'] = 'runtime-files-attempted'; save(directory/'journal.json', journal)
        install_files(plan['files'])
        journal['phase'] = 'restarting'; save(directory/'journal.json', journal)
        restart_services(plan)
        proof = None
        for _ in range(24):
            try:
                proof = application_proof(bundle, plan)
                break
            except Exception:
                time.sleep(5)
        require(proof is not None, 'new_runtime_application_proof_failed')
        validate_ownership(bundle)
        require(all(sha((BASE/name).read_bytes()) == expected for name, expected in plan['protectedFiles'].items()),
                'protected_runtime_material_changed')
        receipt = {'rotationID': bundle['rotationID'], 'bundleSHA256': sha(canonical(bundle)),
                   'status': 'committed', 'proof': proof, 'humanAcceptanceStillRequired': True}
        receipt['signature'] = hmac.new(bundle['new']['bridge'].encode(), canonical(receipt), hashlib.sha256).hexdigest()
        # One durable commit point. A receipt is only published after the
        # watchdog can no longer roll back, and can be republished from journal.
        journal.update(status='committed', phase='complete', receipt=receipt)
        save(directory/'journal.json', journal)
        save(directory/'receipt.json', receipt)
        return receipt
    except Exception:
        state = json.loads((directory/'journal.json').read_text())
        if state.get('status') == 'committed':
            raise RuntimeError('rotation_committed_receipt_publication_pending_use_receipt_action') from None
        try:
            rollback(bundle, plan, journal, directory)
        except Exception:
            journal['status'] = 'rollbackFailed'; save(directory/'journal.json', journal)
        raise RuntimeError('rotation_failed_consult_private_journal_and_recovery') from None
    finally:
        state = json.loads((directory/'journal.json').read_text())
        if state.get('status') in ('committed', 'rolledBack'):
            os.waitpid(watcher, 0)


def main():
    import fcntl
    require(os.name == 'posix', 'rotation_worker_requires_Linux')
    os.umask(0o077)
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=('plan', 'apply', 'recover', 'receipt'))
    parser.add_argument('--bundle', type=pathlib.Path, required=True)
    parser.add_argument('--plan-sha256')
    parser.add_argument('--confirm-maintenance', action='store_true')
    args = parser.parse_args()
    root = BASE/'staging/key-rotation'
    path = args.bundle
    require(path.is_file() and not path.is_symlink() and path.resolve().is_relative_to(root)
            and path.name == 'bundle.json' and path.stat().st_size <= 65536, 'owned_private_rotation_bundle_required')
    directory = path.parent
    require(directory.resolve() == directory and directory.name.startswith('rotate-'), 'owned_rotation_directory_required')
    bundle = json.loads(path.read_text()); bundle_valid(bundle)
    require(directory.name == bundle['rotationID'], 'rotation_directory_identity_mismatch')
    lock_path = BASE/'.backup-stage/backup.lock'
    require(not lock_path.is_symlink() and lock_path.parent.resolve() == lock_path.parent, 'maintenance_lock_redirected')
    with lock_path.open('a+b') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        if args.action == 'plan':
            plan = create_plan(bundle, directory)
            print(json.dumps({'planOnly': True, 'rotationID': bundle['rotationID'],
                              'planSHA256': sha((directory/'plan.json').read_bytes()),
                              'fileCount': len(plan['files']), 'encryptedDocumentCount': len(plan['documents']),
                              'rotatedKeyNames': NAMES, 'writers': plan['writers']}), flush=True)
            return
        require(args.confirm_maintenance or args.action == 'receipt', 'explicit_maintenance_flag_required')
        plan_path = directory/'plan.json'
        require(plan_path.is_file() and not plan_path.is_symlink() and sha(plan_path.read_bytes()) == args.plan_sha256,
                'explicit_reviewed_plan_SHA256_required')
        plan = json.loads(plan_path.read_text())
        require(plan['instanceID'] == bundle['instanceID'] and plan['bundleSHA256'] == sha(canonical(bundle)),
                'reviewed_plan_bundle_or_owner_mismatch')
        def interrupted(_signal, _frame):
            raise InterruptedError('rotation_interrupted_recovery_required')
        for signum in (signal.SIGHUP, signal.SIGTERM, signal.SIGINT):
            signal.signal(signum, interrupted)
        if args.action == 'receipt':
            journal = json.loads((directory/'journal.json').read_text())
            require(journal.get('status') == 'committed' and journal.get('receipt', {}).get('status') == 'committed',
                    'durable_commit_point_not_present')
            receipt = journal['receipt']
            application_proof(bundle, plan)
            save(directory/'receipt.json', receipt)
            print(json.dumps({'rotationID': bundle['rotationID'], 'committedReceiptPublished': True}), flush=True)
        elif args.action == 'recover':
            journal = json.loads((directory/'journal.json').read_text())
            require(journal.get('status') != 'committed', 'committed_rotation_cannot_be_implicitly_reverted')
            if os.getpgrp() != os.getpid():
                os.setsid()
            rollback(bundle, plan, journal, directory)
            print(json.dumps({'rotationID': bundle['rotationID'], 'rolledBack': True}), flush=True)
        else:
            if os.getpgrp() != os.getpid():
                os.setsid()
            result = apply(bundle, plan, directory)
            print(json.dumps({key: value for key, value in result.items() if key != 'signature'}), flush=True)


if __name__ == '__main__':
    try:
        main()
    except Exception as error:
        print(json.dumps({'rotationFailed': True, 'category': type(error).__name__,
                          'message': str(error) if type(error) is RuntimeError else 'private_details_suppressed'}), flush=True)
        sys.exit(1)
