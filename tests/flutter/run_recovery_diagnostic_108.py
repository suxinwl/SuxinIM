from pathlib import Path
import datetime,subprocess,time,urllib.request,json,hashlib,sys
R=Path('O:/');L='android-108-recapture-diagnostic';A=['E:/AndroidDev/sdk/platform-tools/adb.exe','-s','emulator-5588']
def adb(*x):return subprocess.check_output(A+list(x),timeout=30)
def web(method='GET',path='state'):
 q=urllib.request.Request('http://127.0.0.1:17787/'+path,data=b'{}' if method=='POST' else None,headers={'Content-Type':'application/json'},method=method)
 with urllib.request.urlopen(q,timeout=20) as r:return json.load(r)
if adb('shell','pidof','io.openim.upgrade38.test.dev').strip()!=b'4997':raise RuntimeError('Keep exact prior PID')
p=R/f'tests/flutter/{L}-start.json'
if p.exists():raise RuntimeError('Preserve diagnostic history')
p.write_text(json.dumps({'startedAt':datetime.datetime.now(datetime.timezone.utc).isoformat(),'ownPID':4997,'samePIDAsFailedCall1':True,'diagnosticOnly':True,'normal150SecondAcceptance':False,'noHome':True,'menuSelection':'Video Call original menu confirmed bounds 14,920,526,995','build':108},indent=2)+'\n')
adb('shell','input','tap','270','958');time.sleep(5)
accepted=web('POST','accept');print('Original Web accept action completed',flush=True)
samples=[];start=time.monotonic()
for target in [0,15,30]:
 while time.monotonic()-start<target:time.sleep(.3)
 s=web();png=adb('exec-out','screencap','-p');shot=R/f'tests/flutter/{L}-{target:03d}.png';shot.write_bytes(png)
 pid=adb('shell','pidof','io.openim.upgrade38.test.dev').decode().strip()
 if pid!='4997':raise RuntimeError('Original PID changed')
 item={'elapsedSeconds':round(time.monotonic()-start,2),'at':datetime.datetime.now(datetime.timezone.utc).isoformat(),'videos':s.get('videos'),'webErrors':s.get('errors'),'androidPID':4997,'androidScreenshot':shot.relative_to(R).as_posix()}
 samples.append(item);(R/f'tests/flutter/{L}-samples.json').write_text(json.dumps(samples,indent=2)+'\n');print(json.dumps(item),flush=True)
