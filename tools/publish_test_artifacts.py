"""Verify full HTTPS downloads, then optionally publish isolated Chat releases."""
import argparse,datetime,hashlib,json,pathlib,re,secrets,ssl,urllib.request
ROOT=pathlib.Path(__file__).resolve().parents[1]
parser=argparse.ArgumentParser();parser.add_argument('--publish',action='store_true');parser.add_argument('--platform',choices=['all','windows','android'],default='all');args=parser.parse_args()
base='https://192.168.10.11:29880'
tls=ssl.create_default_context(cafile=str(ROOT/'artifacts/dev-cert/OpenIM38-Development-CA.crt'))
http=urllib.request.build_opener(urllib.request.ProxyHandler({}),urllib.request.HTTPSHandler(context=tls))
def call(path,body,token=''):
 req=urllib.request.Request(base+path,json.dumps(body).encode(),{'Content-Type':'application/json','operationID':secrets.token_hex(12),'token':token,'chatToken':token})
 with http.open(req,timeout=30) as response:value=json.load(response)
 if value.get('errCode')!=0:raise RuntimeError(path+': '+str(value.get('errCode')))
 return value.get('data') or {}
windows=json.loads((ROOT/'clients/web-desktop/build-version.json').read_text(encoding='utf-8'))
android=re.search(r'^version:\s*(\d+\.\d+\.\d+)\+(\d+)\s*$',(ROOT/'clients/flutter/pubspec.yaml').read_text(encoding='utf-8'),re.M)
if not android:raise RuntimeError('Android application version/build is missing')
wv,wb=windows['version'],windows['build'];av,ab=android.group(1),int(android.group(2))
entries=[('windows',wv,f'clients/web-desktop/release/OpenIM38-Enterprise-{wv}-win-x64.exe',f'OpenIM38-Enterprise-{wv}-win-x64.exe',wb),('android',av,f'artifacts/android/OpenIM38-{av}+{ab}-development-arm64.apk',f'OpenIM38-{av}-{ab}-development-arm64.apk',ab)]
artifacts=[]
for platform,version,local,remote,build in entries:
 if args.platform!='all' and platform!=args.platform:continue
 path=ROOT/local
 with path.open('rb') as stream:digest=hashlib.file_digest(stream,'sha256').hexdigest()
 url=base+'/downloads/'+remote;downloaded=hashlib.sha256();size=0
 with http.open(url,timeout=60) as response:
  while chunk:=response.read(1024*1024):downloaded.update(chunk);size+=len(chunk)
 if size!=path.stat().st_size or downloaded.hexdigest()!=digest:raise RuntimeError('HTTPS download integrity failed: '+platform)
 artifacts.append({'platform':platform,'version':version,'build':build,'url':url,'sha256':digest,'size':size,'text':'OpenIM38 isolated development test build; synthetic data only.','force':False})
if args.publish:
 state=json.loads((ROOT/'.private/test-accounts.json').read_text(encoding='utf-8'))
 admin=call('/enterprise/v1/admin/login',{'account':state['admin']['account'],'password':hashlib.md5(state['admin']['password'].encode()).hexdigest()})
 for artifact in artifacts:
  call('/enterprise/v1/application/publish',artifact,admin['adminToken'])
  older=call('/enterprise/v1/application/latest',{'platform':artifact['platform'],'version':'3.8.2','build':0})
  current=call('/enterprise/v1/application/latest',{'platform':artifact['platform'],'version':artifact['version'],'build':artifact['build']})
  if not older.get('available') or older.get('sha256')!=artifact['sha256'] or current.get('available'):raise RuntimeError('Published release check failed')
report={'utc':datetime.datetime.now(datetime.timezone.utc).isoformat(),'fullHTTPSDownloadsVerified':True,'published':args.publish,'artifacts':artifacts,'limits':['Windows package is not code signed.','Android uses the project signing key; its Go SDK requires a publicly trusted server certificate or validated system trust deployment. Bundled development CA alone is insufficient.','Publishing and download verification do not by themselves prove an in-place version upgrade.']}
(ROOT/'tests/release-downloads.json').write_text(json.dumps(report,indent=2)+'\n',encoding='utf-8')
print(json.dumps({'verifiedFullDownloads':len(artifacts),'published':args.publish}))
