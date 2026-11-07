#!/usr/bin/env python3
"""Probe the policy hop from enterprise's actual network namespace, without logging secrets."""
import argparse,json,pathlib,subprocess
p=argparse.ArgumentParser();p.add_argument('--input',required=True);a=p.parse_args()
container=json.loads(subprocess.check_output(['docker','inspect','puty-suite-prod-enterprise-service-1']))[0]
environment=dict(x.split('=',1) for x in container['Config']['Env'] if '=' in x)
probe=json.loads(pathlib.Path(a.input).read_text())
code=r'''
import hashlib,hmac,json,secrets,sys,time,urllib.request,urllib.error,urllib.parse
config=json.load(sys.stdin);raw=json.dumps(config['body'],separators=(',',':')).encode();url=config['url'];path=urllib.parse.urlparse(url).path;stamp=str(int(time.time()));nonce=secrets.token_hex(32);signature=hmac.new(config['key'].encode(),('\n'.join([stamp,nonce,'POST',path,hashlib.sha256(raw).hexdigest()])).encode(),hashlib.sha256).hexdigest()
request=urllib.request.Request(url,raw,{'Content-Type':'application/json','X-HRM-Timestamp':stamp,'X-HRM-Nonce':nonce,'X-HRM-Signature':signature},method='POST');start=time.monotonic()
try:
 try: response=urllib.request.urlopen(request,timeout=5)
 except urllib.error.HTTPError as e:response=e
 with response:
  data=json.loads(response.read());print(json.dumps({'status':response.status,'errCode':data.get('errCode'),'allowed':data.get('data',{}).get('allowed'),'elapsed':round(time.monotonic()-start,2)}))
except Exception as e:print(json.dumps({'networkError':type(e).__name__,'elapsed':round(time.monotonic()-start,2)}))
'''
for name,prefix,url in [('host-loopback',[],'http://127.0.0.1:18000/internal/im-access/check'),('enterprise-namespace',['nsenter','-t',str(container['State']['Pid']),'-n'],environment['HRM_ACCESS_CHECK_URL'])]:
    data=json.dumps({'url':url,'key':environment['HRM_BRIDGE_HMAC_KEY'],'body':probe}).encode()
    r=subprocess.run([*prefix,'/usr/bin/python3','-c',code],input=data,capture_output=True,timeout=10)
    print(json.dumps({'hop':name,'result':json.loads(r.stdout) if r.returncode==0 else {'probeError':True}}))
