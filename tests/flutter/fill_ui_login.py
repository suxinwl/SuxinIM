"""Fill the original login form on the isolated AVD, without printing credentials."""
import json
from pathlib import Path
import shlex
import subprocess
import time
ROOT = Path(__file__).absolute().parents[2]
data = json.loads((ROOT / '.private/test-accounts.json').read_text(encoding='utf-8-sig'))
def shell(command):
    subprocess.run(['E:/AndroidDev/sdk/platform-tools/adb.exe', '-s', 'emulator-5588',
                    'shell', command], check=True, capture_output=True, timeout=45)
shell('input tap 465 391')
time.sleep(.5)
shell('input tap 240 391')
time.sleep(.5)
shell('input text ' + shlex.quote(data['users'][103]['email']))
time.sleep(1)
shell('input keyevent 61')
time.sleep(.5)
shell('input tap 429 510')
time.sleep(.5)
shell('input tap 180 510')
time.sleep(.5)
shell('input text ' + shlex.quote(data['password']))
time.sleep(3)
shell('input keyevent 4')
time.sleep(1)
shell('input tap 270 679')
time.sleep(2)
(ROOT / 'tests/flutter/android-ui-login-submitted.png').write_bytes(subprocess.check_output([
    'E:/AndroidDev/sdk/platform-tools/adb.exe', '-s', 'emulator-5588', 'exec-out', 'screencap', '-p']))
print('Synthetic account104 login submitted through original UI')
