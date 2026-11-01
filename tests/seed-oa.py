"""Create a synthetic OA notification account and send native contentType 1400."""
import hashlib,json,pathlib,secrets,ssl,urllib.request
ROOT=pathlib.Path(__file__).resolve().parents[1];BASE='https://192.168.10.11:29880'
TLS=ssl.create_default_context(cafile=str(ROOT/'artifacts/dev-cert/OpenIM38-Development-CA.crt'))
def call(path,data,token=''):
 request=urllib.request.Request(BASE+path,json.dumps(data).encode(),{'Content-Type':'application/json','operationID':secrets.token_hex(12),'token':token,'chatToken':token})
 with urllib.request.urlopen(request,context=TLS,timeout=30) as response:value=json.load(response)
 if value.get('errCode')!=0:raise RuntimeError(path+': '+str(value.get('errCode'))+' '+value.get('errMsg',''))
 return value.get('data') or {}
state=json.loads((ROOT/'.private/test-accounts.json').read_text(encoding='utf-8'))
admin=call('/enterprise/v1/admin/login',{'account':state['admin']['account'],'password':hashlib.md5(state['admin']['password'].encode()).hexdigest()})['adminToken']
account='upgrade38_oa_fixture'
try:call('/enterprise/v1/admin/im/user/add_notification_account',{'userID':account,'nickName':'升级测试OA通知','appMangerLevel':3},admin)
except RuntimeError as error:
 if 'exist' not in str(error).lower():raise
results=[]
for user in state['users'][100:107]:
 value=call('/enterprise/v1/admin/im/msg/send_msg',{'sendID':account,'recvID':user['userID'],'senderNickname':'升级测试OA通知','senderPlatformID':10,'content':{'notificationName':'升级测试OA通知','notificationType':1,'mixType':0,'text':'OpenIM38 OA notification fixture 1400','url':BASE+'/admin/'},'contentType':1400,'sessionType':4,'isOnlineOnly':False,'notOfflinePush':True,'offlinePushInfo':{'title':'合成OA验收','desc':'合成OA验收'}},admin)
 results.append({'userID':user['userID'],'clientMsgID':value.get('clientMsgID'),'serverMsgID':value.get('serverMsgID')})
(ROOT/'tests/oa-api-report.json').write_text(json.dumps({'acceptedByOfficialAPI':True,'notificationAccount':account,'contentType':1400,'results':results,'sdkRenderingVerified':False},indent=2)+'\n')
print(json.dumps({'notificationsAccepted':len(results),'requiresSDKRendering':True}))
