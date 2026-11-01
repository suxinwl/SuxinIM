from pathlib import Path
import subprocess,json,xml.etree.ElementTree as X,hashlib,datetime,base64,time
R=Path('O:/');D=R/'tests/flutter';A=['E:/AndroidDev/sdk/platform-tools/adb.exe','-s','emulator-5588'];P='io.openim.upgrade38.test.dev';label='android-108-cold-video-diagnostic'
def adb(*x):return subprocess.check_output(A+list(x),timeout=35)
stopped=subprocess.run(A+['shell','pidof',P],capture_output=True,timeout=20)
if stopped.returncode!=1 or stopped.stdout.strip():raise RuntimeError('Expected own force-stopped package')
private=R/'.private/android-ui-session-108-precold.json';cert=json.loads(private.read_text())
pair={k:cert[k] for k in ('userID','imToken','chatToken')};pair['platformID']=2
pairSHA=hashlib.sha256(json.dumps(pair,sort_keys=True,separators=(',',':')).encode()).hexdigest()
caSHA=hashlib.sha256(adb('exec-out','cat','/system/etc/security/cacerts/1c34b89e.0')).hexdigest()
expiry={}
for name in ['imToken','chatToken']:
 try:expiry[name]=json.loads(base64.urlsafe_b64decode(cert[name].split('.')[1]+'===')).get('exp')
 except Exception:expiry[name]=None
record={'startedAt':datetime.datetime.now(datetime.timezone.utc).isoformat(),'beforePID':4997,'sameAPK':True,'dataCleared':False,'loggedOutOrRevoked':False,'pairSHA256':pairSHA,'tokenExpiryUnixOnly':expiry,'systemCASHA256BeforeRelaunch':caSHA,'newSourceChangesIncluded':False,'diagnosticOnly':True,'normalAcceptance':False,'harnessNote':'First force-stop succeeded; initial Python pidof check wrongly treated expected exit1 as exception. This continuation verifies package still stopped and resumes exact original APK without another force-stop or credential mutation.'}
adb('logcat','-c');adb('shell','am','start','-n',P+'/cn.rentsoft.flutter.openim.consumer.MainActivity','--ez','enable-impeller','false');time.sleep(10)
newpid=adb('shell','pidof',P).decode().strip()
if not newpid.isdigit() or newpid=='4997':raise RuntimeError('Expected fresh own process')
raw2=adb('exec-out','cat',f'/data/user/0/{P}/shared_prefs/FlutterSharedPreferences.xml');after={n.attrib['name']:n.text for n in X.fromstring(raw2)};cert2=json.loads(after['flutter.loginCertificate']);pair2={k:cert2[k] for k in ('userID','imToken','chatToken')};pair2['platformID']=2
record.update({'newPID':int(newpid),'pairUnchanged':pairSHA==hashlib.sha256(json.dumps(pair2,sort_keys=True,separators=(',',':')).encode()).hexdigest(),'systemCASHA256After':hashlib.sha256(adb('exec-out','cat','/system/etc/security/cacerts/1c34b89e.0')).hexdigest(),'launchArgs':['enable-impeller=false']})
(D/f'{label}-start.json').write_text(json.dumps(record,indent=2)+'\n');(D/f'{label}-home.png').write_bytes(adb('exec-out','screencap','-p'));print(json.dumps(record),flush=True)
