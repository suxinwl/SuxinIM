"""Cover-install only the own114 x64 candidate; retains original test userdata."""
from pathlib import Path
from datetime import datetime, timezone
import hashlib
import json
import re
import subprocess
import time

ROOT = Path('O:/')
OUT = ROOT / 'tests/flutter'
ADB = ['E:/AndroidDev/sdk/platform-tools/adb.exe', '-s', 'emulator-5588']
PACKAGE = 'io.openim.upgrade38.test.dev'
metadata = json.loads((OUT / 'android-main-x64-114.json').read_text())
apk = ROOT / metadata['artifact']
assert hashlib.file_digest(apk.open('rb'), 'sha256').hexdigest() == metadata['sha256']
assert subprocess.check_output(ADB + ['emu', 'avd', 'name'], timeout=30).splitlines()[0] == b'openim38_api30'
assert subprocess.check_output(ADB + ['shell', 'getprop', 'sys.boot_completed'], timeout=30).strip() == b'1'
name = 'android-114-x64-main-install'
assert not (OUT / (name + '.json')).exists()
started = datetime.now(timezone.utc).isoformat()
clock = time.monotonic()
result = subprocess.run(ADB + ['install', '-r', '--abi', 'x86_64', str(apk)],
                        capture_output=True, timeout=240)
(OUT / (name + '.stdout.log')).open('xb').write(result.stdout)
(OUT / (name + '.stderr.log')).open('xb').write(result.stderr)
facts = subprocess.check_output(ADB + ['shell', 'dumpsys', 'package', PACKAGE], timeout=45).decode('utf-8', 'replace')
record = {
    'startedAt': started, 'endedAt': datetime.now(timezone.utc).isoformat(),
    'elapsedSeconds': time.monotonic() - clock, 'serial': 'emulator-5588', 'AVD': 'openim38_api30',
    'APK_SHA256': metadata['sha256'], 'size': metadata['size'], 'sourceSHA256': metadata['sourceSHA256'],
    'replacePreserveData': True, 'explicitABI': 'x86_64', 'exitCode': result.returncode,
    'successTextObserved': b'Success' in result.stdout,
    'notSystemInstallerUpgradeAcceptance': True,
}
assert result.returncode == 0 and record['successTextObserved'], 'Keep install evidence; never blindly reinstall'
assert re.search(r'versionCode=114\b', facts) and re.search(r'versionName=3\.8\.14\b', facts)
assert re.search(r'primaryCpuAbi=x86_64\b', facts)
package_path = subprocess.check_output(ADB + ['shell', 'pm', 'path', PACKAGE], timeout=30).decode().strip()
assert package_path.startswith('package:/data/app/') and '\n' not in package_path
remote = package_path[len('package:'):]
installed = subprocess.check_output(ADB + ['exec-out', 'cat', remote], timeout=90)
assert len(installed) == metadata['size'] and hashlib.sha256(installed).hexdigest() == metadata['sha256']
record.update({'installedVersionCode': 114, 'installedVersionName': '3.8.14', 'primaryCpuAbi': 'x86_64',
               'installedBaseAPK_SHA256': hashlib.sha256(installed).hexdigest(),
               'installedPackagePath': remote, 'fullInstalledBytesVerified': True})
(OUT / (name + '.json')).open('x', encoding='utf-8').write(json.dumps(record, indent=2) + '\n')
print(json.dumps(record))
