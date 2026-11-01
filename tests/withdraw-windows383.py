"""Withdraw only the synthetic Windows duplicate-SDK-logout baseline."""
import hashlib,json,pathlib,sys
ROOT=pathlib.Path(__file__).resolve().parents[1]
sys.path.insert(0,str(ROOT/'tests/enterprise-service'))
from smoke import call
state=json.loads((ROOT/'.private/test-accounts.json').read_text())
admin=call('/enterprise/v1/admin/login',{'account':state['admin']['account'],'password':hashlib.md5(state['admin']['password'].encode()).hexdigest()})
page=call('/enterprise/v1/admin/chat/application/page_versions',{'platforms':['windows'],'pagination':{'pageNumber':1,'showNumber':100}},admin)
matches=[v for v in page.get('versions',page.get('applications',[])) if v.get('platform')=='windows' and v.get('version')=='3.8.3' and v.get('url','').endswith('/OpenIM38-Enterprise-3.8.3-win-x64.exe')]
if matches:call('/enterprise/v1/admin/chat/application/delete_version',{'id':[v['id'] for v in matches]},admin)
(ROOT/'tests/windows383-withdrawal.json').write_text(json.dumps({'withdrawn':matches,'reason':'Explicit logout and server kick could concurrently call WASM SDK logout; new build shares a single in-flight logout operation.'},indent=2)+'\n')
print(json.dumps({'withdrawnChatRecords':len(matches)}))
