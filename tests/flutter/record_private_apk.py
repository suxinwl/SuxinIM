"""Record a separate, signed x86_64 validation APK after a frozen release build."""
from pathlib import Path
import hashlib,json,os,re,shutil,subprocess,sys,zipfile
ROOT=Path(__file__).absolute().parents[2];CLIENT=ROOT/'clients/flutter'
def sha(p):return hashlib.file_digest(p.open('rb'),'sha256').hexdigest()
version,build=re.search(r'^version:\s*([^+\s]+)\+(\d+)',(CLIENT/'pubspec.yaml').read_text(),re.M).groups()
reportPath=ROOT/f'tests/flutter/android-validation-x64-{build}.json'
if reportPath.exists():raise RuntimeError('Preserve existing artifact metadata')
subprocess.run([sys.executable,str(ROOT/'tests/flutter/snapshot_runtime_sources.py'),'--verify'],check=True)
source=json.loads((ROOT/f'tests/flutter/runtime-source-snapshot-{build}.json').read_text())
apk=CLIENT/'build/app/outputs/flutter-apk/app-development-release.apk'
assert apk.is_file()
env=os.environ.copy();env['JAVA_HOME']='E:/AndroidDev/jdk/jdk-17.0.20.1+1'
check=subprocess.run(['E:/AndroidDev/sdk/build-tools/34.0.0/apksigner.bat','verify','--verbose','--print-certs',str(apk)],env=env,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,check=True).stdout.decode(errors='replace')
assert 'Verified using v2 scheme (APK Signature Scheme v2): true' in check
certificate=re.search(r'Signer #1 certificate SHA-256 digest:\s*([a-fA-F0-9]+)',check)[1].lower()
assert certificate=='028253da3ebcb424a5e2e7ae73dddc88fc508a2ce3ccccedd1193dd4f65de8c1'
badging=subprocess.check_output(['E:/AndroidDev/sdk/build-tools/34.0.0/aapt.exe','dump','badging',str(apk)]).decode(errors='replace')
assert re.search(r"package: name='io.openim.upgrade38.test.dev'.*versionCode='"+build+r"'.*versionName='"+re.escape(version)+r"'",badging)
assert "sdkVersion:'24'" in badging and "targetSdkVersion:'35'" in badging
with zipfile.ZipFile(apk) as z:
 libraries={n:{'size':z.getinfo(n).file_size,'sha256':hashlib.sha256(z.read(n)).hexdigest()} for n in sorted(z.namelist()) if n.startswith('lib/') and n.endswith('.so')}
assert {n.split('/')[1] for n in libraries}=={'x86_64'}
digest=sha(apk)
firstCoverage=ROOT/f'tests/flutter/android-validation-x64-{build}-first-coverage.json'
if firstCoverage.is_file() and digest!=json.loads(firstCoverage.read_text())['sha256']:
 raise RuntimeError('Confirmation APK differs from preserved first build; discuss before freezing a new artifact')
saved=ROOT/f'.private/android-validation/OpenIM38-{version}+{build}-validation-x86_64-frozen.apk'
if saved.exists():raise RuntimeError('Never overwrite a signed frozen artifact')
saved.parent.mkdir(parents=True,exist_ok=True);shutil.copyfile(apk,saved);assert sha(saved)==digest
maps=ROOT/f'.private/android-build-maps/{build}-x64-{digest[:8]}';maps.mkdir(parents=True,exist_ok=False);copied=[]
prior=json.loads((ROOT/'tests/flutter/android-validation-x64-107.json').read_text())
priorMaps=json.loads((ROOT/prior['privateMappingArchive']).read_text())
for item in priorMaps['files']:
 original=ROOT/item['source']
 if not original.is_file():continue
 target=maps/Path(item['savedAt']).name;shutil.copyfile(original,target)
 copied.append({'source':item['source'],'savedAt':target.relative_to(ROOT).as_posix(),'sha256':sha(target),'size':target.stat().st_size})
(maps/'manifest.json').write_text(json.dumps({'apkSHA256':digest,'files':copied},indent=2)+'\n')
log=ROOT/f'tests/flutter/android-validation-x64-{build}-coverage-confirmation-build.log'
if not log.is_file():log=ROOT/f'tests/flutter/android-validation-x64-{build}-frozen-build.log'
record={'applicationVersion':version,'applicationBuild':int(build),'applicationID':'io.openim.upgrade38.test.dev','artifact':saved.relative_to(ROOT).as_posix(),'sha256':digest,'size':saved.stat().st_size,'abis':['x86_64'],'minSDK':24,'targetSDK':35,'signatureVerified':True,'signatureScheme':'v2','signingCertificateSHA256':certificate,'sourceSHA256':source['sourceSHA256'],'sourceFiles':source['fileCount'],'sourceManifest':f'tests/flutter/runtime-source-snapshot-{build}.json','sourceReview':f'tests/delivery/android-{build}-path-source-coverage-resolution-review.json','sourceInputsChangedByBuild':0,'entrypoint':'tool/validation_main.dart (normal main UI unless synthetic probe Intent present)','physicalARM64Device':False,'formalARM64Artifact':False,'runtimeAccepted':False,'trustBoundary':'Dedicated rooted API30 AVD system CA required for pinned Go AAR; bundled application CA alone is insufficient','buildLog':log.relative_to(ROOT).as_posix(),'buildLogSHA256':sha(log),'nativeLibraries':libraries,'privateMappingArchive':(maps/'manifest.json').relative_to(ROOT).as_posix()}
if firstCoverage.is_file():
 record['firstCoveragePrivateBuild']=firstCoverage.relative_to(ROOT).as_posix()
 record['confirmationAPKMatchesFirstBuild']=True
 record['coverageConfirmationPre']=f'tests/flutter/coverage-confirmation-pre-{build}.json'
 record['coverageConfirmationPost']=f'tests/flutter/coverage-confirmation-post-{build}.json'
reportPath.write_text(json.dumps(record,indent=2)+'\n')
provenancePath=ROOT/'tests/flutter/build-provenance.json';provenance=json.loads(provenancePath.read_text());provenance['verification']['privateValidationAPK']=record;provenance['verificationStatus']=f'{build} frozen privatex64 APK signed and source postverify passed; actual runtime acceptance pending';provenancePath.write_text(json.dumps(provenance,indent=2)+'\n')
print(json.dumps({'application':version+'+'+build,'sha256':digest,'bytes':saved.stat().st_size,'sourceInputsChanged':0,'signatureVerified':True}))
