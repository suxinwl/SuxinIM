#!/usr/bin/env python3
"""Root-only metadata audit. No credential issuance, mutations, or raw config output."""
import datetime
import fcntl
import hashlib
import hmac
import json
import os
import pathlib
import re
import stat
import subprocess
import sys
import urllib.error
import urllib.parse
import urllib.request
import xml.etree.ElementTree as ET

import yaml

BASE = pathlib.Path('/srv/puty-suite')
INSTANCE = '5ac7c644d9cd4923e2b701f66b52485d'
PROJECT = 'puty-suite-prod'
SERVICES = set(('mysql mongo redis etcd kafka minio livekit enterprise-service '
                'openim-api openim-crontask openim-msggateway openim-msgtransfer openim-push '
                'openim-rpc-auth openim-rpc-conversation openim-rpc-friend openim-rpc-group '
                'openim-rpc-msg openim-rpc-third openim-rpc-user '
                'admin-api chat-api chat-admin chat-business').split())
CRON_FILE = BASE / 'openim/config/openim-server/openim-crontask.yml'


def require(value, code):
    if not value:
        raise RuntimeError(code)


def sha(data):
    return hashlib.sha256(data).hexdigest()


def regular(path, private=False):
    meta = path.lstat()
    require(path.resolve() == path and stat.S_ISREG(meta.st_mode) and meta.st_nlink == 1
            and meta.st_uid == 0 and not meta.st_mode & 0o022
            and (not private or not meta.st_mode & 0o077), 'owned_regular_file_required')
    require(meta.st_size <= 16 << 20, 'metadata_file_too_large')
    return path


def run(argv, body=None, timeout=45):
    result = subprocess.run(argv, input=body, stdout=subprocess.PIPE,
                            stderr=subprocess.DEVNULL, timeout=timeout)
    require(result.returncode == 0 and len(result.stdout) <= 16 << 20,
            'readonly_subprocess_failed')
    return result.stdout


def environment(container):
    result = {}
    for row in container.get('Config', {}).get('Env', []):
        key, separator, value = row.partition('=')
        require(separator and key not in result, 'duplicate_runtime_environment')
        result[key] = value
    return result


def owned_containers():
    ids = run(['docker', 'ps', '-a', '-q', '--filter',
               'label=com.docker.compose.project=' + PROJECT]).decode().split()
    require(len(ids) == 24, 'exactly_24_containers_required')
    result = {}
    for item in json.loads(run(['docker', 'inspect', *ids])):
        labels = item['Config']['Labels']
        service = labels.get('com.docker.compose.service')
        require(service in SERVICES and service not in result
                and labels.get('com.docker.compose.project') == PROJECT
                and labels.get('com.puty.suite') == INSTANCE
                and labels.get('com.puty.environment') == 'production'
                and item['State']['Running'] is True, 'container_ownership_mismatch')
        result[service] = item
    require(set(result) == SERVICES, 'container_service_set_mismatch')
    return result


def mounted_file(container, source):
    destination = pathlib.PurePosixPath('/app/config/openim-crontask.yml')
    candidates = []
    for mount in container.get('Mounts', []):
        parent = pathlib.PurePosixPath(mount.get('Destination', '/invalid'))
        if parent == destination or parent in destination.parents:
            candidates.append((len(parent.parts), parent, mount))
    require(candidates, 'actual_config_bind_missing')
    _, parent, mount = max(candidates, key=lambda row: row[0])
    actual = pathlib.Path(mount.get('Source', '/invalid')) / str(destination.relative_to(parent))
    require(mount.get('Type') == 'bind' and actual == source and mount.get('RW') is False,
            'actual_config_bind_mismatch')
    return mount


def cron_summary(value):
    keys = ('retainChatRecords', 'fileExpireTime', 'deleteObjectType')
    require(isinstance(value, dict) and all(key in value for key in keys), 'cron_fields_missing')
    records, files, kinds = (value[key] for key in keys)
    require(type(records) is int and type(files) is int and isinstance(kinds, list)
            and all(type(v) is str for v in kinds), 'cron_fields_invalid')
    return {'retainChatRecords': records, 'fileExpireTime': files,
            'deleteObjectTypeCount': len(kinds),
            'permanent': records == 0 and files == 0 and not kinds}


MONGO_SCRIPT = r'''
try {
 const c=new Mongo('mongodb://'+encodeURIComponent(process.env.MONGO_INITDB_ROOT_USERNAME)+':'+encodeURIComponent(process.env.MONGO_INITDB_ROOT_PASSWORD)+'@127.0.0.1:27017/admin');
 const databases=c.getDB('admin').runCommand({listDatabases:1,nameOnly:true});
 if(databases.ok!==1 || databases.databases.length>64)throw Error('metadata');
 const rows=[];let collections=0,indexes=0;
 for(const d of databases.databases){const b=c.getDB(d.name),names=b.getCollectionNames();
  if(names.length>10000)throw Error('metadata');
  for(const name of names){collections++;for(const ix of b.getCollection(name).getIndexes()){
   indexes++;if(indexes>100000)throw Error('metadata');
   if(ix.expireAfterSeconds!==undefined){const n=String(ix.expireAfterSeconds);if(!/^[0-9]+$/.test(n))throw Error('ttl');
    rows.push({database:d.name,collection:name,index:ix.name,seconds:n,partial:ix.partialFilterExpression!==undefined,internal:['admin','config','local'].includes(d.name)});}
  }}
 }
 print(JSON.stringify({ok:true,databases:databases.databases.length,collections,indexes,ttl:rows}));
}catch(e){print(JSON.stringify({ok:false,error:'mongo_metadata_failed'}));quit(1);}
'''


def mongo_summary(value):
    require(value.get('ok') is True and all(type(value.get(k)) is int and value[k] >= 0
            for k in ('databases', 'collections', 'indexes')) and isinstance(value.get('ttl'), list),
            'mongo_metadata_invalid')
    rows = []
    for row in value['ttl']:
        require(set(row) == {'database', 'collection', 'index', 'seconds', 'partial', 'internal'}
                and all(isinstance(row[k], str) and len(row[k]) <= 256
                        for k in ('database', 'collection', 'index'))
                and re.fullmatch(r'[0-9]{1,20}', row['seconds'])
                and type(row['partial']) is bool and type(row['internal']) is bool,
                'mongo_ttl_invalid')
        # No dynamic collection/index names (which may contain user IDs) leave the process.
        rows.append({'databaseClass': row['database'] if row['database'] in
                     ('openim38', 'chat38', 'enterprise38', 'admin', 'config', 'local') else 'other',
                     'collectionSHA256': sha(row['collection'].encode()),
                     'indexSHA256': sha(row['index'].encode()), 'expireAfterSeconds': row['seconds'],
                     'partial': row['partial'], 'internal': row['internal']})
    return {**{key: value[key] for key in ('databases', 'collections', 'indexes')},
            'ttl': rows, 'businessTTLCount': sum(not row['internal'] for row in rows)}


def s3_get(secret, resource, query=''):
    require(resource.startswith('/') and not resource.startswith('//') and
            query in ('', 'lifecycle='), 's3_read_scope_invalid')
    now = datetime.datetime.now(datetime.timezone.utc)
    stamp, day = now.strftime('%Y%m%dT%H%M%SZ'), now.strftime('%Y%m%d')
    host, empty = '127.0.0.1:19000', sha(b'')
    headers = 'host:' + host + '\nx-amz-content-sha256:' + empty + '\nx-amz-date:' + stamp + '\n'
    canonical = '\n'.join(('GET', resource, query, headers,
                           'host;x-amz-content-sha256;x-amz-date', empty))
    scope = day + '/us-east-1/s3/aws4_request'
    key = ('AWS4' + secret).encode()
    for value in (day, 'us-east-1', 's3', 'aws4_request'):
        key = hmac.new(key, value.encode(), hashlib.sha256).digest()
    signature = hmac.new(key, ('AWS4-HMAC-SHA256\n' + stamp + '\n' + scope + '\n' +
                              sha(canonical.encode())).encode(), hashlib.sha256).hexdigest()
    request = urllib.request.Request('http://' + host + resource + ('?' + query if query else ''),
        headers={'X-Amz-Date': stamp, 'X-Amz-Content-Sha256': empty,
                 'Authorization': 'AWS4-HMAC-SHA256 Credential=root/' + scope +
                 ', SignedHeaders=host;x-amz-content-sha256;x-amz-date, Signature=' + signature})
    class NoRedirect(urllib.request.HTTPRedirectHandler):
        def redirect_request(self, request, fp, code, message, headers, newurl):
            return None
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), NoRedirect())
    try:
        with opener.open(request, timeout=8) as response:
            return response.status, response.read((2 << 20) + 1)
    except urllib.error.HTTPError as error:
        return error.code, error.read((2 << 20) + 1)


def xml(raw):
    require(len(raw) <= 2 << 20 and b'<!DOCTYPE' not in raw.upper()
            and b'<!ENTITY' not in raw.upper(), 's3_metadata_invalid')
    root = ET.fromstring(raw)
    for node in root.iter():
        node.tag = node.tag.rsplit('}', 1)[-1]
    return root


def lifecycle_summary(status, raw):
    root = xml(raw)
    if status == 404 and root.findtext('Code') == 'NoSuchLifecycleConfiguration':
        return {'configured': False, 'rules': 0, 'enabledExpirationRules': 0,
                'enabledAbortMultipartRules': 0}
    require(status == 200 and root.tag == 'LifecycleConfiguration', 's3_lifecycle_unavailable')
    rules = root.findall('Rule')
    return {'configured': True, 'rules': len(rules),
            'enabledExpirationRules': sum(r.findtext('Status') == 'Enabled' and
                (r.find('Expiration') is not None or r.find('NoncurrentVersionExpiration') is not None)
                for r in rules),
            'enabledAbortMultipartRules': sum(r.findtext('Status') == 'Enabled' and
                r.find('AbortIncompleteMultipartUpload') is not None for r in rules)}


def capacity_summary(containers):
    result = []
    enterprise = containers['enterprise-service']
    verified = environment(enterprise).get('STORAGE_CAPACITY_DATA_VERIFIED') == 'true'
    for service, destination in (('mongo', '/data/db'), ('minio', '/bitnami/minio/data')):
        mounts = [m for m in containers[service].get('Mounts', []) if m.get('Destination') == destination]
        require(len(mounts) == 1 and mounts[0].get('Type') == 'volume' and mounts[0].get('RW') is True,
                'data_volume_mount_invalid')
        mount = mounts[0]
        volume = json.loads(run(['docker', 'volume', 'inspect', mount['Name']]))[0]
        labels = volume.get('Labels') or {}
        source = pathlib.Path(mount['Source'])
        require(volume.get('Driver') == 'local' and volume.get('Scope') == 'local'
                and labels.get('com.puty.suite') == INSTANCE
                and labels.get('com.docker.compose.project') == PROJECT
                and labels.get('com.docker.compose.volume') == service + '-data'
                and volume.get('Mountpoint') == str(source) and source.resolve() == source
                and source.is_dir() and not source.is_symlink(), 'owned_data_volume_required')
        aliases = [m for m in enterprise.get('Mounts', []) if m.get('Destination') == '/capacity/' + service]
        readonly = len(aliases) == 1 and aliases[0].get('Source') == str(source) and aliases[0].get('RW') is False
        info = os.statvfs(source)
        result.append({'kind': service, 'ownedVolume': True, 'capacityAliasReadonly': readonly,
                       'operatorVerified': verified, 'totalBytes': info.f_blocks * info.f_frsize,
                       'availableBytes': info.f_bavail * info.f_frsize})
    return result


def task_summary(raw, origin):
    text = raw.decode('utf-8', errors='replace')
    scope = bool(re.search(r'puty-suite|gosuxin|openim|enterprise-service|/capacity/', text, re.I))
    destructive = bool(re.search(r'\b(?:delete|remove|expire|expiry|retention|prune|cleanup|unlink|rmdir|rm)\b', text, re.I))
    return {'sourceSHA256': sha(origin.encode()), 'contentSHA256': sha(raw),
            'suiteMention': scope, 'cleanupMention': destructive, 'requiresManualReview': scope and destructive}


def external_tasks(containers):
    rows, unreadable = [], 0
    for base in (pathlib.Path('/etc/cron.d'), pathlib.Path('/etc/cron.daily'),
                 pathlib.Path('/etc/cron.hourly'), pathlib.Path('/etc/cron.weekly'),
                 pathlib.Path('/etc/cron.monthly'), pathlib.Path('/var/spool/cron'),
                 pathlib.Path('/www/server/cron'), pathlib.Path('/etc/systemd/system'),
                 pathlib.Path('/usr/lib/systemd/system'),
                 pathlib.Path('/www/server/panel/plugin/supervisor/profile')):
        if not base.exists():
            continue
        if base.resolve() != base or base.is_symlink():
            unreadable += 1
            continue
        for path in base.rglob('*'):
            if path.is_symlink() or not path.is_file():
                continue
            require(len(rows) < 20000, 'task_inventory_bound_exceeded')
            try:
                if path.stat().st_size <= 1 << 20:
                    rows.append(task_summary(path.read_bytes(), str(path)))
                else:
                    unreadable += 1
            except OSError:
                unreadable += 1
    crontab = pathlib.Path('/etc/crontab')
    if crontab.is_file() and not crontab.is_symlink():
        rows.append(task_summary(crontab.read_bytes(), str(crontab)))
    for name, item in containers.items():
        config = item['Config']
        raw = json.dumps([config.get('Entrypoint'), config.get('Cmd')]).encode()
        rows.append(task_summary(raw, 'compose-runtime:' + name))
    return {'scanned': len(rows), 'unreadableOrOversized': unreadable,
            'candidates': [row for row in rows if row['suiteMention'] or row['cleanupMention']],
            'transitiveCommandTargetsAudited': False}


def audit():
    require(os.geteuid() == 0 and BASE.resolve() == BASE, 'root_and_canonical_suite_required')
    marker = json.loads(regular(BASE / 'openim/instance.json').read_bytes())
    activation = json.loads(regular(BASE / 'activation.json').read_bytes())
    require(marker.get('instanceID') == activation.get('instanceID') == INSTANCE
            and marker.get('production') is True and marker.get('testOnly') is False
            and marker.get('stackName') == PROJECT and marker.get('root') == str(BASE / 'openim'),
            'production_marker_mismatch')
    with regular(BASE / '.backup-stage/backup.lock').open('rb') as lock:
        fcntl.flock(lock, fcntl.LOCK_SH | fcntl.LOCK_NB)
        containers = owned_containers()
        compose = yaml.safe_load(regular(BASE / 'openim/compose.yaml').read_bytes())
        require(set(compose['services']) == SERVICES and compose.get('name') == PROJECT,
                'compose_service_set_mismatch')
        mounted_file(containers['openim-crontask'], CRON_FILE)
        runtime = containers['openim-crontask']['Config']
        require(runtime.get('Entrypoint') == ['/app/bin/openim-crontask']
                and runtime.get('Cmd') == ['-c', '/app/config'], 'cron_config_argument_mismatch')
        cron = cron_summary(yaml.safe_load(regular(CRON_FILE).read_bytes()))
        cron['configSHA256'] = sha(CRON_FILE.read_bytes())
        cron['environmentOverrideKeys'] = sorted(key for key, value in
            environment(containers['openim-crontask']).items() if value and key in (
                'IMENV_OPENIM_CRONTASK_RETAINCHATRECORDS', 'IMENV_OPENIM_CRONTASK_FILEEXPIRETIME',
                'IMENV_OPENIM_CRONTASK_DELETEOBJECTTYPE'))
        cron['effectivePermanentConfirmed'] = cron['permanent'] and not cron['environmentOverrideKeys']
        mongo = mongo_summary(json.loads(run(['docker', 'exec', '-i', containers['mongo']['Id'],
            'mongosh', '--nodb', '--quiet', '--file', '/dev/stdin'], MONGO_SCRIPT.encode(), 120)))
        keys = json.loads(regular(BASE / 'openim/secrets.json', private=True).read_bytes())
        secret = keys.get('minio')
        require(isinstance(secret, str) and re.fullmatch(r'[a-f0-9]{64}', secret), 'minio_key_shape_invalid')
        status, raw = s3_get(secret, '/')
        require(status == 200, 's3_bucket_inventory_failed')
        bucket_root = xml(raw)
        require(bucket_root.tag == 'ListAllMyBucketsResult' and bucket_root.find('Buckets') is not None,
                's3_bucket_inventory_invalid')
        buckets = bucket_root.findall('./Buckets/Bucket/Name')
        require(1 <= len(buckets) <= 1000 and any(row.text == 'openim38' for row in buckets),
                's3_bucket_bound_exceeded')
        lifecycle = []
        for bucket in buckets:
            name = bucket.text or ''
            require(re.fullmatch(r'[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]', name), 's3_bucket_name_invalid')
            lifecycle.append({'bucketSHA256': sha(name.encode()), **lifecycle_summary(*s3_get(
                secret, '/' + urllib.parse.quote(name, safe=''), 'lifecycle='))})
        return {'readOnly': True, 'instanceMatches': True, 'ownedRunningContainers': 24,
                'measuredAt': int(datetime.datetime.now(datetime.timezone.utc).timestamp()),
                'cron': cron, 'mongo': mongo, 'minio': lifecycle,
                'capacity': capacity_summary(containers), 'externalTasks': external_tasks(containers),
                'credentialsPrinted': False, 'credentialIssued': False, 'productionWrites': False}


if __name__ == '__main__':
    try:
        print(json.dumps(audit(), separators=(',', ':')))
    except BaseException as error:
        allowed = str(error) if type(error) is RuntimeError and re.fullmatch(r'[a-z0-9_]+', str(error)) else 'audit_dependency_unavailable'
        print(json.dumps({'readOnly': True, 'passed': False, 'error': allowed,
                          'credentialsPrinted': False, 'productionWrites': False}))
        sys.exit(1)
