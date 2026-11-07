#!/usr/bin/env python3
"""Verify and install only this deployment's frozen backend binaries."""
import argparse,hashlib,json,pathlib,re,shutil,subprocess,tarfile
p=argparse.ArgumentParser();p.add_argument('--sha256',required=True);args=p.parse_args()
assert re.fullmatch('[0-9a-f]{64}',args.sha256)
base=pathlib.Path('/srv/puty-suite/openim')
marker=json.loads((base/'instance.json').read_text())
assert marker['production'] is True and marker['testOnly'] is False and marker['root']==str(base)
archive=base.parent/'staging/backend-binaries.tar.gz'
assert hashlib.sha256(archive.read_bytes()).hexdigest()==args.sha256
ids=subprocess.check_output(['docker','ps','-q','--filter','label=com.docker.compose.project=puty-suite-prod'],text=True).split()
for cid in ids:
    labels=json.loads(subprocess.check_output(['docker','inspect','--format','{{json .Config.Labels}}',cid],text=True))
    assert labels.get('com.puty.suite')==marker['instanceID']
    assert labels.get('com.docker.compose.service') in ['mysql','mongo','redis','etcd','kafka','minio','livekit'],'Use the scoped update tool while application containers are running'
stage=base.parent/'staging/backend-verified'
stage.mkdir(mode=0o700,exist_ok=False)
with tarfile.open(archive,'r:gz') as tar:
    for item in tar.getmembers():
        assert item.isfile() and pathlib.PurePosixPath(item.name).name==item.name and not item.name.startswith('.')
    tar.extractall(stage,filter='data')
manifest=json.loads((stage/'backend-binaries.json').read_text())
assert len(manifest)==17
for name,entry in manifest.items():
    file=stage/name
    raw=file.read_bytes()
    assert len(raw)==entry['bytes'] and hashlib.sha256(raw).hexdigest()==entry['sha256'],name
    assert raw[:5]==b'\x7fELF\x02' and raw[18:20]==b'\x3e\x00',name
for name in manifest:
    target=base/'bin'/name
    assert not target.is_dir() and not target.is_symlink()
    shutil.copyfile(stage/name,target)
    target.chmod(0o755)
shutil.copyfile(stage/'backend-binaries.json',base/'bin/backend-binaries.json')
print(json.dumps({'backendInstalled':True,'binaries':len(manifest),'allSHA256Verified':True}))
