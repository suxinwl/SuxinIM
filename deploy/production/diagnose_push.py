#!/usr/bin/env python3
"""Emit only transport error counters and peer addresses; never raw IM logs."""
import collections,json,pathlib,re,subprocess
for service in ['openim-push','openim-msggateway','openim-msgtransfer']:
    name='puty-suite-prod-'+service+'-1'
    p=subprocess.run(['docker','logs','--since','3m',name],capture_output=True)
    raw=(p.stdout+p.stderr).decode(errors='replace')
    folder=pathlib.Path('/srv/puty-suite/openim/logs')/service
    messages=collections.Counter();keys=set()
    for file in sorted(folder.glob('*'))[-2:]:
        if not file.is_file():continue
        lines=file.read_text(errors='replace').splitlines()[-2000:]
        raw+='\n'+'\n'.join(lines)
        for line in lines:
            try:data=json.loads(line)
            except ValueError:continue
            keys.update(data)
            message=str(data.get('msg',data.get('message','')))
            message=re.sub(r'(?i)(?:emp_[a-f0-9]+|[A-Za-z0-9_-]{20,}\.[A-Za-z0-9_-]{20,}\.[A-Za-z0-9_-]+|(?:\d+\.){3}\d+(?::\d+)?|[a-f0-9]{32,})','[redacted]',message)
            if len(message)<=180 and not any(x in message.lower() for x in ['token','password','content','secret','operationid','payload','{"']):messages[message]+=1
    terms=['GetConnsAndOnlinePush','online cache','onlineUserIDs','PushMessage','SendMsgToUser','push failed','DoSubscribe','connection refused','Unavailable','rpc error','deadline exceeded','context canceled','no such host','authentication failed','timeout','panic','ERROR','subscribe','consumer','etcd']
    peers=collections.Counter(re.findall(r'dial tcp ((?:\d+\.){3}\d+:\d+)',raw))
    meta=json.loads(subprocess.check_output(['docker','inspect',name]))[0]
    print(json.dumps({'service':service,'state':meta['State']['Status'],'restarts':meta['RestartCount'],'ips':[x['IPAddress'] for x in meta['NetworkSettings']['Networks'].values()],'logLines':len(raw.splitlines()),'counts':{x:raw.lower().count(x.lower()) for x in terms},'dialPeerCounts':peers,'safeMessages':messages,'logFieldNames':sorted(keys)}))
text=pathlib.Path('/srv/puty-suite/openim/config/openim-server/discovery.yml').read_text()
mode=re.search(r'^\s*enable:\s*(\S+)',text,re.M)
print(json.dumps({'discoveryMode':mode[1] if mode and mode[1] in ['etcd','kubernetes','zookeeper'] else 'unrecognized'}))
