"""Independently recheck a downloaded final run; never overwrites raw summaries.

Usage: python tests/summarize-final-load.py tests/load-final/<run-id>
Creates analysis.json and resource-summary.json only in that final run directory.
No remote operations, fixture reads, percentile averaging or device conclusions.
"""
from __future__ import annotations

import argparse
import datetime
import hashlib
import importlib.util
import json
import pathlib
import re
import statistics

ROOT = pathlib.Path(__file__).resolve().parents[1]
SPEC = importlib.util.spec_from_file_location('final_load_launcher', ROOT / 'tests/start-final-load.py')
LAUNCHER = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(LAUNCHER)


def sha(path):
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def memory_bytes(text):
    match = re.fullmatch(r'([\d.]+)([A-Za-z]+)', text.split(' / ')[0].strip())
    if not match:
        raise ValueError('Unknown Docker memory format')
    units = {'B': 1, 'kB': 1000, 'KB': 1000, 'MB': 1000**2, 'GB': 1000**3,
             'KiB': 1024, 'MiB': 1024**2, 'GiB': 1024**3}
    return float(match.group(1)) * units[match.group(2)]


def summarize_resources(path):
    rows, malformed, outside = [], [], 0
    for index, line in enumerate(path.read_text(encoding='utf-8').splitlines(), 1):
        if not line.strip():
            continue
        try:
            sample = json.loads(line)
            second = float(sample['secondsFromStart'])
            if second < 0 or second > 1800:
                outside += 1
                continue
            business = sample['businessContainers']
            generators = sample['loadGenerator']
            if any(item.get('Name') == 'openim38-upgrade-load' for item in business):
                raise ValueError('Generator incorrectly included in business totals')
            rows.append({
                'seconds': second,
                'businessContainerCount': len(business),
                'generatorCount': len(generators),
                'businessMemoryGiB': sum(memory_bytes(item['MemUsage']) for item in business) / 1024**3,
                'businessCPUCorePercent': sum(float(item['CPUPerc'].rstrip('%')) for item in business),
                'generatorMemoryGiB': sum(memory_bytes(item['MemUsage']) for item in generators) / 1024**3,
                'generatorCPUCorePercent': sum(float(item['CPUPerc'].rstrip('%')) for item in generators),
                'hostAvailableGiB': int(sample['hostMemoryKB']['MemAvailable'].split()[0]) / 1024**2,
            })
        except (ValueError, KeyError, TypeError):
            malformed.append(index)
    rows.sort(key=lambda row: row['seconds'])
    result = {'source': path.name, 'sourceSHA256': sha(path), 'samples': len(rows),
              'outsideCommonWindowSamples': outside, 'malformedSampleLines': malformed,
              'resourceEvidencePresent': bool(rows) and not malformed,
              'notes': ['Business and synthetic Linux generator are separate totals.',
                        'CPU100% means one logical CPU, not100% of all24 vCPUs.',
                        'These samples describe this run; they are not a capacity limit or an AV concurrency result.']}
    if not rows:
        return result
    seconds = [row['seconds'] for row in rows]
    result.update({
        'firstSecond': seconds[0], 'lastSecond': seconds[-1],
        'largestSampleGapSeconds': max((b - a for a, b in zip(seconds, seconds[1:])), default=0),
        'businessContainerCounts': sorted({row['businessContainerCount'] for row in rows}),
        'loadGeneratorCounts': sorted({row['generatorCount'] for row in rows}),
        'expectedContainerCountsEverySample': all(row['businessContainerCount'] == 24 and row['generatorCount'] == 1 for row in rows),
        'businessPeakMemoryGiB': max(row['businessMemoryGiB'] for row in rows),
        'businessPeakCPUCorePercent': max(row['businessCPUCorePercent'] for row in rows),
        'businessAverageCPUCorePercent': statistics.mean(row['businessCPUCorePercent'] for row in rows),
        'generatorPeakMemoryGiB': max(row['generatorMemoryGiB'] for row in rows),
        'generatorPeakCPUCorePercent': max(row['generatorCPUCorePercent'] for row in rows),
        'hostMinimumAvailableGiB': min(row['hostAvailableGiB'] for row in rows),
        'coverageReview': {'sampleBy30Seconds': seconds[0] <= 30, 'sampleAfter1770Seconds': seconds[-1] >= 1770,
                           'maximumGapAtMost45Seconds': max((b - a for a, b in zip(seconds, seconds[1:])), default=0) <= 45},
    })
    return result


def analyze(directory):
    paths = ['run.json', 'summary.json', 'barrier.json', 'linux50a/summary.json', 'linux50b/summary.json',
             'running-binaries-before.json', 'running-binaries-after.json', 'resources.jsonl']
    missing = [name for name in paths if not (directory / name).is_file()]
    if missing:
        return {'recordedAt': datetime.datetime.now(datetime.timezone.utc).isoformat(), 'passed': False,
                'status': 'incomplete_evidence', 'missingFiles': missing,
                'note': 'Missing files are not replaced by inferred pass/fail metrics.'}, None
    load = lambda name: json.loads((directory / name).read_text(encoding='utf-8'))
    run, source, barrier = load('run.json'), load('summary.json'), load('barrier.json')
    parts = [load('linux50a/summary.json'), load('linux50b/summary.json')]
    before, after = load('running-binaries-before.json'), load('running-binaries-after.json')
    lock = json.loads((ROOT / 'tests/load-final/launcher-lock.json').read_text(encoding='utf-8'))
    check = LAUNCHER.summarize(parts)
    resource = summarize_resources(directory / 'resources.jsonl')
    start = datetime.datetime.fromtimestamp(barrier['startAt'] / 1000, datetime.timezone.utc)
    expected_slots = [{'offset': 0, 'count': 50}, {'offset': 50, 'count': 50}]
    checks = {**check['checks'],
              'runAndBarrierIdentity': run.get('runID') == barrier.get('runID') == check['run'],
              'barrierCreatedAfter100Connected': barrier.get('connectedBeforeBarrier') == run.get('connectedBeforeBarrier') == 100,
              'fixed30MinutePlan': barrier.get('requiredSeconds') == run.get('durationSeconds') == 1800,
              'fixed50Plus50Partitions': run.get('partitions') == expected_slots,
              'noEarlySDKStart': all(datetime.datetime.fromisoformat(part['startedAt']) >= start for part in parts),
              'sameOriginalPartitionReports': source.get('partitions') == parts,
              'businessContainersUnchanged': before.get('containerIDs') == after.get('containerIDs'),
              'exact24BusinessContainers': len(before.get('containerIDs', {})) == len(after.get('containerIDs', {})) == 24,
              'businessBinarySetsUnchanged': before.get('binaries') == after.get('binaries') and len(before.get('binaries', {})) == 18,
              'allMountedAndRunningBinaryHashesAgree': all(value['mountedSHA256'] == value['processSHA256'] for value in after['binaries'].values()),
              'matchesLockedFinalBusinessHashes': {name: value['processSHA256'] for name, value in after['binaries'].items()} == lock['businessBinarySHA256'],
              'matchesLockedLoaderHashes': run.get('loaderSHA256') == lock['loadFilesSHA256'],
              'launcherSourceSHAAgrees': run.get('launcherSHA256') == sha(ROOT / 'tests/start-final-load.py'),
              'sourceSummaryPassed': source.get('passed') is True,
              'coordinatorComplete': run.get('phase') == 'complete' and run.get('passed') is True,
              'ownedGeneratorStopped': run.get('ownedLoadContainerStopped') is True,
              'resourceSamplerNoErrors': not run.get('resourceSamplerErrors'),
              'resourceEvidencePresent': resource['resourceEvidencePresent'],
              'resourceContainerCountsComplete': resource.get('expectedContainerCountsEverySample') is True,
              'normalTLSDeclared': run.get('TLSBypass') is False,
              'oldEvidencePreserved': run.get('oldEvidenceOverwritten') is False,
    }
    result = {'recordedAt': datetime.datetime.now(datetime.timezone.utc).isoformat(), 'status': 'complete_evidence',
              'run': check['run'], 'passed': all(checks.values()), 'checks': checks,
              'sdkWindowChecksRecomputed': check['checks'], 'commonWindowSeconds': check['commonWindowSeconds'],
              'sent': check['sent'], 'received': check['received'], 'failed': check['failed'],
              'disconnections': check['disconnections'],
              'partitionLatencyMs': [{'offset': part['offset'], 'count': part['count'], 'latencyMs': part['latencyMs']} for part in parts],
              'latencyNote': 'Partition percentiles are not averaged into a pooled percentile.',
              'sourceEvidenceSHA256': {name: sha(directory / name) for name in paths},
              'launcherLockSHA256': sha(ROOT / 'tests/load-final/launcher-lock.json'),
              'analyzerSHA256': sha(pathlib.Path(__file__).resolve()),
              'limits': ['Read-only local recomputation of downloaded reports; no device/production operation or additional network test.',
                         'Resource coverage flags are descriptive; inspect gaps before claiming continuous resource coverage.',
                         'Not a maximum throughput, physical device, HRM or audio/video capacity conclusion.']}
    return result, resource


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('run_directory')
    args = parser.parse_args()
    directory = pathlib.Path(args.run_directory).resolve()
    if not directory.is_relative_to(ROOT / 'tests/load-final') or not directory.is_dir():
        raise RuntimeError('Use an existing project tests/load-final/<run-id> directory')
    result, resource = analyze(directory)
    (directory / 'analysis.json').write_text(json.dumps(result, indent=2) + '\n', encoding='utf-8')
    if resource is not None:
        (directory / 'resource-summary.json').write_text(json.dumps(resource, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'runDirectory': directory.relative_to(ROOT).as_posix(), 'status': result['status'],
                      'passed': result['passed'], 'failedChecks': [name for name, value in result.get('checks', {}).items() if not value]}))
    return 0 if result['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
