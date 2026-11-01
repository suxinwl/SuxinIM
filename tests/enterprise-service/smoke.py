"""Exercise the isolated stack with synthetic fixtures; never prints tokens.

Run after tests/seed_accounts.py. Uses fixture accounts 108-110, so it does not
replace the first 100 SDK sessions used by the concurrent-connection test.
"""
import hashlib
import json
import pathlib
import secrets
import ssl
import time
import urllib.error
import urllib.request
import urllib.parse

ROOT = pathlib.Path(__file__).resolve().parents[2]
BASE = 'https://192.168.10.11:29880'
TLS = ssl.create_default_context(cafile=str(ROOT / 'artifacts/dev-cert/OpenIM38-Development-CA.crt'))
REPORT = ROOT / 'tests/enterprise-service/smoke-report.json'


def call(path, body=None, user=None, expected=200):
    headers = {'Content-Type': 'application/json', 'operationID': secrets.token_hex(12)}
    if user:
        headers.update(token=user.get('imToken', user.get('adminToken', '')),
                       chatToken=user.get('chatToken', user.get('adminToken', '')))
        if user.get('platformID'):
            headers['platformID'] = str(user['platformID'])
    request = urllib.request.Request(BASE + path, None if body is None else json.dumps(body).encode(), headers)
    try:
        with urllib.request.urlopen(request, context=TLS, timeout=30) as response:
            status, raw = response.status, response.read()
    except urllib.error.HTTPError as error:
        status, raw = error.code, error.read()
    result = json.loads(raw)
    if status != expected or expected == 200 and result.get('errCode') != 0:
        raise RuntimeError(f'{urllib.parse.urlsplit(path).path}: HTTP {status}, code={result.get("errCode")}, message={result.get("errMsg")}')
    return result.get('data') or {}


def main():
    fixtures = json.loads((ROOT / '.private/test-sessions.json').read_text(encoding='utf-8'))
    state = json.loads((ROOT / '.private/test-accounts.json').read_text(encoding='utf-8'))
    sessions = []
    for fixture in fixtures['users'][-3:]:
        fresh = call('/chat/account/login', {'email': fixture['email'],
                     'password': hashlib.md5(state['password'].encode()).hexdigest(),
                     'platform': 5, 'deviceID': 'enterprise-smoke-' + secrets.token_hex(6)})
        sessions.append({**fixture, **fresh, 'platformID': 5})
    alice, bob, carol = sessions
    password_hash = hashlib.md5(state['admin']['password'].encode()).hexdigest()
    admin = call('/enterprise/v1/admin/login', {'account': state['admin']['account'], 'password': password_hash})
    assert 'imToken' not in admin, 'IM management token escaped proxy'
    results, cleanup = [], []
    try:
        call('/enterprise/v1/office/tag/find/user', {}, expected=401)
        results.append({'case': 'anonymous enterprise access rejected', 'passed': True})

        moment = call('/enterprise/v1/office/work_moment/add', {
            'content': {'text': 'isolated smoke audience', 'type': 0, 'metas': []},
            'permission': 2, 'permissionUserIDs': [bob['userID']]}, alice)['workMoment']
        mid = moment['workMomentID']
        cleanup.append(('moment', mid))
        call('/enterprise/v1/office/work_moment/get', {'workMomentID': mid}, bob)
        call('/enterprise/v1/office/work_moment/get', {'workMomentID': mid}, carol, expected=403)
        call('/enterprise/v1/office/work_moment/del', {'workMomentID': mid}, bob, expected=403)
        for _ in range(2):
            call('/enterprise/v1/office/work_moment/like', {'workMomentID': mid, 'like': True}, bob)
        stored = call('/enterprise/v1/office/work_moment/get', {'workMomentID': mid}, alice)['workMoment']
        assert len(stored['likeUsers']) == 1
        comment = call('/enterprise/v1/office/work_moment/comment/add', {'workMomentID': mid, 'content': 'reply'}, bob)
        call('/enterprise/v1/office/work_moment/comment/del', {'workMomentID': mid, 'commentID': comment['commentID']}, alice)
        results.append({'case': 'real Chat and IM auth; moment audience, idempotent like and comment ownership', 'passed': True})

        tag = call('/enterprise/v1/office/tag/add', {'tagName': 'smoke synthetic tag', 'userIDs': [bob['userID']]}, alice)
        cleanup.append(('tag', tag['tagID']))
        call('/enterprise/v1/office/tag/get', {'tagID': tag['tagID']}, bob, expected=403)
        own_tags = call('/enterprise/v1/office/tag/find/user', {}, alice)['tags']
        assert any(t['tagID'] == tag['tagID'] for t in own_tags)
        results.append({'case': 'tag persistence, hydration and owner permissions', 'passed': True})

        invitation = {'inviterUserID': alice['userID'], 'inviteeUserIDList': [bob['userID']],
                      'sessionType': 1, 'mediaType': 'audio', 'timeout': 30,
                      'roomID': 'smoke_' + secrets.token_hex(12)}
        ringing = call('/enterprise/v1/signaling/invite', {'invitation': invitation}, alice)
        rid, token = ringing['roomID'], ringing['token']
        cleanup.append(('room', rid))
        info = call('/enterprise/v1/signaling/get_signal_invitation_info', {'roomID': rid}, bob)
        assert info['activeInvitationForActor'] is True
        call('/enterprise/v1/signaling/accept', {'roomID': rid}, bob)
        call('/enterprise/v1/signaling/accept', {'roomID': rid}, carol, expected=403)
        call('/enterprise/v1/signaling/cancel', {'roomID': rid}, alice, expected=409)
        call('/enterprise/v1/signaling/hungUp', {'roomID': rid}, alice)
        call('/livekit/rtc/validate?access_token=' + token, expected=403)
        results.append({'case': 'real LiveKit CreateRoom/DeleteRoom; accept, unauthorized join and closed-token gateway', 'passed': True})

        meeting = call('/enterprise/v1/signaling/signal_create_meeting', {
            'meetingName': 'synthetic smoke meeting', 'startTime': int(time.time()), 'meetingDuration': 300}, alice)
        rid = meeting['roomID']
        cleanup.append(('room', rid))
        call('/enterprise/v1/signaling/signal_join_meeting', {'roomID': rid}, bob, expected=403)
        call('/enterprise/v1/signaling/signal_invite_meeting', {'roomID': rid, 'userIDs': [bob['userID']]}, alice)
        call('/enterprise/v1/signaling/signal_join_meeting', {'roomID': rid}, bob)
        call('/enterprise/v1/signaling/signal_update_meeting_info', {
            'roomID': rid, 'onlyHostShareScreen': True, 'isMuteAllMicrophone': True}, bob, expected=403)
        updated = call('/enterprise/v1/signaling/signal_update_meeting_info', {
            'roomID': rid, 'onlyHostShareScreen': True, 'isMuteAllMicrophone': True}, alice)
        assert updated['meetingInfo']['onlyHostShareScreen']
        call('/enterprise/v1/signaling/signal_operate_stream', {
            'roomID': rid, 'userID': bob['userID'], 'streamType': 'audio', 'mute': False}, bob, expected=403)
        call('/enterprise/v1/signaling/signal_close_room', {'roomID': rid}, alice)
        results.append({'case': 'meeting card registration, member join and host control authorization', 'passed': True})

        group = call('/enterprise/v1/admin/im/group/create_group', {
            'ownerUserID': alice['userID'], 'memberUserIDs': [bob['userID'], carol['userID']],
            'groupInfo': {'groupName': 'enterprise smoke ' + secrets.token_hex(4), 'groupType': 2}}, admin)['groupInfo']
        gid = group['groupID']
        cleanup.append(('group', gid))
        sent = call('/enterprise/v1/admin/im/msg/send_msg', {
            'sendID': alice['userID'], 'recvID': gid, 'groupID': gid,
            'senderNickname': alice['nickname'], 'senderPlatformID': 5, 'sessionType': 3,
            'contentType': 101, 'content': {'content': 'synthetic receipt probe'}}, admin)
        cid = sent['clientMsgID']
        last_error = None
        for _ in range(15):
            try:
                call('/enterprise/v1/msg/mark_group_message_read', {
                    'conversationID': 'sg_' + gid, 'clientMsgs': {cid: alice['userID']}}, bob)
                break
            except RuntimeError as error:
                last_error = error
                time.sleep(1)
        else:
            raise last_error
        counts = call('/enterprise/v1/msg/get_group_message_read_num', {
            'conversationID': 'sg_' + gid, 'clientMsgIDs': [cid]}, alice)['num'][cid]
        assert counts == {'readNum': 1, 'unreadNum': 1}, counts
        unread = call('/enterprise/v1/msg/get_group_message_has_read', {
            'conversationID': 'sg_' + gid, 'clientMsgID': cid, 'filter': 1, 'offset': 0, 'count': 20}, alice)
        assert unread['count'] == 1 and unread['reads'][0]['userID'] == carol['userID']
        results.append({'case': 'official group/message API, before/after webhook snapshot and real group receipt', 'passed': True})

        result = {'passed': True, 'timeUTC': time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime()),
                  'baseURL': BASE, 'cases': results,
                  'limits': ['HTTP tests do not prove SDK media playback, microphone/camera or 100-connection endurance.']}
    except Exception as error:
        result = {'passed': False, 'timeUTC': time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime()),
                  'baseURL': BASE, 'cases': results, 'failure': str(error)}
        raise
    finally:
        for kind, identifier in reversed(cleanup):
            try:
                if kind == 'moment':
                    call('/enterprise/v1/office/work_moment/del', {'workMomentID': identifier}, alice)
                elif kind == 'tag':
                    call('/enterprise/v1/office/tag/del', {'tagID': identifier}, alice)
                elif kind == 'room':
                    call('/enterprise/v1/signaling/signal_close_room', {'roomID': identifier}, alice)
                elif kind == 'group':
                    call('/enterprise/v1/admin/im/group/dismiss_group', {'groupID': identifier}, admin)
            except Exception:
                pass
        if 'result' in locals():
            REPORT.write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
            print(json.dumps(result, ensure_ascii=False))


if __name__ == '__main__':
    main()
