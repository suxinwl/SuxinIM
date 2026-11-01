"""Snapshot/check only this synthetic AVD session; never print or export its tokens."""
import json,ssl,subprocess,sys,urllib.request,urllib.error,xml.etree.ElementTree as ET
from pathlib import Path
ROOT=Path(__file__).absolute().parents[2]
ADB=['E:/AndroidDev/sdk/platform-tools/adb.exe','-s','emulator-5588']
package='io.openim.upgrade38.test.dev'
private=ROOT/'.private/android-ui-session.json'
if sys.argv[1]=='snapshot':
    xml=subprocess.check_output(ADB+['shell','cat',f'/data/user/0/{package}/shared_prefs/FlutterSharedPreferences.xml'])
    data={n.attrib['name']:n.text for n in ET.fromstring(xml)}
    cert=json.loads(data['flutter.loginCertificate'])
    assert cert['userID']=='upgrade38_u104'
    private.write_text(json.dumps(cert),encoding='utf-8')
    print('Synthetic104 original UI current session snapshot stored privately')
else:
    cert=json.loads(private.read_text())
    context=ssl.create_default_context(cafile=str(ROOT/'artifacts/dev-cert/OpenIM38-Development-CA.crt'))
    headers={'token':cert['imToken'],'chatToken':cert['chatToken'],'platformID':'2','operationID':'native-original-ui-logout'}
    def request(path,data=None):
        req=urllib.request.Request('https://192.168.10.11:29880'+path,
            data=None if data is None else json.dumps(data).encode(),
            headers={**headers,'Content-Type':'application/json'})
        try:
            with urllib.request.urlopen(req,context=context,timeout=15) as response:
                body=json.load(response);return {'status':response.status,'errCode':body.get('errCode')}
        except urllib.error.HTTPError as error:
            return {'status':error.code}
    result={'enterprise':request('/enterprise/v1/rtc/events?after=-1'),
            'im':request('/api/user/get_users_info',{'userIDs':[cert['userID']]}),
            'caValidationBypass':False,'user':'upgrade38_u104','source':'original main UI Log Out'}
    xml=subprocess.check_output(ADB+['shell','cat',f'/data/user/0/{package}/shared_prefs/FlutterSharedPreferences.xml'])
    result['localCertificateRemoved']=not any(n.attrib.get('name')=='flutter.loginCertificate' for n in ET.fromstring(xml))
    result['passed']=result['localCertificateRemoved'] and all(
        x['status'] in (401,403) or x.get('errCode',0)!=0 for x in [result['enterprise'],result['im']])
    (ROOT/'tests/flutter/original-ui-logout-report.json').write_text(json.dumps(result,indent=2)+'\n')
    print(json.dumps(result))
    if not result['passed']:raise SystemExit(1)
