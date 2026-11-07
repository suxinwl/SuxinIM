#!/usr/bin/env python3
"""Read-only, label-only metadata comparison for two reviewed employee mappings."""
import argparse
import collections
import datetime
import hashlib
import json
import os
import pathlib
import re
import secrets
import stat
import subprocess
import sys
import time
import urllib.parse

BASE = pathlib.Path('/srv/puty-suite')
PROJECT = 'puty-suite-prod'
INSTANCE = '5ac7c644d9cd4923e2b701f66b52485d'
INPUT_SHA256 = '29c9013e984cb20a47102d9a064bc7a7d88f93e7863cef6fcd29a7df1235a5f2'
ORIGINS = {'https://192.168.10.11:9885': 'im-lan', 'https://im.szputy.com:9885': 'im-wan',
           'https://192.168.10.11:9880': 'hrm-lan', 'https://hr.szputy.com:9880': 'hrm-wan'}


def require(ok, message):
    if not ok:
        raise RuntimeError(message)


def digest(value):
    return hashlib.sha256(value.encode()).hexdigest()


def regular(path):
    require(path.resolve() == path and path.is_file() and not path.is_symlink()
            and stat.S_ISREG(path.stat().st_mode) and path.stat().st_nlink == 1,
            'Canonical owned regular file required')
    return path


def execute(argv, data=None, timeout=60):
    result = subprocess.run(argv, input=data, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, timeout=timeout)
    require(result.returncode == 0, 'Read-only subprocess unavailable')
    return result.stdout


def owned_container(service):
    ids = execute(['docker', 'ps', '-q', '--filter', 'label=com.docker.compose.project='+PROJECT,
                   '--filter', 'label=com.docker.compose.service='+service]).decode().split()
    require(len(ids) == 1, 'Exactly one owned storage container required')
    labels = json.loads(execute(['docker', 'inspect', '--format', '{{json .Config.Labels}}', ids[0]]))
    require(labels.get('com.puty.suite') == INSTANCE and labels.get('com.docker.compose.project') == PROJECT
            and labels.get('com.docker.compose.service') == service, 'Foreign storage container refused')
    return ids[0]


def validate_subjects(value):
    subjects = value.get('subjects')
    require(value.get('readOnly') is True and isinstance(subjects, list) and len(subjects) == 2
            and {r.get('label') for r in subjects} == {'affected', 'reference'}, 'Exactly two reviewed labels required')
    for row in subjects:
        require(set(row) == {'label', 'employeeID', 'userID', 'employeeNo'}
                and isinstance(row['employeeID'], str) and re.fullmatch('[A-Za-z0-9_-]{1,64}', row['employeeID'])
                and isinstance(row['employeeNo'], str) and re.fullmatch('[0-9]{4}', row['employeeNo'])
                and row['userID'] == 'emp_'+digest(row['employeeID'])[:32], 'Exact employee mapping required')
    require(len({r['employeeID'] for r in subjects}) == len({r['userID'] for r in subjects})
            == len({r['employeeNo'] for r in subjects}) == 2, 'Distinct reviewed identities required')
    return subjects


def verify_mysql(container, subjects):
    literal = lambda v: "CONVERT(X'"+v.encode().hex()+"' USING utf8mb4) COLLATE utf8mb4_bin"
    terms = ['(e.id='+literal(s['employeeID'])+' AND e.employee_no='+literal(s['employeeNo'])+')' for s in subjects]
    sql = ("SET TRANSACTION ISOLATION LEVEL REPEATABLE READ; START TRANSACTION READ ONLY;\n"
        "SELECT COALESCE(JSON_ARRAYAGG(JSON_OBJECT('employeeID',e.id,'employeeNo',e.employee_no,'sourceUID',e.user_id,"
        "'sourceStatus',e.status,'userID',b.openim_user_id,'uid',b.uid,'enabled',b.enabled,'status',b.status,"
        "'desired',b.desired_version,'applied',b.applied_version,'errorEmpty',b.last_error='')),JSON_ARRAY()) "
        "FROM biz_hrm_employee e JOIN biz_openim_employee b ON b.employee_id=e.id WHERE "+' OR '.join(terms)+";\nROLLBACK;\n")
    command = 'export MYSQL_PWD="$MYSQL_ROOT_PASSWORD"; exec mysql -uroot --database=gosuxin_prod --batch --raw --skip-column-names --connect-timeout=10 --default-character-set=utf8mb4'
    rows = json.loads(execute(['docker', 'exec', '-i', container, 'sh', '-ec', command], sql.encode()))
    require(len(rows) == 2, 'Two exact live source/projection mappings required')
    for s in subjects:
        matches = [r for r in rows if r['employeeID'] == s['employeeID'] and r['employeeNo'] == s['employeeNo'] and r['userID'] == s['userID']]
        require(len(matches) == 1, 'Reviewed mapping no longer exact')
        r = matches[0]
        require(r['sourceStatus'] == 'active' and r['status'] == 'ready' and r['enabled'] == 1
                and int(r['sourceUID']) == int(r['uid']) > 1 and r['desired'] == r['applied'] > 0
                and r['errorEmpty'] == 1, 'Reviewed source/projection no longer eligible and applied')


MONGO_SCRIPT = r'''
const uri='mongodb://'+encodeURIComponent(process.env.MONGO_INITDB_ROOT_USERNAME)+':'+encodeURIComponent(process.env.MONGO_INITDB_ROOT_PASSWORD)+'@127.0.0.1:27017/admin';
const conn=new Mongo(uri), im=conn.getDB('openim38'), ent=conn.getDB('enterprise38');
const records=[],stats={};
const keys=new Set(['url','sourceurl','snapshoturl','videourl','imageurl','thumb','original']);
function urls(v,out=[],depth=0){
 if(depth>5||v===null||typeof v!=='object')return out;
 for(const [k,x] of Object.entries(v)){
  if(typeof x==='string'&&keys.has(k.toLowerCase())){if(x.length>0&&x.length<=32768)out.push({field:k.toLowerCase(),url:x});}
  else if(x&&typeof x==='object')urls(x,out,depth+1);
 }
 return out;
}
function visible(m,u){if(m.userID===u)return true;const listed=(m.permissionUsers||[]).some(p=>p.userID===u);return m.permission===0||(m.permission===2&&listed)||(m.permission===3&&!listed);}
for(const s of subjects){
 const friends=im.friend.find({owner_user_id:s.userID},{_id:0,friend_user_id:1}).limit(5001).toArray();
 const groups=im.group_member.find({user_id:s.userID},{_id:0,group_id:1}).limit(1001).toArray();
 if(friends.length>5000||groups.length>1000)throw new Error('scope bound exceeded');
 const authors=[s.userID,...friends.map(r=>r.friend_user_id)], groupIDs=groups.map(r=>r.group_id);
 const candidate=ent.enterprise_documents.find({kind:'moment',owner:{$in:authors}},{data:1}).sort({updated:-1,_id:1}).limit(50).maxTimeMS(20000).toArray();
 let momentVisible=0, malformed=0;
 for(const d of candidate){
  let m;try{m=JSON.parse(Buffer.from(d.data.buffer).toString('utf8'));}catch{malformed++;continue;}
  if(!visible(m,s.userID))continue;momentVisible++;
  records.push({label:s.label,channel:'moment',self:m.userID===s.userID,urls:urls({metas:m.content?.metas||[]})});
 }
 const relevance=[{'msgs.msg.send_id':s.userID},{'msgs.msg.recv_id':s.userID},{'msgs.msg.group_id':{$in:groupIDs}}];
 const after=[{'msgs.msg.send_id':s.userID},{'msgs.msg.recv_id':s.userID},{'msgs.msg.group_id':{$in:groupIDs}}];
 const messages=im.msg.aggregate([{$match:{$or:relevance}},{$unwind:'$msgs'},{$match:{$or:after,'msgs.msg.content_type':{$in:[102,104,105]},'msgs.del_list':{$ne:s.userID},'msgs.revoke':{$eq:null}}},{$sort:{'msgs.msg.send_time':-1}},{$limit:50},{$project:{_id:0,type:'$msgs.msg.content_type',sender:'$msgs.msg.send_id',content:'$msgs.msg.content'}}],{maxTimeMS:20000}).toArray();
 for(const m of messages){let data;try{data=JSON.parse(m.content||'{}');}catch{malformed++;continue;}records.push({label:s.label,channel:'chat-'+m.type,self:m.sender===s.userID,urls:urls(data)});}
 stats[s.label]={friendCount:friends.length,currentGroupCount:groups.length,momentCandidates: candidate.length,momentsVisible:momentVisible,recentMediaMessages:messages.length,malformedMediaJSON:malformed};
}
print(JSON.stringify({records,stats}));
'''


def mongo_metadata(container, subjects):
    script = 'const subjects='+json.dumps([{'label':s['label'], 'userID':s['userID']} for s in subjects])+';\n'+MONGO_SCRIPT
    raw = execute(['docker', 'exec', '-i', container, 'mongosh', '--nodb', '--quiet', '--file', '/dev/stdin'], script.encode(), 90)
    result = json.loads(raw)
    require(len(result['records']) <= 200 and all(len(r['urls']) <= 50 for r in result['records']), 'Metadata scope exceeded')
    return result


def redis_mget(container, keys):
    if not keys:
        return {}
    require(len(keys) <= 2000 and all(re.fullmatch(r'HRM38:(ATTACH|SESSION|EPOCH|USER|CURRENT):[A-Za-z0-9_:.-]+', k) for k in keys), 'Bounded read-only registry keys required')
    command = 'export REDISCLI_AUTH="$REDIS_PASSWORD"; exec redis-cli --json -n 2'
    raw = execute(['docker', 'exec', '-i', container, 'sh', '-ec', command], ('MGET '+' '.join(keys)+'\n').encode())
    rows = json.loads(raw)
    require(isinstance(rows, list) and len(rows) == len(keys), 'Read-only registry response unavailable')
    return dict(zip(keys, rows))


def grant_token(raw):
    try:
        parts = urllib.parse.urlsplit(raw).path.split('/')
        return parts[2] if len(parts) >= 4 and parts[1] == 'attachments' and re.fullmatch('[a-f0-9]{64}', parts[2]) else None
    except ValueError:
        return None


def registry(container, records):
    tokens = {grant_token(u['url']) for r in records for u in r['urls']} - {None}
    grants = redis_mget(container, ['HRM38:ATTACH:'+digest(t) for t in sorted(tokens)])
    objects = {t: json.loads(grants['HRM38:ATTACH:'+digest(t)]) if grants.get('HRM38:ATTACH:'+digest(t)) else None for t in tokens}
    keys = set()
    for a in objects.values():
        if not a:
            continue
        keys.update(['HRM38:SESSION:'+digest(a['sessionID']), 'HRM38:USER:'+a['userID'],
                     'HRM38:EPOCH:'+a['userID'], 'HRM38:CURRENT:'+a['userID']+':'+str(a['platform'])])
    return objects, redis_mget(container, sorted(keys))


def classify(raw, viewer, grants, state, now):
    try:
        u = urllib.parse.urlsplit(raw)
        origin = ORIGINS.get(u.scheme+'://'+u.netloc, 'unconfigured-origin')
        if u.scheme != 'https':
            origin = 'non-https-'+(u.scheme if u.scheme in ('http','blob','data') else 'relative-or-other')
        p = u.path
        kind = ('grant' if p.startswith('/attachments/') else 'stable-api-object' if p.startswith('/api/object/') else
                'stable-object' if p.startswith('/object/') else 'bucket' if p.startswith('/openim38/') else
                'hrm-upload' if p.startswith('/resource/uploads/') else 'public-brand' if p.startswith('/resource/static/brand/') else 'unsupported-path')
        result = {'origin': origin, 'pathKind': kind, 'secondQuestionMark': str(raw.count('?') > 1).lower()}
        query = urllib.parse.parse_qs(u.query)
        signed = 'X-Amz-Signature' in query
        result['querySignature'] = 'present' if signed else 'absent'
        if signed and 'X-Amz-Date' in query and 'X-Amz-Expires' in query:
            try:
                created = datetime.datetime.strptime(query['X-Amz-Date'][0], '%Y%m%dT%H%M%SZ').replace(tzinfo=datetime.timezone.utc).timestamp()
                result['querySignatureExpiry'] = 'expired' if created+int(query['X-Amz-Expires'][0]) <= now else 'not-expired'
            except (ValueError, TypeError):
                result['querySignatureExpiry'] = 'malformed'
        token = grant_token(raw)
        if kind == 'grant':
            a = grants.get(token)
            if not a:
                result['grantRegistry'] = 'missing-or-TTL-expired'
            else:
                result['grantOwnerRelation'] = 'viewer' if a['userID'] == viewer else 'other-user'
                result['grantRegistry'] = 'expired' if int(a.get('expiresAt',0)) <= now else 'present'
                session_raw = state.get('HRM38:SESSION:'+digest(a['sessionID']))
                projection_raw = state.get('HRM38:USER:'+a['userID'])
                v, projection = json.loads(session_raw) if session_raw else {}, json.loads(projection_raw) if projection_raw else {}
                current = state.get('HRM38:CURRENT:'+a['userID']+':'+str(a['platform']))
                epoch = int(state.get('HRM38:EPOCH:'+a['userID']) or 0)
                live = bool(v and projection.get('enabled') is True and not v.get('revoked') and v.get('expiresAt',0)>now
                            and v.get('epoch',0)>=epoch and current==a['sessionID'] and v.get('userID')==a['userID']
                            and v.get('epoch')==a.get('epoch') and v.get('platform')==a.get('platform'))
                result['grantSourceSID'] = 'live-registry' if live else 'not-live-registry'
        return result
    except (ValueError, TypeError, KeyError):
        return {'origin':'malformed-reference','pathKind':'malformed-reference'}


def summarize(metadata, subjects, grants, state, now):
    result = {}
    for subject in subjects:
        counters = collections.Counter()
        selected = [r for r in metadata['records'] if r['label'] == subject['label']]
        for row in selected:
            for entry in row['urls']:
                scope = row['channel']+('/self' if row['self'] else '/other')
                counters[scope+'/referenceCount'] += 1
                for key,value in classify(entry['url'],subject['userID'],grants,state,now).items():
                    counters[scope+'/'+key+'/'+value] += 1
        result[subject['label']] = {'selection': metadata['stats'][subject['label']], 'urlCategoryCounts': dict(sorted(counters.items()))}
    return result


def main():
    import fcntl
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--input', type=pathlib.Path, required=True)
    parser.add_argument('--input-sha256', required=True)
    args = parser.parse_args()
    require(os.name == 'posix' and os.geteuid() == 0 and BASE.resolve() == BASE, 'Owned Linux root read-only operation required')
    stage = BASE/'.backup-stage'
    require(stage.resolve() == stage and stage.is_dir() and args.input.is_relative_to(stage), 'Private canonical evidence path required')
    raw = regular(args.input).read_bytes()
    require(hashlib.sha256(raw).hexdigest() == args.input_sha256 == INPUT_SHA256, 'Only the exact reviewed two-subject input is allowed')
    subjects = validate_subjects(json.loads(raw))
    marker, activation = [json.loads(regular(p).read_text()) for p in (BASE/'openim/instance.json',BASE/'activation.json')]
    require(marker.get('instanceID') == activation.get('instanceID') == INSTANCE and marker.get('production') is True
            and marker.get('testOnly') is False and marker.get('root') == str(BASE/'openim') and marker.get('stackName') == PROJECT, 'Production owner mismatch')
    with regular(stage/'backup.lock').open('rb') as lock:
        fcntl.flock(lock,fcntl.LOCK_SH|fcntl.LOCK_NB)
        mysql,mongo,redis = [owned_container(s) for s in ('mysql','mongo','redis')]
        verify_mysql(mysql,subjects)
        metadata = mongo_metadata(mongo,subjects)
        grants,state = registry(redis,metadata['records'])
        report = {'readOnly':True,'labelsOnly':True,'sourceBindingsVerified':2,'inputSHA256':INPUT_SHA256,
            'capturedAtUTC':datetime.datetime.now(datetime.timezone.utc).isoformat(),'perSubjectCandidateCap':50,
            'messageBodiesOrURLsPersisted':False,'employeeLoginOrCredentialIssuance':False,'objectExistenceProbed':False,
            'registryChecksDoNotReplaceCurrentHRMPolicy':True,
            'comparison':summarize(metadata,subjects,grants,state,int(time.time()))}
        target=stage/('media-comparison-'+secrets.token_hex(8)+'.json')
        with os.fdopen(os.open(target,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600),'w',encoding='utf-8') as output:
            json.dump(report,output,ensure_ascii=True,sort_keys=True);output.write('\n');output.flush();os.fsync(output.fileno())
        print(json.dumps({'report':report,'safeReportPath':str(target),'safeReportSHA256':hashlib.sha256(target.read_bytes()).hexdigest()}))


if __name__ == '__main__':
    try:
        main()
    except Exception as error:
        print(json.dumps({'readOnlyComparisonFailed':True,'category':type(error).__name__,
                          'message':str(error) if type(error) is RuntimeError else 'private_details_suppressed'}))
        sys.exit(1)
