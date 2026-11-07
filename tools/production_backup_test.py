"""Offline security/roundtrip checks. Never connects to SSH or Docker."""
import importlib.util
import io
import json
import pathlib
import struct
import tarfile
import tempfile
import types
import unittest
from unittest import mock

HERE = pathlib.Path(__file__).resolve().parent


def load(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


local = load('production_backup', HERE / 'production_backup.py')
remote = load('backup_remote', HERE.parent / 'deploy/production/backup_remote.py')


class EncryptionTests(unittest.TestCase):
    def setUp(self):
        self.key = bytes(range(32))

    def encoded(self, payload=b'0123456789abcdef' * 700):
        output = io.BytesIO()
        metadata = local.encrypt_stream(io.BytesIO(payload), output, self.key, chunk_bytes=1024)
        return payload, output.getvalue(), metadata

    def test_roundtrip_large_multichunk_and_empty(self):
        for payload in (b'', b'a' * 3500, bytes(range(256)) * 65):
            original, encrypted, metadata = self.encoded(payload)
            plaintext = io.BytesIO()
            actual = local.decrypt_stream(io.BytesIO(encrypted), plaintext, self.key)
            self.assertEqual(plaintext.getvalue(), original)
            self.assertEqual(actual, metadata)
            self.assertEqual(actual['payloadBytes'], len(original))

    def test_ciphertext_tamper_wrong_key_truncation_and_appending_fail(self):
        _, encrypted, _ = self.encoded()
        tampered = bytearray(encrypted)
        tampered[-20] ^= 1
        for broken, key in ((bytes(tampered), self.key), (encrypted, b'x' * 32),
                            (encrypted[:-1], self.key), (encrypted + b'x', self.key),
                            (encrypted[:20], self.key)):
            with self.assertRaises(Exception):
                local.decrypt_stream(io.BytesIO(broken), io.BytesIO(), key)

    def test_chunk_reordering_and_missing_authenticated_eof_fail(self):
        _, encrypted, _ = self.encoded()
        start = 12 + struct.unpack('>I', encrypted[8:12])[0]
        size = 9 + 1024 + 16
        reordered = encrypted[:start] + encrypted[start + size:start + 2 * size] + encrypted[start:start + size] + encrypted[start + 2 * size:]
        with self.assertRaisesRegex(RuntimeError, 'sequence'):
            local.decrypt_stream(io.BytesIO(reordered), io.BytesIO(), self.key)
        with self.assertRaises(RuntimeError):
            local.decrypt_stream(io.BytesIO(encrypted[:start + size]), io.BytesIO(), self.key)

    def test_header_is_authenticated_and_nonces_are_unique_per_archive(self):
        _, first, _ = self.encoded()
        _, second, _ = self.encoded()
        self.assertNotEqual(first, second)
        # Change a valid header character without changing its length.
        offset = first.index(b'archiveID') + len(b'archiveID":"')
        changed = first[:offset] + (b'0' if first[offset:offset + 1] != b'0' else b'1') + first[offset + 1:]
        with self.assertRaises(Exception):
            local.decrypt_stream(io.BytesIO(changed), io.BytesIO(), self.key)


class RestoreReadinessTests(unittest.TestCase):
    def test_restore_failure_records_only_safe_stage_and_sql_codes(self):
        report = {'checks': {}}
        result = types.SimpleNamespace(returncode=1, stdout=b'', stderr=(
            b"ERROR 1045 (28000): private employee SQL/password/credential\n"
            b"ERROR 1045 (28000): duplicated private statement\nERROR 2013 (HY000): private SQL"))
        args = ['docker', 'exec', 'private-container', 'sh', '-ec', 'private password and SQL']
        with mock.patch.object(remote.subprocess, 'run', return_value=result), \
             mock.patch.object(remote, 'event') as event:
            with self.assertRaisesRegex(RuntimeError, 'safe diagnostics recorded') as raised:
                remote.restore_run(report, 'logical-import', 'mysql', 'mysql', args)
        self.assertEqual(report['failure'], {
            'phase': 'logical-import', 'service': 'mysql', 'command': 'docker', 'tool': 'mysql',
            'exitCode': 1, 'sqlErrors': [{'number': 1045, 'sqlState': '28000'},
                                      {'number': 2013, 'sqlState': 'HY000'}]})
        self.assertNotIn('private', json.dumps(report) + str(raised.exception) + str(event.call_args))

    def test_restore_timeout_suppresses_command_and_statement(self):
        report = {'checks': {}}
        args = ['docker', 'private-credential-or-SQL']
        with mock.patch.object(remote.subprocess, 'run', side_effect=remote.subprocess.TimeoutExpired(args, 1)), \
             mock.patch.object(remote, 'event'):
            with self.assertRaisesRegex(RuntimeError, 'safe diagnostics recorded') as raised:
                remote.restore_run(report, 'logical-import', 'mongo', 'mongorestore', args, timeout=1)
        self.assertTrue(report['failure']['timedOut'])
        self.assertIsNone(report['failure']['exitCode'])
        self.assertNotIn('private', json.dumps(report) + str(raised.exception))

    def test_mysql_waits_for_final_pid_and_authenticated_tcp_select(self):
        report = {'checks': {}}
        results = [types.SimpleNamespace(returncode=75, stdout=b'', stderr=b''),
                   types.SimpleNamespace(returncode=1, stdout=b'', stderr=b'ERROR 1045 (28000): private auth'),
                   types.SimpleNamespace(returncode=0, stdout=b'1\n', stderr=b'')]
        with mock.patch.object(remote.subprocess, 'run', side_effect=results) as command, \
             mock.patch.object(remote.time, 'sleep') as pause:
            remote.wait_restore_ready(report, 'mysql', 'disposable-fixture', attempts=3)
        self.assertEqual(command.call_count, 3)
        self.assertEqual(pause.call_count, 2)
        self.assertTrue(report['checks']['mysqlAuthenticatedFinalServerReady'])
        shell = command.call_args.args[0][-1]
        self.assertIn('/proc/1/comm', shell)
        self.assertIn('= mysqld', shell)
        self.assertIn('--protocol=TCP --host=127.0.0.1', shell)
        self.assertIn('SELECT 1', shell)
        self.assertNotIn('mysqladmin', shell)
        self.assertNotIn('failure', report)

    def test_mysql_successful_process_without_query_result_is_not_ready(self):
        report = {'checks': {}}
        result = types.SimpleNamespace(returncode=0, stdout=b'', stderr=b'')
        with mock.patch.object(remote.subprocess, 'run', return_value=result), \
             mock.patch.object(remote, 'event'):
            with self.assertRaisesRegex(RuntimeError, 'authenticated-ready'):
                remote.wait_restore_ready(report, 'mysql', 'disposable-fixture', attempts=1)
        self.assertNotIn('mysqlAuthenticatedFinalServerReady', report['checks'])
        self.assertEqual(report['failure']['phase'], 'ready')

    def test_mongo_waits_for_final_pid_before_authenticated_ping(self):
        report = {'checks': {}}
        results = [types.SimpleNamespace(returncode=75, stdout=b'', stderr=b''),
                   types.SimpleNamespace(returncode=0, stdout=b'', stderr=b'')]
        with mock.patch.object(remote.subprocess, 'run', side_effect=results) as command, \
             mock.patch.object(remote.time, 'sleep'):
            remote.wait_restore_ready(report, 'mongo', 'disposable-fixture', attempts=2)
        self.assertTrue(report['checks']['mongoAuthenticatedFinalServerReady'])
        shell = command.call_args.args[0][-1]
        self.assertIn('= mongod', shell)
        self.assertIn('--host=127.0.0.1', shell)
        self.assertIn('--authenticationDatabase=admin', shell)
        self.assertIn('db.adminCommand({ping:1})', shell)

    def test_readiness_failure_preserves_only_last_safe_error_codes(self):
        report = {'checks': {}}
        result = types.SimpleNamespace(returncode=1, stdout=b'', stderr=b'ERROR 1045 (28000): private password')
        with mock.patch.object(remote.subprocess, 'run', return_value=result), \
             mock.patch.object(remote, 'event'):
            with self.assertRaisesRegex(RuntimeError, 'authenticated-ready'):
                remote.wait_restore_ready(report, 'mysql', 'disposable-fixture', attempts=1)
        self.assertEqual(report['failure']['sqlErrors'], [{'number': 1045, 'sqlState': '28000'}])
        self.assertNotIn('private', json.dumps(report))


class MySQLTableIntegrityTests(unittest.TestCase):
    def test_exact_successful_results_cover_every_captured_table(self):
        output = (b'gosuxin_prod.second\tcheck\tstatus\tOK\n'
                  b'gosuxin_prod.first\tcheck\tstatus\tOK\n')
        self.assertEqual(remote.validate_mysql_table_checks(output, ['first', 'second']), 2)
        self.assertEqual(remote.validate_mysql_table_checks(b'', []), 0)

    def test_missing_extra_duplicate_and_malformed_results_fail_closed(self):
        success = b'gosuxin_prod.first\tcheck\tstatus\tOK\n'
        for output in (b'', success + success,
                       success + b'gosuxin_prod.extra\tcheck\tstatus\tOK\n',
                       b'other_db.first\tcheck\tstatus\tOK\n',
                       b'gosuxin_prod.first\tcheck\tstatus\tOK\textra\n',
                       b'Table\tOp\tMsg_type\tMsg_text\n' + success,
                       b'gosuxin_prod.first\tcheck\tstatus\tOK \n',
                       b'gosuxin_prod.first\tcheck\tstatus\tOK\n\n', b'\xff'):
            with self.subTest(output=output), self.assertRaises(RuntimeError):
                remote.validate_mysql_table_checks(output, ['first'])
        with self.assertRaises(RuntimeError):
            remote.validate_mysql_table_checks(success, ['first', 'second'])

    def test_note_error_warning_info_and_non_ok_status_are_never_success(self):
        success = b'gosuxin_prod.first\tcheck\tstatus\tOK\n'
        for kind, message in (('note', 'The storage engine does not support check'),
                              ('error', 'private employee or SQL detail'),
                              ('warning', 'synthetic warning'), ('info', 'synthetic info'),
                              ('status', 'Table is already up to date'), ('status', 'CORRUPT')):
            row = ('gosuxin_prod.first\tcheck\t' + kind + '\t' + message + '\n').encode()
            for output in (row, row + success, success + row):
                with self.subTest(kind=kind), self.assertRaises(RuntimeError) as raised:
                    remote.validate_mysql_table_checks(output, ['first'])
                self.assertNotIn(message, str(raised.exception))
        with self.assertRaises(RuntimeError):
            remote.validate_mysql_table_checks(b'gosuxin_prod.first\tanalyze\tstatus\tOK\n', ['first'])

    def test_existing_mysql_client_checks_exact_base_table_inventory_and_all_tables(self):
        report = {'checks': {}}
        output = (b'gosuxin_prod.second\tcheck\tstatus\tOK\n'
                  b'gosuxin_prod.first\tcheck\tstatus\tOK\n')
        tables = {'second': 4, 'first': 0}
        with mock.patch.object(remote, 'restore_run', side_effect=[b'first\nsecond\n', output]) as command:
            remote.check_restored_mysql_tables(report, 'disposable-fixture', tables)
        self.assertEqual(command.call_count, 2)
        for call in command.call_args_list:
            self.assertEqual(call.args[1:4], ('validate', 'mysql', 'mysql'))
            args = call.args[4]
            self.assertEqual(args[:3], ['docker', 'exec', 'disposable-fixture'])
            self.assertIn('exec mysql --protocol=TCP --host=127.0.0.1 --port=3306', args[5])
            self.assertIn('--batch --raw --skip-column-names', args[5])
            self.assertNotIn('mysqlcheck', args[5])
        self.assertIn("TABLE_TYPE='BASE TABLE'", command.call_args_list[0].args[4][-1])
        self.assertEqual(command.call_args_list[1].args[4][-1],
                         'CHECK TABLE `gosuxin_prod`.`first`,`gosuxin_prod`.`second`')
        self.assertEqual(tables, {'second': 4, 'first': 0})
        self.assertEqual(report['checks'], {'mysqlBaseTableInventoryExact': True,
                                           'mysqlBaseTablesCheckOK': 2})

    def test_missing_extra_or_duplicate_base_table_blocks_check_statement(self):
        for output in (b'', b'first\nextra\n', b'first\nfirst\n', b'first\tunexpected\n', b'\xff'):
            report = {'checks': {}}
            with self.subTest(output=output), \
                 mock.patch.object(remote, 'restore_run', return_value=output) as command:
                with self.assertRaises(RuntimeError):
                    remote.check_restored_mysql_tables(report, 'disposable-fixture', {'first': 1})
                self.assertEqual(command.call_count, 1)
                self.assertNotIn('mysqlBaseTablesCheckOK', report['checks'])

    def test_zero_exit_with_non_ok_check_output_does_not_validate_restore(self):
        report = {'checks': {}}
        result = [types.SimpleNamespace(returncode=0, stdout=b'first\n', stderr=b''),
                  types.SimpleNamespace(returncode=0,
                      stdout=b'gosuxin_prod.first\tcheck\tnote\tprivate unsupported engine detail\n',
                      stderr=b'')]
        with mock.patch.object(remote.subprocess, 'run', side_effect=result):
            with self.assertRaises(RuntimeError) as raised:
                remote.check_restored_mysql_tables(report, 'disposable-fixture', {'first': 1})
        self.assertNotIn('private', str(raised.exception) + json.dumps(report))
        self.assertNotIn('mysqlBaseTablesCheckOK', report['checks'])

    def test_empty_captured_database_checks_empty_inventory_without_invalid_sql(self):
        report = {'checks': {}}
        with mock.patch.object(remote, 'restore_run', return_value=b'') as command:
            remote.check_restored_mysql_tables(report, 'disposable-fixture', {})
        self.assertEqual(command.call_count, 1)
        self.assertEqual(report['checks']['mysqlBaseTablesCheckOK'], 0)

    def test_unsafe_or_duplicate_table_names_never_reach_process(self):
        for tables in (['unsafe`name'], ['first', 'first'], [None]):
            with self.subTest(tables=tables), mock.patch.object(remote, 'restore_run') as command:
                with self.assertRaises(RuntimeError):
                    remote.check_restored_mysql_tables({'checks': {}}, 'disposable-fixture', tables)
                command.assert_not_called()


class ArchiveAndOwnershipTests(unittest.TestCase):
    def symlink_graph_archive(self, base, links, files=None):
        """Build exact tar metadata without following or creating source links."""
        files = files or {'runtime/gosuxin/profile': b'owned fixture profile'}
        names = [*files, *links]
        directories = sorted({parent.as_posix() for name in names
                              for parent in pathlib.PurePosixPath(name).parents
                              if parent.as_posix() not in ('.', 'runtime')})
        path, inventory = base / 'graph.tar.gz', []
        with tarfile.open(path, 'w:gz') as archive:
            for name, kind, value in ([(n, 'directory', None) for n in directories]
                                      + [(n, 'file', value) for n, value in files.items()]
                                      + [(n, 'symlink', value) for n, value in links.items()]):
                member = tarfile.TarInfo(name)
                member.type = {'directory': tarfile.DIRTYPE, 'file': tarfile.REGTYPE,
                               'symlink': tarfile.SYMTYPE}[kind]
                member.mode = 0o755 if kind == 'directory' else 0o644
                row = {'path': name, 'mode': member.mode, 'uid': member.uid,
                       'gid': member.gid, 'type': kind}
                if kind == 'file':
                    member.size = len(value)
                    row.update(bytes=member.size, sha256=remote.hashlib.sha256(value).hexdigest())
                elif kind == 'symlink':
                    member.linkname = value
                    row['target'] = value
                archive.addfile(member, io.BytesIO(value) if kind == 'file' else None)
                inventory.append(row)
            manifest = {'format': remote.FORMAT, 'files': inventory}
            raw = remote.canonical(manifest)
            member = tarfile.TarInfo('manifest.json')
            member.size = len(raw)
            archive.addfile(member, io.BytesIO(raw))
        return path, manifest

    def test_chained_relative_link_escape_rejected_before_creating_links(self):
        links = {'runtime/x/a': '..', 'runtime/x/b': 'a/../../outside'}
        # Both individually pass the old lexical test, but the combined graph escapes.
        for name, target in links.items():
            remote.symlink_target(name, target)
        with tempfile.TemporaryDirectory() as folder:
            base = pathlib.Path(folder)
            archive, _ = self.symlink_graph_archive(base, links)
            with self.assertRaisesRegex(RuntimeError, 'graph escapes'):
                local.verify_archive(archive)
            target = base / 'isolated'
            target.mkdir()
            with self.assertRaisesRegex(RuntimeError, 'graph escapes'):
                remote.unpack_verified(archive, target)
            self.assertFalse(any(path.is_symlink() for path in target.rglob('*')))
            self.assertFalse((base / 'outside').exists())

    def test_self_and_multi_link_loops_rejected_before_creating_links(self):
        for links in ({'runtime/x/a': 'a'}, {'runtime/x/a': 'b', 'runtime/x/b': 'a'}):
            with self.subTest(links=links), tempfile.TemporaryDirectory() as folder:
                base = pathlib.Path(folder)
                archive, _ = self.symlink_graph_archive(base, links)
                with self.assertRaisesRegex(RuntimeError, 'graph loop'):
                    local.verify_archive(archive)
                target = base / 'isolated'
                target.mkdir()
                with self.assertRaisesRegex(RuntimeError, 'graph loop'):
                    remote.unpack_verified(archive, target)
                self.assertFalse(any(path.is_symlink() for path in target.rglob('*')))

    def test_valid_cross_suite_chain_resolves_dotdot_after_link_expansion(self):
        # Deliberately archive the dependent link first, to check the complete graph.
        links = {'runtime/openim/view': 'alias/../profile',
                 'runtime/openim/alias': '../gosuxin/deep'}
        files = {'runtime/gosuxin/profile': b'owned fixture profile',
                 'runtime/gosuxin/deep/data': b'owned nested fixture'}
        expected = pathlib.PurePosixPath('runtime/gosuxin/profile')
        for implementation in (local, remote):
            self.assertEqual(implementation.validate_symlink_graph(links)['runtime/openim/view'], expected)
        with tempfile.TemporaryDirectory() as folder:
            base = pathlib.Path(folder)
            archive, manifest = self.symlink_graph_archive(base, links, files)
            self.assertEqual(local.verify_archive(archive), manifest)
            # The worker only runs on Linux. Windows native directory-link/..
            # and short-name handling differ; its pure POSIX resolver and local
            # archive verification above still run on every platform.
            if remote.os.name != 'posix':
                return
            target = base / 'isolated'
            target.mkdir()
            self.assertEqual(remote.unpack_verified(archive, target), manifest)
            self.assertEqual((target / 'runtime/openim/view').read_bytes(), files['runtime/gosuxin/profile'])
            self.assertTrue(remote.contained(target / 'runtime/openim/view', target))

    def test_long_symlink_graph_is_bounded_independently_of_member_order(self):
        links = {'runtime/x/l'+str(i): 'l'+str(i+1) for i in range(41)}
        for ordered in (links, dict(reversed(list(links.items())))):
            for implementation in (local, remote):
                with self.assertRaisesRegex(RuntimeError, 'resolution limit'):
                    implementation.validate_symlink_graph(ordered)

    def activation_fixture(self, base, *, cutoff='2026-11-02T16:08:11+00:00', activation_change=None, omit=False):
        owner = 'a' * 32
        runtime = base / 'source/runtime'
        (runtime / 'openim').mkdir(parents=True)
        (runtime / 'gosuxin').mkdir()
        activation = {'instanceID': owner, 'startAt': '2026-11-02T16:08:11+00:00',
                      'dataPolicy': 'fresh-business-data', **(activation_change or {})}
        (runtime / 'openim/instance.json').write_bytes(remote.canonical({
            'instanceID': owner, 'production': True, 'testOnly': False,
            'stackName': remote.PROJECT, 'root': '/srv/puty-suite/openim'}))
        (runtime / 'gosuxin/runtime-environment.json').write_bytes(remote.canonical({
            'GOSUXIN_SYNC_START_AT': cutoff, 'GOSUXIN_REHEARSAL': 'false'}))
        if not omit:
            (runtime / 'activation.json').write_bytes(remote.canonical(activation))
        result = base / 'activation.tar.gz'
        inventory = []
        with tarfile.open(result, 'w:gz') as archive:
            for child in sorted(runtime.iterdir()):
                remote.archive_path(archive, child, 'runtime/' + child.name, inventory, runtime)
            manifest = {'format': remote.FORMAT, 'files': inventory,
                        'instanceID': owner, 'activation': activation}
            raw = remote.canonical(manifest)
            info = tarfile.TarInfo('manifest.json')
            info.size, info.mode = len(raw), 0o600
            archive.addfile(info, io.BytesIO(raw))
        return result, manifest, runtime

    def test_original_activation_is_captured_and_verified_during_isolated_extraction(self):
        with tempfile.TemporaryDirectory() as folder:
            base = pathlib.Path(folder)
            archive, manifest, runtime = self.activation_fixture(base)
            self.assertEqual(remote.read_activation(runtime, 'a' * 32), manifest['activation'])
            self.assertEqual(local.verify_archive(archive), manifest)
            target = base / 'isolated'
            target.mkdir()
            self.assertEqual(remote.unpack_verified(archive, target), manifest)
            self.assertEqual((target / 'runtime/activation.json').read_bytes(), (runtime / 'activation.json').read_bytes())
            restored = json.loads((target / 'runtime/gosuxin/runtime-environment.json').read_text())
            self.assertEqual(restored['GOSUXIN_SYNC_START_AT'], manifest['activation']['startAt'])

    def test_activation_missing_wrong_owner_cutoff_policy_and_timezone_fail_closed(self):
        cases = [
            {'omit': True}, {'activation_change': {'instanceID': 'b' * 32}},
            {'cutoff': '2026-11-03T16:08:11+00:00'},
            {'activation_change': {'dataPolicy': 'reseed'}},
            {'activation_change': {'startAt': '2026-11-02T16:08:11'}, 'cutoff': '2026-11-02T16:08:11'},
        ]
        for case in cases:
            with self.subTest(case=case), tempfile.TemporaryDirectory() as folder:
                base = pathlib.Path(folder)
                archive, _, runtime = self.activation_fixture(base, **case)
                with self.assertRaises(RuntimeError):
                    remote.read_activation(runtime, 'a' * 32)
                with self.assertRaises(RuntimeError):
                    local.verify_archive(archive)
                target = base / 'isolated'
                target.mkdir()
                with self.assertRaises(RuntimeError):
                    remote.unpack_verified(archive, target)

    def fixture(self, base):
        source = base / 'source'
        source.mkdir()
        (source / 'nested').mkdir()
        (source / 'nested/data').write_bytes(b'private fixture bytes')
        (source / 'logs').mkdir()  # A business folder named logs must survive.
        (source / 'logs/object').write_bytes(b'attachment')
        archive_path = base / 'fixture.tar.gz'
        inventory = []
        with tarfile.open(archive_path, 'w:gz', dereference=False) as archive:
            remote.archive_path(archive, source, 'volumes/minio-data', inventory, source)
            manifest = {'format': remote.FORMAT, 'files': inventory}
            raw = remote.canonical(manifest)
            info = tarfile.TarInfo('manifest.json')
            info.size, info.mode = len(raw), 0o600
            archive.addfile(info, io.BytesIO(raw))
        return archive_path, manifest

    def test_manifest_hash_and_real_isolated_file_extraction(self):
        with tempfile.TemporaryDirectory() as folder:
            base = pathlib.Path(folder)
            archive, manifest = self.fixture(base)
            self.assertEqual(local.verify_archive(archive), manifest)
            target = base / 'restore'
            target.mkdir()
            self.assertEqual(remote.unpack_verified(archive, target), manifest)
            self.assertEqual((target / 'volumes/minio-data/nested/data').read_bytes(), b'private fixture bytes')
            self.assertEqual((target / 'volumes/minio-data/logs/object').read_bytes(), b'attachment')
            self.assertFalse((base / 'production').exists())

    def test_inventory_tamper_rejected_before_restore_containers(self):
        with tempfile.TemporaryDirectory() as folder:
            base = pathlib.Path(folder)
            archive, manifest = self.fixture(base)
            raw = archive.read_bytes()
            # Rebuild a structurally valid tar with a false manifest checksum.
            false = base / 'false.tar.gz'
            with tarfile.open(archive, 'r:gz') as original, tarfile.open(false, 'w:gz') as output:
                for member in original:
                    stream = original.extractfile(member) if member.isfile() else None
                    if member.name == 'manifest.json':
                        manifest['files'][2]['sha256'] = '0' * 64
                        value = remote.canonical(manifest)
                        member.size = len(value)
                        stream = io.BytesIO(value)
                    output.addfile(member, stream)
            with self.assertRaisesRegex(RuntimeError, 'inventory'):
                local.verify_archive(false)
            target = base / 'restore'
            target.mkdir()
            with self.assertRaisesRegex(RuntimeError, 'inventory'):
                remote.unpack_verified(false, target)

    def test_path_traversal_absolute_and_escaping_links_rejected(self):
        for name in ('../outside', '/production/file', 'runtime/../../outside', 'runtime\\file'):
            with self.assertRaises(RuntimeError):
                remote.safe_name(name)
        for target in ('/srv/puty-suite/openim', '../../outside', '../../../outside'):
            with self.assertRaises(RuntimeError):
                remote.symlink_target('runtime/link', target)

    def test_wrong_owner_marker_rejected(self):
        info = {'root': remote.IM.as_posix(), 'production': True, 'testOnly': False,
                'stackName': remote.PROJECT, 'instanceID': 'a' * 32}
        remote.validate_identity(info)
        for field, value in (('root', '/srv/other'), ('production', False), ('testOnly', True),
                             ('stackName', 'other'), ('instanceID', '../foreign')):
            with self.assertRaises(RuntimeError):
                remote.validate_identity({**info, field: value})

    def test_only_exact_mysql_transient_socket_link_is_excluded(self):
        with tempfile.TemporaryDirectory() as folder:
            base = pathlib.Path(folder)
            socket = base / 'mysql.sock'
            socket.symlink_to('/var/run/mysqld/mysqld.sock')
            remote.preflight_source(base, 'volumes/mysql-data', base)
            inventory = []
            with tarfile.open(base / 'result.tar', 'w') as archive:
                remote.archive_path(archive, socket, 'volumes/mysql-data/mysql.sock', inventory, base)
            self.assertEqual(inventory, [])
            # Same absolute target in any other storage or path is refused.
            for name in ('volumes/minio-data/mysql.sock', 'volumes/mysql-data/other.sock'):
                with self.assertRaises(RuntimeError):
                    remote.preflight_source(socket, name, base)
            socket.unlink()
            socket.symlink_to('/outside/secret')
            with self.assertRaises(RuntimeError):
                remote.preflight_source(base, 'volumes/mysql-data', base)

    def test_relative_internal_link_survives_and_escaping_link_fails_preflight(self):
        with tempfile.TemporaryDirectory() as folder:
            base = pathlib.Path(folder)
            source = base / 'source'
            source.mkdir()
            (source / 'data').write_bytes(b'owned data')
            link = source / 'link'
            link.symlink_to('data')
            remote.preflight_source(source, 'volumes/minio-data', source)
            inventory = []
            archive_path = base / 'links.tar.gz'
            with tarfile.open(archive_path, 'w:gz') as archive:
                remote.archive_path(archive, source, 'volumes/minio-data', inventory, source)
                manifest = {'format': remote.FORMAT, 'files': inventory}
                raw = remote.canonical(manifest)
                info = tarfile.TarInfo('manifest.json')
                info.size = len(raw)
                archive.addfile(info, io.BytesIO(raw))
            self.assertEqual(local.verify_archive(archive_path), manifest)
            target = base / 'isolated'
            target.mkdir()
            remote.unpack_verified(archive_path, target)
            self.assertEqual((target / 'volumes/minio-data/link').read_bytes(), b'owned data')
            link.unlink()
            link.symlink_to('../outside')
            with self.assertRaisesRegex(RuntimeError, 'backup scope'):
                remote.preflight_source(source, 'volumes/minio-data', source)

    def test_foreign_restore_resources_cannot_be_removed(self):
        response = types.SimpleNamespace(returncode=0, stdout=json.dumps([{'Labels': {'com.puty.backup.restore': 'foreign'}}]).encode())
        with mock.patch.object(remote.subprocess, 'run', return_value=response) as command:
            with self.assertRaisesRegex(RuntimeError, 'Foreign'):
                remote.restore_resource_owned('volume', 'restore-a-mysql', 'restore-a')
            self.assertEqual(command.call_count, 1)
            self.assertEqual(command.call_args.args[0][:3], ['docker', 'volume', 'inspect'])
        with self.assertRaisesRegex(RuntimeError, 'Unexpected'):
            remote.restore_resource_owned('volume', 'production-data', 'restore-a')

    def test_foreign_production_container_cannot_be_started(self):
        with mock.patch.object(remote, 'js', return_value=[{'Config': {'Labels': {'com.puty.suite': 'foreign'}}}]):
            with self.assertRaisesRegex(RuntimeError, 'foreign'):
                remote.owned_container('container-id', 'our-owner')

    def test_recovery_state_resumes_only_original_containers(self):
        state = {'storage': ['storage-id'], 'writers': ['writer-id'], 'owner': 'owner', 'hrm': False}
        states = iter([{'State': {'Running': False}}, {'State': {'Running': True}},
                       {'State': {'Running': False}}, {'State': {'Running': True}}, {'State': {'Running': True}}])
        with mock.patch.object(remote, 'owned_container', side_effect=lambda *_: next(states)), mock.patch.object(remote, 'run') as command:
            remote.resume(state)
        self.assertEqual(command.call_args_list, [mock.call(['docker', 'start', 'storage-id'], timeout=180),
                                                   mock.call(['docker', 'start', 'writer-id'], timeout=180)])

    def test_mid_capture_failure_always_resumes_services(self):
        with tempfile.TemporaryDirectory() as folder:
            stage = pathlib.Path(folder)
            fake = types.SimpleNamespace(stage_root=stage, owner='a' * 32,
                                         activation={'fixture': True},
                                         by_service={name: {'Id': name + '-id', 'Image': 'sha256:' + 'b' * 64,
                                                            'State': {'Running': True}}
                                                     for name in remote.STORAGE | {'writer'}},
                                         counts=lambda: {'mysql': {}, 'mongo': {}},
                                         exec=lambda *_a, **_kw: None, mongo_dbs=remote.EXPECTED_DBS, volumes={})
            status = types.SimpleNamespace(returncode=0, stdout=b'puty-hrm RUNNING pid 1')
            fake_fcntl = types.SimpleNamespace(flock=mock.Mock(), LOCK_EX=1, LOCK_NB=2)
            with mock.patch.object(remote, 'Production', return_value=fake), \
                 mock.patch.object(remote, 'read_activation', return_value=fake.activation), \
                 mock.patch.object(remote, 'preflight_production'), \
                 mock.patch.object(remote, 'fcntl', fake_fcntl), \
                 mock.patch.object(remote, 'watchdog', return_value=123), \
                 mock.patch.object(remote, 'signal', types.SimpleNamespace(signal=mock.Mock(), alarm=mock.Mock(), SIGTERM=15, SIGINT=2, SIGALRM=14)), \
                 mock.patch.object(remote.subprocess, 'run', return_value=status), \
                 mock.patch.object(remote, 'run'), \
                 mock.patch.object(remote, 'owned_container', return_value={'State': {'Running': False}}), \
                 mock.patch.object(remote, 'archive_path', side_effect=RuntimeError('simulated stream failure')), \
                 mock.patch.object(remote, 'resume') as recover, \
                 mock.patch.object(remote.os, 'waitpid', create=True), \
                 mock.patch.object(remote.sys, 'stdout', types.SimpleNamespace(buffer=io.BytesIO())):
                with self.assertRaisesRegex(RuntimeError, 'simulated'):
                    remote.create(120)
                self.assertEqual(recover.call_count, 1)
                original = recover.call_args.args[0]
                self.assertTrue(original['hrm'])
                self.assertEqual(set(original['storage']), {x + '-id' for x in remote.STORAGE})
                self.assertEqual(original['writers'], ['writer-id'])
            self.assertEqual([p.name for p in stage.iterdir()], ['backup.lock'])


if __name__ == '__main__':
    unittest.main()
