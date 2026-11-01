from pathlib import Path
import subprocess,time,datetime,json,re,hashlib
R=Path('O:/');L='android-108-recapture-diagnostic';A=['E:/AndroidDev/sdk/platform-tools/adb.exe','-s','emulator-5588']
def adb(*x):return subprocess.check_output(A+list(x),timeout=30)
if adb('shell','pidof','io.openim.upgrade38.test.dev').strip()!=b'4997':raise RuntimeError('Exact own PID required')
recordPath=R/f'tests/flutter/{L}-manual-diagnostic.json'
if recordPath.exists():raise RuntimeError('Preserve old evidence')
old=set(adb('shell','find','/data/anr','-maxdepth','1','-type','f').decode().splitlines())
log=(R/f'tests/flutter/{L}-hangup-script.log').open('wb')
p=subprocess.Popen([str(R/'.venv-tools/Scripts/python.exe'),str(R/'tests/flutter/hangup_original_call.py'),L],stdout=log,stderr=subprocess.STDOUT)
start=time.monotonic();sent=False;events=[];began=datetime.datetime.now(datetime.timezone.utc).isoformat()
while p.poll() is None and time.monotonic()-start<110:
 raw=adb('logcat','-d','-v','epoch','-s','FlutterWebRTCPlugin:D')
 active={}
 for line in raw.decode(errors='replace').splitlines():
  m=re.match(r'^\s*(\d+\.\d+)\s+4997\s+\d+\s+D\s+FlutterWebRTCPlugin\s*:\s*\[OpenIMRtcNative\]\s+(\S+)\s+job=(\d+)\s+operation=renderer.detach\s+phase=(ownerAcquired|actionEnd|physicalComplete)\b',line)
  if m:
   k=(m[2],m[3])
   if m[4]=='ownerAcquired':active[k]=float(m[1])
   else:active.pop(k,None)
 if active and not sent:
  nowDevice=float(adb('shell','date','+%s').decode().strip())
  stale={str(k):v for k,v in active.items() if nowDevice-v>3}
  if stale:
   if adb('shell','pidof','io.openim.upgrade38.test.dev').strip()!=b'4997':raise RuntimeError('PID changed before diagnostic signal')
   stamp=datetime.datetime.now(datetime.timezone.utc).isoformat();adb('shell','kill','-3','4997');sent=True
   events.append({'signal':'SIGQUIT','pid':4997,'at':stamp,'elapsedSecondsFromHarnessStart':round(time.monotonic()-start,3),'observedRendererDetachPending':stale,'requestedAfterAtLeastSeconds':3,'systemANR':False,'perturbsTiming':True})
   print('One manually requested SIGQUIT captured for stalled detach',flush=True)
 time.sleep(.4)
if p.poll() is None:p.wait(timeout=70)
log.close();time.sleep(3)
files=[]
for index,source in enumerate(sorted(set(adb('shell','find','/data/anr','-maxdepth','1','-type','f').decode().splitlines())-old)):
 if not re.fullmatch(r'/data/anr/[A-Za-z0-9_.:-]+',source):raise RuntimeError('Unexpected trace path')
 raw=adb('exec-out','cat',source)
 if b'----- pid 4997 ' not in raw:continue
 private=R/f'.private/android-raw-evidence/{L}-manual-sigquit-{index}.txt'
 if private.exists():raise RuntimeError('Preserve exact trace history')
 private.write_bytes(raw)
 public,n=re.subn(rb'eyJ[A-Za-z0-9_-]{15,}\.[A-Za-z0-9_-]{20,}\.[A-Za-z0-9_-]{15,}',b'[REDACTED_JWT]',raw)
 if re.search(rb'a=ice-(?:ufrag|pwd):|a=fingerprint:|m=(?:audio|video) [0-9]',public):raise RuntimeError('Raw trace private; review SDP before export')
 output=R/f'tests/flutter/{L}-manual-sigquit-{index}.txt';output.write_bytes(public)
 files.append({'source':source,'privateOriginal':private.relative_to(R).as_posix(),'publicFile':output.relative_to(R).as_posix(),'originalSHA256':hashlib.sha256(raw).hexdigest(),'publicSHA256':hashlib.sha256(public).hexdigest(),'jwtRedactions':n,'otherBytesPreserved':True})
record={'startedAt':began,'diagnosticOnly':True,'events':events,'manualThreadDumps':files,'hangupScriptExitCode':p.returncode,'note':'Active SIGQUIT only if exact own PID detach had not physically completed after 3 seconds; never normal latency/ANR acceptance.'}
recordPath.write_text(json.dumps(record,indent=2)+'\n');print(json.dumps(record),flush=True)
