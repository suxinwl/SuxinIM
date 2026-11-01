"""Authenticated logical exports and disposable, port-free restore checks.

Never stops the source stack. Live exports are explicitly NOT point-in-time
snapshots. Redis, Kafka and etcd volumes are outside this logical backup format.
"""
import argparse,datetime,hashlib,hmac,json,os,pathlib,secrets,subprocess,tarfile,tempfile,time
from stack_guard import ROOT,PROJECT,DOCKER,guard,require_owned_resources,run

DATABASES=('openim38','chat38','enterprise38')
FORMAT='openim38-logical-backup-v1'
AUTH='--quiet --host=127.0.0.1 --username="$MONGO_INITDB_ROOT_USERNAME" --password="$MONGO_INITDB_ROOT_PASSWORD" --authenticationDatabase=admin'
COUNT_SCRIPT='const out={};for (const name of ["openim38","chat38","enterprise38"]){const d=db.getSiblingDB(name);out[name]={};for(const c of d.getCollectionNames().filter(x=>!x.startsWith("system."))){out[name][c]=d.getCollection(c).countDocuments({});}}print(JSON.stringify(out))'
VALIDATE_SCRIPT=COUNT_SCRIPT.replace('out[name][c]=','const v=d.runCommand({validate:c});if(!v.valid)throw new Error("invalid restored collection");out[name][c]=')

def utc():return datetime.datetime.now(datetime.timezone.utc).isoformat()
def sha(path):
 with path.open('rb') as stream:return hashlib.file_digest(stream,'sha256').hexdigest()
def require(condition,message):
 if not condition:raise RuntimeError(message)
def private(path):
 require(not path.is_symlink() and path.resolve().is_relative_to(ROOT),'Private path outside isolated root refused')
 path.parent.mkdir(parents=True,exist_ok=True);path.parent.chmod(0o700)
def mac(path,key):
 signing=hmac.new(key.read_bytes().strip(),b'openim38-backup-integrity-v1',hashlib.sha256).digest()
 digest=hmac.new(signing,digestmod=hashlib.sha256)
 with path.open('rb') as stream:
  for block in iter(lambda:stream.read(1024*1024),b''):digest.update(block)
 return digest.hexdigest()
def safe_member(member):
 name=pathlib.PurePosixPath(member.name)
 require(not name.is_absolute() and '..' not in name.parts and (member.isdir() or member.isfile()),'Unsafe backup archive member refused')
def tar_inventory(path):
 result=[]
 with tarfile.open(path,'r:gz') as archive:
  for member in archive:
   safe_member(member)
   if member.isfile():
    stream=archive.extractfile(member)
    result.append({'path':pathlib.PurePosixPath(member.name).as_posix().removeprefix('./'),'bytes':member.size,'sha256':hashlib.file_digest(stream,'sha256').hexdigest()})
 return sorted(result,key=lambda row:row['path'])
def to_file(command,path,input_path=None):
 with path.open('wb') as output:
  if input_path:
   with input_path.open('rb') as source:result=subprocess.run(command,stdin=source,stdout=output,stderr=subprocess.PIPE)
  else:result=subprocess.run(command,stdout=output,stderr=subprocess.PIPE)
 require(result.returncode==0,'Backup subprocess failed (details withheld because they can contain credentials)')
def run_input(command,path):
 with path.open('rb') as source:run(command,stdin=source)
def volume_name(spec,service,target):
 mounts=[m for m in spec['services'][service]['volumes'] if m['type']=='volume' and m['target']==target]
 require(len(mounts)==1,'Expected one dedicated storage volume')
 return spec['volumes'][mounts[0]['source']]['name']
def image(spec,service):
 value=spec['services'][service]['image']
 require('@sha256:' in value and len(value.rsplit('@sha256:',1)[1])==64,'Unpinned restore image refused')
 return value
def archive_volume(volume,runtime,destination):
 to_file(DOCKER+['run','--rm','--pull=never','--network','none','--read-only','--cap-drop','ALL','--cap-add','DAC_OVERRIDE','--security-opt','no-new-privileges','--mount',f'type=volume,source={volume},target=/source,readonly',runtime,'tar','-C','/source','-czf','-','.'],destination)
def assert_quiesced(spec,cli):
 allowed={'mongo','redis','etcd','kafka','minio','livekit'}
 ids=run(cli+['ps','-a','-q']).decode().split()
 if not ids:return
 for item in json.loads(run(DOCKER+['inspect']+ids)):
  service=item['Config']['Labels'].get('com.docker.compose.service')
  require(not item['State']['Running'] or service in allowed,'Quiesced export requires writers and ingress already stopped; this tool never stops them')

def create(mode):
 spec,cli=guard();require_owned_resources(spec)
 if mode=='quiesced':assert_quiesced(spec,cli)
 key=ROOT/'.private/backup.key';private(key)
 if not key.exists():key.write_text(secrets.token_hex(48)+'\n');key.chmod(0o600)
 require(len(key.read_bytes().strip())>=32,'Backup key is too short')
 key.chmod(0o600)
 output_dir=ROOT/'backups';private(output_dir/'placeholder');output_dir.mkdir(exist_ok=True);output_dir.chmod(0o700)
 identifier=datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ')+'-'+secrets.token_hex(4)
 target=output_dir/('openim38-'+identifier+'.tar.enc')
 with tempfile.TemporaryDirectory(prefix='backup-',dir=ROOT/'.private') as temporary:
  stage=pathlib.Path(temporary);started=utc()
  # Counts are diagnostic only for a live export; unchanged counts cannot prove
  # application-level consistency while documents are concurrently updated.
  count_command=cli+['exec','-T','mongo','sh','-ec','exec mongosh '+AUTH+' --eval "$1"','--',COUNT_SCRIPT]
  before=json.loads(run(count_command))
  require(all(before.get(db) for db in DATABASES),'Refuse an empty/misnamed database backup')
  for database in DATABASES:
   to_file(cli+['exec','-T','mongo','sh','-ec','exec mongodump --quiet --host=127.0.0.1 --username="$MONGO_INITDB_ROOT_USERNAME" --password="$MONGO_INITDB_ROOT_PASSWORD" --authenticationDatabase=admin --db "$1" --archive --gzip','--',database],stage/('mongo-'+database+'.archive.gz'))
  runtime=image(spec,'enterprise-service')
  archive_volume(volume_name(spec,'minio','/bitnami/minio/data'),runtime,stage/'minio-data.tar.gz')
  inventory=tar_inventory(stage/'minio-data.tar.gz')
  (stage/'minio-inventory.json').write_text(json.dumps(inventory,sort_keys=True)+'\n')
  with tarfile.open(stage/'runtime.tar.gz','w:gz') as archive:
   for name in ('compose.yaml','instance.json','secrets.json','config','certs'):
    path=ROOT/name
    require(path.exists() and not path.is_symlink(),'Missing or linked runtime backup input')
    paths=[path]+(list(path.rglob('*')) if path.is_dir() else [])
    for entry in paths:require(not entry.is_symlink() and entry.resolve().is_relative_to(ROOT),'Runtime backup symlink refused')
    archive.add(path,arcname=name)
  after=json.loads(run(count_command))
  if mode=='quiesced':require(before==after,'Database counts changed during the quiesced export')
  files=[{'path':p.name,'bytes':p.stat().st_size,'sha256':sha(p)} for p in sorted(stage.iterdir())]
  manifest={'format':FORMAT,'syntheticOnly':True,'sourceStack':PROJECT,'mode':mode,'pointInTimeSnapshot':False,'captureStarted':started,'captureFinished':utc(),'databases':list(DATABASES),'mongoImage':image(spec,'mongo'),'runtimeImage':runtime,'files':files,'sourceCountsBefore':before,'sourceCountsAfter':after,'excludedState':['Redis','Kafka','etcd'],'warning':'Live export can span different write times; quiesced export still requires queue draining and a separate full recovery procedure.'}
  (stage/'manifest.json').write_text(json.dumps(manifest,indent=2)+'\n')
  with tarfile.open(stage/'payload.tar','w') as archive:
   for path in sorted(stage.iterdir()):
    if path.name!='payload.tar':archive.add(path,arcname=path.name)
  partial=target.with_suffix(target.suffix+'.partial')
  try:
   run(['openssl','enc','-aes-256-cbc','-salt','-pbkdf2','-iter','200000','-pass','file:'+str(key),'-in',str(stage/'payload.tar'),'-out',str(partial)])
   partial.replace(target)
   target.with_suffix(target.suffix+'.hmac.json').write_text(json.dumps({'format':FORMAT,'algorithm':'HMAC-SHA256','value':mac(target,key)})+'\n')
   target.with_suffix(target.suffix+'.sha256').write_text(sha(target)+'  '+target.name+'\n')
  finally:
   if partial.exists():partial.unlink()
 print(json.dumps({'archive':str(target),'mode':mode,'pointInTimeSnapshot':False,'attachmentsIncluded':True,'authenticatedEncryptionEnvelope':True,'keyStoredSeparately':str(key),'sourceServicesStopped':False}))

def extract_verified(backup,key,stage):
 authentication=json.loads(backup.with_suffix(backup.suffix+'.hmac.json').read_text())
 require(authentication.get('format')==FORMAT and hmac.compare_digest(authentication.get('value',''),mac(backup,key)),'Backup authentication failed; refuse to decrypt')
 run(['openssl','enc','-d','-aes-256-cbc','-pbkdf2','-iter','200000','-pass','file:'+str(key),'-in',str(backup),'-out',str(stage/'payload.tar')])
 with tarfile.open(stage/'payload.tar','r:') as archive:
  seen=set()
  for member in archive:
   safe_member(member)
   require(member.isfile() and len(pathlib.PurePosixPath(member.name).parts)==1 and member.name not in seen,'Unexpected or duplicate payload member')
   seen.add(member.name)
   with (stage/member.name).open('wb') as output:
    source=archive.extractfile(member)
    while block:=source.read(1024*1024):output.write(block)
 manifest=json.loads((stage/'manifest.json').read_text())
 require(manifest.get('format')==FORMAT and manifest.get('syntheticOnly') is True and manifest.get('databases')==list(DATABASES),'Wrong backup identity or databases')
 for item in manifest['files']:
  path=stage/item['path']
  require(path.parent==stage and path.is_file() and path.stat().st_size==item['bytes'] and sha(path)==item['sha256'],'Backup file checksum mismatch')
 # Validate nested archives before any restore container is created.
 tar_inventory(stage/'runtime.tar.gz');tar_inventory(stage/'minio-data.tar.gz')
 return manifest

def cleanup_restore(identifier,container,volumes):
 # Never remove a resource unless both its unique name and ownership label match.
 for kind,name in [('container',container)]+[('volume',volume) for volume in volumes]:
  require(name.startswith(identifier+'-'),'Unexpected restore cleanup name')
  result=subprocess.run(DOCKER+[kind,'inspect',name],stdout=subprocess.PIPE,stderr=subprocess.PIPE)
  if result.returncode:continue
  item=json.loads(result.stdout)[0]
  labels=item.get('Config',{}).get('Labels',{}) if kind=='container' else item.get('Labels',{})
  require((labels or {}).get('openim38.restore')==identifier,'Foreign resource cleanup refused')
  run(DOCKER+(['rm','-f',name] if kind=='container' else ['volume','rm',name]))

def restore_check(backup,key,report):
 guard() # Never infer a Docker target from environment or use production context.
 require(backup.is_file() and not backup.is_symlink() and backup.resolve().is_relative_to(ROOT/'backups'),'Use an archive in this isolated backup directory')
 require(key.is_file() and not key.is_symlink(),'Backup key must be a regular private file')
 require(not report.is_symlink() and report.resolve().is_relative_to(ROOT/'tests/backup'),'Restore reports must stay in this test backup directory')
 private(ROOT/'.private/placeholder')
 identifier='openim38-restore-'+secrets.token_hex(8);container=identifier+'-mongo'
 volumes=[identifier+'-mongo-data',identifier+'-minio-data']
 result={'format':FORMAT,'test':'disposable restore exercise','started':utc(),'archive':backup.name,'sourceServicesStopped':False,'hostPorts':[],'network':'none','passed':False}
 try:
  with tempfile.TemporaryDirectory(prefix='restore-',dir=ROOT/'.private') as temporary:
   result['phase']='authenticate and verify archive'
   stage=pathlib.Path(temporary);manifest=extract_verified(backup,key,stage)
   result['sourceMode']=manifest['mode'];result['pointInTimeSnapshot']=False
   for value in ('mongoImage','runtimeImage'):
    require('@sha256:' in manifest[value] and len(manifest[value].rsplit('@sha256:',1)[1])==64,'Unpinned restore image refused')
   for volume in volumes:run(DOCKER+['volume','create','--label','openim38.restore='+identifier,volume])
   env=stage/'mongo.env';env.write_text('MONGO_INITDB_ROOT_USERNAME=restorecheck\nMONGO_INITDB_ROOT_PASSWORD='+secrets.token_hex(32)+'\n');env.chmod(0o600)
   result['phase']='start disposable MongoDB'
   run(DOCKER+['run','-d','--pull=never','--memory','512m','--cpus','1','--name',container,'--label','openim38.restore='+identifier,'--network','none','--env-file',str(env),'--mount',f'type=volume,source={volumes[0]},target=/data/db',manifest['mongoImage']])
   auth=AUTH
   for _ in range(90):
    ping=subprocess.run(DOCKER+['exec',container,'sh','-ec','mongosh '+auth+' --eval "db.adminCommand({ping:1})"'],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
    if ping.returncode==0:break
    time.sleep(1)
   else:raise RuntimeError('Disposable restore MongoDB did not become ready')
   result['phase']='restore MongoDB'
   for database in DATABASES:
    run_input(DOCKER+['exec','-i',container,'sh','-ec','exec mongorestore --quiet '+auth.removeprefix('--quiet ')+' --drop --nsInclude "$1.*" --archive --gzip','--',database],stage/('mongo-'+database+'.archive.gz'))
   result['phase']='validate restored MongoDB'
   result['mongoCollectionCounts']=json.loads(run(DOCKER+['exec',container,'sh','-ec','exec mongosh '+auth+' --eval "$1"','--',VALIDATE_SCRIPT]))
   result['countsMatchSourceBefore']=result['mongoCollectionCounts']==manifest['sourceCountsBefore']
   result['countsMatchSourceAfter']=result['mongoCollectionCounts']==manifest['sourceCountsAfter']
   if manifest['mode']=='quiesced':require(result['countsMatchSourceAfter'],'Restored quiesced database counts differ')
   result['phase']='restore attachment volume'
   run_input(DOCKER+['run','-i','--rm','--pull=never','--network','none','--read-only','--mount',f'type=volume,source={volumes[1]},target=/restore',manifest['runtimeImage'],'tar','-C','/restore','-xzf','-'],stage/'minio-data.tar.gz')
   result['phase']='verify restored attachment contents'
   archive_volume(volumes[1],manifest['runtimeImage'],stage/'restored-minio.tar.gz')
   expected=json.loads((stage/'minio-inventory.json').read_text())
   require(tar_inventory(stage/'restored-minio.tar.gz')==expected,'Restored attachment volume differs from the exported files')
   result['attachmentFilesVerified']=len(expected);result['passed']=True;result['phase']='complete'
 finally:
  try:cleanup_restore(identifier,container,volumes);result['cleanupVerified']=True
  except Exception:result['cleanupVerified']=False;raise
  finally:
   result['finished']=utc();report.parent.mkdir(parents=True,exist_ok=True);report.write_text(json.dumps(result,indent=2)+'\n')
 print(json.dumps({'report':str(report),'passed':True,'cleanupVerified':True,'sourceServicesStopped':False}))

def main():
 parser=argparse.ArgumentParser(description=__doc__);commands=parser.add_subparsers(dest='command',required=True)
 create_parser=commands.add_parser('create');create_parser.add_argument('--mode',choices=['live','quiesced'],required=True)
 restore_parser=commands.add_parser('restore-check');restore_parser.add_argument('--archive',type=pathlib.Path,required=True);restore_parser.add_argument('--key',type=pathlib.Path,default=ROOT/'.private/backup.key');restore_parser.add_argument('--report',type=pathlib.Path,default=ROOT/'tests/backup/restore-report.json')
 args=parser.parse_args()
 if args.command=='create':create(args.mode)
 else:restore_check(args.archive,args.key,args.report)

if __name__=='__main__':
 try:main()
 except Exception as error:raise SystemExit(type(error).__name__+': '+str(error))
