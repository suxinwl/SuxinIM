"""Fail-closed guards shared by the isolated test-stack operations."""
import json
import pathlib
import subprocess

ROOT = pathlib.Path('/srv/openim38-upgrade')
PROJECT = 'openim38-upgrade'
DOCKER = ['docker', '--host', 'unix:///var/run/docker.sock']


def run(args, **kwargs):
    result = subprocess.run(args, stdout=subprocess.PIPE, stderr=subprocess.PIPE, **kwargs)
    if result.returncode:
        # Docker config and database error output may contain credentials.
        raise RuntimeError('Isolated operation failed: ' + args[0] + ' (exit ' + str(result.returncode) + ')')
    return result.stdout


def validate_spec(spec, root=ROOT):
    if spec.get('name') != PROJECT:
        raise RuntimeError('Unexpected Compose project')
    for entry in spec.get('volumes', {}).values():
        if entry.get('external') or entry.get('driver_opts') or not entry.get('name', '').startswith(PROJECT + '_'):
            raise RuntimeError('Foreign or external volume refused')
    for entry in spec.get('networks', {}).values():
        if entry.get('external') or entry.get('driver') not in (None,'bridge') or entry.get('name') != PROJECT + '-net':
            raise RuntimeError('Foreign or external network refused')
    for service in spec.get('services', {}).values():
        if service.get('privileged') or service.get('network_mode'):
            raise RuntimeError('Privileged or host-network service refused')
        for mount in service.get('volumes', []):
            if mount.get('type') == 'bind':
                if not pathlib.Path(mount['source']).resolve().is_relative_to(root):
                    raise RuntimeError('Bind mount outside isolated root refused')
            elif mount.get('type') == 'volume':
                if mount.get('source') not in spec.get('volumes', {}):
                    raise RuntimeError('Untracked named volume refused')
            else:
                raise RuntimeError('Unexpected volume mount type')
        for port in service.get('ports', []):
            if port.get('host_ip') not in ('192.168.10.11', '127.0.0.1', '::1'):
                raise RuntimeError('Unexpected public host-port binding')


def guard():
    if ROOT.resolve() != ROOT or not ROOT.is_dir():
        raise RuntimeError('Expected real isolated root /srv/openim38-upgrade')
    state = json.loads((ROOT / 'instance.json').read_text())
    if (ROOT / 'instance.json').is_symlink():
        raise RuntimeError('Instance metadata must be a local regular file')
    if state.get('testOnly') is not True or state.get('originalDataMigrated') is not False or state.get('stackName') != PROJECT:
        raise RuntimeError('Instance is not the expected synthetic test stack')
    compose_file = ROOT / 'compose.yaml'
    if compose_file.is_symlink() or not compose_file.is_file():
        raise RuntimeError('Compose file must be a local regular file')
    cli = DOCKER + ['compose', '--project-directory', str(ROOT), '-f', str(compose_file), '-p', PROJECT]
    spec = json.loads(run(cli + ['config', '--format', 'json']))
    validate_spec(spec)
    return spec, cli


def require_owned_resources(spec):
    """Before destructive operations, reject collisions with other projects."""
    for kind, entries in (('volume', spec.get('volumes', {})), ('network', spec.get('networks', {}))):
        for entry in entries.values():
            result = subprocess.run(DOCKER + [kind, 'inspect', entry['name']], stdout=subprocess.PIPE, stderr=subprocess.PIPE)
            if result.returncode:
                continue  # Not created yet is normal for a new instance.
            inspected = json.loads(result.stdout)[0]
            if kind == 'volume' and inspected.get('Options'):
                raise RuntimeError('Custom host-backed volume refused')
            if (inspected.get('Labels') or {}).get('com.docker.compose.project') != PROJECT:
                raise RuntimeError('Existing resource is not owned by this Compose project')


if __name__ == '__main__':
    spec, _ = guard()
    require_owned_resources(spec)
    print('Isolated stack and resource ownership verified.')
