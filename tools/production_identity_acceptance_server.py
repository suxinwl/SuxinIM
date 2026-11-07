"""Run on the production host over the existing pinned SSH session.

Input on stdin is the private identity-server-input.json. No extra listeners are
opened. Container environment is read into memory; secret values are never printed.
Snapshot before a public lifecycle change, then run late-epoch after fresh login.
"""
import argparse
import base64
import datetime
import hashlib
import hmac
import json
import os
import pathlib
import re
import secrets
import socket
import subprocess
import sys
import time
import urllib.error
import urllib.parse
import urllib.request


class Failure(Exception):
    pass


def digest(value):
    return hashlib.sha256(value.encode()).hexdigest()


def check(name, value):
    print(json.dumps({'check': name, 'passed': bool(value)}), flush=True)
    if not value:
        raise Failure(name)


class Redis:
    def __init__(self, raw, port):
        endpoint = urllib.parse.urlsplit(raw)
        self.socket = socket.create_connection(('127.0.0.1', port), timeout=3)
        self.file = self.socket.makefile('rb')
        if endpoint.password:
            password = urllib.parse.unquote(endpoint.password)
            username = urllib.parse.unquote(endpoint.username or '')
            self.command('AUTH', username, password) if username else self.command('AUTH', password)
        self.command('SELECT', endpoint.path.strip('/') or '0')

    def command(self, *args):
        values = [str(value).encode() for value in args]
        raw = b'*'+str(len(values)).encode()+b'\r\n'
        raw += b''.join(b'$'+str(len(value)).encode()+b'\r\n'+value+b'\r\n' for value in values)
        self.socket.sendall(raw)
        line = self.file.readline()
        if not line or line[:1] == b'-':
            raise Failure('private_redis_command_failed')
        if line[:1] in (b'+', b':'):
            return line[1:-2]
        if line[:1] == b'$':
            size = int(line[1:-2])
            if size == -1:
                return None
            value = self.file.read(size)
            self.file.read(2)
            return value
        raise Failure('private_redis_response_type')

    def document(self, key):
        raw = self.command('GET', key)
        if raw is None:
            raise Failure('private_session_record_missing')
        return json.loads(raw)

    def session(self, value):
        binding = self.document('HRM38:BIND:'+digest(value['imToken']))
        current = self.document('HRM38:SESSION:'+digest(binding['sessionID']))
        check('private_session_matches_fixture', current['userID'] == value['userID'] and current['userID'].startswith('emp_'))
        return current

    def valid(self, session):
        current = self.document('HRM38:SESSION:'+digest(session['sessionID']))
        pointer = self.command('GET', 'HRM38:CURRENT:'+current['userID']+':'+str(current['platform']))
        projection = self.document('HRM38:USER:'+current['userID'])
        epoch = int(self.command('GET', 'HRM38:EPOCH:'+current['userID']) or 0)
        return not current.get('revoked') and projection.get('enabled') and current['epoch'] >= epoch and current['expiresAt'] > time.time() and pointer.decode() == current['sessionID']


def bridge(key, path, body, nonce=None):
    raw = json.dumps(body, separators=(',', ':')).encode()
    nonce = nonce or secrets.token_hex(32)
    stamp = str(int(time.time()))
    canonical = '\n'.join((stamp, nonce, 'POST', path, hashlib.sha256(raw).hexdigest()))
    signature = hmac.new(key.encode(), canonical.encode(), hashlib.sha256).hexdigest()
    req = urllib.request.Request('http://127.0.0.1:10010'+path, raw, {'Content-Type': 'application/json', 'X-HRM-Timestamp': stamp, 'X-HRM-Nonce': nonce, 'X-HRM-Signature': signature}, method='POST')
    try:
        response = urllib.request.urlopen(req, timeout=30)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        data = json.loads(response.read())
        return response.status, data


def mongo_json(container, code):
    # Credentials stay inside the database container's environment.
    script = """const uri='mongodb://'+encodeURIComponent(process.env.MONGO_INITDB_ROOT_USERNAME)+':'+encodeURIComponent(process.env.MONGO_INITDB_ROOT_PASSWORD)+'@127.0.0.1:27017/admin'; const conn=new Mongo(uri); const db=conn.getDB('openim38');
"""+code
    raw = subprocess.check_output(['docker', 'exec', '-i', container, 'mongosh', '--nodb', '--quiet', '--file', '/dev/stdin'], input=script.encode(), stderr=subprocess.DEVNULL, timeout=30)
    return json.loads(raw)


def private_json(path, value):
    descriptor = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with os.fdopen(descriptor, 'w') as output:
        json.dump(value, output, indent=2)


def storage(env, port, method, key, body_limit=1024):
    # A bounded, authenticated loopback request; never follow a redirect.
    path = urllib.parse.quote('/openim38/'+key, safe='/~.-_')
    host = '127.0.0.1:'+str(port)
    now = datetime.datetime.now(datetime.timezone.utc)
    stamp, day = now.strftime('%Y%m%dT%H%M%SZ'), now.strftime('%Y%m%d')
    empty = hashlib.sha256(b'').hexdigest()
    canonical = '\n'.join((method, path, '', 'host:'+host+'\nx-amz-content-sha256:'+empty+'\nx-amz-date:'+stamp+'\n', 'host;x-amz-content-sha256;x-amz-date', empty))
    region = env.get('OBJECT_STORAGE_REGION') or 'us-east-1'
    scope = day+'/'+region+'/s3/aws4_request'
    signing = ('AWS4'+env['OBJECT_STORAGE_SECRET_ACCESS_KEY']).encode()
    for value in (day, region, 's3', 'aws4_request'):
        signing = hmac.new(signing, value.encode(), hashlib.sha256).digest()
    signature = hmac.new(signing, ('AWS4-HMAC-SHA256\n'+stamp+'\n'+scope+'\n'+hashlib.sha256(canonical.encode()).hexdigest()).encode(), hashlib.sha256).hexdigest()
    auth = 'AWS4-HMAC-SHA256 Credential='+env['OBJECT_STORAGE_ACCESS_KEY_ID']+'/'+scope+', SignedHeaders=host;x-amz-content-sha256;x-amz-date, Signature='+signature
    req = urllib.request.Request('http://'+host+path, headers={'Authorization': auth, 'X-Amz-Date': stamp, 'X-Amz-Content-Sha256': empty}, method=method)
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), NoRedirect())
    try:
        response = opener.open(req, timeout=10)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        body = response.read(body_limit+1)
        if len(body) > body_limit:
            raise Failure('attachment_body_exceeds_fixture_bound')
        return response.status, body, {key.lower(): value for key, value in response.headers.items()}


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *args, **kwargs):
        return None


def exact_fixture_descriptor(item, users, start, end):
    """Only the three explicitly recorded acceptance upload formats are allowed."""
    owner, name = item.get('userID', ''), item.get('name', '')
    common = owner in users and isinstance(name, str) and isinstance(item.get('uploaded'), bool) and isinstance(item.get('createdAt'), int) and start <= item['createdAt'] <= end and isinstance(item.get('bodySHA256'), str) and re.fullmatch(r'[0-9a-f]{64}', item['bodySHA256'])
    if not common or any(ord(c) < 32 or ord(c) == 127 for c in name) or '\\' in name or any(p in ('', '.', '..') for p in name.split('/')):
        return False
    mode, key = item.get('bodyMode'), item.get('objectKey', '')
    if mode == 'identity-form':
        try:
            raw = base64.b64decode(item.get('bodyBase64', ''), validate=True)
        except (ValueError, TypeError):
            return False
        return bool(item.get('platform') == 2 and item.get('size') == len(raw) == 436 and item.get('contentType') == 'text/plain' and re.fullmatch(re.escape(owner)+r'/production-acceptance-[0-9a-f]{24}\.txt', name) and re.fullmatch(r'openim/direct/[0-9]{8}/'+re.escape(owner)+r'/[0-9a-f]{32}\.txt', key) and identity_fixture_body(raw) and hashlib.sha256(raw).hexdigest() == item['bodySHA256'] and hashlib.md5(raw).hexdigest() == item.get('bodyMD5'))
    source_hash = item.get('expectedSourceUploadHash', '')
    if not isinstance(source_hash, str) or not re.fullmatch(r'[0-9a-f]{32}', source_hash) or key != 'openim/data/hash/'+source_hash or item.get('size') != 4096:
        return False
    if mode == 'android-main':
        msg = item.get('clientMsgID', '')
        return bool(item.get('platform') == 2 and item.get('contentType') == 'text/plain' and isinstance(msg, str) and re.fullmatch(r'[0-9a-f]{32}', msg) and re.fullmatch(re.escape(owner+'/msg_file_'+msg+'/')+r'puty-android-main-[0-9a-f]{24}\.txt', name))
    body = bytes(i % 251 for i in range(4096))
    return bool(item.get('platform') in (2, 3, 'windows', 'android') and item.get('contentType') == 'application/octet-stream' and re.fullmatch(re.escape(owner)+r'/puty-client-acceptance-[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\.bin', name) and (mode == 'nonce32' or mode == 'deterministic' and item['bodySHA256'] == hashlib.sha256(body).hexdigest()))


def identity_fixture_body(raw):
    lines = raw.splitlines()
    return len(lines) == 8 and all(lines[n] == b'PUTY production identity acceptance fixture' and re.fullmatch(b'[0-9a-f]{64}', lines[n+1]) and lines[n+1] == lines[1] for n in range(0, 8, 2))


def exact_fixture_body(item, raw):
    if len(raw) != item['size'] or hashlib.sha256(raw).hexdigest() != item['bodySHA256']:
        return False
    mode = item['bodyMode']
    body = bytes(i % 251 for i in range(4096))
    if mode == 'deterministic':
        return raw == body
    if mode == 'nonce32':
        return len(raw) == 4096 and raw[32:] == body[32:]
    if mode == 'android-main':
        return len(raw) == 4096
    if mode == 'identity-form':
        return raw == base64.b64decode(item['bodyBase64'], validate=True) and identity_fixture_body(raw)
    return False


def attachment_cleanup(args, env, payload):
    employees = payload.get('fixtureEmployeeIDs', [])
    users = payload.get('fixtureIMUserIDs', [])
    check('attachment_exact_two_fixture_bindings', len(employees) == len(users) == 2 and len(set(users)) == 2 and all(user == 'emp_'+digest(str(employee))[:32] for employee, user in zip(employees, users)))
    start, end = payload['createdAfter'], payload['createdBefore']
    check('attachment_time_window_is_bounded', isinstance(start, int) and isinstance(end, int) and 0 < end-start <= 86400 and end <= time.time()+5)
    name_re = '^(?:'+'|'.join(re.escape(user) for user in users)+')/production-acceptance-[0-9a-f]{24}\\.txt$'
    client_scope = args.action.startswith('client-')
    client_uploads = payload.get('clientUploads', []) if client_scope else []
    client_names = {}
    if client_scope:
        check('client_attachment_exact_list_is_bounded', isinstance(client_uploads, list) and 1 <= len(client_uploads) <= 10)
        for item in client_uploads:
            check('client_attachment_explicit_record_is_fixture_bound', exact_fixture_descriptor(item, users, start, end))
            check('client_attachment_names_are_unique', item['name'] not in client_names)
            client_names[item['name']] = item
    def metadata(row):
        owner = row['name'].split('/', 1)[0]
        if client_scope:
            item = client_names.get(row['name'])
            return item is not None and owner == item['userID'] and row.get('user_id', owner) == owner and row['key'] == item['objectKey'] and row['size'] == item['size'] and row['content_type'] == item['contentType'] and (item['bodyMode'] != 'identity-form' or row['group'] == 'acceptance') and start <= row['createdAt'] <= end and row['engine'] == 'minio' and re.fullmatch(r'[0-9a-f]{24}', row['_id']) and row['references'] >= 0
        return owner in users and row.get('user_id', owner) == owner and row['group'] == 'acceptance' and row['content_type'] == 'text/plain' and re.fullmatch(name_re, row['name']) and start <= row['createdAt'] <= end and row['engine'] == 'minio' and re.fullmatch(r'[0-9a-f]{24}', row['_id']) and row['size'] <= 1024 and row['size'] > 0 and row['references'] == 0 and re.fullmatch(r'openim/direct/[0-9]{8}/'+re.escape(owner)+r'/[0-9a-f]{32}\.txt', row['key'])
    def verify(row, expected_sha=None):
        check('attachment_metadata_scoped_to_exact_fixture', metadata(row))
        code, _, headers = storage(env, args.minio_port, 'HEAD', row['key'])
        check('attachment_HEAD_matches_recorded_size', code == 200 and int(headers.get('content-length', -1)) == row['size'])
        code, raw, get_headers = storage(env, args.minio_port, 'GET', row['key'], 4096 if client_scope else 1024)
        item = client_names.get(row['name']) if client_scope else None
        marker = exact_fixture_body(item, raw) if client_scope else identity_fixture_body(raw)
        sha = hashlib.sha256(raw).hexdigest()
        md5 = hashlib.md5(raw).hexdigest()
        if client_scope:
            check('client_attachment_exact_source_hash_matches_body', sha == item['bodySHA256'] and (md5 == item['bodyMD5'] if item['bodyMode'] == 'identity-form' else hashlib.md5(md5.encode()).hexdigest() == item['expectedSourceUploadHash']))
        # The locked 3.8 SetObject implementation omits user_id/hash from
        # Mongo. Name and storage key must independently bind the same fixture;
        # the authenticated object ETag supplies its content checksum.
        check('attachment_exact_fixture_body_and_hash_verified', code == 200 and marker and len(raw) == row['size'] and headers.get('etag', '').strip('"') == md5 and get_headers.get('etag', '').strip('"') == md5 and (not row.get('hash') or row['hash'] == 'etag_'+md5) and (expected_sha is None or expected_sha == sha))
        return sha
    path = pathlib.Path(args.inventory_file)
    if client_scope and args.inventory_file == '/root/puty-identity-attachment-inventory.json':
        path = pathlib.Path('/root/puty-client-attachment-inventory.json')
    if args.action in ('attachment-inventory', 'client-attachment-inventory'):
        selector = {'$or': [{'name': item['name'], 'key': item['objectKey']} for item in client_uploads]} if client_scope else {'group': 'acceptance', 'content_type': 'text/plain', 'name': {'$regex': name_re}}
        code = "const q=EJSON.parse("+json.dumps(json.dumps(selector))+ "); q.create_time={$gte:new Date("+str(start*1000)+"),$lte:new Date("+str(end*1000)+")}; const rows=db.s3.find(q).limit(101).toArray().map(d=>({_id:d._id.toHexString(),name:d.name,user_id:d.user_id,engine:d.engine,key:d.key,size:Number(d.size),hash:d.hash,group:d.group,content_type:d.content_type,rawDocument:EJSON.stringify(d,{relaxed:false}),createdAt:Math.floor(d.create_time.getTime()/1000),references:Number(db.s3.countDocuments({engine:d.engine,key:d.key,_id:{$ne:d._id}}))})); print(JSON.stringify(rows));"
        rows = mongo_json(args.mongo_container, code)
        check('attachment_inventory_count_is_bounded', isinstance(rows, list) and len(rows) <= 100)
        if client_scope:
            found_names = {row['name'] for row in rows}
            check('client_attachment_confirmed_records_found', found_names == set(client_names) and len(rows) == len(client_uploads))
            for row in rows:
                peers = [other['_id'] for other in rows if other['engine'] == row['engine'] and other['key'] == row['key'] and other['_id'] != row['_id']]
                check('client_attachment_no_reference_outside_exact_approved_aliases', row['references'] == len(peers))
                row['approvedPeerIDs'] = peers
        shape = {field: sorted({type(row.get(field)).__name__ for row in rows}) for field in ('_id', 'name', 'engine', 'key', 'size', 'createdAt', 'references', 'rawDocument')}
        print(json.dumps({'check': 'attachment_inventory_field_types', 'types': shape}), flush=True)
        check('attachment_inventory_required_field_types', all(isinstance(row.get(field), str) for row in rows for field in ('_id', 'name', 'engine', 'key', 'rawDocument')) and all(isinstance(row.get(field), int) for row in rows for field in ('size', 'createdAt', 'references')))
        for row in rows:
            row['sha256'] = verify(row)
        private_json(path, {'scope': payload, 'rows': rows, 'plannedAt': int(time.time()), 'applyRequired': True})
        print(json.dumps({'check': 'attachment_inventory_saved_private_plan_only', 'passed': True, 'count': len(rows), 'planSHA256': hashlib.sha256(path.read_bytes()).hexdigest()}), flush=True)
        return
    plan = json.loads(path.read_text())
    check('attachment_apply_scope_matches_approved_plan', plan['scope'] == payload and len(plan['rows']) <= 100)
    check('attachment_apply_requires_explicit_flag', args.apply)
    for row in plan['rows']:
        verify(row, row['sha256'])
        # Revalidate every exact metadata field and the reference count before
        # touching the object. The root waits for fixture clients to quiesce.
        q = 'EJSON.parse('+json.dumps(row['rawDocument'])+',{relaxed:false})'
        if client_scope:
            remaining = [other for other in plan['rows'] if other['engine'] == row['engine'] and other['key'] == row['key'] and not other.get('metadataDeletedAt')]
            approved = '['+','.join('EJSON.parse('+json.dumps(other['rawDocument'])+',{relaxed:false})' for other in remaining)+']'
            code = 'const q='+q+', approved='+approved+'; print(JSON.stringify({matches:db.s3.countDocuments(q),references:db.s3.countDocuments({engine:q.engine,key:q.key,_id:{$ne:q._id}}),approvedExact:db.s3.countDocuments({$or:approved}),outsideApproved:db.s3.countDocuments({engine:q.engine,key:q.key,_id:{$nin:approved.map(x=>x._id)}})}));'
            state = mongo_json(args.mongo_container, code)
            check('client_attachment_current_references_are_only_exact_remaining_aliases', state == {'matches': 1, 'references': len(remaining)-1, 'approvedExact': len(remaining), 'outsideApproved': 0})
            if len(remaining) > 1:
                result = mongo_json(args.mongo_container, 'const q='+q+'; print(JSON.stringify({deleted:db.s3.deleteOne(q).deletedCount,remaining:db.s3.countDocuments({_id:q._id})}));')
                check('client_attachment_exact_alias_deleted_without_deleting_shared_object', result == {'deleted': 1, 'remaining': 0})
                code, _, headers = storage(env, args.minio_port, 'HEAD', row['key'])
                check('client_attachment_shared_object_preserved_for_remaining_approved_alias', code == 200 and int(headers.get('content-length', -1)) == row['size'])
                row['metadataDeletedAt'] = int(time.time())
                row['aliasOnly'] = True
                private_json(path, plan)
                continue
        code = 'const q='+q+'; print(JSON.stringify({matches:db.s3.countDocuments(q),references:db.s3.countDocuments({engine:q.engine,key:q.key,_id:{$ne:q._id}})}));'
        state = mongo_json(args.mongo_container, code)
        check('attachment_exact_document_unchanged_and_unshared', state == {'matches': 1, 'references': 0})
        code, _, _ = storage(env, args.minio_port, 'DELETE', row['key'])
        check('attachment_exact_MinIO_key_deleted', code in (200, 204))
        code, _, _ = storage(env, args.minio_port, 'HEAD', row['key'])
        check('attachment_deleted_MinIO_key_is_404', code == 404)
        row['objectDeletedAt'] = int(time.time())
        private_json(path, plan)
        code = 'const q='+q+'; if(db.s3.countDocuments({engine:q.engine,key:q.key,_id:{$ne:q._id}})!==0) throw Error("new_reference"); print(JSON.stringify({deleted:db.s3.deleteOne(q).deletedCount}));'
        result = mongo_json(args.mongo_container, code)
        check('attachment_exact_Mongo_document_deleted', result == {'deleted': 1})
        result = mongo_json(args.mongo_container, 'const q='+q+'; print(JSON.stringify({remaining:db.s3.countDocuments({_id:q._id})}));')
        check('attachment_deleted_exact_Mongo_id_is_absent', result == {'remaining': 0})
        row['metadataDeletedAt'] = int(time.time())
        private_json(path, plan)
    check('attachment_bounded_cleanup_complete', True)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('action', choices=['snapshot', 'late-epoch', 'hmac', 'attachment-range', 'refresh-exchange', 'attachment-inventory', 'attachment-cleanup', 'client-attachment-inventory', 'client-attachment-cleanup'])
    parser.add_argument('--container', default='openim38-enterprise')
    parser.add_argument('--redis-port', type=int, default=16379)
    parser.add_argument('--state-file', default='/root/puty-identity-acceptance-snapshot.json')
    parser.add_argument('--mongo-container', default='openim38-mongo')
    parser.add_argument('--minio-port', type=int, default=19000)
    parser.add_argument('--inventory-file', default='/root/puty-identity-attachment-inventory.json')
    parser.add_argument('--apply', action='store_true')
    args = parser.parse_args()
    # docker inspect is captured directly; it is never echoed or logged.
    container = json.loads(subprocess.check_output(['docker', 'inspect', args.container], stderr=subprocess.DEVNULL))[0]
    env = dict(value.split('=', 1) for value in container['Config']['Env'] if '=' in value)
    key = env['HRM_BRIDGE_HMAC_KEY']
    if args.action == 'hmac':
        nonce = secrets.token_hex(32)
        first = bridge(key, '/internal/hrm/unknown-acceptance-operation', {}, nonce)
        second = bridge(key, '/internal/hrm/unknown-acceptance-operation', {}, nonce)
        check('private_HMAC_valid_request_reaches_router', first[0] == 404)
        check('private_HMAC_nonce_replay_rejected', second[0] == 401)
        return
    payload = json.load(sys.stdin)
    if args.action in ('attachment-inventory', 'attachment-cleanup', 'client-attachment-inventory', 'client-attachment-cleanup'):
        attachment_cleanup(args, env, payload)
        return
    if args.action == 'attachment-range':
        uri = urllib.parse.urlsplit(payload['attachmentURL'])
        check('private_attachment_input_bounded', uri.scheme == 'https' and uri.hostname in ('192.168.10.11', 'im.szputy.com') and uri.path.startswith('/attachments/'))
        for port, label in ((18090, 'enterprise'),):
            req = urllib.request.Request('http://127.0.0.1:'+str(port)+uri.path+'?'+uri.query, headers={'Host': uri.netloc, 'Range': 'bytes=2-9', 'Accept-Encoding': 'identity', 'X-GoSuxin-Client-IP': '192.168.10.1', 'X-Puty-Entry': 'lan'})
            try:
                response = urllib.request.urlopen(req, timeout=15)
            except urllib.error.HTTPError as error:
                response = error
            with response:
                raw = response.read()
                print(json.dumps({'check': label+'_direct_range_structural', 'status': response.status, 'bodyBytes': len(raw), 'hasContentRange': bool(response.headers.get('Content-Range')), 'acceptRanges': response.headers.get('Accept-Ranges', '')}), flush=True)
        return
    check('private_fixture_input_bounded', isinstance(payload.get('sessions'), list) and 1 <= len(payload['sessions']) <= 2)
    redis = Redis(env['HRM_REDIS_URL'], args.redis_port)
    current = {value['slot']: redis.session(value) for value in payload['sessions']}
    if args.action == 'refresh-exchange':
        for session in current.values():
            started = time.monotonic()
            result = bridge(key, '/internal/hrm/exchange', {'ticket': secrets.token_hex(32), 'userID': session['userID'], 'sessionID': session['sessionID'], 'platform': session['platform'], 'policyPlatform': session.get('policyPlatform', session['platform']), 'expiresAt': session['expiresAt'], 'epoch': session['epoch']})
            text = json.dumps(result[1])
            category = next((word for word in ('session cannot be reused', 'session exchange rejected', 'employee is not enabled', 'ticket identity changed', 'duplicate', 'document exists', 'version conflict', 'CreateToken', 'get_user_token', 'context deadline exceeded', 'unavailable') if word in text), 'unspecified')
            print(json.dumps({'check': 'direct_same_SID_refresh_exchange', 'status': result[0], 'code': result[1].get('errCode'), 'category': category, 'seconds': round(time.monotonic()-started, 2)}), flush=True)
        return
    state_file = pathlib.Path(args.state_file)
    if args.action == 'snapshot':
        check('snapshot_sessions_are_live', all(redis.valid(value) for value in current.values()))
        descriptor = os.open(state_file, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
        with os.fdopen(descriptor, 'w') as output:
            json.dump(current, output)
        check('private_snapshot_saved_without_printing_identifiers', True)
        return
    previous = json.loads(state_file.read_text())
    for slot, fresh in current.items():
        old = previous[slot]
        check('fresh_lifecycle_epoch_advanced', fresh['userID'] == old['userID'] and fresh['epoch'] > old['epoch'] and fresh['sessionID'] != old['sessionID'])
        check('fresh_session_live_before_delayed_revoke', redis.valid(fresh))
        result = bridge(key, '/internal/hrm/revoke', {'userID': old['userID'], 'all': True, 'epoch': old['epoch']+1})
        check('delayed_epoch_revoke_accepted', result[0] == 200 and result[1].get('errCode') == 0)
        check('delayed_epoch_does_not_revoke_fresh_session', redis.valid(fresh))
        result = bridge(key, '/internal/hrm/exchange', {'ticket': secrets.token_hex(32), 'userID': old['userID'], 'sessionID': secrets.token_hex(32), 'platform': old['platform'], 'policyPlatform': old.get('policyPlatform', old['platform']), 'epoch': old['epoch'], 'expiresAt': int(time.time())+4*3600})
        check('old_epoch_exchange_rejected', result[0] in (401, 403))
        result = bridge(key, '/internal/hrm/revoke', {'userID': old['userID'], 'sessionID': old['sessionID'], 'platform': old['platform']})
        check('late_old_SID_with_platform_revoke_is_safe', result[0] == 200 and redis.valid(fresh))
        result = bridge(key, '/internal/hrm/revoke', {'userID': fresh['userID'], 'sessionID': fresh['sessionID'], 'all': True})
        check('ambiguous_SID_all_revoke_rejected_without_side_effect', result[0] == 400 and redis.valid(fresh))


if __name__ == '__main__':
    try:
        main()
    except Exception as error:
        print(json.dumps({'acceptanceFailed': True, 'errorType': type(error).__name__, 'check': str(error) if isinstance(error, Failure) else 'private_details_suppressed'}), flush=True)
        sys.exit(1)
