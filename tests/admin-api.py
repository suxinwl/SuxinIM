"""Real isolated admin contracts, negative authorization, multipart integrity."""
import hashlib,json,pathlib,secrets,ssl,urllib.error,urllib.parse,urllib.request
ROOT=pathlib.Path(__file__).resolve().parents[1]
BASE='https://192.168.10.11:29880'
TLS=ssl.create_default_context(cafile=str(ROOT/'artifacts/dev-cert/OpenIM38-Development-CA.crt'))
def request(path,body,token='',method='POST',raw=False,headers=None):
 h={'Content-Type':'application/json','operationID':secrets.token_hex(12),'token':token,'chatToken':token};h.update(headers or {})
 data=body if isinstance(body,bytes) else None if body is None else json.dumps(body).encode()
 req=urllib.request.Request(path if path.startswith('https://') else BASE+path,data,h,method=method)
 try:
  with urllib.request.urlopen(req,context=TLS,timeout=30) as response:payload=response.read();status=response.status
 except urllib.error.HTTPError as e:payload=e.read();status=e.code
 if raw:return status,payload
 value=json.loads(payload)
 if status>=400 or value.get('errCode')!=0:raise RuntimeError(f'{path}: {status}, {value.get("errCode")}, {value.get("errMsg")}')
 return value.get('data') or {}
state=json.loads((ROOT/'.private/test-accounts.json').read_text(encoding='utf-8'))
session=request('/enterprise/v1/admin/login',{'account':state['admin']['account'],'password':hashlib.md5(state['admin']['password'].encode()).hexdigest()})
assert not session.get('imToken')
token=session['adminToken'];api='/enterprise/v1/admin/im/';chat='/enterprise/v1/admin/chat/';business='/enterprise/v1/admin/business/'
cases=[]
for prefix in (api,chat,business):
 for route in ('config/get_config','config/set_config','restart'):
  status,_=request(prefix+route,{},token,raw=True)
  assert status==403,(prefix,route,status)
cases.append({'case':'All admin proxies deny operational configuration and restart routes','passed':True})
for route in ('/enterprise/v1/admin/chat/account/./login','/enterprise/v1/admin/chat//account/login/'):
 login=request(route,{'account':state['admin']['account'],'password':hashlib.md5(state['admin']['password'].encode()).hexdigest()})
 assert login.get('adminToken') and not login.get('imToken')
cases.append({'case':'Canonicalized admin login aliases do not expose IM administrator credentials','passed':True})
# A structural check on both official online-status response formats.
def no_credentials(value):
 if isinstance(value,dict):
  for key,item in value.items():
   normalized=''.join(c for c in key.lower() if c.isalnum())
   assert normalized not in {'token','tokens','imtoken','chattoken','refreshtoken','tokenstates'},key
   no_credentials(item)
 elif isinstance(value,list):
  for item in value:no_credentials(item)
for route in ('user/get_users_online_token_detail','user/get_users_online_status'):
 online=request(api+route,{'userIDs':[state['users'][i]['userID'] for i in (0,100,107)]},token)
 no_credentials(online)
cases.append({'case':'Both online-status APIs redact token fields and token-keyed maps','passed':True})
users=request(business+'user/search/full',{'keyword':'','normal':1,'genders':0,'pagination':{'pageNumber':1,'showNumber':20}},token)
assert users.get('total',0)>=110;cases.append({'case':'Chat user search scalar genders and pagination','passed':True})
bad_status,bad_body=request(chat+'account/add_user',{'platform':5,'user':{'userID':'bad-fixture','account':'invalid_account','email':'bad@openim38.test','nickname':'invalid','password':hashlib.md5(b'unused').hexdigest()}},token,raw=True)
bad=json.loads(bad_body);assert bad.get('errCode')!=0;cases.append({'case':'Invalid account returns JSON error instead of silent empty HTTP200','passed':True})
# Same modern multipart contract as the admin uploader, including signed headers/query.
content=secrets.token_bytes(2*1024*1024+31);content_hash=hashlib.md5(content).hexdigest();name='imAdmin38/admin-api-fixture-'+secrets.token_hex(8)+'.bin'
init=request(api+'object/initiate_multipart_upload',{'hash':hashlib.md5(content_hash.encode()).hexdigest(),'size':len(content),'maxParts':1,'cause':'test-fixture','name':name,'contentType':'application/octet-stream'},token)
upload=init['upload'];signed=upload.get('sign') or request(api+'object/auth_sign',{'uploadID':upload['uploadID'],'partNumbers':[1]},token);part=signed['parts'][0]
url=urllib.parse.urlsplit(part.get('url') or signed['url']);query=dict(urllib.parse.parse_qs(url.query))
for item in (signed.get('query') or {},part.get('query') or {}):query.update(item)
put=urllib.parse.urlunsplit(url._replace(query=urllib.parse.urlencode(query,doseq=True)))
headers={key:','.join(values) for item in (signed.get('header') or {},part.get('header') or {}) for key,values in item.items()}
status,_=request(put,content,method='PUT',raw=True,headers=headers);assert status in (200,204)
finished=request(api+'object/complete_multipart_upload',{'uploadID':upload['uploadID'],'parts':[content_hash],'name':name,'contentType':'application/octet-stream'},token)
assert finished['url'].startswith(BASE+'/api/object/'),finished['url']
status,downloaded=request(finished['url'],None,method='GET',raw=True)
assert status==200 and hashlib.sha256(downloaded).digest()==hashlib.sha256(content).digest()
cases.append({'case':'Signed multipart upload + HTTPS API redirect + attachment SHA256 integrity','bytes':len(content),'passed':True})
for origin in ['http://127.0.0.1:7777','openim38://app','null']:
 req=urllib.request.Request(finished['url'],headers={'Origin':origin})
 with urllib.request.urlopen(req,context=TLS,timeout=30) as response:
  cors=response.headers.get_all('Access-Control-Allow-Origin')
  corp=response.headers.get_all('Cross-Origin-Resource-Policy')
  assert cors==[origin] and corp==['cross-origin'],{'origin':origin,'cors':cors,'corp':corp}
 cases.append({'case':'Attachment cross-origin TLS GET with exact CORS and CORP','origin':origin,'passed':True})
(ROOT/'tests/admin/api-report.json').write_text(json.dumps({'cases':cases},indent=2)+'\n')
print(json.dumps({'passedCases':len(cases),'allPassed':True}))
