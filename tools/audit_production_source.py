"""Check pending source changes for the actual generated production secrets."""
import base64,hashlib,json,pathlib,subprocess
ROOT=pathlib.Path(__file__).resolve().parents[1]
private=ROOT/'.private/production'
values=[]
def add_value(category, value):
    if not isinstance(value, str) or len(value) < 16:
        return
    values.append((category, value.encode()))
    try:
        values.append((category+':base64', base64.b64encode(bytes.fromhex(value))))
    except ValueError:
        pass
for file in ['suite-secrets.json','openim/secrets.json']:
    for key,value in json.loads((private/file).read_text()).items():
        if key not in ['instanceID','startAt','adminUsername'] and isinstance(value,str) and len(value)>=16:
            values.append((file+':'+key,value.encode()))
            try:
                values.append((file+':'+key+':base64',base64.b64encode(bytes.fromhex(value))))
            except ValueError:
                pass
            if key=='bootstrap':
                values.append((file+':'+key+':md5',hashlib.md5(value.encode()).hexdigest().encode()))
runtime=json.loads((private/'gosuxin/runtime-environment.json').read_text(encoding='utf-8-sig'))
add_value('runtime:WeCom-encryption',runtime['WECOM_CREDENTIAL_KEY'])
push_registration = private/'push-registration.json'
if push_registration.exists():
    push = json.loads(push_registration.read_text(encoding='utf-8-sig'))
    if push.get('format') != 'puty-android-push-registration-v1':
        raise RuntimeError('Unexpected private push configuration format')
    # Official client SDK registration identifiers are separate from privileged
    # server API credentials. Only the latter are forbidden in public sources.
    for provider, fields in {
        'getui': ('serverAppKey', 'serverMasterSecret'),
        'huawei': ('serverOAuthClientSecret',),
        'xiaomi': ('serverAppSecret',),
        'oppo': ('serverMasterSecret',),
    }.items():
        section = push.get(provider, {})
        for field in fields:
            add_value('push-server:'+provider+':'+field, section.get(field, ''))
admin_override = private/'admin-login.json'
if admin_override.exists():
    password = json.loads(admin_override.read_text(encoding='utf-8-sig')).get('password', '')
    if len(password) >= 8:
        values.append(('current-admin-password', password.encode()))
        values.append(('current-admin-password:md5', hashlib.md5(password.encode()).hexdigest().encode()))
for bundle_path in sorted((private/'key-rotation').glob('rotate-*/bundle.json')):
    bundle=json.loads(bundle_path.read_text(encoding='utf-8'))
    if bundle.get('format')!='puty-runtime-key-rotation-v1':
        raise RuntimeError('Unexpected private rotation bundle format')
    for side in ('old','new'):
        for key,value in bundle[side].items():
            add_value('runtime-rotation:'+side+':'+key,value)
suffixes={'.go','.py','.ps1','.sh','.js','.mjs','.ts','.tsx','.vue','.java','.kt','.dart','.json','.yaml','.yml','.xml','.gradle','.properties','.toml','.md','.txt','.ini','.env','.swift','.m','.mm','.c','.h'}
findings=[];scanned=0
repositories=[('openim',ROOT),('gosuxin',pathlib.Path('E:/PUTY_AI/kaoqinstyem/gosuxinwl'))]
for label,repository in repositories:
    paths=set()
    for args in [['diff','HEAD','--name-only','-z'],['ls-files','--others','--exclude-standard','-z']]:
        paths.update(p for p in subprocess.check_output(['git',*args],cwd=repository,stderr=subprocess.PIPE).decode().split('\0') if p)
    for relative in sorted(paths):
        file=repository/relative
        if not file.is_file() or file.suffix.lower() not in suffixes or file.stat().st_size>2*1024*1024:continue
        data=file.read_bytes();scanned+=1
        for category,value in values:
            if value in data:findings.append({'file':label+'/'+relative,'category':category})
report={'scannedPendingSourceFiles':scanned,'actualSecretFindings':findings,'passed':not findings}
(private/'acceptance/source-secret-audit.json').write_text(json.dumps(report,indent=2)+'\n')
print(json.dumps(report));raise SystemExit(1 if findings else 0)
