"""Small API30 UI driver; fixture values enter adb stdin, never tool argv/logs."""
import argparse
import json
import pathlib
import re
import shlex
import subprocess
import xml.etree.ElementTree as ET

ROOT = pathlib.Path(__file__).resolve().parents[3]
ADB = pathlib.Path('E:/AndroidDev/sdk/platform-tools/adb.exe')
SERIAL = 'emulator-5582'
EVIDENCE = ROOT / '.private/production/acceptance/organization-android-final'

def adb(*args, stdin=None):
    result = subprocess.run([str(ADB), '-s', SERIAL, *args], input=stdin, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=45)
    if result.returncode: raise RuntimeError('ADB_OPERATION_REJECTED')
    return result.stdout

def fixture():
    data = json.loads((ROOT / '.private/production/acceptance/organization-test-accounts.json').read_text(encoding='utf-8-sig'))
    if data['namespace'] != 'organization-20261103' or len(data['accounts']) != 2: raise ValueError('Fixture namespace rejected')
    return data

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('action', choices=['snapshot', 'input', 'tap', 'key'])
    parser.add_argument('value')
    parser.add_argument('second', nargs='?')
    args = parser.parse_args()
    EVIDENCE.mkdir(parents=True, exist_ok=True)
    if args.action == 'input':
        people = fixture()['accounts']
        values = {'account': people[0]['username'], 'password':people[0]['password'], 'phone':people[1]['mobile'], 'name':people[1]['name'], 'employee_no':people[1]['username'], 'marker':'puty-organization-android-final-20261103'}
        if args.value == 'captcha' and args.second and re.fullmatch('-?[0-9]{1,4}', args.second): values['captcha'] = args.second
        if args.value not in values: raise ValueError('Unsupported input field')
        value = str(values[args.value])
        if any(ord(c) < 32 for c in value): raise ValueError('Unexpected control character')
        # No remote interactive TTY echo; the adb process argv contains no secret.
        adb('shell', '-T', stdin=('input text ' + shlex.quote(value.replace(' ', '%s')) + '\nexit\n').encode())
        print(json.dumps({'inputField':args.value,'valuePrinted':False}))
    elif args.action == 'tap':
        x, y = int(args.value), int(args.second)
        if not 0 <= x <= 4000 or not 0 <= y <= 4000: raise ValueError('Unexpected coordinate')
        adb('shell', 'input', 'tap', str(x), str(y))
        print(json.dumps({'tap':[x,y]}))
    elif args.action == 'key':
        if args.value == 'CLEAR':
            adb('shell','input','keyevent','KEYCODE_MOVE_END',*(['KEYCODE_DEL']*64))
            print(json.dumps({'key':'CLEAR'}))
            return
        if args.value not in ('BACK','ENTER','MOVE_END','DEL'): raise ValueError('Unsupported key')
        adb('shell', 'input', 'keyevent', 'KEYCODE_'+args.value)
        print(json.dumps({'key':args.value}))
    else:
        if not re.fullmatch('[a-z0-9-]{1,48}',args.value): raise ValueError('Unexpected evidence name')
        adb('shell','uiautomator','dump','/sdcard/puty-org-window.xml')
        xml = adb('exec-out','cat','/sdcard/puty-org-window.xml')
        (EVIDENCE/(args.value+'.xml')).write_bytes(xml)
        png = adb('exec-out','screencap','-p')
        output = EVIDENCE/(args.value+'.png')
        output.write_bytes(png)
        nodes = list(ET.fromstring(xml).iter('node'))
        summaries=[]
        safe_names={'登录','密码','验证码','组织架构','消息','通讯录','发现','我','我的','手机号/邮箱/账号','统一账号 / 工号','统一账号/工号','请输入统一账号','请输入密码','发送消息','发消息','添加好友','全部同事','搜索手机号、姓名或工号','搜索','确认','确定','取消','退出登录','设置','刷新组织架构','搜索同事','重试','发送','返回上级部门'}
        for node in nodes:
            text=node.get('text','') or node.get('content-desc','')
            if node.get('class')=='android.widget.EditText': summaries.append({'type':'input','bounds':node.get('bounds'),'password':node.get('password')})
            elif text in safe_names: summaries.append({'label':text,'bounds':node.get('bounds')})
        people = fixture()['accounts']
        # The entered employee name itself is not a result row.
        labels = [node.get('text','')+' '+node.get('content-desc','') for node in nodes if node.get('class') != 'android.widget.EditText']
        proof = {'screenshot':str(output),'safeUI':summaries,'actorRecordVisible':any(people[0]['name'] in label for label in labels),'counterpartRecordVisible':any(people[1]['name'] in label for label in labels)}
        (EVIDENCE/(args.value+'-summary.json')).write_text(json.dumps(proof,ensure_ascii=True,indent=2),encoding='utf-8')
        print(json.dumps(proof,ensure_ascii=True))

if __name__ == '__main__': main()
