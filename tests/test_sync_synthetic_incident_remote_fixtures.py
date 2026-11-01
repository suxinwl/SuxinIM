"""Pure local fake-data tests; no remote fixture, network, SSH or device access."""
import copy
import importlib.util
from pathlib import Path
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location('remote_sync', ROOT / 'tools/sync_synthetic_incident_remote_fixtures.py')
sync = importlib.util.module_from_spec(spec)
spec.loader.exec_module(sync)


class RemoteFixtureGuards(unittest.TestCase):
    def setUp(self):
        self.old, self.new = 'fake-old-local-only', 'fake-replacement-local-only'
        for key, value in (('OLD_SHA', sync.password_sha(self.old)), ('NEW_SHA', sync.password_sha(self.new))):
            manager = patch.object(sync, key, value)
            manager.start()
            self.addCleanup(manager.stop)
        users = [{'userID': uid, 'account': 'im38u' + uid[-3:], 'email': 'u' + uid[-3:] + '@openim38.test',
                  'nickname': 'preserve-' + uid, 'extra': {'nested': 'unchanged'}} for uid in sorted(sync.IDS)]
        self.accounts = {'baseURL': sync.BASE, 'password': self.old,
                         'admin': {'account': 'independent-admin', 'password': 'fake-admin-different'},
                         'adminSession': {'adminToken': 'fake-admin-token-unchanged'}, 'custom': {'x': 7}, 'users': users}
        self.accounts['users'][0]['password'] = self.old
        self.sessions = {'baseURL': sync.BASE, 'custom': [1, 2], 'users': [
            {**user, 'imToken': 'fake-old-im', 'chatToken': 'fake-old-chat', 'platformID': 5}
            for user in users]}
        self.payload = {'schema': 1, 'baseURL': sync.BASE, 'caFileSHA256': sync.CA_SHA,
                        'newSecretSHA256': sync.NEW_SHA, 'password': self.new, 'users': [
                            {**{k: user[k] for k in ('userID', 'account', 'email')},
                             'imToken': 'fake-new-im', 'chatToken': 'fake-new-chat', 'platformID': 2}
                            for user in users]}

    def test_replace_only_approved_fields_preserves_admin_and_metadata(self):
        old_a, old_s = copy.deepcopy(self.accounts), copy.deepcopy(self.sessions)
        a, s = sync.synchronized_data(self.accounts, self.sessions, self.payload)
        self.assertEqual(a['password'], self.new)
        self.assertEqual(a['users'][0]['password'], self.new)
        self.assertNotIn('password', a['users'][1])
        self.assertEqual(a['admin'], old_a['admin'])
        self.assertEqual(a['adminSession'], old_a['adminSession'])
        self.assertEqual(a['custom'], old_a['custom'])
        self.assertEqual(s['custom'], old_s['custom'])
        for before, after in zip(old_s['users'], s['users']):
            for key in before.keys() - {'imToken', 'chatToken', 'platformID'}:
                self.assertEqual(after[key], before[key])
            self.assertEqual(after['platformID'], 2)
        self.assertEqual(self.accounts, old_a)
        self.assertEqual(self.sessions, old_s)

    def test_same_payload_idempotent_and_partial_commit_supported(self):
        a, s = sync.synchronized_data(self.accounts, self.sessions, self.payload)
        self.assertEqual(sync.synchronized_data(a, s, self.payload), (a, s))
        self.assertEqual(sync.synchronized_data(a, self.sessions, self.payload), (a, s))

    def test_host_ca_secret_or_admin_in_payload_rejected(self):
        for key, value in (('baseURL', 'https://example.com'), ('caFileSHA256', '0' * 64),
                           ('password', self.old), ('newSecretSHA256', '0' * 64), ('admin', {})):
            payload = copy.deepcopy(self.payload)
            payload[key] = value
            with self.assertRaises(sync.Refused):
                sync.validate_payload(payload)

    def test_exact_user_identity_required(self):
        for key, value in (('userID', 'real-employee'), ('account', 'realAccount'), ('email', 'real@example.com'),
                           ('platformID', 0), ('platformID', True), ('imToken', '')):
            payload = copy.deepcopy(self.payload)
            payload['users'][0][key] = value
            with self.assertRaises(sync.Refused):
                sync.validate_payload(payload)
        missing = copy.deepcopy(self.payload)
        missing['users'].pop()
        with self.assertRaises(sync.Refused):
            sync.validate_payload(missing)

    def test_canonical_unrelated_secret_or_admin_shared_refused(self):
        for mutate in (lambda a: a.update(password='unrelated-secret'),
                       lambda a: a['admin'].update(password=self.old),
                       lambda a: a['users'][0].update(password='unrelated-secret'),
                       lambda a: a.update(baseURL='http://localhost:10008')):
            a = copy.deepcopy(self.accounts)
            mutate(a)
            with self.assertRaises(sync.Refused):
                sync.validate_fixtures(a, self.sessions)

    def test_probe_metadata_does_not_output_secret_values(self):
        import json
        encoded = json.dumps(sync.fixture_metadata(self.accounts))
        for value in (self.old, self.new, 'fake-admin-different', 'fake-admin-token-unchanged'):
            self.assertNotIn(value, encoded)
        self.assertTrue(sync.fixture_metadata(self.accounts)['globalPasswordMatchesIncidentOld'])

    def test_path_escape_rejected_before_filesystem(self):
        for path in (Path('/etc/shadow'), sync.ROOT / '../other/file'):
            with self.assertRaises(sync.Refused), patch.object(sync.os, 'lstat') as read:
                sync.checked_path(path)
            read.assert_not_called()

    def test_symlink_parent_refused(self):
        from pathlib import PurePosixPath
        import stat
        from types import SimpleNamespace
        root = PurePosixPath('/srv/openim38-upgrade')
        private = root / '.private'
        def fake_lstat(path):
            mode = stat.S_IFLNK if path == private else stat.S_IFDIR
            return SimpleNamespace(st_mode=mode | 0o700)
        with patch.object(sync, 'ROOT', root), patch.object(sync.os, 'lstat', side_effect=fake_lstat) as reader:
            with self.assertRaises(sync.Refused):
                sync.checked_path(private / 'test-accounts.json')
            reader.assert_any_call(private)


if __name__ == '__main__':
    unittest.main()
