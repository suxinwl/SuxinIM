"""Read-only remote preflight. No fixtures, tokens, Docker env or cert keys printed."""
import hashlib
import json
import pathlib
import subprocess

ROOT = pathlib.Path('/srv/openim38-upgrade')
NAME = 'openim38-upgrade-load'
DOCKER = ['docker', '--host', 'unix:///var/run/docker.sock']


def sha(path):
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


result = {'rootIsReal': ROOT.resolve() == ROOT, 'loadTopLevel': [], 'knownFiles': []}
load = ROOT / 'load'
if load.is_dir():
    result['loadTopLevel'] = [{'name': path.name, 'directory': path.is_dir()} for path in sorted(load.iterdir())]
    for name in ['load-100.mjs', 'load-static-server.mjs', 'package.json', 'node_modules/playwright/package.json',
                 'static/load-host.html', 'static/load.html', 'static/load-client.js', 'static/openIM.wasm',
                 'static/sql-wasm.wasm', 'static/wasm_exec.js']:
        path = load / name
        result['knownFiles'].append({'file': name, 'exists': path.is_file(),
                                     'sha256': sha(path) if path.is_file() else None})
raw = subprocess.run(DOCKER + ['inspect', NAME], capture_output=True, text=True)
if raw.returncode:
    result['containerPresent'] = False
else:
    container = json.loads(raw.stdout)[0]
    result.update({'containerPresent': True, 'container': {
        'name': container['Name'], 'id': container['Id'], 'running': container['State']['Running'],
        'status': container['State']['Status'], 'image': container['Config']['Image'],
        'labels': container['Config'].get('Labels', {}), 'entrypoint': container['Config']['Entrypoint'],
        'command': container['Config']['Cmd'], 'workingDir': container['Config']['WorkingDir'],
        'networkMode': container['HostConfig']['NetworkMode'], 'privileged': container['HostConfig']['Privileged'],
        'memory': container['HostConfig']['Memory'], 'nanoCpus': container['HostConfig']['NanoCpus'],
        'pidsLimit': container['HostConfig'].get('PidsLimit'), 'shmSize': container['HostConfig']['ShmSize'],
        'mounts': [{key: value.get(key) for key in ('Type', 'Source', 'Destination', 'RW')} for value in container['Mounts']],
    }})
port = subprocess.run(['ss', '-H', '-ltn', 'sport', '=', ':7778'], capture_output=True, text=True)
result['hostPort7778InUse'] = bool(port.stdout.strip())
result['privateFixtureRead'] = False
result['plannedFixtureFields'] = ['userID', 'imToken', 'platformID=5']
result['staticServerCommand'] = 'node /load/load-static-server.mjs /load/static (127.0.0.1:7778); no Vite required'
print(json.dumps(result, indent=2))
