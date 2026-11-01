"""Offline safety/acceptance fixtures; never talks to Docker or real sessions."""
import copy
import importlib.util
import pathlib
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location('final_load', ROOT / 'tests/start-final-load.py')
LOAD = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(LOAD)


def owned():
    return {'Name': '/' + LOAD.NAME, 'Id': 'fixed-owned-id', 'State': {'Running': False, 'Status': 'exited'},
            'Config': {'Image': 'fixed-image', 'Entrypoint': ['sleep'], 'Cmd': ['infinity'], 'WorkingDir': '/load',
                       'Labels': {'com.openim38.role': 'load-test'}, 'Env': []},
            'HostConfig': {'NetworkMode': 'host', 'Privileged': False, 'PidMode': '', 'CapAdd': None,
                           'Memory': 18 * 1024**3, 'NanoCpus': 8_000_000_000, 'PidsLimit': 1536, 'ShmSize': 1024**3},
            'Mounts': [{'Type': 'bind', 'Source': source, 'Destination': destination, 'RW': writable}
                       for destination, (source, writable) in LOAD.EXPECTED_MOUNTS.items()]}


def parts():
    return [{'sharedRun': 'final-fixture', 'phase': 'complete', 'offset': offset, 'count': 50,
             'clients': 50, 'connected': 50, 'elapsedSeconds': 1806, 'sent': 3000, 'received': 3000,
             'failed': 0, 'disconnections': 0, 'pageErrors': [],
             'startedAt': '2026-11-01T00:00:00+00:00', 'finishedAt': '2026-11-01T00:30:06+00:00'} for offset in (0, 50)]


class FinalLoadTests(unittest.TestCase):
    def test_exact_stopped_owner_accepted(self):
        LOAD.validate_container(owned(), {'containerID': 'fixed-owned-id', 'image': 'fixed-image'})

    def test_foreign_id_running_or_writable_fixture_refused(self):
        for mutate in (lambda c: c.update(Id='foreign'),
                       lambda c: c['State'].update(Running=True),
                       lambda c: c['Mounts'][1].update(RW=True),
                       lambda c: c['HostConfig'].update(Privileged=True)):
            container = owned(); mutate(container)
            with self.assertRaises(RuntimeError):
                LOAD.validate_container(container, {'containerID': 'fixed-owned-id', 'image': 'fixed-image'})

    def test_exact_minimal_001_to100_only(self):
        users = [{'userID': f'upgrade38_u{index:03}', 'imToken': 'x' * 30, 'platformID': 5} for index in range(1, 101)]
        self.assertEqual(LOAD.validate_sessions({'users': users}), 100)
        wrong = copy.deepcopy(users); wrong[-1]['userID'] = 'upgrade38_u104'
        with self.assertRaises(RuntimeError):
            LOAD.validate_sessions(wrong)

    def test_ready_requires50_real_connected_and_zero_errors(self):
        value = {**parts()[0], 'phase': 'preparing'}
        self.assertTrue(LOAD.valid_ready(value, 0, 'final-fixture'))
        for change in ({'connected': 49}, {'disconnections': 1}, {'failed': 1}, {'pageErrors': ['synthetic']}, {'phase': 'running'}):
            self.assertFalse(LOAD.valid_ready({**value, **change}, 0, 'final-fixture'))

    def test_full_window_pass_fixture(self):
        self.assertTrue(LOAD.summarize(parts())['passed'])

    def test_any_disconnect_or_incomplete_group_fails(self):
        for change in ({'disconnections': 1}, {'connected': 49}, {'clients': 49}, {'count': 49},
                       {'failed': 1}, {'pageErrors': ['synthetic']}, {'phase': 'running'},
                       {'elapsedSeconds': 1799}, {'received': 2999}):
            value = parts(); value[0].update(change)
            self.assertFalse(LOAD.summarize(value)['passed'], change)

    def test_short_common_overlap_fails_despite_individual_duration(self):
        value = parts(); value[1]['startedAt'] = '2026-11-01T00:00:10+00:00'
        self.assertFalse(LOAD.summarize(value)['passed'])


if __name__ == '__main__':
    unittest.main()
