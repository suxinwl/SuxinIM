import importlib.util
import json
import pathlib
import tempfile
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location('final_collector', ROOT / 'tests/collect-final-load.py')
COLLECT = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(COLLECT)


class CollectorTests(unittest.TestCase):
    def test_safe_metadata_and_public_hashes_are_allowed(self):
        COLLECT.scan_payload('run.json', json.dumps({'runID': COLLECT.RUN, 'tokensPrinted': False,
                                                    'minimalFixtureSHA256': 'a' * 64}).encode())

    def test_sensitive_nested_or_serialized_json_fields_are_refused(self):
        for value in ({'users': [{'imToken': 'synthetic'}]}, {'password': None},
                      {'error': '{"chatToken":"synthetic"}'}, {'private_key': 'synthetic'}):
            with self.assertRaises(ValueError):
                COLLECT.scan_payload('run.json', json.dumps(value).encode())

    def test_jsonl_and_log_credential_fields_are_refused(self):
        for name in ('resources.jsonl', 'linux50a.log'):
            with self.assertRaises(ValueError):
                COLLECT.scan_payload(name, b'{"stage":"synthetic","accessToken":"not-a-real-token"}\n')

    def test_private_pem_and_jwt_shape_are_refused(self):
        for data in (b'-----BEGIN RSA PRIVATE KEY-----\n' + b'A' * 90 + b'\n-----END RSA PRIVATE KEY-----',
                     b'eyJ' + b'A' * 20 + b'.' + b'B' * 20 + b'.' + b'C' * 20):
            with self.assertRaises(ValueError):
                COLLECT.scan_payload('linux50a.log', data)

    def test_allowlist_and_ordinary_file_guard(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = pathlib.Path(temporary).resolve()
            with self.assertRaises(ValueError):
                COLLECT.read_member(root, '../.private/test-sessions.json')
            (root / 'run.json').mkdir()
            with self.assertRaises(ValueError):
                COLLECT.read_member(root, 'run.json')

    def test_symlink_is_refused(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = pathlib.Path(temporary).resolve()
            target = root / 'ordinary.json'; target.write_text('{}')
            try:
                (root / 'run.json').symlink_to(target)
            except OSError:
                self.skipTest('This local Windows process cannot create symlinks; Linux O_NOFOLLOW remains enforced')
            with self.assertRaises(ValueError):
                COLLECT.read_member(root, 'run.json')


if __name__ == '__main__':
    unittest.main()
