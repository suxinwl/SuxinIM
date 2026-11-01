"""Verify current bytes and resolved local package coverage for one frozen build.

This checks named manifests and package roots. Exhaustive default-source/resource
enumeration is a separate review; root membership alone is not proof of it.
"""
import argparse
import datetime
import hashlib
import json
import os
from pathlib import Path
import urllib.parse

import yaml

ROOT = Path(__file__).absolute().parents[2]
CLIENT = ROOT / 'clients/flutter'


def logical_path(path):
    # GetFinalPathNameW may add this prefix only for long paths. Normalize it
    # for containment/equality, then restore it only when opening file bytes.
    value = str(path.resolve())
    prefix = chr(92) * 2 + '?' + chr(92)
    if value.lower().startswith((prefix + 'UNC' + chr(92)).lower()):
        value = chr(92) * 2 + value[len(prefix + 'UNC' + chr(92)):]
    elif value.startswith(prefix):
        value = value[len(prefix):]
    return Path(value)


def sha(path):
    path = logical_path(path)
    if os.name == 'nt' and not str(path).startswith(chr(92) * 2 + '?'):
        path = Path(chr(92) * 2 + '?' + chr(92) + str(path))
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--build', type=int, required=True)
    parser.add_argument('--check-only', action='store_true', help='Verify without writing or replacing an evidence report')
    args = parser.parse_args()
    if not 108 <= args.build <= 9999:
        parser.error('Supported frozen build range is 108..9999')
    output = ROOT / f'tests/flutter/path-dependency-coverage-{args.build}.json'
    if output.exists() and not args.check_only:
        parser.error('Report exists; do not overwrite historical evidence')
    manifest_paths = [ROOT / f'tests/flutter/{prefix}-{args.build}.json'
                      for prefix in ('runtime-source-snapshot', 'sdk-source-snapshot')]
    snapshot, sdk = [json.loads(path.read_text('utf-8')) for path in manifest_paths]
    if snapshot['applicationBuild'] != args.build:
        raise ValueError('Application manifest names a different build')
    for manifest in (snapshot, sdk):
        if manifest['fileCount'] != len(manifest['files']):
            raise ValueError('Manifest file count differs')
        for relative, expected in manifest['files'].items():
            path = logical_path(ROOT / relative)
            path.relative_to(logical_path(ROOT))
            if sha(path) != expected:
                raise ValueError('Current bytes differ: ' + relative)
    config = CLIENT / '.dart_tool/package_config.json'
    resolved = {}
    external_count = 0
    for item in json.loads(config.read_text('utf-8'))['packages']:
        uri = item['rootUri']
        path = (Path(urllib.parse.unquote(urllib.parse.urlparse(uri).path.lstrip('/')))
                if uri.startswith('file:') else config.parent / urllib.parse.unquote(uri))
        path = logical_path(path)
        try:
            relative = path.relative_to(logical_path(ROOT)).as_posix()
        except ValueError:
            external_count += 1
            continue
        resolved[item['name']] = {'root': path, 'relative': relative}
    records, edges = [], []
    for name, item in sorted(resolved.items()):
        relative = item['relative']
        if relative.startswith('tools/flutter-3.32.8/'):
            records.append({'package': name, 'root': relative, 'coverage': 'separately pinned Flutter runtime'})
            continue
        if relative == 'sdks/flutter':
            records.append({'package': name, 'root': relative, 'coverage': manifest_paths[1].relative_to(ROOT).as_posix(),
                            'fileCount': sdk['fileCount'], 'sourceSHA256': sdk['sourceSHA256']})
            continue
        if relative != 'clients/flutter' and not relative.startswith('clients/flutter/'):
            raise ValueError('Unclassified project-local package: ' + name)
        files = [p for p in snapshot['files'] if p.startswith(relative + '/')]
        if relative + '/pubspec.yaml' not in files or not any(
                p.startswith(relative + '/' + part + '/') for p in files for part in ('lib', 'android', 'ios', 'src')):
            raise ValueError('Missing package config/runtime root coverage: ' + name)
        records.append({'package': name, 'root': relative,
                        'coverage': manifest_paths[0].relative_to(ROOT).as_posix(), 'coveredFiles': len(files),
                        'pubspecSHA256': sha(item['root'] / 'pubspec.yaml')})
        pubspec = yaml.safe_load((item['root'] / 'pubspec.yaml').read_text('utf-8'))
        for section in ('dependencies', 'dev_dependencies', 'dependency_overrides'):
            for dependency, spec in (pubspec.get(section) or {}).items():
                if not isinstance(spec, dict) or 'path' not in spec:
                    continue
                declared = logical_path(item['root'] / spec['path'])
                target = resolved.get(dependency)
                if target is None:
                    raise ValueError('Unresolved local path dependency: ' + name + '/' + dependency)
                edges.append({'owner': name, 'section': section, 'dependency': dependency,
                              'declaredPath': str(spec['path']), 'resolvedPackageMatches': declared == target['root'],
                              'effectiveOverride': declared != target['root']})
    result = {'createdAt': datetime.datetime.now(datetime.timezone.utc).isoformat(), 'applicationBuild': args.build,
              'scope': __doc__, 'packageConfigSHA256': sha(config), 'manifests': [
                  {'path': path.relative_to(ROOT).as_posix(), 'sha256': sha(path)} for path in manifest_paths],
              'snapshotToolSHA256': sha(ROOT / 'tests/flutter/snapshot_runtime_sources.py'),
              'toolSHA256': sha(Path(__file__)), 'packages': records, 'declaredPathEdges': edges,
              'externalResolvedPackagesNotTraversed': external_count, 'allNamedManifestBytesMatch': True,
              'allLocalRootsCovered': True,
              'limits': ['Does not independently enumerate every possible runtime input or external hosted package.',
                         'Shadowed path declarations are reported separately from effective resolved package roots.']}
    if not args.check_only:
        with output.open('x', encoding='utf-8') as stream:
            stream.write(json.dumps(result, indent=2) + '\n')
    print(json.dumps({'report': None if args.check_only else output.relative_to(ROOT).as_posix(),
                      'projectResolvedPackages': len(records), 'pathEdges': len(edges), 'allCovered': True}))


if __name__ == '__main__':
    main()
