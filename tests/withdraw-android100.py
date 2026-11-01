"""Withdraw only the identified synthetic Android startup-failure baseline."""
import hashlib,json,pathlib,sys
ROOT=pathlib.Path(__file__).resolve().parents[1]
sys.path.insert(0,str(ROOT/'tests/enterprise-service'))
from smoke import call
state=json.loads((ROOT/'.private/test-accounts.json').read_text())
admin=call('/enterprise/v1/admin/login',{'account':state['admin']['account'],'password':hashlib.md5(state['admin']['password'].encode()).hexdigest()})
page=call('/enterprise/v1/admin/chat/application/page_versions',{'platforms':['android'],'pagination':{'pageNumber':1,'showNumber':100}},admin)
items=page.get('versions',page.get('applications',[]))
matches=[v for v in items if v.get('platform')=='android' and v.get('version')=='3.8.3' and v.get('url','').endswith('/OpenIM38-3.8.3-100-development-arm64.apk')]
if matches:call('/enterprise/v1/admin/chat/application/delete_version',{'id':[v['id'] for v in matches]},admin)
report={'withdrawn':matches,'reason':'Main UI runtime regression: unconfigured legacy Getui PushManager dependency crashed after native login. Fixed replacement is a new application version; published hashes are not overwritten.'}
(ROOT/'tests/android100-withdrawal.json').write_text(json.dumps(report,indent=2)+'\n')
print(json.dumps({'withdrawnChatRecords':len(matches)}))
