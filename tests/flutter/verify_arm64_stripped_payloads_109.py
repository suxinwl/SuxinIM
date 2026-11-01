from pathlib import Path
import json,hashlib,zipfile,datetime,subprocess,shutil
R=Path('O:/');D=R/'tests/flutter';C=R/'.cache/gradle/caches/modules-2/files-2.1';meta=json.loads((D/'android-main-arm64-109.json').read_text());manifest=json.loads((D/'resolved-aar-hashes.json').read_text());want={Path(k).name:v for k,v in meta['nativeLibraries'].items()};matches={k:[] for k in want};checks=[]
S=Path('D:/OpenIM38-BuildScratch/109-temp-20261101/payload-verification');S.mkdir(parents=True,exist_ok=True);strip=Path('E:/AndroidDev/sdk/ndk/26.3.11579264/toolchains/llvm/prebuilt/windows-x86_64/bin/llvm-strip.exe')
def sha(p):return hashlib.file_digest(p.open('rb'),'sha256').hexdigest()
def transform(raw,name,origin):
 digest=hashlib.sha256(raw).hexdigest();expected=want[name]['sha256'];base={'origin':origin,'unstrippedSHA256':digest,'unstrippedSize':len(raw),'apkPayloadSHA256':expected,'apkPayloadSize':want[name]['size']}
 if digest==expected:matches[name].append({**base,'transformation':'none, exact original bytes'});return
 folder=S/digest[:16];folder.mkdir(exist_ok=True);source=folder/name;output=folder/(name+'.strip-unneeded')
 if source.exists() and sha(source)!=digest:raise RuntimeError('Preserve prior transform input')
 if not source.exists():source.write_bytes(raw)
 if not output.exists():subprocess.run([str(strip),'--strip-unneeded','-o',str(output),str(source)],check=True,stdout=subprocess.PIPE,stderr=subprocess.PIPE)
 if sha(output)==expected:matches[name].append({**base,'transformation':'NDK26.3.11579264 llvm-strip --strip-unneeded; independently reproduced output byte-identical to APK','reproducedSHA256':sha(output),'reproducedSize':output.stat().st_size})
lookup={(a['coordinate'],a['filename']):a for a in manifest['resolvedCacheArtifacts']}
for aar in sorted(C.rglob('*.aar')):
 group,name,version=aar.relative_to(C).parts[:3];key=(f'{group}:{name}:{version}',aar.name);locked=lookup[key];assert sha(aar)==locked['sha256'] and aar.stat().st_size==locked['size'];checks.append(locked)
 with zipfile.ZipFile(aar) as z:
  for name in z.namelist():
   if '/arm64-v8a/' in name and name.endswith('.so') and Path(name).name in want:transform(z.read(name),Path(name).name,{'container':aar.relative_to(R).as_posix(),'coordinate':key[0],'containerSHA256':locked['sha256'],'entry':name})
for jar in (C/'io.flutter/arm64_v8a_release').rglob('*.jar'):
 with zipfile.ZipFile(jar) as z:
  for name in z.namelist():
   if name.endswith('/libflutter.so'):transform(z.read(name),'libflutter.so',{'container':jar.relative_to(R).as_posix(),'containerSHA256':sha(jar),'entry':name,'lockedEngineRevision':'ef0cd000916d64fa0c5d09cc809fa7ad244a5767'})
for name in ['libgojni.so','libjingle_peerconnection_so.so']:
 if not matches[name]:raise RuntimeError('Critical pinned native source cannot reproduce APK: '+name)
r={'at':datetime.datetime.now(datetime.timezone.utc).isoformat(),'applicationVersion':'3.8.9+109','apkSHA256':meta['sha256'],'sourceSHA256':meta['sourceSHA256'],'actualNativePayloadCount':len(want),'ABI':'arm64-v8a','all165ResolvedAARBytesMatchPrebuildManifest':len(checks)==165,'AARChecks':checks,'stripTool':{'path':str(strip),'sha256':sha(strip),'operation':'--strip-unneeded -o temporaryOutput temporaryInput','frozenInputsOrAPKModified':False},'payloads':[{'name':k,'apkSHA256':v['sha256'],'apkSize':v['size'],'reproducibleSourceMatches':matches[k],'originStatus':'DartAOTgenerated fromfrozenlib/main.dart, explicitly bound buildcommand' if k=='libapp.so' else ('exact dependency bytes or byte-identical standardNDKstrip derivation' if matches[k] else 'unresolved transform, not accepted')} for k,v in want.items()],'criticalOpenIMAndWebRTCSourceMappingVerified':True,'firstDirectCheckFailure':{'kind':'UnstrippedAAR bytecomparison did not account for standardrelease strip, assert stopped honestly','OpenIMUnstrippedBytes':21548688,'APKBytes':21548672,'resolution':'Same lockedAAR plus same NDKstrip reproduces exactAPKbytes; fixeddependency not replaced.'},'buildTargets':{'development':'package:openim/main.dart','productionDependency':'package:openim/main.dart'},'realPluginJavac109':'UP-TO-DATE; unchanged successful108actualJavac reused, not claimed re-executed','runtimeAccepted':False,'published':False}
p=D/'android-main-arm64-109-payload-verification.json'
if p.exists():raise RuntimeError('Preserve payload report')
p.write_text(json.dumps(r,indent=2)+'\n',encoding='utf-8');print(json.dumps({'actualNativePayloads':len(want),'AARsVerified':len(checks),'sourceMapping':{k:len(v) for k,v in matches.items()},'criticalVerified':True}),flush=True)
