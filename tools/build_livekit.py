"""Build the pinned, patched LiveKit with its own Go 1.26.9, never system Go.

Produces Linux amd64 server + targeted Linux regression test executable. On
Windows the latter must be run on the isolated Linux test host, as documented.
"""
import argparse,hashlib,json,os,pathlib,platform,shutil,subprocess,tarfile,urllib.request,zipfile

ROOT=pathlib.Path(__file__).resolve().parents[1]
LOCK=ROOT/'livekit.lock.json'

def digest(path):
 with path.open('rb') as source:return hashlib.file_digest(source,'sha256').hexdigest()

def bootstrap(lock):
 system='windows' if os.name=='nt' else 'linux'
 if platform.machine().lower() not in ('amd64','x86_64'):raise RuntimeError('LiveKit build runtime is pinned to amd64')
 runtime=ROOT/'tools/go1.26.9'; executable=runtime/('bin/go.exe' if os.name=='nt' else 'bin/go')
 if executable.exists():return executable
 asset=next(row for row in lock['toolchain']['files'] if row['os']==system)
 archive=ROOT/'.cache/downloads'/asset['filename'];archive.parent.mkdir(parents=True,exist_ok=True)
 if not archive.exists():
  with urllib.request.urlopen('https://go.dev/dl/'+asset['filename'],timeout=120) as source,archive.with_suffix('.partial').open('wb') as output:shutil.copyfileobj(source,output)
  archive.with_suffix('.partial').replace(archive)
 if digest(archive)!=asset['sha256']:raise RuntimeError('LiveKit runtime checksum mismatch')
 runtime.mkdir(parents=True,exist_ok=True)
 def destination(name):
  parts=pathlib.PurePosixPath(name).parts
  if not parts or parts[0]!='go' or '..' in parts:raise RuntimeError('Unexpected Go archive layout')
  path=runtime.joinpath(*parts[1:]).resolve()
  if not path.is_relative_to(runtime.resolve()):raise RuntimeError('Runtime archive path escape')
  return path
 if os.name=='nt':
  with zipfile.ZipFile(archive) as package:
   for member in package.infolist():
    path=destination(member.filename)
    if member.is_dir():path.mkdir(parents=True,exist_ok=True)
    else:
     path.parent.mkdir(parents=True,exist_ok=True)
     with package.open(member) as source,path.open('wb') as output:shutil.copyfileobj(source,output)
 else:
  with tarfile.open(archive,'r:gz') as package:
   for member in package:
    path=destination(member.name)
    if member.isdir():path.mkdir(parents=True,exist_ok=True)
    elif member.isfile():
     path.parent.mkdir(parents=True,exist_ok=True)
     with package.extractfile(member) as source,path.open('wb') as output:shutil.copyfileobj(source,output)
     path.chmod(member.mode & 0o777)
    else:raise RuntimeError('Unexpected runtime archive link/device')
 (runtime/'runtime.lock.json').write_text(json.dumps({'version':'go1.26.9','url':'https://go.dev/dl/'+asset['filename'],'sha256':asset['sha256']},indent=2)+'\n')
 return executable

def build():
 parser=argparse.ArgumentParser(description=__doc__)
 parser.add_argument('--skip-tests',action='store_true',help='Skip compiling the targeted Linux regression executable')
 args=parser.parse_args();lock=json.loads(LOCK.read_text())
 go=pathlib.Path(os.environ['OPENIM38_LIVEKIT_GO']) if os.environ.get('OPENIM38_LIVEKIT_GO') else bootstrap(lock)
 env=os.environ.copy();env.update({'GOROOT':str(go.parent.parent),'GOTOOLCHAIN':'local','GOMAXPROCS':'2','GOMEMLIMIT':os.environ.get('OPENIM38_LIVEKIT_MEMORY','768MiB'),'GOFLAGS':'-p=1','GOOS':'linux','GOARCH':'amd64','CGO_ENABLED':'0','GOMODCACHE':str(ROOT/'.cache/go-mod'),'GOCACHE':str(ROOT/'.cache/go126-build'),'GOTMPDIR':str(ROOT/'.cache/go126-tmp')})
 for name in ('GOCACHE','GOTMPDIR','GOMODCACHE'):pathlib.Path(env[name]).mkdir(parents=True,exist_ok=True)
 version=subprocess.check_output([str(go),'version'],env=env,text=True).strip()
 if 'go1.26.9 ' not in version:raise RuntimeError('LiveKit requires its pinned Go 1.26.9 toolchain')
 source=ROOT/lock['source'];out=ROOT/'artifacts/linux-amd64/livekit';out.mkdir(parents=True,exist_ok=True)
 def run(arguments):subprocess.run([str(go)]+arguments,cwd=source,env=env,check=True)
 if not args.skip_tests:run(['test','-mod=readonly','-c','-trimpath','-o',str(out/'livekit-turn.test'),'./pkg/service'])
 run(['build','-mod=readonly','-trimpath','-buildvcs=false','-ldflags=-s -w','-o',str(out/'livekit-server'),'./cmd/server'])
 files=[{'file':p.name,'bytes':p.stat().st_size,'sha256':digest(p)} for p in sorted(out.iterdir()) if p.name in ('livekit-server','livekit-turn.test')]
 (out/'SHA256SUMS').write_text(''.join(row['sha256']+'  '+row['file']+'\n' for row in files))
 if os.name!='nt' and not args.skip_tests:subprocess.run([str(out/'livekit-turn.test'),'-docker=false','-test.run','TestOpenIM38|TestTURNRelayPolicy_Allows|TestGetTURNMediaPorts|TestGetTURNLocalIPs','-test.v'],check=True)
 (out/'build-provenance.json').write_text(json.dumps({'repository':lock['repository'],'tag':lock['tag'],'commit':lock['commit'],'patchSHA256':lock.get('patchSHA256'),'go':version,'target':'linux/amd64','cgo':False,'dependencySums':{'go.mod':digest(source/'go.mod'),'go.sum':digest(source/'go.sum')},'files':files,'testsCompiled':not args.skip_tests,'linuxTestsExecutedByThisBuild':os.name!='nt' and not args.skip_tests},indent=2)+'\n')
 print(json.dumps({'built':'LiveKit v1.13.11 + OpenIM38 TURN compatibility','files':files}))

if __name__=='__main__':build()
