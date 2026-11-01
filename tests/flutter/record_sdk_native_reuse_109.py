from pathlib import Path
import json,hashlib,datetime,subprocess
R=Path('O:/');D=R/'tests/flutter'
def sha(p):return hashlib.file_digest(p.open('rb'),'sha256').hexdigest()
now=datetime.datetime.now(datetime.timezone.utc).isoformat()
old=json.loads((D/'sdk-source-snapshot-108.json').read_text());sdkRoot=R/'sdks/flutter'
assert subprocess.check_output(['git','-C',str(sdkRoot),'rev-parse','HEAD'],text=True).strip()==old['commit']
status=subprocess.check_output(['git','-C',str(sdkRoot),'status','--porcelain','--untracked-files=no'],text=True)
if status.strip():raise RuntimeError('Pinned SDK tracked bytes changed')
for name,expected in old['files'].items():
 if sha(R/name)!=expected:raise RuntimeError('SDK source changed: '+name)
old.update({'verifiedAt':now,'applicationBinding':'3.8.9+109','reuseOf':'tests/flutter/sdk-source-snapshot-108.json','all194NamedFilesVerifiedUnchanged':True})
p=D/'sdk-source-snapshot-109.json'
if p.exists():raise RuntimeError('Keep previous109 SDK snapshot')
p.write_text(json.dumps(old,indent=2)+'\n',encoding='utf-8')
nativeReport=D/'native-deadline-review-108.json';native=json.loads(nativeReport.read_text());check=D/'native-source-check-108.json';lock=json.loads(check.read_text());assert lock['before']==lock['after'] and lock['identical']
checks=[]
for item in lock['after']:
 path=R/item['path'];actual=sha(path)
 if actual!=item['sha256'] or path.stat().st_size!=item['bytes']:raise RuntimeError('Native input changed: '+item['path'])
 checks.append({**item,'sameAs108':True})
logs=[]
for kind in ['standardJavaSuite','androidProductionJavac']:
 item=native['actuallyExecuted'][kind]['log'];path=R/item['path'];assert sha(path)==item['sha256'] and path.stat().st_size==item['bytes'];logs.append(item)
reuse={'applicationVersion':'3.8.9+109','generatedAt':now,'status':'native_source_and_test_inputs_identical_reusing108_boundary_results','sourceReport':'tests/flutter/native-deadline-review-108.json','sourceReportSHA256':sha(nativeReport),'sourceCheck':native['sourceCheck'],'boundaryCases':88,'rerunPerformed':False,'scope':'Exact95 productionJava, test/stub and formal-script inputs checked against final108 pre/post lock; unchanged passing88-case boundary and actualAndroid-classpathJavac logs retained. Not109 device or ABI runtime acceptance.','allInputsUnchanged':True,'inputCount':len(checks),'checks':checks,'reusedSuccessfulLogs':logs,'runtimeAccepted':False}
p=D/'native-verification-reuse-109.json'
if p.exists():raise RuntimeError('Preserve native reuse record')
p.write_text(json.dumps(reuse,indent=2)+'\n',encoding='utf-8');print(json.dumps({'sdkFiles':len(old['files']),'nativeInputs':len(checks),'nativeCasesReused':88,'nativeRerun':False}))
