"""Temporarily enable four callbacks in the fixed isolated stack, or restore them.

Run on the test server as root; emits no callback URL or instance credential.
"""
import argparse,datetime,hashlib,json,pathlib,re,subprocess
ROOT=pathlib.Path('/srv/openim38-upgrade')
parser=argparse.ArgumentParser();parser.add_argument('action',choices=['enable','restore','events','status']);args=parser.parse_args()
if ROOT.resolve()!=ROOT or not (ROOT/'compose.yaml').is_file():raise SystemExit('Wrong isolated root')
conf=ROOT/'config/openim-server/webhooks.yml';save=ROOT/'.private/optional-webhook-window.json'
names=['afterUserOnline','afterUserOffline','beforeOfflinePush','afterSingleMsgRead']
commands=['callbackAfterUserOnlineCommand','callbackAfterUserOfflineCommand','callbackBeforeOfflinePushCommand','callbackAfterSingleMsgReadCommand']
compose=['docker','compose','-p','openim38-upgrade','--project-directory',str(ROOT),'-f',str(ROOT/'compose.yaml')]
def digest(raw):return hashlib.sha256(raw.encode()).hexdigest()
if args.action=='status':
 text=conf.read_text();flags={}
 for name in names+['beforeSendGroupMsg','afterSendGroupMsg','afterMsgSaveDB']:
  match=re.search(r'(?m)^'+name+r':\n[ \t]+enable:[ \t]*(true|false)[ \t]*$',text)
  if not match:raise SystemExit('Missing callback setting: '+name)
  flags[name]=match.group(1)=='true'
 report={'utc':datetime.datetime.now(datetime.timezone.utc).isoformat(),'callbackFlags':flags,'configSHA256':digest(text),'temporaryWindowAbsent':not save.exists(),'onlyIsolatedStack':True}
 (ROOT/'logs/optional-webhook-restored.json').write_text(json.dumps(report,indent=2)+'\n');print(json.dumps(report));raise SystemExit(0)
if args.action=='enable':
 if save.exists():raise SystemExit('Window already exists; restore before enabling again')
 text=conf.read_text();original=text
 for name in names:
  text,count=re.subn(r'(?m)^('+name+r':\n[ \t]+enable:)[ \t]*false[ \t]*$',r'\1 true',text)
  if count!=1:raise SystemExit('Expected disabled callback: '+name)
 state={'original':original,'enabledSHA256':digest(text),'since':datetime.datetime.now(datetime.timezone.utc).isoformat()}
 save.write_text(json.dumps(state));save.chmod(0o600);conf.write_text(text)
elif args.action=='restore':
 state=json.loads(save.read_text())
 if digest(conf.read_text())!=state['enabledSHA256']:raise SystemExit('Callback config changed concurrently; refusing overwrite')
 conf.write_text(state['original']);save.unlink()
else:
 state=json.loads(save.read_text())
 query='const dbx=db.getSiblingDB("enterprise38");const rows=dbx.enterprise_documents.find({kind:"webhook",updated:{$gte:ISODate('+json.dumps(state['since'])+')}}).toArray();print(JSON.stringify(rows.filter(r=>'+json.dumps(commands)+'.includes(r.owner)).map(r=>{const raw=EJSON.serialize(r.data);const d=JSON.parse(typeof r.data==="string"?r.data:Buffer.from(raw.$binary.base64,"base64").toString());return {command:r.owner,updated:r.updated,userID:d.userID,userIDList:d.userIDList,platformID:d.platformID,conversationID:d.conversationID,clientMsgID:d.clientMsgID,sendID:d.sendID,seqs:d.Seqs||d.seqs||d.readSeqs,connID:d.connID};})));'
 shell='mongosh --quiet -u "$MONGO_INITDB_ROOT_USERNAME" -p "$MONGO_INITDB_ROOT_PASSWORD" --authenticationDatabase admin --file /dev/stdin'
 result=subprocess.run(compose+['exec','-T','mongo','sh','-c',shell],input=query,text=True,capture_output=True)
 if result.returncode:raise SystemExit('Mongo callback query failed: '+result.stderr[-1400:])
 rows=json.loads(result.stdout.strip());report={'since':state['since'],'events':rows,'counts':{name:sum(r['command']==name for r in rows) for name in commands}}
 (ROOT/'logs/optional-webhook-events.json').write_text(json.dumps(report,indent=2,default=str)+'\n');print(json.dumps(report));raise SystemExit(0)
subprocess.run(compose+['restart','openim-msggateway','openim-rpc-msg','openim-push'],check=True)
print(json.dumps({'action':args.action,'callbacks':names,'onlyIsolatedStack':True}))
