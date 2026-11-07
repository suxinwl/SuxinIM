"""Exercise production HTTPS using normal HRM authentication; keep tokens private."""
import argparse
import getpass
import base64
import hashlib
import json
import pathlib
import re
import ssl
import time
import urllib.error
import urllib.request

from cryptography.hazmat.primitives import padding
from cryptography.hazmat.primitives.ciphers import Cipher, algorithms, modes

ROOT = pathlib.Path(__file__).resolve().parents[1]
SOURCE = pathlib.Path('E:/PUTY_AI/kaoqinstyem/gosuxinwl')
PRIVATE = ROOT / '.private/production/acceptance'
PRIVATE.mkdir(parents=True, exist_ok=True)
ORIGIN = 'https://192.168.10.11:9880'
context = ssl.create_default_context(cafile=str(ROOT / 'deploy/certificates/Puty-Enterprise-Root-CA.crt'))
ENV = (SOURCE / 'web/.env').read_text(encoding='utf-8')
VERIFY = re.search(r'VITE_ENCRYPT\s*=\s*([^\r\n]+)', ENV)[1].strip().strip("'\"")
CRYPTO = (SOURCE / 'web/src/utils/CryptoJS.ts').read_text(encoding='utf-8')
KEY, IV = [s.encode() for s in re.findall(r"Utf8.parse\('([^']+)'\)", CRYPTO)[:2]]


def crypt(value, decrypt=False):
    c = Cipher(algorithms.AES(KEY), modes.CBC(IV))
    if decrypt:
        d = c.decryptor()
        raw = d.update(base64.b64decode(value)) + d.finalize()
        p = padding.PKCS7(128).unpadder()
        return (p.update(raw) + p.finalize()).decode()
    p = padding.PKCS7(128).padder()
    raw = p.update(value.encode()) + p.finalize()
    e = c.encryptor()
    return base64.b64encode(e.update(raw) + e.finalize()).decode()


def request(path, method='GET', data=None, auth=True):
    timestamp = str(int(time.time()))
    headers = {'apiverify': base64.b64encode((hashlib.md5((VERIFY + timestamp).encode()).hexdigest()+'#'+timestamp).encode()).decode(), 'locale':'zh-CN'}
    state = PRIVATE/'hrm-session.json'
    if auth and state.exists():
        headers['Authorization'] = crypt(json.loads(state.read_text())['token']+'#'+timestamp)
    if data is not None:
        headers['Content-Type'] = 'application/json'
    req = urllib.request.Request(ORIGIN+path, data=json.dumps(data).encode() if data is not None else None, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, context=context, timeout=90) as response:
            body=response.read()
            return response.status, json.loads(body)
    except urllib.error.HTTPError as e:
        return e.code, json.loads(e.read())


def main():
    parser=argparse.ArgumentParser()
    parser.add_argument('action', choices=['captcha','login','request','credentials'])
    parser.add_argument('--captcha')
    parser.add_argument('--path', default='/admin/user/getUserinfo')
    parser.add_argument('--method', default='GET')
    parser.add_argument('--json-file')
    parser.add_argument('--output')
    a=parser.parse_args()
    if a.action=='credentials':
        from production_backup import protect
        target=ROOT/'.private/production/admin-login.json'
        value=json.loads(target.read_text(encoding='utf-8'))
        password=getpass.getpass('Current HRM admin password (private file only): ')
        assert password and value.get('username')
        value['password']=password
        target.write_text(json.dumps(value,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
        protect(target)
        print(json.dumps({'privateCredentialsUpdated':True,'passwordPrinted':False}))
    elif a.action=='captcha':
        status, body=request('/common/basetool/getCaptcha?type=image',auth=False)
        assert body['code']==0, body.get('message')
        d=body['data']
        (PRIVATE/'captcha.json').write_text(json.dumps(d))
        (PRIVATE/'captcha.png').write_bytes(base64.b64decode(d['img'].split(',')[-1]))
        print(json.dumps({'status':status,'captcha':str(PRIVATE/'captcha.png'),'enabled':d['show']}))
    elif a.action=='login':
        suite=json.loads((ROOT/'.private/production/suite-secrets.json').read_text())
        override=ROOT/'.private/production/admin-login.json'
        credentials=json.loads(override.read_text(encoding='utf-8')) if override.exists() else {}
        username=credentials.get('username') if credentials.get('password') else suite['adminUsername']
        password=credentials.get('password') or suite['bootstrap']
        d=json.loads((PRIVATE/'captcha.json').read_text())
        status,body=request('/admin/user/login','POST',{'username':username,'password':hashlib.md5(password.encode()).hexdigest(),'captcha':a.captcha or '', 'codeid':d['id']},False)
        if body.get('code')==0:
            token=crypt(body['data'],True)
            (PRIVATE/'hrm-session.json').write_text(json.dumps({'token':token,'createdAt':int(time.time())}))
        print(json.dumps({'status':status,'code':body.get('code'),'message':body.get('message'),'authenticated':body.get('code')==0},ensure_ascii=False))
    else:
        data=json.loads(pathlib.Path(a.json_file).read_text(encoding='utf-8-sig')) if a.json_file else ({} if a.method=='POST' else None)
        status,body=request(a.path,a.method,data)
        if a.output:
            out=PRIVATE/a.output
            assert out.resolve().is_relative_to(PRIVATE.resolve())
            out.write_text(json.dumps(body,ensure_ascii=False,indent=2),encoding='utf-8')
        payload=body.get('data')
        print(json.dumps({'status':status,'code':body.get('code'),'message':body.get('message'),'dataKeys':list(payload) if isinstance(payload,dict) else type(payload).__name__,'saved':bool(a.output)},ensure_ascii=False))


if __name__=='__main__':
    main()
