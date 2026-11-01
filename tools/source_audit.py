"""Inventory original source and record upstream modifications without editing it."""
import datetime,difflib,hashlib,io,json,os,pathlib,subprocess,tarfile
ROOT=pathlib.Path(__file__).resolve().parents[1]
PARENT=ROOT.parent
SKIP={'.git','node_modules','.dart_tool','.gradle','build','dist','.idea','.vscode','Pub','Cache','ephemeral'}
def sha(path):
 with path.open('rb') as f:return hashlib.file_digest(f,'sha256').hexdigest()
def entries(directory):
 for folder,dirs,files in os.walk(directory):
  dirs[:]=[x for x in dirs if x not in SKIP]
  for name in sorted(files):
   path=pathlib.Path(folder)/name
   if path.suffix in {'.log','.exe','.dll','.so','.aar','.apk','.zip','.wasm'}:continue
   if path.is_file():yield {'path':path.relative_to(directory).as_posix(),'size':path.stat().st_size,'sha256':sha(path)}
sources=[PARENT/'OpenIM3.5'/name for name in ['open-im-server-enterprise-3.5.1-alpha.1-e-v1.1.3','chat-enterprise-1.5.0-e-v1.1.0','openim-electron-enterprise-toc-enterprise-rtc','open-im-flutter-enterprise-toc-rtc-3.5.1-e-1.1.4']]+[PARENT/'OpenIM3.4/openim-admin-3']
reports=ROOT/'docs/provenance';reports.mkdir(parents=True,exist_ok=True)
original=[]
for source in sources:
 if not source.is_dir():raise RuntimeError('Missing original source: '+str(source))
 values=list(entries(source));original.append({'source':str(source),'files':values,'fileCount':len(values)})
(reports/'original-source-sha256.json').write_text(json.dumps(original,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
patches=[];git_baselines=[]
for item in json.loads((ROOT/'upstream-sources.lock.json').read_text()):
 archive=ROOT/'.cache/downloads'/(item['repository'].rsplit('/',1)[1]+'-'+item['commit']+'.tar.gz')
 if archive.exists():
  if sha(archive)!=item['archiveSha256']:raise RuntimeError('Archive hash mismatch')
  baseline=tarfile.open(archive,'r:gz')
 elif item.get('retrieval')=='git fixed tag':
  content=subprocess.check_output(['git','-C',str(ROOT/item['path']),'archive','--format=tar','--prefix=source/',item['commit']])
  git_baselines.append({'component':item['path'],'commit':item['commit'],'localGitArchiveSHA256':hashlib.sha256(content).hexdigest(),'note':'Archive generated locally from the locked Git commit, not an upstream download hash.'})
  baseline=tarfile.open(fileobj=io.BytesIO(content),mode='r:')
 else:continue
 seen=set()
 with baseline as tar:
  for member in tar.getmembers():
   if not member.isfile():continue
   rel=pathlib.PurePosixPath(*pathlib.PurePosixPath(member.name).parts[1:])
   seen.add(str(rel))
   current=ROOT/item['path']/rel
   before=tar.extractfile(member).read();old=hashlib.sha256(before).hexdigest()
   if not current.exists():patches.append({'component':item['path'],'path':str(rel),'status':'missing','upstreamSha256':old});continue
   now=sha(current)
   if old==now:continue
   record={'component':item['path'],'path':str(rel),'status':'modified','upstreamSha256':old,'currentSha256':now}
   if current.suffix in {'.go','.mod','.sum','.yml','.yaml','.json','.ts','.dart'} and current.stat().st_size<200000:
    try:
     diff=''.join(difflib.unified_diff(before.decode('utf-8').splitlines(True),current.read_text(encoding='utf-8').splitlines(True),fromfile='upstream/'+str(rel),tofile='merged/'+str(rel)))
     dest=reports/'patches'/item['path']/pathlib.Path(str(rel)+'.patch');dest.parent.mkdir(parents=True,exist_ok=True);dest.write_text(diff,encoding='utf-8');record['patch']=dest.relative_to(ROOT).as_posix()
    except UnicodeError:pass
   patches.append(record)
 for entry in entries(ROOT/item['path']):
  if entry['path'] in seen or entry['path']=='.upstream.json':continue
  # Generated binaries and build outputs are inventoried separately.
  if entry['path'].split('/')[0] in {'dist','artifacts','logs','test-results'}:continue
  patches.append({'component':item['path'],'path':entry['path'],'status':'added','currentSha256':entry['sha256']})
(reports/'upstream-modifications.json').write_text(json.dumps(patches,indent=2)+'\n',encoding='utf-8')
(reports/'git-baselines.json').write_text(json.dumps(git_baselines,indent=2)+'\n',encoding='utf-8')
print(json.dumps({'originalDirectories':len(original),'originalFiles':sum(x['fileCount'] for x in original),'upstreamModifications':len(patches),'originalsEdited':False}))
