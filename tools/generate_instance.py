"""Generate a fresh isolated instance from the fixed upstream configuration.

Requires PyYAML==6.0.3. Secrets live under .private and are never printed.
Run once; subsequent runs keep the existing identity and never overwrite data.
"""
import argparse,base64,json,pathlib,secrets,shutil
import yaml
ROOT=pathlib.Path(__file__).resolve().parents[1]

def harden_storage_process_arguments(compose):
 """Change only three static argv definitions; credentials stay in ENV/config.

 This function deliberately uses builtins only, so the rotation worker can use
 the identical dict patch after parsing its existing Compose YAML. Double '$'
 is required here: Compose must leave expansion to the container's shell.
 """
 services = compose.get('services', {})
 if not all(isinstance(services.get(name), dict)
            and isinstance(services[name].get('healthcheck'), dict)
            for name in ('mongo', 'redis')):
  raise ValueError('storage_process_model_invalid')
 redis_script = (
  'umask 077; '
  'case "$$REDIS_PASSWORD" in ""|*[!0-9a-f]*) exit 1;; esac; '
  '[ "$${#REDIS_PASSWORD}" -eq 64 ] || exit 1; '
  'puty_redis_config=$$(mktemp /tmp/puty-redis.XXXXXX.conf); '
  "printf 'requirepass %s\\nappendonly yes\\n' "
  '"$$REDIS_PASSWORD" > "$$puty_redis_config"; '
  'exec redis-server "$$puty_redis_config"'
 )
 services['redis']['command'] = ['sh', '-ec', redis_script]
 services['redis']['healthcheck']['test'] = [
  'CMD-SHELL',
  'REDISCLI_AUTH="$$REDIS_PASSWORD" redis-cli --no-auth-warning '
  '-h 127.0.0.1 ping | grep -qx PONG'
 ]
 mongo_script = (
  'const c=new Mongo("mongodb://"+'
  'encodeURIComponent(process.env.MONGO_INITDB_ROOT_USERNAME)+":"+'
  'encodeURIComponent(process.env.MONGO_INITDB_ROOT_PASSWORD)+'
  '"@127.0.0.1:27017/admin");'
  'quit(c.getDB("admin").runCommand({ping:1}).ok===1?0:1);'
 )
 services['mongo']['healthcheck']['test'] = [
  'CMD', 'mongosh', '--nodb', '--quiet', '--eval', mongo_script
 ]
 return compose

def dump(path,value):
 path.parent.mkdir(parents=True,exist_ok=True)
 path.write_text(yaml.safe_dump(value,allow_unicode=True,sort_keys=False),encoding='utf-8')

def generate(destination,host):
 destination=destination.resolve();destination.mkdir(parents=True,exist_ok=True)
 secret_path=destination/'secrets.json'
 if secret_path.exists():keys=json.loads(secret_path.read_text())
 else:
  keys={key:secrets.token_hex(32) for key in ['mongo','redis','minio','im','chat','livekit','webhook','session']}
  keys['livekit_key']='LK'+secrets.token_hex(8)
  keys['kafka_cluster']=base64.urlsafe_b64encode(secrets.token_bytes(16)).decode().rstrip('=')
  secret_path.write_text(json.dumps(keys,indent=2)+'\n');secret_path.chmod(0o600)
 images=json.loads((ROOT/'deploy/images.lock.json').read_text())
 for component in ['openim-server','chat']:
  source=ROOT/'services'/component/'config';target=destination/'config'/component
  shutil.copytree(source,target,dirs_exist_ok=True)
  def edit(name,change):
   path=target/name;value=yaml.safe_load(path.read_text(encoding='utf-8')) or {};change(value);dump(path,value)
  edit('mongodb.yml',lambda v:v.update(uri='',address=['mongo:27017'],database='openim38' if component=='openim-server' else 'chat38',username='root',password=keys['mongo'],authSource='admin'))
  edit('redis.yml',lambda v:v.update(address=['redis:6379'],password=keys['redis'],db=0 if component=='openim-server' else 1))
  edit('discovery.yml',lambda v:v.update(enable='etcd',etcd={'rootDirectory':'openim38' if component=='openim-server' else 'chat38','address':['etcd:2379'],'username':'','password':''}))
  edit('log.yml',lambda v:v.update(storageLocation='/app/logs',isStdout=True,isJson=True,remainLogLevel=3))
  for file in target.glob('*.yml'):
   value=yaml.safe_load(file.read_text(encoding='utf-8')) or {}
   if isinstance(value,dict) and 'rpc' in value:
    rpc=value['rpc'];rpc['listenIP']='0.0.0.0';rpc['autoSetPorts']=False;rpc['ports']=rpc.get('ports',[10200])[:1]
    rpc['registerIP']=''
   if isinstance(value,dict) and 'prometheus' in value:
    value['prometheus']['enable']=False;value['prometheus']['autoSetPorts']=False;value['prometheus']['ports']=value['prometheus'].get('ports',[12000])[:1]
   dump(file,value)
  if component=='openim-server':
   edit('share.yml',lambda v:v.update(secret=keys['im'],imAdminUserID=['imAdmin38'],multiLogin={'policy':4,'maxNumOneEnd':1}))
   edit('kafka.yml',lambda v:v.update(address=['kafka:9092']))
   edit('minio.yml',lambda v:v.update(bucket='openim38',accessKeyID='root',secretAccessKey=keys['minio'],internalAddress='minio:9000',externalAddress=f'https://{host}:29885',publicRead=False))
   edit('openim-msggateway.yml',lambda v:v['longConnSvr'].update(ports=[10001]))
   edit('openim-push.yml',lambda v:v.update(enable=''))
   edit('webhooks.yml',lambda v:v.update(url=f"http://enterprise-service:18090/internal/openim/webhook/{keys['webhook']}"))
   edit('webhooks.yml',lambda v:[setting.update(enable=name in ['beforeSendGroupMsg','afterSendGroupMsg','afterMsgSaveDB']) for name,setting in v.items() if isinstance(setting,dict)])
   edit('webhooks.yml',lambda v:v['beforeSendGroupMsg'].update(failedContinue=False,timeout=5,allowedTypes=['100-999']))
   edit('webhooks.yml',lambda v:[v[name].update(allowedTypes=['100-999']) for name in ['afterSendGroupMsg','afterMsgSaveDB']])
  else:
   edit('share.yml',lambda v:v.update(openIM={'apiURL':'http://openim-api:10002','secret':keys['im'],'adminUserID':'imAdmin38'},chatAdmin=['chatAdmin38']))
   edit('chat-rpc-admin.yml',lambda v:v.update(secret=keys['chat']))
   def chat_config(v):
    v['verifyCode']['superCode']='';v['verifyCode']['mail']['enable']=False
    v['liveKit']={'url':'http://livekit:7880','key':keys['livekit_key'],'secret':keys['livekit']}
    v['allowRegister']=False
   edit('chat-rpc-chat.yml',chat_config)
 # The public signalling URL ALWAYS passes through enterprise-service's live
 # session/room generation guard. The raw LiveKit API has no host port.
 dump(destination/'config/livekit.yml',{'port':7880,'bind_addresses':['0.0.0.0'],'rtc':{'tcp_port':27881,'udp_port':27882,'use_external_ip':False,'node_ip':host,'advertise_internal_ip':True,'stun_servers':[]},'keys':{keys['livekit_key']:keys['livekit']},'room':{'auto_create':False,'empty_timeout':300,'departure_timeout':20},'turn':{'enabled':True,'domain':host,'udp_port':23478,'tls_port':25349,'advertised_tls_port':25349,'allow_local_media_peers':True,'cert_file':'/certs/server.crt','key_file':'/certs/server.key','external_tls':False,'allow_restricted_peer_cidrs':[f'{host}/32'],'relay_range_start':27900,'relay_range_end':28155},'logging':{'level':'info'},'webhook':{'api_key':keys['livekit_key'],'urls':['http://enterprise-service:18090/internal/livekit/webhook']}})
 deps={'mongo':{'condition':'service_healthy'},'redis':{'condition':'service_healthy'},'etcd':{'condition':'service_started'},'kafka':{'condition':'service_started'}}
 services={}
 def image(name):return images[name]['image']
 services['mongo']={'image':image('mongo'),'restart':'unless-stopped','environment':{'MONGO_INITDB_ROOT_USERNAME':'root','MONGO_INITDB_ROOT_PASSWORD':keys['mongo']},'volumes':['mongo-data:/data/db'],'healthcheck':{'test':['CMD-SHELL','mongosh --quiet -u "$$MONGO_INITDB_ROOT_USERNAME" -p "$$MONGO_INITDB_ROOT_PASSWORD" --authenticationDatabase admin --eval "db.adminCommand({ping:1})"'],'interval':'10s','timeout':'5s','retries':20}}
 services['redis']={'image':image('redis'),'restart':'unless-stopped','environment':{'REDIS_PASSWORD':keys['redis']},'command':['sh','-ec','exec redis-server --requirepass "$$REDIS_PASSWORD" --appendonly yes'],'volumes':['redis-data:/data'],'healthcheck':{'test':['CMD-SHELL','redis-cli -a "$$REDIS_PASSWORD" ping | grep PONG'],'interval':'5s','timeout':'3s','retries':20}}
 services['etcd']={'image':image('etcd'),'restart':'unless-stopped','environment':{'ALLOW_NONE_AUTHENTICATION':'yes','ETCD_LISTEN_CLIENT_URLS':'http://0.0.0.0:2379','ETCD_ADVERTISE_CLIENT_URLS':'http://etcd:2379','ETCD_NAME':'openim38','ETCD_INITIAL_CLUSTER':'openim38=http://etcd:2380','ETCD_LISTEN_PEER_URLS':'http://0.0.0.0:2380','ETCD_INITIAL_ADVERTISE_PEER_URLS':'http://etcd:2380'},'volumes':['etcd-data:/bitnami/etcd']}
 services['kafka']={'image':image('kafka'),'restart':'unless-stopped','environment':{'KAFKA_ENABLE_KRAFT':'yes','KAFKA_KRAFT_CLUSTER_ID':keys['kafka_cluster'],'KAFKA_CFG_NODE_ID':'1','KAFKA_CFG_PROCESS_ROLES':'controller,broker','KAFKA_CFG_CONTROLLER_QUORUM_VOTERS':'1@kafka:9093','KAFKA_CFG_CONTROLLER_LISTENER_NAMES':'CONTROLLER','KAFKA_CFG_LISTENERS':'PLAINTEXT://:9092,CONTROLLER://:9093','KAFKA_CFG_ADVERTISED_LISTENERS':'PLAINTEXT://kafka:9092','KAFKA_CFG_LISTENER_SECURITY_PROTOCOL_MAP':'CONTROLLER:PLAINTEXT,PLAINTEXT:PLAINTEXT','KAFKA_CFG_AUTO_CREATE_TOPICS_ENABLE':'true','KAFKA_CFG_NUM_PARTITIONS':'8','KAFKA_CFG_OFFSETS_TOPIC_REPLICATION_FACTOR':'1','KAFKA_CFG_TRANSACTION_STATE_LOG_REPLICATION_FACTOR':'1','KAFKA_CFG_TRANSACTION_STATE_LOG_MIN_ISR':'1','ALLOW_PLAINTEXT_LISTENER':'yes','KAFKA_HEAP_OPTS':'-Xms256m -Xmx512m'},'volumes':['kafka-data:/bitnami/kafka']}
 services['minio']={'image':image('minio'),'restart':'unless-stopped','environment':{'MINIO_ROOT_USER':'root','MINIO_ROOT_PASSWORD':keys['minio'],'MINIO_DEFAULT_BUCKETS':'openim38','MINIO_API_CORS_ALLOW_ORIGIN':f'https://{host}:29880'},'volumes':['minio-data:/bitnami/minio/data']}
 for component,binaries in [('openim-server',['openim-api','openim-msggateway','openim-msgtransfer','openim-push','openim-crontask','openim-rpc-auth','openim-rpc-user','openim-rpc-friend','openim-rpc-group','openim-rpc-msg','openim-rpc-conversation','openim-rpc-third']),('chat',['chat-api','admin-api','chat-rpc','admin-rpc'])]:
  for binary in binaries:
   name={'admin-rpc':'chat-admin','chat-rpc':'chat-business'}.get(binary,binary)
   logdir='logs/'+binary;(destination/logdir).mkdir(parents=True,exist_ok=True)
   services[name]={'image':image('runtime'),'restart':'unless-stopped','entrypoint':['/app/bin/'+binary],'command':['-c','/app/config'],'volumes':[f'./bin/{binary}:/app/bin/{binary}:ro',f'./config/{component}:/app/config:ro',f'./{logdir}:/app/logs'],'depends_on':deps.copy(),'tmpfs':['/tmp']}
 env={'LISTEN_ADDR':':18090','MONGO_URI':f"mongodb://root:{keys['mongo']}@mongo:27017/?authSource=admin",'MONGO_DATABASE':'enterprise38','CHAT_ADMIN_GRPC':'chat-admin:30200','OPENIM_AUTH_GRPC':'openim-rpc-auth:10200','OPENIM_API':'http://openim-api:10002','OPENIM_SECRET':keys['im'],'OPENIM_ADMIN_ID':'imAdmin38','CHAT_API':'http://chat-api:10008','CHAT_ADMIN_API':'http://admin-api:10009','LIVEKIT_HTTP_URL':'http://livekit:7880','LIVEKIT_PUBLIC_URL':f'wss://{host}:29880/livekit','LIVEKIT_API_KEY':keys['livekit_key'],'LIVEKIT_API_SECRET':keys['livekit'],'WEBHOOK_SECRET':keys['webhook'],'ENTERPRISE_SESSION_KEY':keys['session'],'ALLOWED_UPDATE_HOSTS':host}
 (destination/'logs/enterprise-service').mkdir(parents=True,exist_ok=True)
 services['enterprise-service']={'image':image('runtime'),'restart':'unless-stopped','entrypoint':['/app/bin/enterprise-service'],'environment':env,'volumes':['./bin/enterprise-service:/app/bin/enterprise-service:ro','./logs/enterprise-service:/app/logs'],'depends_on':{'mongo':{'condition':'service_healthy'},'chat-admin':{'condition':'service_started'},'openim-rpc-auth':{'condition':'service_started'}},'tmpfs':['/tmp']}
 services['livekit']={'image':image('runtime'),'restart':'unless-stopped','entrypoint':['/app/bin/livekit-server'],'command':['--config','/etc/livekit.yml'],'ports':[f'{host}:27881:27881/tcp',f'{host}:27882:27882/udp',f'{host}:23478:23478/udp',f'{host}:25349:25349/tcp'],'volumes':['./bin/livekit-server:/app/bin/livekit-server:ro','./config/livekit.yml:/etc/livekit.yml:ro','./certs:/certs:ro']}
 services['nginx']={'image':image('nginx'),'restart':'unless-stopped','ports':[f'{host}:29880:29880/tcp',f'{host}:29885:29885/tcp',f'{host}:23478:23478/tcp'],'volumes':['./config/nginx.conf:/etc/nginx/nginx.conf:ro','./certs:/certs:ro','./www:/usr/share/nginx/html:ro'],'depends_on':{'enterprise-service':{'condition':'service_started'},'openim-api':{'condition':'service_started'},'chat-api':{'condition':'service_started'}}}
 for service in services.values():service['networks']=['openim38'];service['pull_policy']='never';service['logging']={'driver':'json-file','options':{'max-size':'20m','max-file':'3'}}
 dump(destination/'compose.yaml',harden_storage_process_arguments({'name':'openim38-upgrade','services':services,'networks':{'openim38':{'name':'openim38-upgrade-net','driver':'bridge'}},'volumes':{name:{} for name in ['mongo-data','redis-data','etcd-data','kafka-data','minio-data']}}))
 shutil.copy2(ROOT/'deploy/nginx.conf',destination/'config/nginx.conf')
 for name in ['certs','www/admin','bin']:(destination/name).mkdir(parents=True,exist_ok=True)
 (destination/'instance.json').write_text(json.dumps({'host':host,'testOnly':True,'stackName':'openim38-upgrade','originalDataMigrated':False},indent=2)+'\n')
 print(json.dumps({'instanceGenerated':str(destination),'secretsPrinted':False}))

if __name__=='__main__':
 parser=argparse.ArgumentParser();parser.add_argument('--output',type=pathlib.Path,default=ROOT/'.private/instance');parser.add_argument('--host',default='192.168.10.11');args=parser.parse_args();generate(args.output,args.host)
