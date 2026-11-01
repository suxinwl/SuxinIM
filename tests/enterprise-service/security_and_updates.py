"""Real-stack revocation and Chat application-catalog checks on synthetic users.

Only fixture users 108-110 are logged in/revoked. Passwords are restored in
finally, and temporary Linux metadata records are deleted from Chat afterward.
"""
import hashlib
import json
import pathlib
import secrets
import time
import urllib.error
import urllib.request

from smoke import BASE, ROOT, TLS, call

REPORT = ROOT / 'tests/enterprise-service/security-update-report.json'


def digest(value):
    return hashlib.md5(value.encode()).hexdigest()


def main():
    state = json.loads((ROOT / '.private/test-accounts.json').read_text(encoding='utf-8'))
    users = state['users'][-3:]
    admin = call('/enterprise/v1/admin/login', {
        'account': state['admin']['account'], 'password': digest(state['admin']['password'])})
    password = state['password']
    results = []
    cleanup_rooms = []
    linux_ids = []
    blocked = None
    changed = None

    def login(user, platform=3, raw_password=None):
        data = call('/chat/account/login', {'email': user['email'], 'password': digest(raw_password or password),
                    'platform': platform, 'deviceID': 'enterprise-security-' + secrets.token_hex(6)})
        return {**user, **data, 'platformID': platform}

    try:
        for endpoint in ('get_admin_token', 'get_user_token'):
            call('/enterprise/v1/admin/im/auth/' + endpoint, {}, admin, expected=403)
        for endpoint in ('/user/rtc/get_token', '/rtc/get_token_for_video_meeting'):
            call('/enterprise/v1/admin/business' + endpoint, {}, admin, expected=403)
            request = urllib.request.Request(BASE + '/chat' + endpoint, b'{}', {'Content-Type': 'application/json'})
            try:
                urllib.request.urlopen(request, context=TLS, timeout=15)
            except urllib.error.HTTPError as error:
                assert error.code == 403, 'raw Chat RTC route was not blocked'
            else:
                raise AssertionError('raw Chat RTC route bypassed invitation boundary')
        pc = login(users[0], 3)
        call('/enterprise/v1/admin/im/auth/force_logout', {'userID': users[1]['userID'], 'platformID': 3}, pc, expected=403)
        results.append({'case': 'management token issuance denied; ordinary user cannot use management proxy', 'passed': True})

        web = login(users[0], 5)
        mobile = login(users[0], 2)
        for session in (pc, web, mobile):
            call('/enterprise/v1/office/tag/find/user', {}, session)
        meeting = call('/enterprise/v1/signaling/signal_create_meeting', {
            'meetingName': 'old media token probe', 'startTime': int(time.time()), 'meetingDuration': 300}, pc)
        cleanup_rooms.append((meeting['roomID'], users[0]))
        time.sleep(1.2)
        new_pc = login(users[0], 3)
        call('/enterprise/v1/office/tag/find/user', {}, pc, expected=401)
        for session in (new_pc, web, mobile):
            call('/enterprise/v1/office/tag/find/user', {}, session)
        call('/livekit/rtc/validate?access_token=' + meeting['token'], expected=401)
        call('/enterprise/v1/signaling/signal_close_room', {'roomID': meeting['roomID']}, new_pc)
        results.append({'case': 'policy 4: PC+Web+mobile coexist; new PC kicks old PC and old media session', 'passed': True})

        victim = login(users[1], 3)
        call('/enterprise/v1/admin/im/auth/force_logout', {'userID': victim['userID'], 'platformID': 3}, admin)
        call('/enterprise/v1/office/tag/find/user', {}, victim, expected=401)
        results.append({'case': 'administrator force_logout invalidates old enterprise access', 'passed': True})

        victim = login(users[1], 3)
        call('/enterprise/v1/admin/chat/block/add', {'userID': victim['userID'], 'reason': 'isolated synthetic verification'}, admin)
        blocked = victim['userID']
        call('/enterprise/v1/office/tag/find/user', {}, victim, expected=401)
        try:
            login(users[1], 3)
        except RuntimeError:
            pass
        else:
            raise AssertionError('blocked account logged in')
        call('/enterprise/v1/admin/chat/block/del', {'userIDs': [victim['userID']]}, admin)
        blocked = None
        login(users[1], 3)
        results.append({'case': 'block rejects old sessions and new login; unblock restores fresh login', 'passed': True})

        victim = login(users[2], 3)
        temporary = secrets.token_urlsafe(20) + 'A9'
        call('/chat/account/password/change', {
            'userID': victim['userID'], 'currentPassword': digest(password), 'newPassword': digest(temporary)},
            {'adminToken': victim['chatToken']})
        changed = victim['userID']
        call('/enterprise/v1/office/tag/find/user', {}, victim, expected=401)
        login(users[2], 3, temporary)
        call('/enterprise/v1/admin/chat/user/password/reset', {'userID': victim['userID'], 'newPassword': digest(password)}, admin)
        changed = None
        login(users[2], 3)
        results.append({'case': 'normal password change revokes old Chat+IM sessions; credentials restored', 'passed': True})

        number = int(time.time())
        version1, version2 = f'0.0.1-smoke.{number}', f'0.0.2-smoke.{number}'
        payload = b'isolated metadata probe; not a client installer\n'
        metadata = {'platform': 'linux', 'version': version1, 'build': number,
                    'url': BASE + '/downloads/intentionally-missing-metadata-probe.bin',
                    'sha256': hashlib.sha256(payload).hexdigest(), 'size': len(payload), 'force': False,
                    'text': 'Temporary metadata-only API verification; do not install'}
        call('/enterprise/v1/application/publish', metadata, new_pc, expected=403)
        published = call('/enterprise/v1/application/publish', metadata, admin)
        linux_ids.append(published['chatVersionID'])
        available = call('/enterprise/v1/application/latest', {'platform': 'linux', 'version': '0.0.0', 'build': 0})
        assert available['available'] and not available['force'] and available['sha256'] == metadata['sha256']
        forced = {**metadata, 'version': version2, 'build': number + 1, 'force': True}
        published = call('/enterprise/v1/application/publish', forced, admin)
        linux_ids.append(published['chatVersionID'])
        available = call('/enterprise/v1/application/latest', {'platform': 'linux', 'version': version1, 'build': number})
        assert available['available'] and available['force'] and available['version'] == version2
        no_update = call('/enterprise/v1/application/latest', {'platform': 'linux', 'version': '9.0.0', 'build': number + 10})
        assert no_update['available'] is False
        call('/enterprise/v1/application/publish', {**forced, 'sha256': 'ff' * 32}, admin, expected=409)
        call('/enterprise/v1/application/publish', {**metadata, 'version': '0.0.0', 'build': number + 2}, admin, expected=409)
        call('/enterprise/v1/application/publish', {**forced, 'version': '0.0.3', 'sha256': 'invalid'}, admin, expected=400)
        call('/enterprise/v1/application/publish', {**forced, 'version': '0.0.3', 'url': 'http://untrusted.invalid/a.bin'}, admin, expected=400)
        results.append({'case': 'real Chat catalog Add/Latest; optional/forced metadata, no downgrade, immutable checksum and validation', 'passed': True,
                        'limits': 'Linux metadata probe is not an installer or a client installation test.'})

        result = {'passed': True, 'baseURL': BASE, 'timeUTC': time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime()), 'cases': results}
    except Exception as error:
        result = {'passed': False, 'baseURL': BASE, 'timeUTC': time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime()),
                  'cases': results, 'failure': str(error)}
        raise
    finally:
        if blocked:
            call('/enterprise/v1/admin/chat/block/del', {'userIDs': [blocked]}, admin)
        if changed:
            call('/enterprise/v1/admin/chat/user/password/reset', {'userID': changed, 'newPassword': digest(password)}, admin)
        for rid, user in cleanup_rooms:
            try:
                session = login(user)
                call('/enterprise/v1/signaling/signal_close_room', {'roomID': rid}, session)
            except Exception:
                pass
        if linux_ids:
            call('/enterprise/v1/admin/chat/application/delete_version', {'id': linux_ids}, admin)
        if 'result' in locals():
            REPORT.write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
            print(json.dumps(result, ensure_ascii=False))


if __name__ == '__main__':
    main()
