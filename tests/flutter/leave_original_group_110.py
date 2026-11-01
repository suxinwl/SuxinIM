"""Leave the current original110 synthetic group, keeping the two Web peers active."""
from pathlib import Path
import datetime, hashlib, json, re, subprocess, time, urllib.request
import xml.etree.ElementTree as ET

ROOT=Path('O:/'); ADB=['E:/AndroidDev/sdk/platform-tools/adb.exe','-s','emulator-5588']
PKG='io.openim.upgrade38.test.dev'; GROUP='2901057568'; MARKER='a110g1'
OUT=ROOT/'tests/flutter/android-110-group-original-leave.json'
if OUT.exists(): raise RuntimeError('preserve previous result')
def adb(*args): return subprocess.check_output(ADB+list(args),timeout=35)
def hierarchy():
    temp='/data/local/tmp/openim110-group-leave.xml'
    try:
        adb('shell','uiautomator','dump',temp)
        return list(ET.fromstring(adb('exec-out','cat',temp)).iter('node'))
    finally: adb('shell','rm','-f',temp)
def focus():
    line=next((x for x in adb('shell','dumpsys','window').decode(errors='replace').splitlines() if 'mCurrentFocus='in x),'')
    if PKG+'/' not in line: raise RuntimeError('own activity required')
def get(path):
    with urllib.request.urlopen('http://127.0.0.1:17788'+path,timeout=30) as response:return json.load(response)
focus()
if adb('emu','avd','name').splitlines()[0]!=b'openim38_api30':raise RuntimeError('own AVD required')
if adb('shell','pidof',PKG).decode().strip()!='7093':raise RuntimeError('same accepted process required')
before_anr=adb('shell','dumpsys','activity','lastanr');nodes=hierarchy()
labels={x.get('text')or x.get('content-desc')or ''for x in nodes}
if 'Hang Up'not in labels or not any('Android104 RTC collaboration'in x for x in labels):raise RuntimeError('original fixed group call required')
started=datetime.datetime.now(datetime.timezone.utc).isoformat();start=time.monotonic()
adb('shell','input','tap','282','970');hangup_completed=time.monotonic()-start
focus();nodes=hierarchy();labels={x.get('text')or x.get('content-desc')or ''for x in nodes}
if 'Hang Up'in labels or not any('Android104 RTC collaboration'in x for x in labels):raise RuntimeError('group chat return required before input')
if len([x for x in nodes if x.get('class')=='android.widget.EditText'])!=1:raise RuntimeError('one original composer required')
adb('shell','input','tap','200','1057');focus();adb('shell','input','text',MARKER);adb('shell','input','tap','499','662')
input_completed=time.monotonic()-start;samples=[];proof={}
for _ in range(12):
    if adb('shell','pidof',PKG).decode().strip()!='7093':raise RuntimeError('process changed')
    camera=adb('shell','dumpsys','media.camera').decode(errors='replace');m=re.search(r'Active Camera Clients:\s*(\[[\s\S]*?\])',camera);active=m.group(1).strip()if m else None
    if active and not active[1:-1].strip():active='[]'
    messages=get('/messages')
    for peer in messages.get('peers',[]):
        for msg in peer.get('messages',[]):
            if msg.get('sendID')=='upgrade38_u104'and msg.get('contentType')==101 and msg.get('text')==MARKER and msg.get('groupID')==GROUP:
                proof[peer['peer']]={key:msg.get(key)for key in ['clientMsgID','contentType','sendID','groupID','text']}
    samples.append({'elapsedSeconds':round(time.monotonic()-start,3),'activeCameraClients':active,'exactMessageReceivedByPeers':sorted(proof)})
    if active=='[]'and set(proof)=={'102','103'}:break
    time.sleep(2)
after_anr=adb('shell','dumpsys','activity','lastanr')
r={'startedAt':started,'finishedAt':datetime.datetime.now(datetime.timezone.utc).isoformat(),'PID':7093,'groupID':GROUP,'marker':MARKER,'hangupADBCompletedSeconds':round(hangup_completed,3),'inputADBCompletedSeconds':round(input_completed,3),'exactSDKMessageProof':proof,'samples':samples,'ownCameraReleased':samples[-1]['activeCameraClients']=='[]','lastANRBeforeSHA256':hashlib.sha256(before_anr).hexdigest(),'lastANRAfterSHA256':hashlib.sha256(after_anr).hexdigest(),'lastANRUnchanged':before_anr==after_anr,'WebPeersWereNotHungUpByThisTool':True,'toolSHA256':hashlib.sha256(Path(__file__).read_bytes()).hexdigest()}
with OUT.open('x',encoding='utf-8')as f:json.dump(r,f,indent=2);f.write('\n')
print(json.dumps(r))
