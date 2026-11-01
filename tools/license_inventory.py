"""Collect declared licenses and preserved notices, without changing dependencies."""
import hashlib,importlib.metadata,json,os,pathlib,re,urllib.parse,urllib.request
import yaml
ROOT=pathlib.Path(__file__).resolve().parents[1]
def notices(folder):
 values=[]
 for path in folder.glob('*'):
  if path.is_file() and path.name.lower().startswith(('license','notice','copying')) and path.stat().st_size<4000000:
   with path.open('rb') as f:digest=hashlib.file_digest(f,'sha256').hexdigest()
   values.append({'file':path.name,'sha256':digest})
 return values
records={}
def npm_folders(directory):
 if not directory.is_dir():return
 for path in directory.iterdir():
  if not path.is_dir() or path.name.startswith('.'):continue
  if path.name.startswith('@'):
   for scoped in path.iterdir():
    if scoped.is_dir():yield scoped;yield from npm_folders(scoped/'node_modules')
  else:yield path;yield from npm_folders(path/'node_modules')
for component in ['web-desktop','admin']:
 directory=ROOT/'clients'/component/'node_modules'
 for folder in npm_folders(directory):
  if not (folder/'package.json').is_file():continue
  try:value=json.loads((pathlib.Path(folder)/'package.json').read_text(encoding='utf-8'))
  except (ValueError,UnicodeError):continue
  if not value.get('name') or not value.get('version'):continue
  key='npm:'+value['name']+'@'+value['version']
  entry=records.setdefault(key,{'ecosystem':'npm','name':value['name'],'version':value['version'],'declaredLicense':value.get('license',value.get('licenses')),'notices':notices(pathlib.Path(folder)),'components':[]})
  if component not in entry['components']:entry['components'].append(component)
for folder,dirs,files in os.walk(ROOT/'.cache/go-mod'):
 dirs[:]=[name for name in dirs if name not in {'cache','.git'}]
 path=pathlib.Path(folder)
 if '@' not in path.name:continue
 name,version=path.relative_to(ROOT/'.cache/go-mod').as_posix().rsplit('@',1)
 dirs[:]=[]
 records['go:'+name+'@'+version]={'ecosystem':'go cached source','name':name,'version':version,'notices':notices(path)}
for path in (ROOT/'clients/flutter/local_plugin/compat').glob('*'):
 if path.is_dir():records['flutter:'+path.name]={'ecosystem':'Flutter vendored compatibility source','name':path.name,'notices':notices(path)}
config=ROOT/'clients/flutter/.dart_tool/package_config.json'
if config.is_file():
 for package in json.loads(config.read_text(encoding='utf-8'))['packages']:
  uri=urllib.parse.urljoin(config.as_uri(),package['rootUri'].rstrip('/')+'/')
  if urllib.parse.urlparse(uri).scheme!='file':continue
  directory=pathlib.Path(urllib.request.url2pathname(urllib.parse.urlparse(uri).path))
  spec=directory/'pubspec.yaml'
  value=yaml.safe_load(spec.read_text(encoding='utf-8')) if spec.is_file() else {}
  value=value if isinstance(value,dict) else {}
  version=str(value.get('version') or 'SDK/path source')
  records['dart:'+package['name']+'@'+version]={'ecosystem':'Dart/Flutter resolved source','name':package['name'],'version':version,'declaredLicense':value.get('license'),'notices':notices(directory)}
python_root=ROOT/'.venv-tools/Lib/site-packages'
if python_root.is_dir():
 selected={re.sub(r'[-_.]+','-',line.split('==')[0]).lower() for line in (ROOT/'tools/requirements-tools.txt').read_text().splitlines() if line and not line.startswith('#')}
 for distribution in importlib.metadata.distributions(path=[str(python_root)]):
  name=distribution.metadata['Name']
  if re.sub(r'[-_.]+','-',name).lower() not in selected:continue
  preserved=[]
  for item in distribution.files or []:
   if not any(part.lower().startswith(('license','notice','copying')) for part in pathlib.PurePosixPath(str(item)).parts):continue
   path=pathlib.Path(distribution.locate_file(item)).resolve()
   if path.is_relative_to(python_root.resolve()) and path.is_file():
    with path.open('rb') as stream:digest=hashlib.file_digest(stream,'sha256').hexdigest()
    preserved.append({'file':str(item).replace('\\','/'),'sha256':digest})
  records['python:'+name+'@'+distribution.version]={'ecosystem':'Python build/deployment tools','name':name,'version':distribution.version,'declaredLicense':distribution.metadata.get('License-Expression') or distribution.metadata.get('License'),'notices':preserved}
result={'scope':'Declared npm/Python metadata, cached Go source notices, vendored Flutter notices and resolved Dart/Flutter package source notices. Dependency lock files determine the build graph. Original component licenses remain in source.','packages':list(records.values())}
(ROOT/'docs/dependency-licenses.json').write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
print(json.dumps({'dependencyRecords':len(records),'noticesHashed':sum(len(x['notices']) for x in records.values())}))
