"""Check only already-logged-out credentials after exact fixture cleanup."""
import json
import sys

import production_identity_acceptance as a


def main():
    a.REPORT = a.PRIVATE/'identity-after-cleanup-denials-final-report.json'
    state = a.read(a.PRIVATE/'identity-post-rotation-state.json')
    for slot in ('aandroid', 'awindows', 'bweb'):
        session = state[slot]
        result = a.native(session)
        a.record('deleted_fixture_native_token_rejected_'+slot, result[0] == 200 and (result[1].get('errCode'), result[1].get('errMsg')) in ((1506, 'TokenKickedError'), (1507, 'TokenNotExistError')), status=result[0], code=result[1].get('errCode'))
        result = a.call('/chat/user/find/full', {'userIDs': [session['userID']]}, session)
        a.record('deleted_fixture_Chat_token_rejected_'+slot, result[0] in (200, 401, 403) and result[1].get('errCode') == 20101, status=result[0], code=result[1].get('errCode'))
        result = a.live(session)
        a.record('deleted_fixture_enterprise_pair_rejected_'+slot, result[0] == 401 and result[1].get('errCode') == 1501, status=result[0], code=result[1].get('errCode'))
        result = a.call('/sso/refresh', {'sessionToken': session['sessionToken']})
        body = result[1] or {}
        a.record('deleted_fixture_opaque_SSO_rejected_'+slot, result[0] in (401, 403) and not (body.get('data') or {}).get('imToken'), status=result[0], code=body.get('code', body.get('errCode')))
    for name in ('_attachment', '_priorAndroidAttachment'):
        result = a.http(state[name]['grant']['url'])
        a.record('deleted_fixture_old_attachment_grant_rejected_'+name, result[0] in (401, 403), status=result[0])


if __name__ == '__main__':
    try:
        main()
    except Exception as error:
        print(json.dumps({'denialVerificationFailed': True, 'errorType': type(error).__name__, 'check': str(error) if isinstance(error, a.CheckFailure) else 'private_details_suppressed'}), flush=True)
        sys.exit(1)
