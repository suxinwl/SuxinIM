"""Assemble and verify frozen Linux binaries for the owned production stack."""
import hashlib,json,pathlib,tarfile
ROOT=pathlib.Path(__file__).resolve().parents[1]
OUT=ROOT/'.private/production'
build=ROOT/'services/.build/linux'
expected=['openim-api','openim-msggateway','openim-msgtransfer','openim-push','openim-crontask','openim-rpc-auth','openim-rpc-user','openim-rpc-friend','openim-rpc-group','openim-rpc-msg','openim-rpc-conversation','openim-rpc-third','chat-api','chat-rpc','admin-api','admin-rpc','enterprise-service']
sources={}
manifest={}
for group in ['openim','chat','enterprise']:
    entries=json.loads((build/group/'SHA256.json').read_text(encoding='utf-8-sig'))
    if isinstance(entries,dict): entries=[entries]
    for entry in entries:
        path=pathlib.Path(entry['Path'])
        if path.name not in expected: continue
        digest=hashlib.sha256(path.read_bytes()).hexdigest()
        assert digest.lower()==entry['Hash'].lower(),path.name
        sources[path.name]=path
        manifest[path.name]={'sha256':digest,'bytes':path.stat().st_size}
assert set(sources)==set(expected),set(expected)-set(sources)
record=OUT/'backend-binaries.json'
record.write_text(json.dumps(manifest,indent=2)+'\n',encoding='utf-8')
archive=OUT/'backend-binaries.tar.gz'
with tarfile.open(archive,'w:gz',compresslevel=1) as tar:
    for name,path in sorted(sources.items()): tar.add(path,arcname=name)
    tar.add(record,arcname='backend-binaries.json')
report={'archive':str(archive),'binaries':len(sources),'bytes':archive.stat().st_size,'sha256':hashlib.sha256(archive.read_bytes()).hexdigest()}
(OUT/'backend-archive.json').write_text(json.dumps(report,indent=2)+'\n',encoding='utf-8')
print(json.dumps(report))
