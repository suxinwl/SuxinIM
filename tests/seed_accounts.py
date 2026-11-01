"""Create synthetic fixtures through official APIs in the isolated stack only."""
import hashlib,json,os,pathlib,secrets,ssl,subprocess,time,urllib.error,urllib.request
ROOT=pathlib.Path(__file__).resolve().parents[1]
BASE='https://192.168.10.11:29880'
INTERNAL=os.environ.get('OPENIM38_INTERNAL')=='1'
CONTEXT=None if INTERNAL else ssl.create_default_context(cafile=str(ROOT/'artifacts/dev-cert/OpenIM38-Development-CA.crt'))
STATE=ROOT/'.private/test-accounts.json'
def md5(value):return hashlib.md5(value.encode()).hexdigest()
def call(path,data,token=None):
 headers={'Content-Type':'application/json','operationID':secrets.token_hex(12)}
 if token:headers.update(token=token,chatToken=token)
 address=BASE+path
 if INTERNAL:
  if path.startswith('/enterprise/v1/admin/'):
   component='admin-api';port=10009;suffix=path.removeprefix('/enterprise/v1/admin/chat/') if path!='/enterprise/v1/admin/login' else 'account/login'
  elif path.startswith('/chat/'):
   component='chat-api';port=10008;suffix=path.removeprefix('/chat/')
  else:raise RuntimeError('Not an approved synthetic account endpoint')
  container='openim38-upgrade-'+component+'-1'
  ip=subprocess.check_output(['docker','inspect','-f','{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}',container],text=True).strip()
  address=f'http://{ip}:{port}/'+suffix
 request=urllib.request.Request(address,json.dumps(data).encode(),headers)
 try:
  with urllib.request.urlopen(request,context=CONTEXT,timeout=45) as response:result=json.load(response)
 except urllib.error.HTTPError as error:
  payload=error.read().decode(errors='replace');raise RuntimeError(path+' HTTP '+str(error.code)+' '+payload[:800]) from None
 if result.get('errCode')!=0:raise RuntimeError(path+' rejected: '+str(result.get('errCode'))+' '+result.get('errMsg',''))
 value=result.get('data') or {}
 if INTERNAL and path=='/enterprise/v1/admin/login':value.pop('imToken',None)
 return value
def main():
 STATE.parent.mkdir(parents=True,exist_ok=True)
 if STATE.exists():state=json.loads(STATE.read_text())
 else:
  password=secrets.token_urlsafe(18)+'aA7'
  state={'baseURL':BASE,'admin':{'account':'chatAdmin38','password':secrets.token_urlsafe(24)},'password':password,'users':[{'userID':f'upgrade38_u{i:03}','account':f'im38u{i:03}','email':f'u{i:03}@openim38.test','nickname':f'升级测试{i:03}'} for i in range(1,111)]}
  STATE.write_text(json.dumps(state,ensure_ascii=False,indent=2)+'\n',encoding='utf-8');STATE.chmod(0o600)
 admin=state['admin']
 # The official Chat account validator allows letters and digits only.
 for user in state['users']:user['account']=user['account'].replace('_','')
 try:login=call('/enterprise/v1/admin/login',{'account':admin['account'],'password':md5(admin['password'])})
 except RuntimeError:
  login=call('/enterprise/v1/admin/login',{'account':admin['account'],'password':md5(admin['account'])})
  call('/enterprise/v1/admin/chat/account/change_password',{'userID':login['adminUserID'],'currentPassword':md5(admin['account']),'newPassword':md5(admin['password'])},login['adminToken'])
  login=call('/enterprise/v1/admin/login',{'account':admin['account'],'password':md5(admin['password'])})
 if login.get('imToken'):raise RuntimeError('Management credential escaped backend boundary')
 state['adminSession']=login
 STATE.write_text(json.dumps(state,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
 sessions=[]
 for index,user in enumerate(state['users']):
  try:
   call('/enterprise/v1/admin/chat/account/add_user',{'platform':5,'deviceID':'openim38-fixture','user':{**user,'password':md5(state['password']),'RegisterType':1}},login['adminToken'])
  except RuntimeError as error:
   # Existing fixtures are accepted ONLY after an actual credential login below.
   if 'exist' not in str(error).lower() and 'registered' not in str(error).lower():raise
  token=call('/chat/account/login',{'email':user['email'],'password':md5(state['password']),'platform':5,'deviceID':'openim38-load-'+str(index)})
  if not token.get('imToken') or not token.get('chatToken'):raise RuntimeError('Incomplete fixture login')
  sessions.append({**user,**token,'platformID':5})
  if (index+1)%10==0:print(json.dumps({'syntheticAccountsReady':index+1}),flush=True)
 (ROOT/'.private/test-sessions.json').write_text(json.dumps({'baseURL':BASE,'users':sessions},ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
 print(json.dumps({'fixtures':len(sessions),'tokensPrinted':False}),flush=True)
if __name__=='__main__':main()
