"""Clean-install the unchanged ARM64 candidate only on the new owned API30 AVD."""
from pathlib import Path
import datetime
import hashlib
import json
import subprocess
import xml.etree.ElementTree as XML

ROOT = Path('O:/')
OUT = ROOT / 'tests/flutter/android-109-clean-arm-main-install.json'
APK = ROOT / '.private/android-validation/OpenIM38-3.8.9+109-main-arm64-candidate.apk'
ADB = ['E:/AndroidDev/sdk/platform-tools/adb.exe', '-s', 'emulator-5588']
PACKAGE = 'io.openim.upgrade38.test.dev'
EXPECTED = '141ef706479719a644a6676d244ec85412e5c1fb09af3df16d3a6d30e06cd7c1'


def run(*args, timeout=60):
    return subprocess.run(ADB + list(args), capture_output=True, timeout=timeout)


def adb(*args, timeout=60):
    result = run(*args, timeout=timeout)
    result.check_returncode()
    return result.stdout


if OUT.exists():
    raise RuntimeError('Preserve the original clean installation evidence')
assert hashlib.sha256(APK.read_bytes()).hexdigest() == EXPECTED
assert adb('emu', 'avd', 'name').splitlines()[0] == b'openim38_api30_arm109'
assert adb('shell', 'getprop', 'sys.boot_completed').strip() == b'1'
assert b'uid=0(root)' in adb('shell', 'id')
assert not run('shell', 'pm', 'path', PACKAGE).stdout.strip()
ca = hashlib.sha256(adb('exec-out', 'cat', '/system/etc/security/cacerts/1c34b89e.0')).hexdigest()
assert ca == '90817f305832a3c2398af0384e76004096647aeeac7095a250c6a5a4845655f0'
record = {
    'startedAt': datetime.datetime.now(datetime.timezone.utc).isoformat(),
    'APK': APK.relative_to(ROOT).as_posix(), 'APK_SHA256': EXPECTED,
    'entrypoint': 'lib/main.dart', 'validationDartWrapper': False,
    'devNativeProbeChannelPresent': True,
    'AVD': 'openim38_api30_arm109', 'serial': 'emulator-5588',
    'cleanInstall': True, 'upgradeOfExistingPackage': False,
    'oldAVD': 'openim38_api30', 'oldAVDDataPreserved': True,
    'physicalARMDevice': False, 'systemCASHA256': ca,
    'ABIOverrideArgument': None, 'installedSuccessfully': False,
}
OUT.write_text(json.dumps(record, indent=2) + '\n')
result = run('install', str(APK), timeout=240)
for kind, value in [('stdout', result.stdout), ('stderr', result.stderr)]:
    (ROOT / f'tests/flutter/android-109-clean-arm-main-install-{kind}.txt').write_bytes(value)
record.update({
    'completedAt': datetime.datetime.now(datetime.timezone.utc).isoformat(),
    'returnCode': result.returncode,
    'stdout': result.stdout.decode('utf-8', 'replace'),
    'stderr': result.stderr.decode('utf-8', 'replace'),
    'installedSuccessfully': result.returncode == 0 and b'Success' in result.stdout,
})
OUT.write_text(json.dumps(record, indent=2) + '\n')
result.check_returncode()
dump = adb('shell', 'dumpsys', 'package', PACKAGE).decode('utf-8', 'replace')
lines = [line.strip() for line in dump.splitlines() if any(key in line for key in [
    'versionCode=', 'versionName=', 'primaryCpuAbi=', 'secondaryCpuAbi='])]
packages = XML.fromstring(adb('exec-out', 'cat', '/data/system/packages.xml'))
own = [node for node in packages.findall('package') if node.get('name') == PACKAGE]
assert len(own) == 1
record.update({
    'packageFacts': lines,
    'packageABIAttributes': {key: value for key, value in own[0].attrib.items()
                             if 'abi' in key.lower() or key == 'name'},
    'guestProperties': {key: adb('shell', 'getprop', key).decode().strip() for key in [
        'ro.product.cpu.abilist', 'ro.dalvik.vm.native.bridge',
        'ro.ndk_translation.version', 'ro.build.version.sdk']},
    'runtimeAccepted': False,
})
assert any('versionCode=109' in line for line in lines)
assert own[0].get('primaryCpuAbi') == 'arm64-v8a'
OUT.write_text(json.dumps(record, indent=2) + '\n')
print(json.dumps(record, indent=2))
