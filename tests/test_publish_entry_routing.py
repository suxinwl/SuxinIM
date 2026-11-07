"""Fault tests of the real coordinated publisher; no deploy commands are run."""
import importlib.util
import pathlib
import sys
import types
import unittest
from unittest.mock import patch


ROOT = pathlib.Path(__file__).resolve().parents[1]
SOURCE = ROOT / 'deploy/production/publish_entry_routing.py'
SPEC = importlib.util.spec_from_file_location('entry_publisher_under_test', SOURCE)
PUBLISHER = importlib.util.module_from_spec(SPEC)
# The transition is platform independent. Windows lacks these Linux-only imports;
# no mocked Linux function is invoked and Operations/main are never instantiated.
platform_stubs = {name: types.ModuleType(name) for name in ('fcntl', 'pwd')
                  if importlib.util.find_spec(name) is None}
with patch.dict(sys.modules, platform_stubs):
    SPEC.loader.exec_module(PUBLISHER)


class InjectedFailure(RuntimeError):
    pass


class MockOperations:
    """Track disk, mounted processes, and partial side effects independently."""
    names = tuple(sorted(PUBLISHER.NAMES))

    def __init__(self, faults=()):
        self.faults = set(faults)
        self.calls = {}
        self.trace = []
        self.disk = {name: 'old' for name in self.names}
        self.running = self.disk.copy()
        self.host_disk = self.host_running = 'old'
        self.nginx_disk = self.nginx_running = 'old'
        self.manifest = 'old'
        self.verified = False
        self.safety_observations = 0

    def begin(self, operation):
        occurrence = self.calls.get(operation, 0) + 1
        self.calls[operation] = occurrence
        self.trace.append((operation, occurrence, 'begin'))
        self.observe(operation, occurrence, 'before')
        return occurrence

    def observe(self, operation, occurrence, point):
        self.trace.append((operation, occurrence, point))
        self.safety_observations += 1
        # A mount/runtime is independent of atomic replacement of its disk file.
        # All live producers must agree with the running strict/legacy host.
        for name, version in self.running.items():
            if version is not None and version != self.host_running:
                raise AssertionError(f'Incompatible running producer at {operation}/{point}: {name}')
        if (operation, occurrence, point) in self.faults:
            self.faults.remove((operation, occurrence, point))
            raise InjectedFailure(f'{operation}/{occurrence}/{point}')

    def nginx_new(self):
        n = self.begin('nginx_new')
        self.nginx_disk = 'new'
        self.observe('nginx_new', n, 'config')
        self.nginx_running = 'new'
        self.observe('nginx_new', n, 'reload')

    def nginx_old(self):
        n = self.begin('nginx_old')
        self.nginx_disk = self.nginx_running = 'old'
        self.observe('nginx_old', n, 'restored')

    def stop(self):
        n = self.begin('stop')
        for index, name in enumerate(self.names, 1):
            self.running[name] = None
            self.observe('stop', n, f'stopped{index}')

    def host_new(self):
        n = self.begin('host_new')
        assert all(value is None for value in self.running.values())
        self.host_disk = 'new'
        self.observe('host_new', n, 'file')
        self.host_running = 'new'
        self.observe('host_new', n, 'ready')

    def host_old(self):
        n = self.begin('host_old')
        assert all(value is None for value in self.running.values())
        self.host_disk = 'old'
        self.observe('host_old', n, 'file')
        self.host_running = 'old'
        self.observe('host_old', n, 'ready')

    def backends_new(self):
        n = self.begin('backends_new')
        assert all(value is None for value in self.running.values())
        for index, name in enumerate(self.names, 1):
            self.disk[name] = 'new'
            self.observe('backends_new', n, f'copied{index}')

    def backends_old(self):
        n = self.begin('backends_old')
        assert all(value is None for value in self.running.values())
        for index, name in enumerate(self.names, 1):
            self.disk[name] = 'old'
            self.observe('backends_old', n, f'copied{index}')
        self.manifest = 'old'
        self.observe('backends_old', n, 'manifest')

    def start_and_check(self):
        n = self.begin('start_and_check')
        assert len(set(self.disk.values())) == 1
        assert next(iter(self.disk.values())) == self.host_running
        for index, name in enumerate(self.names, 1):
            self.running[name] = self.disk[name]
            self.observe('start_and_check', n, f'started{index}')
        self.observe('start_and_check', n, 'readiness')

    def commit(self):
        n = self.begin('commit')
        self.manifest = 'new'
        self.observe('commit', n, 'manifest')
        self.observe('commit', n, 'verify')

    def verify_old(self):
        n = self.begin('verify_old')
        assert set(self.disk.values()) == set(self.running.values()) == {'old'}
        assert self.host_disk == self.host_running == self.manifest == 'old'
        assert self.nginx_disk == self.nginx_running == 'old'
        self.verified = True
        self.observe('verify_old', n, 'verified')


class TransitionTests(unittest.TestCase):
    def restored(self, fault):
        ops = MockOperations([fault])
        with self.assertRaisesRegex(RuntimeError, 'original release restored') as result:
            PUBLISHER.transition(ops)
        self.assertIsInstance(result.exception.__cause__, InjectedFailure)
        self.assertTrue(ops.verified)
        self.assertFalse(ops.faults)
        self.assertGreater(ops.safety_observations, 4)
        self.assertEqual(ops.trace[-1], ('verify_old', 1, 'verified'))
        return ops

    def test_success_new_host_and_all_six_new_processes_commit(self):
        ops = MockOperations()
        PUBLISHER.transition(ops)
        self.assertEqual(set(ops.disk.values()), {'new'})
        self.assertEqual(set(ops.running.values()), {'new'})
        self.assertEqual((ops.host_disk, ops.host_running, ops.manifest), ('new',) * 3)
        self.assertEqual((ops.nginx_disk, ops.nginx_running), ('new', 'new'))
        self.assertNotIn('host_old', ops.calls)
        self.assertEqual(list(ops.calls), ['nginx_new', 'stop', 'host_new', 'backends_new',
                                          'start_and_check', 'commit'])

    def test_nginx_failure_restores_configuration_without_stopping_backends(self):
        for point in ('config', 'reload'):
            with self.subTest(point=point):
                ops = self.restored(('nginx_new', 1, point))
                self.assertNotIn('stop', ops.calls)
                self.assertNotIn('host_new', ops.calls)

    def test_partial_stop_failure_stops_every_writer_before_rollback_replacement(self):
        for count in (1, 3, 6):
            with self.subTest(count=count):
                ops = self.restored(('stop', 1, f'stopped{count}'))
                self.assertEqual(ops.calls['stop'], 2)
                self.assertNotIn('host_new', ops.calls)
                self.assertLess(ops.trace.index(('stop', 2, 'stopped6')),
                                ops.trace.index(('host_old', 1, 'begin')))

    def test_host_failure_after_file_or_restart_restores_old_host(self):
        for point in ('file', 'ready'):
            with self.subTest(point=point):
                self.restored(('host_new', 1, point))

    def test_backend_partial_copy_failure_restores_all_files_and_manifest(self):
        for count in (1, 3, 6):
            with self.subTest(count=count):
                self.restored(('backends_new', 1, f'copied{count}'))

    def test_partial_start_or_readiness_failure_stops_new_mounts_before_host_restore(self):
        for point in ('started1', 'started3', 'started6', 'readiness'):
            with self.subTest(point=point):
                ops = self.restored(('start_and_check', 1, point))
                self.assertEqual(ops.calls['start_and_check'], 2)
                self.assertLess(ops.trace.index(('stop', 2, 'stopped6')),
                                ops.trace.index(('host_old', 1, 'begin')))

    def test_commit_failure_after_manifest_replacement_restores_manifest_and_processes(self):
        for point in ('manifest', 'verify'):
            with self.subTest(point=point):
                self.restored(('commit', 1, point))

    def test_rollback_failures_never_claim_restoration_or_start_old_with_new_host(self):
        for failure in (('stop', 2, 'stopped3'), ('host_old', 1, 'file'),
                        ('host_old', 1, 'ready'), ('backends_old', 1, 'copied3'),
                        ('start_and_check', 2, 'started3'),
                        ('nginx_old', 1, 'restored'), ('verify_old', 1, 'before')):
            with self.subTest(failure=failure):
                ops = MockOperations([('commit', 1, 'manifest'), failure])
                with self.assertRaisesRegex(RuntimeError, 'verified rollback incomplete') as result:
                    PUBLISHER.transition(ops)
                self.assertIsInstance(result.exception.__cause__, InjectedFailure)
                self.assertFalse(ops.verified)
                self.assertFalse(ops.faults)
                # In particular stop-failure cannot proceed to replacing the host.
                if failure[0] == 'stop':
                    self.assertNotIn('host_old', ops.calls)
                    self.assertEqual(ops.host_running, 'new')
                    self.assertEqual({v for v in ops.running.values() if v}, {'new'})


if __name__ == '__main__':
    unittest.main(verbosity=2)
