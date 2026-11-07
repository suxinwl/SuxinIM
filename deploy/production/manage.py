#!/usr/bin/env python3
"""Operate only the explicitly owned PUTY production stack; never prune data."""
import argparse
import json
import pathlib
import subprocess

BASE = pathlib.Path('/srv/puty-suite/openim')


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('action', choices=['status', 'start', 'stop', 'restart'])
    parser.add_argument('services', nargs='*')
    args = parser.parse_args()
    if BASE.resolve() != BASE:
        raise RuntimeError('Unexpected production root')
    info = json.loads((BASE / 'instance.json').read_text())
    if info.get('root') != str(BASE) or not info.get('production') or info.get('testOnly') is not False or info.get('stackName') != 'puty-suite-prod':
        raise RuntimeError('Production ownership marker mismatch')
    docker = ['docker', 'compose', '--project-directory', str(BASE), '-f', str(BASE / 'compose.yaml'), '-p', 'puty-suite-prod']
    names = subprocess.check_output(docker + ['config', '--services'], text=True).splitlines()
    if any(name not in names for name in args.services):
        raise RuntimeError('Unknown service')
    ids = subprocess.check_output(['docker', 'ps', '-aq', '--filter', 'label=com.docker.compose.project=puty-suite-prod'], text=True).split()
    for container in ids:
        labels = json.loads(subprocess.check_output(['docker', 'inspect', '--format', '{{json .Config.Labels}}', container], text=True))
        if labels.get('com.puty.suite') != info['instanceID']:
            raise RuntimeError('Existing container belongs to another deployment')
    operation = {'status': ['ps'], 'start': ['up', '-d', '--no-build'], 'stop': ['stop'], 'restart': ['restart']}[args.action]
    subprocess.run(docker + operation + args.services, check=True)


if __name__ == '__main__':
    main()
