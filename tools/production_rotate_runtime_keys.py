"""Prepare a private eight-key bundle; commit local generators only after server proof."""
import argparse
import base64
import hashlib
import hmac
import importlib.util
import json
import os
import pathlib
import secrets
import sys
import time
import subprocess
import production_backup as backup

ROOT = pathlib.Path(__file__).resolve().parents[1]
PRIVATE = ROOT/'.private/production'
spec = importlib.util.spec_from_file_location('runtime_key_worker', ROOT/'deploy/production/rotate_runtime_keys.py')
worker = importlib.util.module_from_spec(spec)
spec.loader.exec_module(worker)


def protect(path):
    backup.protect(path)
    if os.name == 'nt':
        # Do not print ACL identities. Fresh private folders should contain only
        # the current operator (plus trusted SYSTEM/Administrators if explicit).
        code = "$acl=Get-Acl -LiteralPath $env:PUTY_ROTATION_ACL_PATH; @($acl.Access | ForEach-Object { [PSCustomObject]@{sid=$_.IdentityReference.Translate([System.Security.Principal.SecurityIdentifier]).Value;allow=($_.AccessControlType -eq 'Allow');inherited=$_.IsInherited} }) | ConvertTo-Json -Compress"
        result = subprocess.run(['powershell', '-NoProfile', '-NonInteractive', '-Command', code],
                                env={**os.environ, 'PUTY_ROTATION_ACL_PATH': str(path)}, capture_output=True, check=True)
        rows = json.loads(result.stdout or b'[]')
        if isinstance(rows, dict):
            rows = [rows]
        worker.require(not any(row['allow'] and (row['inherited'] or row['sid'] in (
            'S-1-1-0', 'S-1-5-11', 'S-1-5-32-545')) for row in rows), 'private_rotation_ACL_too_broad')


def private_save(path, value):
    protect(path.parent)
    worker.save(path, value)
    protect(path)


def refresh_local_plan(bundle_path):
    bundle_path = bundle_path.absolute()
    worker.require(bundle_path.is_file() and not bundle_path.is_symlink()
                   and bundle_path.resolve().is_relative_to(PRIVATE/'key-rotation'), 'private_bundle_path_required')
    bundle = json.loads(bundle_path.read_text()); worker.bundle_valid(bundle)
    protect(bundle_path.parent.parent)
    protect(bundle_path.parent)
    protect(bundle_path)
    private_save(bundle_path.parent/'local-plan.json', {'rotationID': bundle['rotationID'],
                 'bundleSHA256': worker.sha(worker.canonical(bundle)), 'files': worker.render_files(PRIVATE, bundle, local=True)})
    print(json.dumps({'localPlanRefreshed': True, 'rotationID': bundle['rotationID'],
                      'bundleUnchanged': True, 'productionModified': False}), flush=True)


def prepare():
    suite = json.loads((PRIVATE/'suite-secrets.json').read_text(encoding='utf-8-sig'))
    keys = json.loads((PRIVATE/'openim/secrets.json').read_text(encoding='utf-8-sig'))
    activation = json.loads((PRIVATE/'activation.json').read_text(encoding='utf-8-sig'))
    runtime = json.loads((PRIVATE/'gosuxin/runtime-environment.json').read_text(encoding='utf-8-sig'))
    old = {name: suite['bridge'] if name == 'bridge' else keys[name] for name in worker.NAMES}
    protected = ([value for name, value in suite.items() if name != 'bridge' and isinstance(value, str)]
                 + [value for name, value in keys.items() if name not in worker.NAMES and isinstance(value, str)]
                 + [runtime['WECOM_CREDENTIAL_KEY']])
    worker.require(not any(value in protected for value in old.values()), 'rotating_key_collides_with_protected_suite_material')
    bundle = {'format': 'puty-runtime-key-rotation-v1', 'rotationID': 'rotate-'+secrets.token_hex(12),
              'instanceID': suite['instanceID'], 'startAt': activation['startAt'], 'createdAt': int(time.time()),
              'old': old, 'new': {name: secrets.token_hex(32) for name in worker.NAMES},
              'preservedIMSHA256': worker.sha(worker.canonical({name: value for name, value in keys.items() if name not in worker.NAMES})),
              'preservedWeComSHA256': worker.sha(runtime['WECOM_CREDENTIAL_KEY'].encode()),
              'preservedSuiteSHA256': worker.sha(worker.canonical({name: value for name, value in suite.items() if name != 'bridge'}))}
    worker.bundle_valid(bundle)
    worker.require(activation['instanceID'] == bundle['instanceID']
                   and runtime['GOSUXIN_SYNC_START_AT'] == bundle['startAt']
                   and runtime['GOSUXIN_IM_HMAC_KEY'] == old['bridge'], 'private_generator_identity_mismatch')
    folder = PRIVATE/'key-rotation'/bundle['rotationID']
    folder.parent.mkdir(parents=True, mode=0o700, exist_ok=True)
    protect(folder.parent)
    folder.mkdir(mode=0o700)
    protect(folder)
    private_save(folder/'bundle.json', bundle)
    private_save(folder/'local-plan.json', {'rotationID': bundle['rotationID'],
                'bundleSHA256': worker.sha(worker.canonical(bundle)), 'files': worker.render_files(PRIVATE, bundle, local=True)})
    print(json.dumps({'preparedOnly': True, 'rotationID': bundle['rotationID'],
                      'privateBundlePath': str(folder/'bundle.json'), 'bundleSHA256': worker.sha(worker.canonical(bundle)),
                      'rotatedKeyNames': worker.NAMES, 'serverPlanStillRequired': True,
                      'productionModified': False, 'privateActiveGeneratorsModified': False}), flush=True)


def commit(bundle_path, receipt_path):
    bundle_path = bundle_path.absolute()
    worker.require(bundle_path.is_file() and not bundle_path.is_symlink()
                   and bundle_path.resolve().is_relative_to(PRIVATE/'key-rotation'), 'private_bundle_path_required')
    bundle = json.loads(bundle_path.read_text()); worker.bundle_valid(bundle)
    receipt = json.loads(receipt_path.read_text())
    signature = receipt.pop('signature', '')
    expected = hmac.new(bundle['new']['bridge'].encode(), worker.canonical(receipt), hashlib.sha256).hexdigest()
    worker.require(hmac.compare_digest(signature, expected) and receipt.get('status') == 'committed'
                   and receipt.get('rotationID') == bundle['rotationID']
                   and receipt.get('bundleSHA256') == worker.sha(worker.canonical(bundle)), 'server_commit_receipt_not_authentic')
    plan = json.loads((bundle_path.parent/'local-plan.json').read_text())
    worker.require(plan['bundleSHA256'] == worker.sha(worker.canonical(bundle)), 'local_plan_bundle_mismatch')
    for row in plan['files']:
        worker.require(row['path'] in (*worker.FILES, 'suite-secrets.json'), 'local_generator_path_outside_allowlist')
        path = PRIVATE/row['path']
        worker.require(path.is_file() and not path.is_symlink() and path.resolve().is_relative_to(PRIVATE)
                       and worker.sha(path.read_bytes()) in (row['beforeSHA256'], row['afterSHA256']),
                       'private_generator_changed_since_prepare')
    for row in plan['files']:
        protect((PRIVATE/row['path']).parent)
        worker.private_write(PRIVATE/row['path'], base64.b64decode(row['after']))
        protect(PRIVATE/row['path'])
    suite = json.loads((PRIVATE/'suite-secrets.json').read_text())
    keys = json.loads((PRIVATE/'openim/secrets.json').read_text())
    worker.require(worker.sha(worker.canonical({name: value for name, value in suite.items() if name != 'bridge'})) == bundle['preservedSuiteSHA256']
                   and worker.sha(worker.canonical({name: value for name, value in keys.items() if name not in worker.NAMES})) == bundle['preservedIMSHA256'],
                   'protected_generator_material_changed')
    print(json.dumps({'rotationID': bundle['rotationID'], 'privateGeneratorsCommitted': True,
                      'excludedKeysPreserved': True, 'productionModified': False}), flush=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest='action', required=True)
    commands.add_parser('prepare')
    refresh = commands.add_parser('refresh-local-plan')
    refresh.add_argument('--bundle', type=pathlib.Path, required=True)
    command = commands.add_parser('commit-local')
    command.add_argument('--bundle', type=pathlib.Path, required=True)
    command.add_argument('--receipt', type=pathlib.Path, required=True)
    args = parser.parse_args()
    if args.action == 'prepare':
        prepare()
    elif args.action == 'refresh-local-plan':
        refresh_local_plan(args.bundle)
    else:
        commit(args.bundle, args.receipt)


if __name__ == '__main__':
    try:
        main()
    except Exception as error:
        print(json.dumps({'rotationPreparationFailed': True, 'category': type(error).__name__,
                          'message': str(error) if type(error) is RuntimeError else 'private_details_suppressed'}), flush=True)
        sys.exit(1)
