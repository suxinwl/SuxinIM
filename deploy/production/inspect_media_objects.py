#!/usr/bin/env python3
"""Bounded exact Mongo alias/key and MinIO HEAD checks; never reads image bodies."""
import argparse
import collections
import datetime
import hashlib
import hmac
import http.client
import importlib.util
import json
import os
import pathlib
import re
import secrets
import sys
import urllib.parse

DEPENDENCY_SHA256 = '1f7a177d97b19f4f8e1d0c257e66db80ff2b66494af2bd6970879d1c7016de86'
parent = pathlib.Path(__file__).with_name('inspect_media_comparison.py')
if hashlib.sha256(parent.read_bytes()).hexdigest() != DEPENDENCY_SHA256:
    raise RuntimeError('Frozen read-only metadata helper required')
spec = importlib.util.spec_from_file_location('bounded_media_metadata', parent)
probe = importlib.util.module_from_spec(spec)
spec.loader.exec_module(probe)
require = probe.require
HOST = '127.0.0.1:19000'
BUCKET = 'openim38'


def safe_name(value):
    return (isinstance(value, str) and 0 < len(value.encode()) <= 1024 and not value.startswith('/')
            and not any(ord(ch) < 32 or ord(ch) == 127 or ch == '\\' for ch in value)
            and not any(part in ('.','..','') for part in value.split('/')))


def descriptor(raw):
    try:
        u = urllib.parse.urlsplit(raw)
        if u.scheme+'://'+u.netloc not in {k for k,v in probe.ORIGINS.items() if v.startswith('im-')} or u.username or u.password:
            return None
        if re.search(r'%(?![a-fA-F0-9]{2})', u.path):
            return None
        p = urllib.parse.unquote_to_bytes(u.path).decode('utf-8')
        kind,value = None,None
        if p.startswith('/api/object/'):
            kind,value = 'alias',p[len('/api/object/'):]
        elif p.startswith('/object/'):
            kind,value = 'alias',p[len('/object/'):]
        elif p.startswith('/openim38/'):
            kind,value = 'key',p[len('/openim38/'):]
        elif p.startswith('/attachments/'):
            parts = p.split('/',3)
            if len(parts)==4 and re.fullmatch('[a-f0-9]{64}',parts[2]) and parts[3].startswith('openim38/'):
                kind,value = 'key',parts[3][len('openim38/'):]
        return (kind,value) if kind and safe_name(value) else None
    except (ValueError,UnicodeError):
        return None


MAPPING_SCRIPT = r'''
const uri='mongodb://'+encodeURIComponent(process.env.MONGO_INITDB_ROOT_USERNAME)+':'+encodeURIComponent(process.env.MONGO_INITDB_ROOT_PASSWORD)+'@127.0.0.1:27017/admin';
const conn=new Mongo(uri),objects=conn.getDB('openim38').getCollection('s3');
const result=[];
for(const item of requested){
 const filter={engine:'minio'};filter[item.kind==='alias'?'name':'key']=item.value;
 const rows=objects.find(filter,{_id:0,engine:1,name:1,key:1,size:1}).limit(2).maxTimeMS(10000).toArray();
 if(rows.length===0){const other={engine:{$ne:'minio'}};other[item.kind==='alias'?'name':'key']=item.value;const foreign=objects.find(other,{_id:1}).limit(1).toArray();result.push({kind:item.kind,value:item.value,state:foreign.length?'unsupported-engine':'mapping-absent'});continue;}
 if(item.kind==='alias'&&rows.length!==1){result.push({kind:item.kind,value:item.value,state:'ambiguous-alias'});continue;}
 const row=rows[0];result.push({kind:item.kind,value:item.value,state:'mapped',engine:row.engine,name:row.name,key:row.key,size:String(row.size)});
}
print(JSON.stringify(result));
'''


def mappings(mongo, requested):
    require(len(requested)<=256 and sum(x[0]=='alias' for x in requested)<=128 and sum(x[0]=='key' for x in requested)<=128, 'Exact metadata mapping bound exceeded')
    rows=[{'kind':kind,'value':value} for kind,value in sorted(requested)]
    code='const requested='+json.dumps(rows)+';\n'+MAPPING_SCRIPT
    result=json.loads(probe.execute(['docker','exec','-i',mongo,'mongosh','--nodb','--quiet','--file','/dev/stdin'],code.encode(),90))
    require(len(result)==len(rows) and {(r['kind'],r['value']) for r in result}==set(requested), 'Exact mapping response mismatch')
    out={}
    for row in result:
        require(row['state'] in ('mapped','mapping-absent','unsupported-engine','ambiguous-alias'), 'Unknown mapping shape')
        if row['state']=='mapped':
            good=(row.get('engine')=='minio' and safe_name(row.get('name')) and safe_name(row.get('key'))
                  and re.fullmatch('[0-9]+',row.get('size',''))
                  and (row['name']==row['value'] if row['kind']=='alias' else row['key']==row['value']))
            if not good:
                row={'kind':row['kind'],'value':row['value'],'state':'unsupported-schema'}
        out[(row['kind'],row['value'])]=row
    return out


def minio_credentials(container):
    ports=json.loads(probe.execute(['docker','inspect','--format','{{json .NetworkSettings.Ports}}',container]))
    require(ports.get('9000/tcp')==[{'HostIp':'127.0.0.1','HostPort':'19000'}], 'Owned MinIO API must be exact loopback port 19000')
    raw=probe.execute(['docker','exec',container,'sh','-ec','printf "%s\\n%s\\n" "$MINIO_ROOT_USER" "$MINIO_ROOT_PASSWORD"'])
    lines=raw.decode().splitlines()
    require(len(lines)==2 and re.fullmatch('[A-Za-z0-9_-]{1,128}',lines[0]) and re.fullmatch('[a-f0-9]{64}',lines[1]), 'Scoped MinIO credential shape unavailable')
    return lines[0],lines[1]


def head_headers(key, access, secret, now):
    require(safe_name(key), 'Unsafe exact object key')
    path='/'+BUCKET+'/'+urllib.parse.quote(key,safe='/-_.~')
    stamp=now.astimezone(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ')
    day=stamp[:8]
    scope=day+'/us-east-1/s3/aws4_request'
    payload=hashlib.sha256(b'').hexdigest()
    signed='host;x-amz-content-sha256;x-amz-date'
    canonical='HEAD\n'+path+'\n\nhost:'+HOST+'\nx-amz-content-sha256:'+payload+'\nx-amz-date:'+stamp+'\n\n'+signed+'\n'+payload
    message='AWS4-HMAC-SHA256\n'+stamp+'\n'+scope+'\n'+hashlib.sha256(canonical.encode()).hexdigest()
    signing=('AWS4'+secret).encode()
    for value in (day,'us-east-1','s3','aws4_request'):
        signing=hmac.new(signing,value.encode(),hashlib.sha256).digest()
    signature=hmac.new(signing,message.encode(),hashlib.sha256).hexdigest()
    return path,{'Host':HOST,'X-Amz-Date':stamp,'X-Amz-Content-Sha256':payload,
                 'Authorization':'AWS4-HMAC-SHA256 Credential='+access+'/'+scope+', SignedHeaders='+signed+', Signature='+signature}


def head_object(key, access, secret):
    path,headers=head_headers(key,access,secret,datetime.datetime.now(datetime.timezone.utc))
    connection=http.client.HTTPConnection('127.0.0.1',19000,timeout=8)
    try:
        connection.request('HEAD',path,headers=headers)
        response=connection.getresponse()
        status=response.status
        length=response.getheader('Content-Length')
        return {'status':200,'size':int(length) if length and length.isdigit() else None} if status==200 else {'status':404} if status==404 else {'status':'inconclusive'}
    except (OSError,http.client.HTTPException):
        return {'status':'inconclusive'}
    finally:
        connection.close()


def summarize(metadata, mapped, heads):
    result={}
    for label in ('affected','reference'):
        counts=collections.Counter()
        for record in metadata['records']:
            if record['label']!=label or record['channel'] not in ('moment','chat-102'):
                continue
            scope=record['channel']+('/self' if record['self'] else '/other')
            for field in record['urls']:
                counts[scope+'/references']+=1
                entry=descriptor(field['url'])
                if entry is None:
                    counts[scope+'/unsupported-reference']+=1
                    continue
                row=mapped[entry]
                counts[scope+'/'+entry[0]+'/'+row['state']]+=1
                if row['state']!='mapped':
                    continue
                check=heads[row['key']]
                counts[scope+'/HEAD/'+str(check['status'])]+=1
                if check['status']==200:
                    counts[scope+'/size/'+('match' if check['size']==int(row['size']) else 'unknown-or-mismatch')]+=1
        result[label]={'selection':metadata['stats'][label],'exactObjectCounts':dict(sorted(counts.items()))}
    return result


def main():
    import fcntl
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--input',type=pathlib.Path,required=True)
    parser.add_argument('--input-sha256',required=True)
    args=parser.parse_args()
    require(os.name=='posix' and os.geteuid()==0 and probe.BASE.resolve()==probe.BASE,'Owned Linux root read-only operation required')
    stage=probe.BASE/'.backup-stage'
    require(stage.resolve()==stage and stage.is_dir() and args.input.is_relative_to(stage),'Canonical private input required')
    raw=probe.regular(args.input).read_bytes()
    require(hashlib.sha256(raw).hexdigest()==args.input_sha256==probe.INPUT_SHA256,'Exact two-subject input required')
    subjects=probe.validate_subjects(json.loads(raw))
    marker,activation=[json.loads(probe.regular(p).read_text()) for p in (probe.BASE/'openim/instance.json',probe.BASE/'activation.json')]
    require(marker.get('instanceID')==activation.get('instanceID')==probe.INSTANCE and marker.get('production') is True
            and marker.get('testOnly') is False and marker.get('root')==str(probe.BASE/'openim') and marker.get('stackName')==probe.PROJECT,'Owner mismatch')
    with probe.regular(stage/'backup.lock').open('rb') as lock:
        fcntl.flock(lock,fcntl.LOCK_SH|fcntl.LOCK_NB)
        mysql,mongo,minio=[probe.owned_container(service) for service in ('mysql','mongo','minio')]
        probe.verify_mysql(mysql,subjects)
        metadata=probe.mongo_metadata(mongo,subjects)
        requested={descriptor(f['url']) for row in metadata['records'] if row['channel'] in ('moment','chat-102') for f in row['urls']}-{None}
        mapped=mappings(mongo,requested)
        keys={row['key'] for row in mapped.values() if row['state']=='mapped'}
        require(len(keys)<=128,'Exact HEAD key bound exceeded')
        access,secret=minio_credentials(minio)
        heads={key:head_object(key,access,secret) for key in sorted(keys)}
        del access,secret
        report={'readOnly':True,'labelsOnly':True,'inputSHA256':probe.INPUT_SHA256,'perSubjectCandidateCap':50,
            'capturedAtUTC':datetime.datetime.now(datetime.timezone.utc).isoformat(),'exactMappingCount':len(mapped),
            'exactHEADKeyCount':len(keys),'pixelBytesRead':0,'userSessionsOrTokensRead':False,
            'bucketListed':False,'redirectsFollowed':False,'HTTPMethod':'HEAD','bucket':BUCKET,
            'comparison':summarize(metadata,mapped,heads)}
        target=stage/('media-object-comparison-'+secrets.token_hex(8)+'.json')
        with os.fdopen(os.open(target,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600),'w',encoding='utf-8') as output:
            json.dump(report,output,sort_keys=True);output.write('\n');output.flush();os.fsync(output.fileno())
        print(json.dumps({'report':report,'safeReportPath':str(target),'safeReportSHA256':hashlib.sha256(target.read_bytes()).hexdigest()}))


if __name__=='__main__':
    try:
        main()
    except Exception as error:
        print(json.dumps({'readOnlyObjectComparisonFailed':True,'category':type(error).__name__,
                          'message':str(error) if type(error) is RuntimeError else 'private_details_suppressed'}))
        sys.exit(1)
