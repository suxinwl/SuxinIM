"""Compare sanitized server /proc executable audit with local delivery files."""
import hashlib,json,pathlib
ROOT=pathlib.Path(__file__).resolve().parents[2]
report=json.loads((ROOT/'tests/delivery/running-binaries.json').read_text(encoding='utf-8'))
for item in report['binaries']:
 candidates=[ROOT/'artifacts/linux-amd64'/module/item['binary'] for module in ['openim-server','chat','livekit']]
 candidates.append(ROOT/'services/enterprise-service/dist/linux-amd64'/item['binary'])
 paths=[path for path in candidates if path.is_file()]
 if len(paths)!=1:raise SystemExit('Expected one local artifact for '+item['binary'])
 path=paths[0]
 with path.open('rb') as stream:digest=hashlib.file_digest(stream,'sha256').hexdigest()
 item.update(artifact=path.relative_to(ROOT).as_posix(),localSHA256=digest,localMatchesMounted=digest==item['mountedSHA256'],localMatchesRunningProcess=digest==item['processSHA256'])
report['binaryCount']=len(report['binaries']);report['allRunning']=all(item['running'] for item in report['binaries'])
report['allLocalMatches']=all(item['localMatchesMounted'] and item['localMatchesRunningProcess'] for item in report['binaries'])
(ROOT/'tests/delivery/binary-alignment.json').write_text(json.dumps(report,indent=2)+'\n',encoding='utf-8')
print(json.dumps({key:report[key] for key in ['containers','binaryCount','allRunning','allLocalMatches']}))
if not report['allLocalMatches'] or not report['allRunning']:raise SystemExit(1)
