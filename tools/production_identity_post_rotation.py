"""Small post-rotation production check. Credentials and object URLs stay private."""
import base64
import hashlib
import json
import secrets
import sys
import time
import urllib.parse

import production_identity_acceptance as a

a.REPORT = a.PRIVATE / 'identity-post-rotation-report.json'
a.STATE = a.PRIVATE / 'identity-post-rotation-state.json'


def check_android_attachment(session):
    rows = a.read(a.PRIVATE / 'client-uploads-android-final-received.json')
    a.record('post_rotation_android_exact_received_message', isinstance(rows, list) and len(rows) == 1)
    message = rows[0]
    source = message['fileElem']['sourceUrl'] if 'sourceUrl' in message['fileElem'] else message['fileElem']['sourceURL']
    grant = a.expect('post_rotation_prior_android_object_reauthorize', a.call('/enterprise/v1/attachment/authorize', {'url': source}, session))
    result = a.http(grant['url'])
    expected = '279eeff3c42ccd12e2c1a6e6efb0b7fbf6d2d598146ad4d5e3c47887f17983bc'
    a.record('post_rotation_prior_android_object_bytes_preserved', result[0] == 200 and len(result[2]) == 4096 and hashlib.sha256(result[2]).hexdigest() == expected, status=result[0])
    a.verify_attachment(grant, result[2])
    path = urllib.parse.unquote(urllib.parse.urlsplit(grant['url']).path).split('/', 3)[3]
    a.record('post_rotation_prior_android_object_exact_storage_path', path.startswith('openim38/openim/data/hash/') and len(path.rsplit('/', 1)[1]) == 32)
    state = a.read(a.STATE)
    state['_priorAndroidAttachment'] = {'grant': grant, 'storagePath': path, 'bodySHA256': expected, 'bodyMD5': hashlib.md5(result[2]).hexdigest(), 'name': message['sendID']+'/msg_file_'+message['clientMsgID']+'/'+message['fileElem']['fileName'], 'sendTime': message['sendTime']}
    a.save(a.STATE, state)


def smoke():
    sessions = a.read(a.STATE)
    slots = ('aandroid', 'awindows', 'bweb')
    sockets = []
    try:
        for slot in slots:
            session = sessions[slot]
            a.expect('post_rotation_native_'+slot, a.native(session))
            a.expect('post_rotation_enterprise_'+slot, a.live(session))
            a.expect('post_rotation_signaling_'+slot, a.call('/enterprise/v1/signaling/get_signal_invitation_info_start_app', {}, session))
        old = a.read(a.PRIVATE/'identity-state.json')
        old_native = a.native(next(value for value in old.values() if isinstance(value, dict) and value.get('imToken')))
        a.record('post_rotation_old_IM_signature_rejected', old_native[0] == 200 and old_native[1].get('errCode') == 1505, status=old_native[0], code=old_native[1].get('errCode'))
        previous = sessions['awindows']
        refreshed = a.expect('post_rotation_windows_fresh_refresh', a.call('/sso/refresh', {'sessionToken': previous['sessionToken']}))
        a.record('post_rotation_refresh_keeps_SID_new_pair', refreshed['sessionToken'] == previous['sessionToken'] and refreshed['imToken'] != previous['imToken'] and refreshed['chatToken'] != previous['chatToken'])
        refreshed.update(_platform=3, _fixture=0)
        sessions['awindows'] = refreshed
        a.save(a.STATE, sessions)
        a.expect('post_rotation_refreshed_native', a.native(refreshed))
        a.expect('post_rotation_old_generation_same_SID_retained', a.live(previous))
        a.expect('post_rotation_cross_platform_pair_denied', a.live({**refreshed, 'chatToken': sessions['aandroid']['chatToken']}), False)
        a.expect('post_rotation_cross_user_pair_denied', a.live({**refreshed, 'chatToken': sessions['bweb']['chatToken']}), False)
        sockets = [a.socket_for(sessions[slot]) for slot in slots]
        for ws in sockets:
            ws.ping()
        time.sleep(1)
        marker = 'PUTY_POST_ROTATION_SINGLE_'+secrets.token_hex(12)
        a.ws_send(sockets[0], sessions['aandroid'], sessions['bweb']['userID'], marker)
        a.receive_marker(sockets[2], marker, 'post_rotation_android_to_web')
        conversation = 'si_'+'_'.join(sorted([sessions['aandroid']['userID'], sessions['bweb']['userID']]))
        a.history(sessions['bweb'], conversation, marker, 'post_rotation_single')
        reply = 'PUTY_POST_ROTATION_REPLY_'+secrets.token_hex(12)
        a.ws_send(sockets[2], sessions['bweb'], sessions['aandroid']['userID'], reply)
        a.receive_marker(sockets[0], reply, 'post_rotation_web_to_android')
        a.upload(sessions['aandroid'])
        check_android_attachment(sessions['bweb'])
        a.record('post_rotation_business_smoke_complete', True)
    finally:
        for ws in sockets:
            ws.close()
        for slot in slots:
            session = sessions.get(slot)
            if session:
                a.expect('post_rotation_exact_logout_'+slot, a.call('/sso/logout', {'sessionToken': session['sessionToken']}))


def inventory_input():
    """Normalize only recorded exact aliases; never discover names by a prefix."""
    import production_identity_acceptance_server as server
    original = a.read(a.PRIVATE/'identity-client-attachment-inventory-input.json')
    state = a.read(a.STATE)
    prior = state['_priorAndroidAttachment']
    message = a.read(a.PRIVATE/'client-uploads-android-final-received.json')[0]
    android_time = int(prior['sendTime'])
    if android_time > 10**12:
        android_time //= 1000
    source_hash = prior['storagePath'].rsplit('/', 1)[1]
    android = {'userID': message['sendID'], 'name': prior['name'], 'size': 4096, 'bodySHA256': prior['bodySHA256'], 'platform': 2, 'uploaded': True, 'createdAt': android_time, 'bodyMode': 'android-main', 'expectedSourceUploadHash': source_hash, 'objectKey': 'openim/data/hash/'+source_hash, 'clientMsgID': message['clientMsgID'], 'contentType': 'text/plain'}
    attachment = state['_attachment']
    encoded = attachment['uploadID']
    meta = json.loads(base64.b64decode(encoded+'='*(-len(encoded)%4)))
    raw = base64.b64decode(attachment['contentBase64'], validate=True)
    owner = state['aandroid']['userID']
    rows = a.read(a.PRIVATE/'identity-post-rotation-retry-report.json')
    created = next(row['time'] for row in rows if row['check'] == 'upload_complete_current_SID' and row['passed'])
    form = {'userID': owner, 'name': meta['name'], 'size': len(raw), 'bodySHA256': hashlib.sha256(raw).hexdigest(), 'bodyMD5': hashlib.md5(raw).hexdigest(), 'bodyBase64': attachment['contentBase64'], 'platform': 2, 'uploaded': True, 'createdAt': created, 'bodyMode': 'identity-form', 'objectKey': meta['key'], 'contentType': meta['contentType']}
    if meta['group'] != 'acceptance' or meta['size'] != len(raw):
        raise a.CheckFailure('exact_form_metadata_source_mismatch')
    windows = [{**item, 'contentType': 'application/octet-stream'} for item in original['clientUploads']]
    payload = {**original, 'createdBefore': int(time.time()), 'clientUploads': windows+[android, form]}
    if len(payload['clientUploads']) != 7 or not all(server.exact_fixture_descriptor(item, payload['fixtureIMUserIDs'], payload['createdAfter'], payload['createdBefore']) for item in payload['clientUploads']):
        raise a.CheckFailure('final_exact_descriptors_failed_validation')
    output = a.PRIVATE/'identity-final-attachment-inventory-input.json'
    a.save(output, payload)
    print(json.dumps({'normalizedExactAliases': 7, 'distinctStorageKeys': len({item['objectKey'] for item in payload['clientUploads']}), 'inputSHA256': hashlib.sha256(output.read_bytes()).hexdigest()}))


if __name__ == '__main__':
    try:
        inventory_input() if len(sys.argv) == 2 and sys.argv[1] == 'inventory-input' else smoke()
    except Exception as error:
        print(json.dumps({'postRotationFailed': True, 'errorType': type(error).__name__, 'check': str(error) if isinstance(error, a.CheckFailure) else 'private_details_suppressed'}), flush=True)
        sys.exit(1)
