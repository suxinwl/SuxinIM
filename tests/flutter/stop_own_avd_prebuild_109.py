from pathlib import Path
import subprocess,datetime,json,hashlib,time,xml.etree.ElementTree as X
R=Path('O:/');D=R/'tests/flutter';A=['E:/AndroidDev/sdk/platform-tools/adb.exe','-s','emulator-5588'];P='io.openim.upgrade38.test.dev'
def adb(*x):return subprocess.check_output(A+list(x),timeout=25)
subprocess.run([str(R/'.venv-tools/Scripts/python.exe'),str(D/'snapshot_runtime_sources.py'),'--verify'],check=True)
subprocess.run([str(R/'.venv-tools/Scripts/python.exe'),str(D/'audit_path_dependency_coverage.py'),'--build','109','--check-only'],check=True)
if b'openim38_api30' not in adb('emu','avd','name') or adb('shell','getprop','ro.build.version.sdk').strip()!=b'30':raise RuntimeError('Only ownAPI30AVD')
pid=adb('shell','pidof',P).decode().strip();raw=adb('exec-out','cat',f'/data/user/0/{P}/shared_prefs/FlutterSharedPreferences.xml');pref={n.attrib['name']:n.text for n in X.fromstring(raw)};cert=json.loads(pref['flutter.loginCertificate']);assert cert['userID']=='upgrade38_u104';pair={k:cert[k] for k in ['userID','imToken','chatToken']};pair['platformID']=2
record={'at':datetime.datetime.now(datetime.timezone.utc).isoformat(),'action':'Graceful adb emu kill only this task openim38_api30/emulator5588 before isolated ARM64 build','avd':'openim38_api30','serial':'emulator-5588','apiLevel':30,'priorOwnAppPID':int(pid),'priorInstalledApplicationBuild':108,'userdataDeleted':False,'loggedOutOrRevoked':False,'cachedPairSHA256':hashlib.sha256(json.dumps(pair,sort_keys=True,separators=(',',':')).encode()).hexdigest(),'systemCASHA256BeforeStop':hashlib.sha256(adb('exec-out','cat','/system/etc/security/cacerts/1c34b89e.0')).hexdigest(),'frozen109SourceSHA256':json.loads((D/'runtime-source-snapshot-109.json').read_text())['sourceSHA256'],'productChangesDuringBuildAllowed':False}
out=adb('emu','kill').decode(errors='replace');record['gracefulEmulatorResponse']=out.strip()
for i in range(20):
 s=subprocess.run(A+['get-state'],capture_output=True,timeout=10)
 if s.returncode!=0:break
 time.sleep(.5)
record['serialNoLongerReady']=s.returncode!=0;(D/'android-109-prebuild-own-avd-stop.json').write_text(json.dumps(record,indent=2)+'\n',encoding='utf-8');print(json.dumps(record),flush=True)
