import subprocess,json,time,shlex,xml.etree.ElementTree as X
from pathlib import Path
r=Path('O:/'); p=json.loads((r/'.private/test-accounts.json').read_text(encoding='utf-8-sig'))['password'];a=['E:/AndroidDev/sdk/platform-tools/adb.exe','-s','emulator-5588']
def call(*args):return subprocess.check_output(a+list(args),timeout=45)
call('shell','input tap 200 658');time.sleep(.6);call('shell','input tap 429 510');call('shell','input tap 180 510');call('shell','input text '+shlex.quote(p));time.sleep(3);call('shell','input keyevent 4');time.sleep(1)
call('shell','uiautomator dump /sdcard/check-ui.xml')
x=X.fromstring(call('shell','cat /sdcard/check-ui.xml'));fields=[n.get('text','') for n in x.iter('node') if n.get('class')=='android.widget.EditText'];ok=bool(fields) and fields[-1]==p
print(json.dumps({'fieldCount':len(fields),'passwordExact':ok,'passwordLength':len(fields[-1]) if fields else None}));call('shell','rm /sdcard/check-ui.xml')
# The field was visible from the previous failed comparison; hide it before submit.
call('shell','input tap 465 510');time.sleep(.4)
if ok:call('shell','input tap 270 679');print('Exact synthetic UI credentials submitted')
