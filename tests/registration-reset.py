"""Verify synthetic SMTP, registration and forgotten-password revocation.

Runs only in /srv/openim38-upgrade, never sends external mail or logs tokens.
"""
import hashlib,json,os,pathlib,secrets,ssl,time,urllib.error,urllib.request
ROOT=pathlib.Path(__file__).resolve().parents[1]
assert ROOT==pathlib.Path('/srv/openim38-upgrade')
TLS=ssl.create_default_context(cafile=str(ROOT/'certs/ca.crt'));BASE='https://192.168.10.11:29880'
HTTP=urllib.request.build_opener(urllib.request.ProxyHandler({}),urllib.request.HTTPSHandler(context=TLS))
def md5(value):return hashlib.md5(value.encode()).hexdigest()
def call(path,body=None,session=None,success=True):
 headers={'Content-Type':'application/json','operationID':secrets.token_hex(12)}
 if session:headers.update(token=session['imToken'] if path.startswith('/enterprise/') or path.startswith('/api/') else session['chatToken'],chatToken=session['chatToken'],platformID='3')
 request=urllib.request.Request(BASE+path,None if body is None else json.dumps(body).encode(),headers)
 try:
  with HTTP.open(request,timeout=30) as response:status=response.status;result=json.load(response)
 except urllib.error.HTTPError as error:
  status=error.code;raw=error.read()
  try:result=json.loads(raw)
  except ValueError:raise RuntimeError(path.split('?')[0]+' HTTP '+str(status)+' non-JSON response') from None
 accepted=status<400 and result.get('errCode')==0
 if accepted!=success:raise RuntimeError(path+' unexpected acceptance: '+str(status)+' '+str(result.get('errCode'))+' '+result.get('errMsg',''))
 return result.get('data') or {}
def code_for(address,used_for):
 before=time.time();call('/chat/account/code/send',{'email':address,'usedFor':used_for,'platform':3})
 for _ in range(60):
  value=json.loads((ROOT/'.private/smtp-mailbox.json').read_text()) if (ROOT/'.private/smtp-mailbox.json').exists() else {}
  if value.get(address,{}).get('receivedAt',0)>=before:return value[address]['code']
  time.sleep(.25)
 raise RuntimeError('SMTP fixture did not receive verification code')
address='register'+secrets.token_hex(6)+'@openim38.test';password=secrets.token_urlsafe(20)+'Aa9'
verification=code_for(address,1)
user=call('/chat/account/register',{'platform':3,'deviceID':'upgrade38-register-test','verifyCode':verification,'autoLogin':True,'user':{'email':address,'password':md5(password),'nickname':'合成注册找回密码验收','gender':1}})
assert user.get('imToken') and user.get('chatToken')
cases=[{'case':'Synthetic SMTP delivered code; public register auto-login returns Chat+IM','passed':True}]
meeting=call('/enterprise/v1/signaling/signal_create_meeting',{'meetingName':'forgot-password revocation fixture','startTime':int(time.time()),'meetingDuration':300},user)
new_password=secrets.token_urlsafe(22)+'Aa8';verification=code_for(address,2)
call('/chat/account/password/reset',{'email':address,'verifyCode':'wrong-code','password':md5(new_password)},success=False)
call('/chat/account/password/reset',{'email':address,'verifyCode':verification,'password':md5(new_password)})
call('/chat/user/find/full',{'userIDs':[user['userID']]},user,success=False)
call('/api/user/get_users_info',{'userIDs':[user['userID']]},user,success=False)
call('/enterprise/v1/office/tag/find/user',{},user,success=False)
call('/livekit/rtc/validate?access_token='+meeting['token'],success=False)
call('/chat/account/login',{'email':address,'password':md5(password),'platform':3,'deviceID':'old-password'},success=False)
current=call('/chat/account/login',{'email':address,'password':md5(new_password),'platform':3,'deviceID':'reset-password'})
cases.append({'case':'Forgotten-password reset rejects wrong code; revokes old Chat/IM/enterprise/media; new password works','passed':True})
call('/enterprise/v1/signaling/signal_close_room',{'roomID':meeting['roomID']},current)
# Public change endpoint also allows omission of userID; its authenticated
# identity must still be passed to the IM revocation operation.
call('/chat/account/password/change',{'currentPassword':md5(new_password),'newPassword':md5(password)},current)
call('/api/user/get_users_info',{'userIDs':[user['userID']]},current,success=False)
call('/chat/user/find/full',{'userIDs':[user['userID']]},current,success=False)
fresh=call('/chat/account/login',{'email':address,'password':md5(password),'platform':3,'deviceID':'restored-fixture'})
cases.append({'case':'Change password without userID revokes the authenticated account and permits fresh login','passed':True})
private=ROOT/'.private/registration-fixture.json';private.write_text(json.dumps({'email':address,'password':password,**fresh})+'\n');private.chmod(0o600)
(ROOT/'logs/registration-reset-report.json').write_text(json.dumps({'timeUTC':time.strftime('%Y-%m-%dT%H:%M:%SZ',time.gmtime()),'syntheticOnly':True,'cases':cases},indent=2)+'\n')
print(json.dumps({'passedCases':len(cases),'allPassed':True,'secretsPrinted':False}))
