"""Trust the development CA only in this task's dedicated AVD mount namespaces.

Never edits the shared SDK system image or any historical AVD. Bind mounts vanish
on AVD reboot. This is test setup, not an installation instruction for user phones.
"""
from pathlib import Path
import argparse
import hashlib
import json
import subprocess

ROOT = Path(__file__).absolute().parents[2]
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--output', type=Path)
args = parser.parse_args()
destination = args.output.absolute() if args.output else ROOT / 'tests/flutter/avd-ca-setup.json'
if args.output and (destination.parent.resolve() != (ROOT / 'tests/flutter').resolve() or destination.exists()):
    raise RuntimeError('Explicit CA report must be new and directly under tests/flutter')
ADB = ['E:/AndroidDev/sdk/platform-tools/adb.exe', '-s', 'emulator-5588']
def run(*args):
    return subprocess.check_output(ADB + list(args), timeout=60).decode('utf-8', 'replace').strip()
avd_name = run('shell', 'getprop', 'ro.boot.qemu.avd_name')
if not avd_name:
    avd_name = run('emu', 'avd', 'name').splitlines()[0]
if avd_name not in ['openim38_api35', 'openim38_api30', 'openim38_api30_arm109']:
    raise RuntimeError('Refusing to alter another AVD')
if 'uid=0(root)' not in run('shell', 'id'):
    raise RuntimeError('Run adb root on this dedicated userdebug AVD first')
ca = ROOT / 'artifacts/dev-cert/OpenIM38-Development-CA.crt'
subject_hash = subprocess.check_output(['C:/Program Files/Git/usr/bin/openssl.exe',
    'x509', '-in', str(ca), '-subject_hash_old', '-noout'], timeout=10).decode().strip()
target = '/data/local/tmp/openim38-system-cacerts'
run('shell', 'am', 'force-stop', 'io.openim.upgrade38.test.dev')
run('shell', 'mkdir', '-p', target)
directories = [d for d in ['/system/etc/security/cacerts', '/apex/com.android.conscrypt/cacerts']
    if run('shell', f'if [ -d {d} ]; then echo yes; else echo no; fi') == 'yes']
for directory in directories:
    run('shell', f'cp {directory}/* {target}/')
remote_ca = target + '/' + subject_hash + '.0'
run('push', str(ca), remote_ca)
run('shell', f'chmod 755 {target}; chmod 644 {target}/*')
run('shell', f'chcon -R u:object_r:system_security_cacerts_file:s0 {target}')
zygotes = run('shell', 'pidof', 'zygote64', 'zygote').split()
for directory in directories:
    run('shell', f'mount --bind {target} {directory}')
    for pid in zygotes:
        if not pid.isdigit(): raise RuntimeError('Invalid zygote PID')
        run('shell', f'nsenter -t {pid} -m -- mount --bind {target} {directory}')
report = {'avd': avd_name, 'serial': 'emulator-5588', 'rootedTestAVD': True,
    'certificateSHA256': hashlib.file_digest(ca.open('rb'), 'sha256').hexdigest(),
    'subjectHash': subject_hash, 'zygotePIDs': zygotes, 'sharedSystemImageModified': False,
    'method': 'Private AVD runtime bind mounts, removed by reboot',
    'endUserPhoneInstruction': False}
destination.write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
print(json.dumps(report))
