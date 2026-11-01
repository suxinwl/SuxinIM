from pathlib import Path
import json,hashlib,zipfile,datetime,re
R=Path('O:/');D=R/'tests/flutter';C=R/'.cache/gradle/caches/modules-2/files-2.1';meta=json.loads((D/'android-main-arm64-109.json').read_text());manifest=json.loads((D/'resolved-aar-hashes.json').read_text());want={Path(k).name:v for k,v in meta['nativeLibraries'].items()};matches={k:[] for k in want};aarChecks=[]
def sha(p):return hashlib.file_digest(p.open('rb'),'sha256').hexdigest()
byCoord={(x['coordinate'],x['filename']):x for x in manifest['resolvedCacheArtifacts']}
for aar in sorted(C.rglob('*.aar')):
 group,name,version=aar.relative_to(C).parts[:3];key=(f'{group}:{name}:{version}',aar.name);locked=byCoord[key];digest=sha(aar)
 if digest!=locked['sha256'] or aar.stat().st_size!=locked['size']:raise RuntimeError('AAR changed '+key[0])
 aarChecks.append({'coordinate':key[0],'filename':aar.name,'sha256':digest,'size':aar.stat().st_size})
 with zipfile.ZipFile(aar) as z:
  for n in z.namelist():
   if '/arm64-v8a/' not in n or not n.endswith('.so') or Path(n).name not in want:continue
   v=hashlib.sha256(z.read(n)).hexdigest()
   if v==want[Path(n).name]['sha256']:matches[Path(n).name].append({'container':aar.relative_to(R).as_posix(),'coordinate':key[0],'containerSHA256':digest,'entry':n,'entrySHA256':v,'entrySize':z.getinfo(n).file_size})
flutter=R/'tools/flutter-3.32.8/bin/cache/artifacts/engine/android-arm64-release/libflutter.so'
if flutter.is_file() and sha(flutter)==want['libflutter.so']['sha256']:matches['libflutter.so'].append({'container':flutter.relative_to(R).as_posix(),'entrySHA256':sha(flutter),'entrySize':flutter.stat().st_size})
for jar in (C/'io.flutter/arm64_v8a_release').rglob('*.jar'):
 with zipfile.ZipFile(jar) as z:
  for n in z.namelist():
   if n.endswith('/libflutter.so') and hashlib.sha256(z.read(n)).hexdigest()==want['libflutter.so']['sha256']:
    matches['libflutter.so'].append({'container':jar.relative_to(R).as_posix(),'containerSHA256':sha(jar),'entry':n,'entrySHA256':want['libflutter.so']['sha256'],'entrySize':z.getinfo(n).file_size})
for name in ['libgojni.so','libjingle_peerconnection_so.so']:
 if not matches[name]:raise RuntimeError('FixedAAR critical ARM payload does not match: '+name)
r={'at':datetime.datetime.now(datetime.timezone.utc).isoformat(),'apkSHA256':meta['sha256'],'applicationVersion':'3.8.9+109','sourceSHA256':meta['sourceSHA256'],'actualNativePayloadCount':len(want),'abi':'arm64-v8a','all165ResolvedAARBytesStillMatchPrebuildManifest':len(aarChecks)==165,'AARChecks':aarChecks,'payloads':[{**{'name':k,'apkSHA256':v['sha256'],'size':v['size']},'exactSourceMatches':matches[k],'originStatus':'generatedDartAOT, bound by frozen main buildcommand' if k=='libapp.so' else ('exact dependency bytes' if matches[k] else 'requires separate strip/engine transform trace')} for k,v in want.items()],'criticalOpenIMAndWebRTCExactMatches':True,'buildEntrypoints':{'development':'package:openim/main.dart','productionDependencyTask':'package:openim/main.dart'},'realPluginJavac109':'UP-TO-DATE (not re-executed); prior108 actualclass compile reused under unchangedJava source','published':False,'runtimeAccepted':False}
p=D/'android-main-arm64-109-payload-verification.json'
if p.exists():raise RuntimeError('Preserve payload evidence')
p.write_text(json.dumps(r,indent=2)+'\n',encoding='utf-8');print(json.dumps({'actualNativePayloads':len(want),'allAARs':len(aarChecks),'matched':{k:len(v) for k,v in matches.items()},'criticalExact':True}),flush=True)
