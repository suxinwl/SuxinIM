from pathlib import Path
import subprocess,json,xml.etree.ElementTree as X,hashlib,datetime,base64,time
R=Path('O:/');D=R/'tests/flutter';A=['E:/AndroidDev/sdk/platform-tools/adb.exe','-s','emulator-5588'];P='io.openim.upgrade38.test.dev';label='android-108-cold-video-diagnostic'
def adb(*x):return subprocess.check_output(A+list(x),timeout=35)
if b'openim38_api30' not in adb('emu','avd','name'):raise RuntimeError('Own AVD only')
beforePID=adb('shell','pidof',P).decode().strip()
if beforePID!='4997':raise RuntimeError('Keep pre-cold process evidence exact')
raw=adb('exec-out','cat',f'/data/user/0/{P}/shared_prefs/FlutterSharedPreferences.xml');prefs={n.attrib['name']:n.text for n in X.fromstring(raw)};cert=json.loads(prefs['flutter.loginCertificate'])
if cert['userID']!='upgrade38_u104':raise RuntimeError('Synthetic104 only')
pair={k:cert[k] for k in ('userID','imToken','chatToken')};pair['platformID']=2
pairSHA=hashlib.sha256(json.dumps(pair,sort_keys=True,separators=(',',':')).encode()).hexdigest()
private=R/'.private/android-ui-session-108-precold.json'
if private.exists():raise RuntimeError('Preserve prior private snapshot')
private.write_text(json.dumps(cert),encoding='utf-8')
expiry={}
for name in ['imToken','chatToken']:
 try:
  payload=json.loads(base64.urlsafe_b64decode(cert[name].split('.')[1]+'==='));expiry[name]=payload.get('exp')
 except Exception:expiry[name]=None
ca=adb('exec-out','cat','/system/etc/security/cacerts/1c34b89e.0');caSHA=hashlib.sha256(ca).hexdigest()
record={'startedAt':datetime.datetime.now(datetime.timezone.utc).isoformat(),'beforePID':int(beforePID),'sameAPK':True,'dataCleared':False,'loggedOutOrRevoked':False,'pairSHA256':pairSHA,'tokenExpiryUnixOnly':expiry,'systemCASHA256Before':caSHA,'newSourceChangesIncluded':False,'diagnosticOnly':True,'normalAcceptance':False}
adb('shell','am','force-stop',P)
if adb('shell','pidof',P).strip():raise RuntimeError('Own package did not stop')
adb('logcat','-c')
adb('shell','am','start','-n',P+'/cn.rentsoft.flutter.openim.consumer.MainActivity','--ez','enable-impeller','false')
time.sleep(10)
newpid=adb('shell','pidof',P).decode().strip()
if not newpid.isdigit() or newpid==beforePID:raise RuntimeError('Expected fresh own process')
raw2=adb('exec-out','cat',f'/data/user/0/{P}/shared_prefs/FlutterSharedPreferences.xml');after={n.attrib['name']:n.text for n in X.fromstring(raw2)};cert2=json.loads(after['flutter.loginCertificate'])
pair2={k:cert2[k] for k in ('userID','imToken','chatToken')};pair2['platformID']=2
record.update({'newPID':int(newpid),'pairUnchanged':pairSHA==hashlib.sha256(json.dumps(pair2,sort_keys=True,separators=(',',':')).encode()).hexdigest(),'systemCASHA256After':hashlib.sha256(adb('exec-out','cat','/system/etc/security/cacerts/1c34b89e.0')).hexdigest(),'launchArgs':['enable-impeller=false']})
(D/f'{label}-start.json').write_text(json.dumps(record,indent=2)+'\n')
(D/f'{label}-home.png').write_bytes(adb('exec-out','screencap','-p'));print(json.dumps(record),flush=True)
