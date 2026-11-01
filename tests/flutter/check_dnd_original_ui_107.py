import json,ssl,subprocess,time,urllib.request,hashlib,xml.etree.ElementTree as ET
from pathlib import Path
root=Path('O:/')
adb=['E:/AndroidDev/sdk/platform-tools/adb.exe','-s','emulator-5588']
pkg='io.openim.upgrade38.test.dev'
raw=subprocess.check_output(adb+['shell','cat',f'/data/user/0/{pkg}/shared_prefs/FlutterSharedPreferences.xml'])
cert=json.loads({n.attrib['name']:n.text for n in ET.fromstring(raw)}['flutter.loginCertificate'])
assert cert['userID']=='upgrade38_u104'
ctx=ssl.create_default_context(cafile=str(root/'artifacts/dev-cert/OpenIM38-Development-CA.crt'))
headers={'token':cert['imToken'],'Content-Type':'application/json','operationID':'android107-partial-dnd'}
def query():
 req=urllib.request.Request('https://192.168.10.11:29880/api/user/get_users_info',data=json.dumps({'userIDs':[cert['userID']]}).encode(),headers=headers)
 with urllib.request.urlopen(req,context=ctx,timeout=15) as r: response=json.load(r)
 assert response['errCode']==0
 return response['data']['usersInfo'][0]['globalRecvMsgOpt']
def tap():
 subprocess.run(adb+['shell','input','tap','478','150'],check=True,stdout=subprocess.DEVNULL)
 time.sleep(2)
def shot(label):
 target=root/'tests/flutter'/f'android-107-dnd-{label}.png'
 assert not target.exists()
 subprocess.run(adb+['shell','screencap','-p','/sdcard/dnd.png'],check=True,stdout=subprocess.DEVNULL)
 subprocess.run(adb+['pull','/sdcard/dnd.png',str(target)],check=True,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
 return {'path':str(target.relative_to(root)),'sha256':hashlib.sha256(target.read_bytes()).hexdigest()}
report={'appVersion':'3.8.7+107','source':'Original Account Setup UI switch; normal-TLS IM API only observes persisted value','scope':'Partial non-media function on failed RTC candidate; not upgrade acceptance','userID':'upgrade38_u104','values':[],'screenshots':[]}
initial=query();assert initial==0,initial
report['values'].append(initial)
try:
 report['screenshots'].append(shot('before'))
 tap();report['values'].append(query());report['screenshots'].append(shot('enabled'))
 assert report['values'][-1]==2
finally:
 if query()!=initial: tap()
 report['values'].append(query());report['screenshots'].append(shot('restored'))
 report['restored']=report['values'][-1]==initial
 report['passed']=report['values']==[0,2,0]
 (root/'tests/flutter/android-107-dnd-original-ui.json').write_text(json.dumps(report,indent=2)+'\n')
print(json.dumps({'passed':report['passed'],'values':report['values'],'restored':report['restored']}))

