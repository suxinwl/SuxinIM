"""Offline stdlib-only regression for generated/rotated storage argv contracts."""
import ast
import copy
import json
import os
import pathlib
import subprocess
import tempfile
import unittest


ROOT = pathlib.Path(__file__).resolve().parents[1]
SOURCE = ROOT / 'tools/generate_instance.py'
TREE = ast.parse(SOURCE.read_text(encoding='utf-8'))
PATCH_NODE = next(node for node in TREE.body
                  if isinstance(node, ast.FunctionDef)
                  and node.name == 'harden_storage_process_arguments')
SCOPE = {}
exec(compile(ast.Module(body=[PATCH_NODE], type_ignores=[]), str(SOURCE), 'exec'), SCOPE)
patch = SCOPE['harden_storage_process_arguments']


def model():
    return {'name': 'owned-project', 'volumes': {'owned-data': {}},
            'services': {
                'mongo': {'environment': {'MONGO_INITDB_ROOT_USERNAME': 'root',
                                          'MONGO_INITDB_ROOT_PASSWORD': 'b' * 64},
                          'healthcheck': {'test': ['old'], 'interval': '10s', 'retries': 20},
                          'volumes': ['./relative:/owned'], 'labels': {'owner': 'kept'}},
                'redis': {'environment': {'REDIS_PASSWORD': 'a' * 64},
                          'command': ['old'], 'ports': ['127.0.0.1:16379:6379'],
                          'healthcheck': {'test': ['old'], 'interval': '5s', 'timeout': '3s'},
                          'volumes': ['owned-data:/data']},
                'other': {'command': ['unchanged'], 'environment': {'KEY': 'c' * 64}}}}


def without_targets(value):
    value = copy.deepcopy(value)
    del value['services']['redis']['command']
    del value['services']['redis']['healthcheck']['test']
    del value['services']['mongo']['healthcheck']['test']
    return value


class StorageArgumentsTests(unittest.TestCase):
    def test_only_three_reviewed_fields_change(self):
        before = model()
        after = patch(copy.deepcopy(before))
        self.assertEqual(without_targets(before), without_targets(after))
        self.assertNotEqual(before['services']['redis']['command'],
                            after['services']['redis']['command'])

    def test_idempotent(self):
        once = patch(model())
        self.assertEqual(once, patch(copy.deepcopy(once)))

    def test_rotation_worker_and_generator_have_identical_model_patch(self):
        source = ROOT / 'deploy/production/rotate_runtime_keys.py'
        tree = ast.parse(source.read_text(encoding='utf-8'))
        node = next(node for node in tree.body
                    if isinstance(node, ast.FunctionDef)
                    and node.name == 'harden_storage_process_arguments')
        scope = {}
        exec(compile(ast.Module(body=[node], type_ignores=[]), str(source), 'exec'), scope)
        self.assertEqual(patch(model()), scope[node.name](model()))

    def test_generator_calls_shared_patch_before_dump(self):
        generator = next(node for node in TREE.body
                         if isinstance(node, ast.FunctionDef) and node.name == 'generate')
        self.assertTrue(any(isinstance(node, ast.Call)
                            and isinstance(node.func, ast.Name)
                            and node.func.id == 'harden_storage_process_arguments'
                            for node in ast.walk(generator)))

    def test_credentials_are_not_substituted_into_command_argv(self):
        services = patch(model())['services']
        commands = [services['redis']['command'], services['redis']['healthcheck']['test'],
                    services['mongo']['healthcheck']['test']]
        text = json.dumps(commands)
        for sentinel in ('a' * 64, 'b' * 64, 'c' * 64):
            self.assertNotIn(sentinel, text)
        self.assertNotIn('--requirepass', text)
        self.assertNotIn(' -p ', text)
        self.assertNotIn(' -a ', text)

    def test_redis_requires_restricted_config_and_compose_escaping(self):
        command = patch(model())['services']['redis']['command']
        self.assertEqual(command[:2], ['sh', '-ec'])
        script = command[2]
        self.assertIn('umask 077;', script)
        self.assertIn('$$(mktemp /tmp/puty-redis.XXXXXX.conf)', script)
        self.assertIn('printf ', script)
        self.assertIn('[ "$${#REDIS_PASSWORD}" -eq 64 ]', script)
        self.assertTrue(script.endswith('exec redis-server "$$puty_redis_config"'))
        self.assertIn('REDISCLI_AUTH="$$REDIS_PASSWORD"',
                      patch(model())['services']['redis']['healthcheck']['test'][1])

    def test_mongo_uses_fixed_js_authentication_from_environment(self):
        command = patch(model())['services']['mongo']['healthcheck']['test']
        self.assertEqual(command[:5], ['CMD', 'mongosh', '--nodb', '--quiet', '--eval'])
        self.assertIn('process.env.MONGO_INITDB_ROOT_PASSWORD', command[5])
        self.assertIn('process.env.MONGO_INITDB_ROOT_USERNAME', command[5])
        self.assertIn('new Mongo(', command[5])
        self.assertIn('runCommand({ping:1})', command[5])

    def test_missing_owned_service_rejected_without_partial_patch(self):
        value = model()
        del value['services']['mongo']['healthcheck']
        before = copy.deepcopy(value)
        with self.assertRaisesRegex(ValueError, '^storage_process_model_invalid$'):
            patch(value)
        self.assertEqual(before, value)

    @unittest.skipUnless(os.name == 'posix', 'real shell/config mode check requires POSIX')
    def test_actual_shell_exec_passes_only_private_config_path(self):
        with tempfile.TemporaryDirectory() as folder:
            stub = pathlib.Path(folder) / 'redis-server'
            stub.write_text('#!/bin/sh\nset -eu\n'
                            'test "$#" = 1\n'
                            'test "$(stat -c %a "$1")" = 600\n'
                            'test "$(head -n 1 "$1")" = "requirepass $REDIS_PASSWORD"\n'
                            'test "$(tail -n 1 "$1")" = "appendonly yes"\n'
                            'printf "argv_path_only_verified\\n"\nrm -f "$1"\n')
            stub.chmod(0o700)
            script = patch(model())['services']['redis']['command'][2].replace('$$', '$')
            environment = {**os.environ, 'REDIS_PASSWORD': 'a' * 64,
                           'PATH': folder + os.pathsep + os.environ.get('PATH', '')}
            result = subprocess.run(['sh', '-ec', script], env=environment,
                                    capture_output=True, text=True, timeout=10)
            self.assertEqual((result.returncode, result.stdout, result.stderr),
                             (0, 'argv_path_only_verified\n', ''))
            environment['REDIS_PASSWORD'] = 'not-hex;invalid'
            result = subprocess.run(['sh', '-ec', script], env=environment,
                                    capture_output=True, text=True, timeout=10)
            self.assertNotEqual(result.returncode, 0)
            self.assertEqual(result.stdout + result.stderr, '')


if __name__ == '__main__':
    unittest.main()
