"""Preserve the first108 build honestly before corrected-scope confirmation."""
from pathlib import Path
import hashlib,json,shutil,datetime
root=Path(__file__).absolute().parents[2];base=root/'tests/flutter'
def sha(p):return hashlib.file_digest(p.open('rb'),'sha256').hexdigest()
manifest=json.loads((base/'runtime-source-snapshot-108.json').read_text());assert manifest['fileCount']==3224 and manifest['sourceSHA256'].startswith('f56c3cb1')
changed=[x for x,d in manifest['files'].items() if sha(root/x)!=d];assert not changed,changed
apk=root/'clients/flutter/build/app/outputs/flutter-apk/app-development-release.apk';digest=sha(apk);dest=root/'.private/android-validation/OpenIM38-3.8.8+108-validation-x86_64-first-coverage-partial.apk';assert not dest.exists();shutil.copyfile(apk,dest);assert sha(dest)==digest
compat=json.loads((root/'clients/flutter/local_plugin/compat/PATCH-MANIFEST.json').read_text());locked={'clients/flutter/local_plugin/compat/'+p['package']+'/'+f['path']:f['sha256'] for p in compat['packages'] for f in p['modifiedFiles']}
review=json.loads((root/'tests/delivery/android-108-path-source-coverage-review.json').read_text());omitted=[]
def find(value):
 if isinstance(value,dict):
  if 'path' in value and '/com/example/' in value['path'] and 'sha256' in value: omitted.append(value)
  for v in value.values():find(v)
 elif isinstance(value,list):
  for v in value:find(v)
find(review);omitted={x['path']:x for x in omitted};assert len(omitted)==5
bindings=[]
for name,item in sorted(omitted.items()):
 current=sha(root/name);assert current==item['sha256']
 bindings.append({'path':name,'sha256':current,'explicitIndividualHashBoundBeforeBuild':locked.get(name)==current,'priorBinding':'Archived prebuild PATCH-MANIFEST124/root hash review' if locked.get(name)==current else 'No confirmed explicit individual prebuild hash; first individually locked during this build'})
log=base/'android-validation-x64-108-frozen-build.log'
r={'applicationVersion':'3.8.8+108','artifact':dest.relative_to(root).as_posix(),'sha256':digest,'size':dest.stat().st_size,'recordedAtUTC':datetime.datetime.now(datetime.timezone.utc).isoformat(),'buildLog':'tests/flutter/android-validation-x64-108-frozen-build.log','buildLogSHA256':sha(log),'declared3224InputsPostChecked':len(manifest['files']),'declared3224InputChanges':changed,'originalSourceSHA256':manifest['sourceSHA256'],'scopeCorrectionInputs':bindings,'allEffectiveInputsPreFrozenClaimed':False,'runtimeAccepted':False,'status':'First private build preserved, incomplete prebuild source coverage; corrected-scope pre-build-post incremental confirmation still required'}
out=base/'android-validation-x64-108-first-coverage.json';assert not out.exists();out.write_text(json.dumps(r,indent=2)+'\n');print(json.dumps({'firstAPK':digest,'bytes':dest.stat().st_size,'declaredInputsChanged':len(changed),'onlyBuildTimeIndividualLocks':sum(not x['explicitIndividualHashBoundBeforeBuild'] for x in bindings)}))
