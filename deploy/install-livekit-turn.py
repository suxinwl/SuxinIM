"""Install only the pinned LiveKit binary/config in the isolated synthetic stack.

Upload livekit-server.next into bin first, then supply its published SHA-256.
Never changes another service, host firewall, gateway, or production directory.
"""
import argparse,datetime,hashlib,json,os,pathlib,secrets,shutil,subprocess
import yaml
from stack_guard import ROOT,DOCKER,guard,require_owned_resources

def sha(path):
 with path.open('rb') as source:return hashlib.file_digest(source,'sha256').hexdigest()

def install(expected):
 spec,cli=guard();require_owned_resources(spec)
 incoming=ROOT/'bin/livekit-server.next';binary=ROOT/'bin/livekit-server'
 if not incoming.is_file() or incoming.is_symlink() or incoming.resolve().parent!=ROOT/'bin' or sha(incoming)!=expected:
  raise RuntimeError('Missing or mismatched isolated LiveKit binary')
 runtime=spec['services']['enterprise-service']['image']
 if '@sha256:' not in runtime:raise RuntimeError('Pinned runtime image required')
 backup=ROOT/'.private'/('livekit-before-'+datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ')+'-'+secrets.token_hex(4))
 backup.mkdir(parents=True);backup.chmod(0o700)
 for source in (ROOT/'compose.yaml',ROOT/'config/livekit.yml',binary):
  if source.exists():
   if source.is_symlink() or not source.resolve().is_relative_to(ROOT):raise RuntimeError('Linked LiveKit input refused')
   destination=backup/source.name;shutil.copyfile(source,destination);destination.chmod(0o600)
 configuration=ROOT/'config/livekit.yml';value=yaml.safe_load(configuration.read_text())
 value['rtc']['advertise_internal_ip']=True
 value['turn'].update({'advertised_tls_port':25349,'allow_local_media_peers':True,'allow_restricted_peer_cidrs':['192.168.10.11/32'],'relay_range_start':27900,'relay_range_end':28155})
 compose=ROOT/'compose.yaml';stack=yaml.safe_load(compose.read_text());service=stack['services']['livekit']
 service.update({'image':runtime,'entrypoint':['/app/bin/livekit-server'],'command':['--config','/etc/livekit.yml'],'ports':['192.168.10.11:27881:27881/tcp','192.168.10.11:27882:27882/udp','192.168.10.11:23478:23478/udp','192.168.10.11:25349:25349/tcp'],'volumes':['./bin/livekit-server:/app/bin/livekit-server:ro','./config/livekit.yml:/etc/livekit.yml:ro','./certs:/certs:ro']})
 # Atomic files, followed by recreation so the bind mount sees the new inode.
 for path,content in ((configuration,value),(compose,stack)):
  temporary=path.with_name(path.name+'.livekit-next');temporary.write_text(yaml.safe_dump(content,sort_keys=False));temporary.chmod(0o600);os.replace(temporary,path)
 incoming.chmod(0o755);os.replace(incoming,binary)
 guard();subprocess.run(cli+['up','-d','--no-deps','--force-recreate','livekit'],check=True)
 actual=subprocess.check_output(cli+['exec','-T','livekit','sha256sum','/app/bin/livekit-server'],text=True).split()[0]
 if actual!=expected:raise RuntimeError('Running LiveKit mount checksum differs')
 print(json.dumps({'component':'livekit','binarySHA256':actual,'onlyOriginalMediaPorts':True,'otherServicesRecreated':False,'previousConfiguration':str(backup)}))

if __name__=='__main__':
 parser=argparse.ArgumentParser(description=__doc__);parser.add_argument('--sha256',required=True)
 args=parser.parse_args()
 if len(args.sha256)!=64 or any(c not in '0123456789abcdef' for c in args.sha256):raise SystemExit('Supply lowercase SHA-256')
 install(args.sha256)
