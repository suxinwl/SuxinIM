"""Submit the original API30 email login form using synthetic account104."""
import json,subprocess,time,shlex,xml.etree.ElementTree as X,re
from pathlib import Path
R=Path('O:/');d=json.loads((R/'.private/test-accounts.json').read_text(encoding='utf-8-sig'));A=['E:/AndroidDev/sdk/platform-tools/adb.exe','-s','emulator-5588']
def run(*a):
 try:return subprocess.check_output(A+list(a),timeout=60,stderr=subprocess.PIPE)
 except (subprocess.CalledProcessError,subprocess.TimeoutExpired):
  # Test credentials are injected into adb arguments; never include a failed
  # command or its unredacted stderr in public diagnostic output.
  raise RuntimeError('Own-AVD login action failed') from None
def shape():
 try:
  run('shell','uiautomator dump /sdcard/openim-login-check.xml')
  return X.fromstring(run('shell','cat /sdcard/openim-login-check.xml'))
 finally:run('shell','rm -f /sdcard/openim-login-check.xml')
def fields():return [n for n in shape().iter('node') if n.get('class')=='android.widget.EditText']
def center(n):
 a,b,c,e=map(int,re.findall(r'\d+',n.get('bounds')));return a,b,c,e,(b+e)//2
f=fields();assert len(f)==2
x,y,right,bottom,ey=center(f[0]);_,_,_,_,py=center(f[1])
run('shell',f'input tap {right+17} {ey}');time.sleep(.5)
run('shell',f'input tap 200 {ey}');time.sleep(1)
run('shell','input text '+shlex.quote(d['users'][103]['email']));time.sleep(2)
run('shell',f'input tap 180 {py}');time.sleep(1)
run('shell','input keyevent 123; for i in $(seq 1 40); do input keyevent 67; done')
run('shell','input text '+shlex.quote(d['password']));time.sleep(2);run('shell','input keyevent 4');time.sleep(.5)
run('shell',f'input tap {right+17} {py}');time.sleep(.5)
try:
 f=fields();status={'emailExact':len(f)==2 and f[0].get('text')==d['users'][103]['email'],'passwordExact':len(f)==2 and f[1].get('text')==d['password'],'passwordLength':len(f[-1].get('text','')) if f else None}
finally:run('shell',f'input tap {right+17} {py}')
(R/'tests/flutter/api30-ui-credential-input.json').write_text(json.dumps(status,indent=2)+'\n');print(json.dumps(status))
if not all([status['emailExact'],status['passwordExact']]):raise RuntimeError('Input mismatch, no login submitted')
for n in shape().iter('node'):
 if (n.get('text') or n.get('content-desc'))=='Login':
  x,y,z,w=map(int,re.findall(r'\d+',n.get('bounds')));run('shell',f'input tap {(x+z)//2} {(y+w)//2}');print('Original UI synthetic login submitted');break
