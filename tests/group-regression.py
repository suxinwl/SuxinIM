"""Fixed-release group permission regressions, on synthetic accounts only."""
import hashlib,json,pathlib,secrets,sys,time
ROOT=pathlib.Path(__file__).resolve().parents[1]
sys.path.insert(0,str(ROOT/'tests/enterprise-service'))
from smoke import call
state=json.loads((ROOT/'.private/test-accounts.json').read_text(encoding='utf-8'))
admin=call('/enterprise/v1/admin/login',{'account':state['admin']['account'],'password':hashlib.md5(state['admin']['password'].encode()).hexdigest()})
people=state['users'][-3:];old,new,member=[x['userID'] for x in people]
created=call('/enterprise/v1/admin/im/group/create_group',{'ownerUserID':old,'memberUserIDs':[new,member,state['users'][100]['userID']],'groupInfo':{'groupName':'3.8 群权限合成回归','groupType':2}},admin)
gid=created['groupInfo']['groupID'];prefix='/enterprise/v1/admin/im/group/';cases=[]
try:
 call(prefix+'mute_group_member',{'groupID':gid,'userID':new,'mutedSeconds':300},admin)
 call(prefix+'transfer_group',{'groupID':gid,'oldOwnerUserID':old,'newOwnerUserID':new},admin)
 data=call(prefix+'get_group_members_info',{'groupID':gid,'userIDs':[old,new,member]},admin)
 by_id={u['userID']:u for u in data['members']}
 assert by_id[new]['roleLevel']==100 and by_id[new]['muteEndTime']==0
 assert by_id[old]['roleLevel']!=100
 cases.append({'case':'Muted member becomes owner; mute removed and previous owner demoted','passed':True})
 call(prefix+'set_group_member_info',{'members':[{'groupID':gid,'userID':member,'roleLevel':60}]},admin)
 data=call(prefix+'get_group_members_info',{'groupID':gid,'userIDs':[member]},admin)
 assert data['members'][0]['roleLevel']==60
 cases.append({'case':'Updated members[] API persists roleLevel60','passed':True})
 login=call('/chat/account/login',{'email':people[0]['email'],'password':hashlib.md5(state['password'].encode()).hexdigest(),'platform':5,'deviceID':'group-regression'})
 session={**login,'platformID':5}
 for source in [-1,0,5]:
  try:call('/api/group/join_group',{'groupID':gid,'joinSource':source},session)
  except RuntimeError as error:
   if 'code=1001' not in str(error):raise
  else:raise AssertionError('Invalid joinSource accepted')
 cases.append({'case':'joinSource -1, 0 and 5 rejected by fixed protocol validation','passed':True})
finally:
 call(prefix+'dismiss_group',{'groupID':gid},admin)
(ROOT/'tests/group-regression-report.json').write_text(json.dumps({'groupID':gid,'syntheticOnly':True,'cases':cases,'sdkRoleNotificationObserved':False},indent=2)+'\n')
print(json.dumps({'passedCases':len(cases),'sdkRoleNotificationObserved':False}))
