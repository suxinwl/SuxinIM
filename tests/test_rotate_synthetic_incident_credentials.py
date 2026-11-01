"""Local guards only. Never opens fixtures, network, subprocess, or the AVD."""
import copy
import contextlib
import importlib.util
import io
from pathlib import Path
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location('rotation', ROOT / 'tools/rotate_synthetic_incident_credentials.py')
rotation = importlib.util.module_from_spec(spec)
spec.loader.exec_module(rotation)


class RotationGuards(unittest.TestCase):
    def setUp(self):
        self.old = 'local-fixture-not-an-actual-credential'
        self.sha = patch.object(rotation, 'OLD_SHA', rotation.digest(self.old.encode()))
        self.sha.start()
        self.addCleanup(self.sha.stop)
        self.accounts = {'baseURL': rotation.BASE, 'password': self.old,
                         'admin': {'account': 'independentAdmin', 'password': 'different-local-fixture'},
                         'users': [{'userID': uid, 'account': 'im38u' + uid[-3:],
                                    'email': 'u' + uid[-3:] + '@openim38.test', 'nickname': uid}
                                   for uid in rotation.IDS]}
        self.sessions = {'baseURL': rotation.BASE, 'users': [
            {**user, 'chatToken': 'fake-chat', 'imToken': 'fake-im', 'platformID': 5}
            for user in self.accounts['users']]}

    def test_exact_scope_accepts(self):
        rotation.validate_scope(self.accounts, self.sessions)

    def test_host_and_user_set_rejected(self):
        for fixture, key, value in ((self.accounts, 'baseURL', 'https://example.com'),
                                     (self.sessions, 'baseURL', 'http://192.168.10.11:29880')):
            bad = copy.deepcopy(fixture)
            bad[key] = value
            with self.assertRaises(rotation.GuardError):
                rotation.validate_scope(bad if fixture is self.accounts else self.accounts,
                                        bad if fixture is self.sessions else self.sessions)
        self.accounts['users'][0]['userID'] = 'realEmployee'
        with self.assertRaises(rotation.GuardError):
            rotation.validate_scope(self.accounts, self.sessions)

    def test_old_sha_admin_and_email_guards(self):
        for mutate in (lambda a: a.update(password='unrelated-secret'),
                       lambda a: a['admin'].update(password=self.old),
                       lambda a: a['users'][0].update(email='real@example.com')):
            a = copy.deepcopy(self.accounts)
            mutate(a)
            with self.assertRaises(rotation.GuardError):
                rotation.validate_scope(a, self.sessions)

    def test_platform_and_dual_token_guards(self):
        for key, value in (('platformID', 0), ('platformID', True), ('chatToken', ''), ('imToken', None)):
            s = copy.deepcopy(self.sessions)
            s['users'][0][key] = value
            with self.assertRaises(rotation.GuardError):
                rotation.validate_scope(self.accounts, s)

    def test_path_and_root_guards(self):
        with self.assertRaises(rotation.GuardError):
            rotation.fixed_path('.private/../unrelated.json')
        with self.assertRaises(rotation.GuardError):
            rotation.validate_root(Path('C:/unrelated'))

    def test_existing_plan_never_overwritten(self):
        class Existing:
            def exists(self):
                return True
        with patch.object(rotation, 'validate_root'), patch.object(rotation, 'fixed_path', return_value=Existing()), \
                patch.object(rotation, 'private_write') as writer:
            with self.assertRaises(rotation.GuardError):
                rotation.prepare()
            writer.assert_not_called()

    def test_client_redirect_rejected_without_following(self):
        with self.assertRaises(rotation.GuardError):
            rotation.NoRedirect().redirect_request(None, None, 302, '', {}, 'https://example.com')

    def test_public_report_excludes_secret_and_tokens(self):
        report = rotation.public_status({'users': {uid: {} for uid in rotation.IDS},
                                         'status': 'prepared', 'fixtureCommitted': False,
                                         'newSecretSHA256': 'a' * 64, 'newPassword': 'do-not-publish'})
        import json
        encoded = json.dumps(report)
        self.assertNotIn('do-not-publish', encoded)
        self.assertNotIn('newPassword', encoded)

    def test_only_explicit_chat_and_im_denials_accepted(self):
        for im in (1506, 1507):
            self.assertTrue(rotation.seed_pair_denied({'chatCode': 20101, 'imCode': im}))
        for codes in (None, {}, {'chatCode': 0, 'imCode': 1506},
                      {'chatCode': 20101, 'imCode': 0}, {'chatCode': 20101, 'imCode': 9999}):
            self.assertFalse(rotation.seed_pair_denied(codes))
        report = rotation.public_status({'users': {'one': {
            'seedBefore': {'chatCode': 20101, 'imCode': 1507},
            'seedAfter': {'chatCode': 20101, 'imCode': 1507}}},
            'status': 'executing', 'fixtureCommitted': False, 'newSecretSHA256': 'a' * 64})
        self.assertEqual(report['seedAfterRequiredRevokedCodes'], 1)
        self.assertEqual(report['seedAfterIMTokenNotExist1507'], 1)
        self.assertEqual(report['seedBeforeValidBoth'], 0)
        self.assertEqual(report['seedBeforeValidChat'], 0)
        self.assertEqual(report['seedBeforeValidIM'], 0)

    def test_uncertain_reset_and_partial_fixture_commit_resume_same_secret(self):
        # Entire filesystem/API are in-memory boundaries. This does not read
        # .private, run icacls, or perform TLS/network actions.
        store = {rotation.ACCOUNTS: rotation.encode(self.accounts),
                 rotation.SESSIONS: rotation.encode(self.sessions), rotation.CA: b'fake-ca'}
        class MemoryPath:
            def __init__(self, key):
                self.key = key
            def exists(self):
                return self.key in store
            def read_bytes(self):
                return store[self.key]
        interrupted = {'reset': True, 'commit': True}
        reset_values = []
        reset_users = set()
        def write(path, raw, exclusive=False):
            if exclusive and path.exists():
                raise rotation.GuardError('existing fake plan')
            if path.key == rotation.SESSIONS and interrupted['commit'] and reset_users:
                interrupted['commit'] = False
                raise rotation.GuardError('simulated interrupted second replacement')
            store[path.key] = raw
        old_md5 = rotation.md5(self.old)
        class FakeClient:
            def call(self, path, body, headers=None):
                if path.endswith('/admin/login'):
                    return 0, {'adminToken': 'fake-admin-only'}
                if path.endswith('/user/password/reset'):
                    reset_values.append(body['newPassword'])
                    reset_users.add(body['userID'])
                    if interrupted['reset']:
                        interrupted['reset'] = False
                        raise rotation.GuardError('simulated lost reset response')
                    return 0, {}
                if path == '/chat/account/login':
                    if body['password'] == old_md5:
                        return 20001, {}
                    uid = 'upgrade38_u' + body['email'][1:4]
                    return 0, {'userID': uid, 'chatToken': 'new-chat-' + uid, 'imToken': 'new-im-' + uid}
                uid = body['userIDs'][0]
                if uid not in reset_users:
                    return 0, {}
                return (20101 if path.startswith('/chat/') else 1506), {}
        with patch.object(rotation, 'validate_root'), \
                patch.object(rotation, 'fixed_path', side_effect=MemoryPath), \
                patch.object(rotation, 'private_write', side_effect=write), \
                patch.object(rotation, 'CA_SHA', rotation.digest(b'fake-ca')), \
                patch.object(rotation, 'Client', FakeClient), contextlib.redirect_stdout(io.StringIO()):
            plan = rotation.prepare()
            replacement = plan['newPassword']
            with self.assertRaises(rotation.GuardError):
                rotation.prepare()
            with self.assertRaises(rotation.GuardError):
                rotation.execute()  # reset applied but response lost
            with self.assertRaises(rotation.GuardError):
                rotation.execute()  # all verified; account replaced, session replace interrupted
            finished = rotation.execute()
            self.assertEqual(finished['status'], 'complete')
            self.assertEqual(finished['newPassword'], replacement)
            self.assertEqual(set(reset_values), {rotation.md5(replacement)})
            self.assertEqual(len(reset_values), 111)  # only uncertain first reset repeated
            self.assertEqual(rotation.parse(store[rotation.ACCOUNTS])['password'], replacement)
            self.assertEqual(len(rotation.parse(store[rotation.SESSIONS])['users']), 110)
            self.assertEqual(rotation.public_status(finished)['seedBeforeValidBoth'], 110)


if __name__ == '__main__':
    unittest.main()
