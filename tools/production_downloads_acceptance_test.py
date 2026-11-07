"""Offline public update response checks, no credentials, network or real packages."""
import copy
import unittest
from tools.production_downloads_acceptance import validate_current, validate_latest


class UpdateAcceptanceTests(unittest.TestCase):
    def setUp(self):
        self.artifact = {'platform':'windows', 'version':'3.8.11', 'build':381101,
                         'fileName':'PUTY_IM-3.8.11-win-x64.exe', 'sha256':'a'*64, 'size':1000}
        self.latest = {'errCode':0, 'data':{**{k:v for k,v in self.artifact.items() if k!='fileName'},
            'available':True, 'force':True,
            'url':'https://192.168.10.11:9885/downloads/PUTY_IM-3.8.11-win-x64.exe'}}

    def test_exact_current_metadata_and_mandatory_policy(self):
        validate_latest(self.latest, self.artifact, '192.168.10.11', True)
        validate_current({'errCode':0, 'data':{'available':False}})

    def test_default_nonmandatory_policy_rejects_forced_offer(self):
        with self.assertRaises(AssertionError):
            validate_latest(self.latest, self.artifact, '192.168.10.11')
        value = copy.deepcopy(self.latest)
        value['data']['force'] = False
        self.assertIs(validate_latest(value, self.artifact, '192.168.10.11'), value['data'])

    def test_force_matches_both_explicit_policy_values_exactly(self):
        for required in (False, True):
            for force in (False, True, None, 0, 1, 'false', 'true'):
                value = copy.deepcopy(self.latest)
                value['data']['force'] = force
                with self.subTest(required=required, force=force):
                    if type(force) is bool and force is required:
                        self.assertIs(validate_latest(value, self.artifact, '192.168.10.11', required), value['data'])
                    else:
                        with self.assertRaises(AssertionError):
                            validate_latest(value, self.artifact, '192.168.10.11', required)

    def test_optional_or_missing_or_nonboolean_force_never_passes_required_check(self):
        for force in (False, None, 1, 'true'):
            value = copy.deepcopy(self.latest)
            value['data']['force'] = force
            with self.assertRaises(AssertionError):
                validate_latest(value, self.artifact, '192.168.10.11', True)
        value = copy.deepcopy(self.latest)
        value['data']['force'] = False
        validate_latest(value, self.artifact, '192.168.10.11', False)

    def test_each_immutable_field_or_wrong_entry_is_rejected(self):
        for key, invalid in (('version','3.8.10'), ('build',381001), ('size',999), ('sha256','b'*64),
                             ('platform','android'), ('url','https://im.szputy.com:9885/downloads/other.exe')):
            value = copy.deepcopy(self.latest)
            value['data'][key] = invalid
            with self.assertRaises(AssertionError):
                validate_latest(value, self.artifact, '192.168.10.11', True)
        value = copy.deepcopy(self.latest)
        value['data']['url'] = value['data']['url'].replace('192.168.10.11', 'im.szputy.com')
        validate_latest(value, self.artifact, 'im.szputy.com', True)

    def test_current_offer_or_api_error_is_not_success(self):
        for value in ({'errCode':0,'data':{'available':True}}, {'errCode':0,'data':{'available':0}},
                      {'errCode':1,'data':{'available':False}}, {'errCode':0,'data':None}):
            with self.assertRaises(AssertionError):
                validate_current(value)


if __name__ == '__main__':
    unittest.main()
