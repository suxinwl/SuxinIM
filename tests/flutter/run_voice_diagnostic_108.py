from pathlib import Path
import subprocess,datetime,json,time,urllib.request
R=Path('O:/');D=R/'tests/flutter';A=['E:/AndroidDev/sdk/platform-tools/adb.exe','-s','emulator-5588'];P='io.openim.upgrade38.test.dev';L='android-108-voice-diagnostic'
def adb(*x):return subprocess.check_output(A+list(x),timeout=30)
def web(method='GET',path='state'):
 req=urllib.request.Request('http://127.0.0.1:17787/'+path,data=b'{}' if method=='POST' else None,headers={'Content-Type':'application/json'},method=method)
 with urllib.request.urlopen(req,timeout=20) as f:return json.load(f)
pid=adb('shell','pidof',P).decode().strip()
if pid!='7825':raise RuntimeError('Expected cold own PID')
callmeta=D/f'{L}-call-start.json'
if callmeta.exists():raise RuntimeError('Preserve diagnosis')
callmeta.write_text(json.dumps({'at':datetime.datetime.now(datetime.timezone.utc).isoformat(),'ownPID':int(pid),'diagnosticOnly':True,'originalMenu':'Voice Call','normal150SecondAcceptance':False,'SIGQUIT':False,'HomeOrBackground':False},indent=2)+'\n')
adb('logcat','-c');adb('shell','input','tap','270','880');time.sleep(5);web('POST','accept');print('Original Voice Call accepted',flush=True)
samples=[];start=time.monotonic()
for target in [0,15,30]:
 while time.monotonic()-start<target:time.sleep(.3)
 state=web();shot=D/f'{L}-{target:03d}.png';shot.write_bytes(adb('exec-out','screencap','-p'))
 if adb('shell','pidof',P).decode().strip()!=pid:raise RuntimeError('PID changed')
 item={'elapsedSeconds':round(time.monotonic()-start,2),'at':datetime.datetime.now(datetime.timezone.utc).isoformat(),'videos':state.get('videos'),'webErrors':state.get('errors'),'androidPID':int(pid),'androidScreenshot':shot.relative_to(R).as_posix()}
 samples.append(item);(D/f'{L}-samples.json').write_text(json.dumps(samples,indent=2)+'\n');print(json.dumps(item),flush=True)
# Original voice hangup is sampled separately after confirming actual UI button position.
print('Voice30s samples complete; awaiting actualUI hangup position',flush=True)
