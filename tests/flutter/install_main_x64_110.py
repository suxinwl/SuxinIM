"""Install the frozen private x64 main candidate on this task's own API30 AVD.

This preserves application data and records credential digests only. It does not
publish an artifact, clear data, log out, or change system TLS verification.
"""
from pathlib import Path
import base64
import datetime
import hashlib
import json
import subprocess
import xml.etree.ElementTree as XML

ROOT = Path('O:/')
OUT = ROOT / 'tests/flutter/android-110-x64-main-install.json'
APK = ROOT / '.private/android-validation/OpenIM38-3.8.10+110-main-x64-candidate.apk'
ADB = ['E:/AndroidDev/sdk/platform-tools/adb.exe', '-s', 'emulator-5588']
PACKAGE = 'io.openim.upgrade38.test.dev'
EXPECTED = json.loads((ROOT / 'tests/flutter/android-main-x64-110.json').read_text())['sha256']


def adb(*args, timeout=60):
    return subprocess.check_output(ADB + list(args), timeout=timeout)


def pair_info():
    xml = adb('exec-out', 'cat', f'/data/user/0/{PACKAGE}/shared_prefs/FlutterSharedPreferences.xml')
    prefs = {node.attrib['name']: node.text for node in XML.fromstring(xml)}
    certificate = json.loads(prefs['flutter.loginCertificate'])
    if certificate['userID'] != 'upgrade38_u104':
        raise RuntimeError('Expected only the assigned synthetic account104')
    pair = {key: certificate[key] for key in ['userID', 'imToken', 'chatToken']}
    pair['platformID'] = 2
    digest = hashlib.sha256(json.dumps(pair, sort_keys=True, separators=(',', ':')).encode()).hexdigest()
    expiry = {}
    for key in ['imToken', 'chatToken']:
        expiry[key] = json.loads(base64.urlsafe_b64decode(pair[key].split('.')[1] + '===' )).get('exp')
    return {'userID': pair['userID'], 'pairSHA256': digest, 'tokenExpiryUnixOnly': expiry}


if OUT.exists():
    raise RuntimeError('Preserve the original installation evidence')
if hashlib.sha256(APK.read_bytes()).hexdigest() != EXPECTED:
    raise RuntimeError('Frozen private candidate hash mismatch')
if adb('emu', 'avd', 'name').splitlines()[0] != b'openim38_api30':
    raise RuntimeError('Own API30 AVD only')
if adb('shell', 'getprop', 'sys.boot_completed').strip() != b'1':
    raise RuntimeError('AVD boot is incomplete')
if b'uid=0(root)' not in adb('shell', 'id'):
    raise RuntimeError('Dedicated rooted test AVD required')
ca = hashlib.sha256(adb('exec-out', 'cat', '/system/etc/security/cacerts/1c34b89e.0')).hexdigest()
if ca != '90817f305832a3c2398af0384e76004096647aeeac7095a250c6a5a4845655f0':
    raise RuntimeError('Restore the development CA through the audited AVD mount setup first')
before = pair_info()
record = {
    'startedAt': datetime.datetime.now(datetime.timezone.utc).isoformat(),
    'APK': APK.relative_to(ROOT).as_posix(), 'APK_SHA256': EXPECTED,
    'entrypoint': 'lib/main.dart', 'validationDartWrapper': False,
    'devNativeProbeChannelPresent': True,
    'AVD': 'openim38_api30', 'serial': 'emulator-5588',
    'physicalARMDevice': False, 'systemCASHA256': ca,
    'dataCleared': False, 'loggedOutOrRevoked': False,
    'pairBefore': before, 'installedSuccessfully': False,
}
OUT.write_text(json.dumps(record, indent=2) + '\n')
result = adb('install', '-r', str(APK), timeout=420).decode('utf-8', 'replace').strip()
if 'Success' not in result:
    raise RuntimeError('Installation did not succeed: ' + result)
dump = adb('shell', 'dumpsys', 'package', PACKAGE).decode('utf-8', 'replace')
lines = [line.strip() for line in dump.splitlines() if any(key in line for key in ['versionCode=', 'versionName=', 'primaryCpuAbi=', 'secondaryCpuAbi='])]
if not any('versionCode=110' in line for line in lines):
    raise RuntimeError('Installed package versionCode does not match110')
after = pair_info()
record.update({
    'installedSuccessfully': True, 'installationOutput': result,
    'completedAt': datetime.datetime.now(datetime.timezone.utc).isoformat(),
    'packageFacts': lines, 'pairAfter': after,
    'pairPreserved': before == after,
    'guestProperties': {key: adb('shell', 'getprop', key).decode().strip() for key in [
        'ro.product.cpu.abilist', 'ro.dalvik.vm.native.bridge',
        'ro.ndk_translation.version', 'ro.build.version.sdk']},
    'runtimeAccepted': False,
})
if before != after:
    raise RuntimeError('Installation changed the cached credential pair unexpectedly')
OUT.write_text(json.dumps(record, indent=2) + '\n')
print(json.dumps(record, indent=2))
