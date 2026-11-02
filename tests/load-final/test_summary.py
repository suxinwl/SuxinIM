import copy
import importlib.util
import json
import pathlib
import tempfile
import unittest
from unittest.mock import patch

ROOT = pathlib.Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location('final_summary', ROOT / 'tests/summarize-final-load.py')
SUMMARY = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(SUMMARY)


def sample(second):
    return {'secondsFromStart': second, 'hostMemoryKB': {'MemAvailable': '20971520 kB'},
            'businessContainers': [{'Name': 'business-' + str(i), 'MemUsage': '10MiB / 32GiB', 'CPUPerc': '0.5%'} for i in range(24)],
            'loadGenerator': [{'Name': 'openim38-upgrade-load', 'MemUsage': '2GiB / 18GiB', 'CPUPerc': '125.0%'}]}


class FinalSummaryTests(unittest.TestCase):
    def test_memory_units_and_separate_generator(self):
        self.assertEqual(SUMMARY.memory_bytes('1.5GiB / 18GiB'), 1.5 * 1024**3)
        with tempfile.TemporaryDirectory() as directory:
            path = pathlib.Path(directory) / 'resources.jsonl'
            path.write_text('\n'.join(json.dumps(sample(t)) for t in (-10, 0, 1800, 1810)))
            result = SUMMARY.summarize_resources(path)
            self.assertEqual(result['samples'], 2)
            self.assertEqual(result['outsideCommonWindowSamples'], 2)
            self.assertEqual(result['businessPeakMemoryGiB'], 240 / 1024)
            self.assertEqual(result['generatorPeakMemoryGiB'], 2)
            self.assertEqual(result['businessPeakCPUCorePercent'], 12)
            self.assertEqual(result['hostMinimumAvailableGiB'], 20)
            self.assertTrue(result['expectedContainerCountsEverySample'])

    def test_malformed_sample_cannot_be_called_complete_resources(self):
        with tempfile.TemporaryDirectory() as directory:
            path = pathlib.Path(directory) / 'resources.jsonl'
            path.write_text(json.dumps(sample(0)) + '\nnot-json\n')
            result = SUMMARY.summarize_resources(path)
            self.assertFalse(result['resourceEvidencePresent'])
            self.assertEqual(result['malformedSampleLines'], [2])

    def test_missing_reports_are_not_inferred_pass(self):
        with tempfile.TemporaryDirectory() as directory:
            result, resource = SUMMARY.analyze(pathlib.Path(directory))
            self.assertFalse(result['passed'])
            self.assertEqual(result['status'], 'incomplete_evidence')
            self.assertIsNone(resource)

    def test_synthetic_complete_reports_recompute_and_a_disconnect_fails(self):
        with tempfile.TemporaryDirectory() as directory:
            # The real launcher lock belongs to one inspected test host. Unit
            # tests must not require its ignored reports or deployed binaries.
            suite = pathlib.Path(directory) / 'synthetic-suite'
            (suite / 'tests/load-final').mkdir(parents=True)
            (suite / 'tests/start-final-load.py').write_text('# synthetic launcher identity only\n')
            lock = {
                'loadFilesSHA256': {'load-100.mjs': 'a' * 64, 'load-static-server.mjs': 'b' * 64,
                                    'static/load-client.js': 'c' * 64},
                'businessBinarySHA256': {'synthetic-service-' + str(index): format(index, '064x')
                                         for index in range(18)},
            }
            (suite / 'tests/load-final/launcher-lock.json').write_text(json.dumps(lock))
            fixture_root = patch.object(SUMMARY, 'ROOT', suite)
            fixture_root.start()
            self.addCleanup(fixture_root.stop)
            path = pathlib.Path(directory) / 'synthetic-run'
            path.mkdir()
            write = lambda name, value: (path / name).write_text(json.dumps(value))
            parts = [{'sharedRun': 'synthetic-unit-only', 'phase': 'complete', 'offset': offset, 'count': 50,
                      'clients': 50, 'connected': 50, 'elapsedSeconds': 1806, 'sent': 3000, 'received': 3000,
                      'failed': 0, 'disconnections': 0, 'pageErrors': [], 'latencyMs': {'p95': offset + 100},
                      'startedAt': '2026-11-01T00:00:00+00:00', 'finishedAt': '2026-11-01T00:30:06+00:00'} for offset in (0, 50)]
            for group, value in zip(('linux50a', 'linux50b'), parts):
                (path / group).mkdir(); write(group + '/summary.json', value)
            write('run.json', {'runID': 'synthetic-unit-only', 'phase': 'complete', 'passed': True, 'durationSeconds': 1800,
                              'connectedBeforeBarrier': 100, 'partitions': [{'offset': 0, 'count': 50}, {'offset': 50, 'count': 50}],
                              'ownedLoadContainerStopped': True, 'resourceSamplerErrors': [], 'TLSBypass': False,
                              'oldEvidenceOverwritten': False, 'loaderSHA256': lock['loadFilesSHA256'],
                              'launcherSHA256': SUMMARY.sha(suite / 'tests/start-final-load.py')})
            write('barrier.json', {'runID': 'synthetic-unit-only', 'startAt': 1793491200000,
                                  'connectedBeforeBarrier': 100, 'requiredSeconds': 1800})
            binary = {name: {'mountedSHA256': value, 'processSHA256': value} for name, value in lock['businessBinarySHA256'].items()}
            ids = {'synthetic-' + str(index): 'unit-only-' + str(index) for index in range(24)}
            write('running-binaries-before.json', {'containerIDs': ids, 'binaries': binary})
            write('running-binaries-after.json', {'containerIDs': ids, 'binaries': binary})
            (path / 'resources.jsonl').write_text(json.dumps(sample(0)) + '\n' + json.dumps(sample(1800)))
            write('summary.json', {'passed': True, 'partitions': parts})
            result, _ = SUMMARY.analyze(path)
            self.assertTrue(result['passed'], result['checks'])
            changed = copy.deepcopy(parts); changed[0]['disconnections'] = 1
            write('linux50a/summary.json', changed[0])
            result, _ = SUMMARY.analyze(path)
            self.assertFalse(result['passed'])
            self.assertFalse(result['checks']['noDisconnections'])
            write('linux50a/summary.json', parts[0])
            lock['loadFilesSHA256']['load-100.mjs'] = 'd' * 64
            (suite / 'tests/load-final/launcher-lock.json').write_text(json.dumps(lock))
            result, _ = SUMMARY.analyze(path)
            self.assertFalse(result['passed'])
            self.assertFalse(result['checks']['matchesLockedLoaderHashes'])


if __name__ == '__main__':
    unittest.main()
