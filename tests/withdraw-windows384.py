"""Withdraw only the synthetic Windows baseline with the upstream DB-close bridge defect."""
import hashlib,json,pathlib,sys
ROOT=pathlib.Path(__file__).resolve().parents[1]
sys.path.insert(0,str(ROOT/'tests/enterprise-service'))
from smoke import call
state=json.loads((ROOT/'.private/test-accounts.json').read_text(encoding='utf-8'))
admin=call('/enterprise/v1/admin/login',{'account':state['admin']['account'],'password':hashlib.md5(state['admin']['password'].encode()).hexdigest()})
page=call('/enterprise/v1/admin/chat/application/page_versions',{'platforms':['windows'],'pagination':{'pageNumber':1,'showNumber':100}},admin)
matches=[v for v in page.get('versions',page.get('applications',[])) if v.get('platform')=='windows' and v.get('version')=='3.8.4' and v.get('url','').endswith('/OpenIM38-Enterprise-3.8.4-win-x64.exe')]
if matches:call('/enterprise/v1/admin/chat/application/delete_version',{'id':[v['id'] for v in matches]},admin)
(ROOT/'tests/windows384-withdrawal.json').write_text(json.dumps({'withdrawn':matches,'reason':'The pinned official WASM calls window.close from IndexDB.Close on logout. Electron closes the main window; a scoped database-close bridge is delivered in the next application revision.'},indent=2)+'\n',encoding='utf-8')
print(json.dumps({'withdrawnChatRecords':len(matches)}))
