"""Create only synthetic fixture friendships through official IM APIs.

Run on the isolated server with Docker access. Never prints management tokens.
"""
import json,pathlib,secrets,subprocess,urllib.request
ROOT=pathlib.Path(__file__).resolve().parents[1]
if ROOT!=pathlib.Path('/srv/openim38-upgrade'):raise SystemExit('Isolated server only')
def endpoint():
 ip=subprocess.check_output(['docker','inspect','-f','{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}','openim38-upgrade-openim-api-1'],text=True).strip()
 return f'http://{ip}:10002'
BASE=endpoint()
def call(path,body,token=''):
 req=urllib.request.Request(BASE+path,json.dumps(body).encode(),{'Content-Type':'application/json','operationID':secrets.token_hex(12),'token':token})
 with urllib.request.urlopen(req,timeout=30) as response:value=json.load(response)
 if value.get('errCode')!=0:raise RuntimeError(path+': '+str(value.get('errCode'))+' '+value.get('errMsg',''))
 return value.get('data') or {}
keys=json.loads((ROOT/'secrets.json').read_text())
token=call('/auth/get_admin_token',{'secret':keys['im'],'userID':'imAdmin38'})['token']
users=json.loads((ROOT/'.private/test-sessions.json').read_text())['users']
for i,user in enumerate(users[:100]):
 call('/friend/import_friend',{'ownerUserID':user['userID'],'friendUserIDs':[users[(i+1)%100]['userID']]},token)
for user in users[100:107]:
 call('/friend/import_friend',{'ownerUserID':user['userID'],'friendUserIDs':[other['userID'] for other in users[100:107] if other['userID']!=user['userID']]},token)
print(json.dumps({'syntheticRingFriends':100,'interactiveFriends':7,'tokensPrinted':False}))
