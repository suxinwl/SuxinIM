"""Record a private normal-main x64 candidate; never publish or overwrite artifacts."""
from pathlib import Path
import json,hashlib,zipfile,subprocess,shutil,os,re,datetime
R=Path(__file__).absolute().parents[2];C=R/'clients/flutter';D=R/'tests/flutter'
def sha(p):return hashlib.file_digest(p.open('rb'),'sha256').hexdigest()
version,build=re.search(r'^version:\s*([^+\s]+)\+(\d+)',(C/'pubspec.yaml').read_text(),re.M).groups()
if build!='110':raise RuntimeError('This evidence tool is narrowly for candidate110')
report=D/f'android-main-x64-{build}.json'
if report.exists():raise RuntimeError('Preserve signed candidate metadata')
subprocess.run([os.sys.executable,str(D/'snapshot_runtime_sources.py'),'--verify'],check=True)
subprocess.run([os.sys.executable,str(D/'audit_path_dependency_coverage.py'),'--build',build,'--check-only'],check=True)
source=json.loads((D/f'runtime-source-snapshot-{build}.json').read_text());apk=C/'build/app/outputs/flutter-apk/app-development-release.apk';log=D/f'android-main-x64-{build}-frozen-build.log';start=D/f'android-main-x64-{build}-build-start.json'
if not apk.is_file() or not log.is_file() or not start.is_file():raise RuntimeError('Actual candidate, buildlog and command metadata required')
command=json.loads(start.read_text());assert command['entrypoint']=='lib/main.dart' and command['abi']=='x86_64' and command['sourceSHA256']==source['sourceSHA256']
env=os.environ.copy();env['JAVA_HOME']='E:/AndroidDev/jdk/jdk-17.0.20.1+1'
output=subprocess.check_output(['E:/AndroidDev/sdk/build-tools/34.0.0/apksigner.bat','verify','--verbose','--print-certs',str(apk)],env=env,stderr=subprocess.STDOUT).decode(errors='replace');assert 'Verified using v2 scheme (APK Signature Scheme v2): true' in output
cert=re.search(r'Signer #1 certificate SHA-256 digest:\s*([a-fA-F0-9]+)',output)[1].lower();assert cert=='028253da3ebcb424a5e2e7ae73dddc88fc508a2ce3ccccedd1193dd4f65de8c1'
badging=subprocess.check_output(['E:/AndroidDev/sdk/build-tools/34.0.0/aapt.exe','dump','badging',str(apk)]).decode(errors='replace');assert re.search(r"package: name='io.openim.upgrade38.test.dev'.*versionCode='"+build+r"'.*versionName='"+re.escape(version)+r"'",badging);assert "sdkVersion:'24'" in badging and "targetSdkVersion:'35'" in badging
with zipfile.ZipFile(apk) as archive:
 libs={n:{'size':archive.getinfo(n).file_size,'sha256':hashlib.sha256(archive.read(n)).hexdigest()} for n in sorted(archive.namelist()) if n.startswith('lib/') and n.endswith('.so')}
 dexBytes=b''.join(archive.read(n) for n in archive.namelist() if re.fullmatch(r'classes\d*\.dex',n))
 observationMarkers={marker.decode():marker in dexBytes for marker in [b'javaRemoveSinkBegin',b'javaRemoveSinkEnd',b'leaseAssociation',b'camera.waitForClosed',b'substepBegin',b'wrapperJava#']}
 assert all(observationMarkers.values()),'Selected release APK is missing new native observation bytecode markers'
assert {n.split('/')[1] for n in libs}=={'x86_64'}
priorLibraries=json.loads((D/'android-main-x64-109.json').read_text(encoding='utf-8'))['nativeLibraries']
assert set(libs)==set(priorLibraries) and len(libs)==11
assert all(libs[name]==priorLibraries[name] for name in libs if not name.endswith('/libapp.so'))
digest=sha(apk);saved=R/f'.private/android-validation/OpenIM38-{version}+{build}-main-x64-candidate.apk'
if saved.exists():raise RuntimeError('Do not overwrite a signed candidate')
shutil.copyfile(apk,saved);assert sha(saved)==digest
maps=R/f'.private/android-build-maps/{build}-x64-{digest[:8]}';maps.mkdir(parents=True,exist_ok=False);copied=[]
prior=json.loads((R/'.private/android-build-maps/108-x64-34f0fb2b/manifest.json').read_text())
for item in prior['files']:
 p=R/item['source']
 if p.is_file():
  dest=maps/Path(item['savedAt']).name;shutil.copyfile(p,dest);copied.append({'source':item['source'],'savedAt':dest.relative_to(R).as_posix(),'sha256':sha(dest),'size':dest.stat().st_size})
(maps/'manifest.json').write_text(json.dumps({'apkSHA256':digest,'files':copied},indent=2)+'\n',encoding='utf-8')
r={'recordedAt':datetime.datetime.now(datetime.timezone.utc).isoformat(),'applicationVersion':version,'applicationBuild':int(build),'applicationID':'io.openim.upgrade38.test.dev','artifact':saved.relative_to(R).as_posix(),'sha256':digest,'size':saved.stat().st_size,'abis':['x86_64'],'minSDK':24,'targetSDK':35,'signatureVerified':True,'signatureScheme':'v2','signingCertificateSHA256':cert,'sourceSHA256':source['sourceSHA256'],'sourceFiles':source['fileCount'],'sourceManifest':f'tests/flutter/runtime-source-snapshot-{build}.json','sourceInputsChangedByBuild':0,'entrypoint':'lib/main.dart','entrypointEvidence':start.relative_to(R).as_posix(),'privateCandidate':True,'published':False,'formalAcceptedRelease':False,'runtimeAccepted':False,'physicalARM64Device':False,'plannedAVDBoundary':'API30 native x86_64 dedicatedAVD; this is a private x64 validation artifact, not the ARM64 deliverable.','developmentNativeChannelBoundary':'Development MainActivity retains the runtimeIntent-only native probe channel; normal lib/main.dart does not import/call the validationDart wrapper, and no synthetic credentials are embedded.','trustBoundary':'FixedGoAAR requires dedicatedAVD systemCA for this private endpoint; ordinary phone trust remains separate.','buildLog':log.relative_to(R).as_posix(),'buildLogSHA256':sha(log),'nativeLibraries':libs,'privateMappingArchive':(maps/'manifest.json').relative_to(R).as_posix()}
r['newNativeObservationMarkersPresentInDEX']=observationMarkers
r['nativeDependenciesUnchangedFrom109']=10
report.write_text(json.dumps(r,indent=2)+'\n',encoding='utf-8')
p=D/'build-provenance.json';provenance=json.loads(p.read_text());provenance['android']['currentCandidateBuilt']=True;provenance['verification']['privateNormalMainX64Candidate']=r;provenance['verificationStatus']='110 signed private x64 normal-main candidate built; complete frozen-source postverification passed. Actual x64 originalUI acceptance pending; historical ARM109 translated startup failed, no publication.';p.write_text(json.dumps(provenance,indent=2)+'\n',encoding='utf-8')
print(json.dumps({'version':version+'+'+build,'abi':'x86_64','sha256':digest,'bytes':saved.stat().st_size,'sourceInputsChanged':0,'signatureVerified':True,'runtimeAccepted':False}))
