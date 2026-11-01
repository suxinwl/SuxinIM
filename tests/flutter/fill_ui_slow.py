import json, subprocess, time, shlex
from pathlib import Path
r=Path('O:/');d=json.loads((r/'.private/test-accounts.json').read_text(encoding='utf-8-sig'));a=['E:/AndroidDev/sdk/platform-tools/adb.exe','-s','emulator-5588','shell']
def cmd(c): subprocess.run(a+[c],check=True,capture_output=True,timeout=45)
def fill(x,y,clear,text):
 cmd(f'input tap {clear} {y}');time.sleep(.6);cmd(f'input tap {x} {y}');time.sleep(.6)
 for ch in text:
  cmd('input text '+shlex.quote(ch));time.sleep(.22)
 time.sleep(2)
fill(240,391,465,d['users'][103]['email']);cmd('input keyevent 4');time.sleep(1);fill(180,510,429,d['password']);cmd('input keyevent 4');time.sleep(2)
print('Input injected character by character; not yet submitted')
