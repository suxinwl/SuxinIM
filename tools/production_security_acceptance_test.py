import contextlib
import copy
import importlib.util
import io
import json
import pathlib
import tempfile
import time
import unittest
from unittest.mock import patch, Mock

SPEC=importlib.util.spec_from_file_location('security_acceptance',pathlib.Path(__file__).with_name('production_security_acceptance.py'))
m=importlib.util.module_from_spec(SPEC); SPEC.loader.exec_module(m)


def ok(data=None): return 200, {'code':0,'data':data}
def no(): return 403, {'errCode':403}
def fixtures():
 old=[{'uid':i+10,'employeeID':'old-'+str(i)} for i in range(6)]
 accounts=[{'uid':9001+i,'employeeID':'new-'+str(i),'username':m.ACCOUNTS[i],'name':m.NAMES[i],
            'password':'synthetic-strong-password-'+str(i),'mobile':'000000000'+('09' if i==0 else '10'),'wecom_userid':'',
            'userID':'emp_'+m.hashlib.sha256(('new-'+str(i)).encode()).hexdigest()[:32], 'remark':m.NAMESPACE+' fixture '+'a'*32+' '+str(i)} for i in range(2)]
 return {'namespace':m.NAMESPACE,'runID':'a'*32,'instanceID':m.INSTANCE,'departmentID':53,'accounts':accounts,
         'excludedUIDs':sorted(a['uid'] for a in old),'excludedEmployeeIDs':sorted(a['employeeID'] for a in old),
         'roles':[{'id':9101+i,'name':m.ROLE_NAMES[i],'remark':m.NAMESPACE+' isolated policy role '+'a'*32} for i in range(2)]},old


class Fake:
 def __init__(self,value,zone='lan'):
  self.fixture=value;self.zone=zone;self.actual_zone=zone;self.calls=[];self.sessions={};self.hosts={};self.tickets={};self.serial=0
  self.account={a['username']:{'uid':a['uid'],'password':m.md5(a['password']),'epoch':1,'must':True,'fail':0,'locked':0,'person':m.matrix('allow'),'rev':0} for a in value['accounts']}
  self.role_policy={};self.hrm=type('Hrm',(),{'crypt':staticmethod(lambda value,decrypt=False:value)})()
  self.inject_challenge_token=False;self.dependency=False
 def item(self,uid): return next(v for v in self.account.values() if v['uid']==uid)
 def state(self,item):return {'uid':item['uid'],'mustChangePassword':item['must'],'securityVersion':item['epoch'],'recentFailures':item['fail'],'locked':item['locked']>time.time(),'lockedUntil':item['locked']}
 def effective(self,uid,z,c):
  item=self.item(uid); rs=list(self.role_policy.values()) if uid==9001 else []
  login=not rs or any(r['matrix'][z][c]['login']=='allow' for r in rs)
  save=not rs or any(r['matrix'][z][c]['save']=='allow' for r in rs)
  personal=item['person'][z][c]
  if personal['login']!='inherit':login=personal['login']=='allow'
  if personal['save']!='inherit':save=personal['save']=='allow'
  return {'zone':z,'logicalClient':c,'login':login,'preview':login,'save':login and save,'securityVersion':item['epoch'],'policyRevision':1,'networkRevision':1}
 def answer(self,label): self.calls.append(('captcha',label));return {'captchaId':'valid','captcha':'answer'}
 def admin(self,path,method='GET',value=None):
  self.calls.append((method,path,copy.deepcopy(value)))
  route=path.split('?')[0]; q=m.urllib.parse.parse_qs(m.urllib.parse.urlsplit(path).query)
  if route==m.IM_BASE+'employees':return ok([{'id':a['employeeID'],'projection':{'enabled':True},'desiredVersion':self.item(a['uid'])['epoch'],'appliedVersion':self.item(a['uid'])['epoch'],'error':''} for a in self.fixture['accounts']])
  if route==m.IM_BASE+'access/employees':return ok([{'employeeID':a['employeeID'],'uid':a['uid'],'userID':a['userID'],'eligible':not self.item(a['uid'])['must'] and not self.item(a['uid'])['locked']>time.time()} for a in self.fixture['accounts']])
  if route.endswith('/lockPolicy'):return ok({'windowSeconds':600,'failures':5,'lockSeconds':1800,'revision':1})
  if route.endswith('/security'):return ok(self.state(self.item(int(q['id'][0]))))
  if route.endswith('/unlock'):
   item=self.item(value['id']);item.update(fail=0,locked=0);item['epoch']+=1;return ok({'unlocked':True})
  if route.endswith('/resetPassword'):
   item=self.item(value['id']);item.update(password=value['password'],must=True);item['epoch']+=1;return ok({'reset':True,'mustChangePassword':True})
  if route.endswith('/accessPolicy'):
   role='/role/' in route
   uid=value['id'] if method=='POST' else int(q['id'][0])
   if role:
    if method=='POST':self.role_policy[uid]={'matrix':copy.deepcopy(value['matrix']),'revision':self.role_policy.get(uid,{}).get('revision',0)+1};self.item(9001)['epoch']+=1
    return ok(self.role_policy.get(uid,{'matrix':m.matrix('allow'),'revision':0}))
   item=self.item(uid)
   if method=='POST':item['person']=copy.deepcopy(value['matrix']);item['rev']+=1;item['epoch']+=1
   return ok({'revision':item['rev'],'matrix':copy.deepcopy(item['person']),'effective':{z:{c:self.effective(uid,z,c) for c in m.CLIENTS} for z in m.ZONES}})
  raise AssertionError('Unhandled synthetic admin route: '+route)
 def valid(self,session):
  item=self.item(session['_uid']);return not item['must'] and not item['locked']>time.time() and item['epoch']==session['_epoch']
 def public(self,path,value=None,method='POST',session=None):
  self.calls.append((method,path,copy.deepcopy(value)))
  if self.dependency and path in ('/sso/refresh','/enterprise/v1/session/state'):return 503,{'code':503}
  if path=='/sso/login':
   item=self.account[value['account']]
   if value.get('captchaId')!='valid' or item['locked']>time.time():return no()
   if value['password']!=item['password']:
    item['fail']+=1
    if item['fail']==5:item.update(fail=0,locked=int(time.time())+1800);item['epoch']+=1
    return no()
   item['fail']=0
   if item['must']:
    ticket=m.hashlib.sha256(('ticket'+str(self.serial)).encode()).hexdigest();self.serial+=1;self.tickets[ticket]=(item,item['epoch'])
    data={'reasonCode':'PASSWORD_CHANGE_REQUIRED','requiresPasswordChange':True,'expiresAt':int(time.time())+300,'changeTicket':ticket}
    if self.inject_challenge_token:data['imToken']='forbidden-synthetic-token'
    return 428,{'code':428,'data':data}
   client=value['policyClient'];policy=self.effective(item['uid'],self.actual_zone,client)
   if not policy['login']:return no()
   token='synthetic-session-'+str(self.serial);self.serial+=1
   result={'userID':next(a['userID'] for a in self.fixture['accounts'] if a['uid']==item['uid']),'imToken':'synthetic-im-'+token,'chatToken':'synthetic-chat-'+token,'sessionToken':token,'policyClient':client,'effectivePolicy':policy,'_uid':item['uid'],'_epoch':item['epoch'],'_client':client}
   self.sessions[token]=result;return ok(copy.deepcopy(result))
  if path=='/sso/password/status':
   old=self.tickets.get(value['changeTicket']);return ok({'reasonCode':'PASSWORD_CHANGE_REQUIRED','requiresPasswordChange':True,'expiresAt':int(time.time())+300}) if old and old[0]['epoch']==old[1] else no()
  if path=='/sso/password/cancel':self.tickets.pop(value['changeTicket'],None);return ok({'cancelled':True})
  if path=='/sso/password/change':
   old=self.tickets.get(value.get('changeTicket'))
   if old:
    item,epoch=old
    if epoch!=item['epoch']:return no()
    del self.tickets[value['changeTicket']]
   else:
    actor=self.sessions.get(value.get('sessionToken'))
    if not actor or not self.valid(actor):return no()
    item=self.item(actor['_uid'])
    if item['password']!=value['oldPassword']:return no()
   item.update(password=value['newPassword'],must=False);item['epoch']+=1
   return ok({'changed':True,'requiresLogin':True})
  if path=='/sso/refresh':
   actor=self.sessions.get(value['sessionToken']);return ok(copy.deepcopy(actor)) if actor and self.valid(actor) else no()
  if path=='/enterprise/v1/session/state':
   if not session or not self.valid(session):return no()
   policy=self.effective(session['_uid'],self.actual_zone,session['_client'])
   return ok({'epoch':session['_epoch'],'logicalClient':session['_client'],'loginAllowed':policy['login'],'saveAllowed':policy['save'],'effectivePolicy':policy})
  if path=='/sso/logout':self.sessions.pop(value['sessionToken'],None);return ok({})
  raise AssertionError('Unhandled synthetic public route: '+path)
 def revocation_state(self,session,timeout):return self.public('/enterprise/v1/session/state',method='GET',session=session)
 def host(self,path,method='GET',value=None,token=None):
  self.calls.append((method,path,copy.deepcopy(value)))
  if path=='/admin/user/login':
   item=self.account[value['username']]
   if item['password']!=value['password'] or item['must'] or item['locked']>time.time():return no()
   token='synthetic-host-'+str(self.serial);self.serial+=1;self.hosts[token]=(item['uid'],item['epoch']);return ok(token)
  uid,epoch=self.hosts.get(token,(9001,-1));item=self.item(uid)
  return ok({'authenticated':True}) if item['epoch']==epoch and not item['locked']>time.time() else (200,{'code':401})


class Tests(unittest.TestCase):
 def engine(self,must=False,zone='lan'):
  value,_=fixtures();t=Fake(value,zone)
  if not must:
   for item in t.account.values():item['must']=False
  checks=[];return m.Acceptance(t,value,record=lambda name,passed:checks.append((name,passed))),t,checks
 def test_dry_run_has_no_transport_or_private_reads(self):
  with patch.object(m,'Transport',side_effect=AssertionError('network')),patch.object(m,'load',side_effect=AssertionError('private read')),contextlib.redirect_stdout(io.StringIO()) as stream:m.main(['dry-run'])
  result=json.loads(stream.getvalue());self.assertEqual(result['productionRequests'],0);self.assertFalse(result['globalPolicyWrites'])
 def test_new_namespace_and_all_six_disjoint(self):
  value,old=fixtures();self.assertEqual(len(m.validate_fixture(value,old)),2)
  for field,new in [('uid',10),('username','qa_im_a'),('employeeID','old-0')]:
   changed=copy.deepcopy(value);changed['accounts'][0][field]=new
   with self.subTest(field=field),self.assertRaises(m.Rejected):m.validate_fixture(changed,old)
 def test_matrix_independent_2_by_5_cells(self):
  value=m.matrix();self.assertEqual(sum(len(row) for row in value.values()),10)
  value['lan']['pc']['login']='deny';self.assertEqual(value['internet']['pc']['login'],'inherit');self.assertEqual(value['lan']['ios']['login'],'inherit')
 def test_dependency_is_not_denial(self):
  for result in [(503,{'code':503}),(404,{}),(200,{'code':1}),(200,{'code':0})]:self.assertFalse(m.denied(result))
  self.assertTrue(m.denied((200,{'code':401})));self.assertTrue(m.denied(no()))
 def test_role_or_personal_override_actual_model_and_control_unchanged(self):
  engine,t,checks=self.engine();before=t.item(9002)['epoch'];engine.roles();self.assertTrue(all(v for _,v in checks));self.assertEqual(t.item(9002)['epoch'],before);self.assertEqual(len([n for n,_ in checks if n.startswith('role_or_')]),10)
 def test_captcha_not_counted_five_wrongs_lock_1800_and_unlock_all_old_sessions(self):
  engine,t,checks=self.engine();engine.lock();self.assertTrue(all(v for _,v in checks));self.assertFalse(t.state(t.item(9001))['locked']);self.assertEqual(len([call for call in t.calls if call[0]=='captcha' and call[1].startswith('wrong-')]),5);self.assertEqual(t.item(9002)['epoch'],1)
 def test_self_change_old_password_reset_restricted_ticket_replay_and_epoch(self):
  engine,t,checks=self.engine();engine.password();self.assertTrue(all(v for _,v in checks));self.assertFalse(t.item(9001)['must']);self.assertTrue(any(n=='after-admin-reset_ticket_replay_denied' for n,_ in checks));self.assertEqual(t.item(9002)['epoch'],1)
 def test_bootstrap_ticket_does_not_issue_business_tokens(self):
  engine,t,checks=self.engine(True);engine.bootstrap();self.assertTrue(all(v for _,v in checks));self.assertTrue(all(not v['must'] for v in t.account.values()))
 def test_injected_full_token_in_428_fails_before_change(self):
  engine,t,_=self.engine(True);t.inject_challenge_token=True
  with self.assertRaisesRegex(m.Rejected,'restricted_no_session'):engine.restricted_change(engine.accounts[0],'bootstrap')
  self.assertFalse(any(call[1]=='/sso/password/change' for call in t.calls if len(call)>1))
 def test_dependency_cannot_pass_old_epoch_rejection(self):
  engine,t,_=self.engine();old=engine.login(engine.accounts[0],'capture');t.dependency=True
  with self.assertRaises(m.Rejected):engine.assert_revoked([old])
 def test_all_five_clients_deny_then_preview_save_and_both_zone_view(self):
  for zone in m.ZONES:
   engine,t,checks=self.engine(zone=zone);engine.clients();self.assertTrue(all(v for _,v in checks));self.assertEqual(len([n for n,_ in checks if n.startswith('login_denied_')]),5);self.assertEqual(len([n for n,_ in checks if n.startswith('two_zone_matrix_')]),10)
 def test_wan_url_cannot_claim_internet_when_actual_source_lan(self):
  engine,t,_=self.engine(zone='internet');t.actual_zone='lan'
  with self.assertRaisesRegex(m.Rejected,'real_source_zone'):engine.clients()
 def test_department_guard_is_get_only_and_refuses_enabled_or_leader(self):
  department={'id':53,'pid':0,'business_id':1,'status':0,'leader_uid':0,'im_group_enabled':0,'name':'TEMP QA 20261103','remark':'Temporary synthetic organization acceptance department'}
  calls=[]
  def admin(path,method='GET',value=None):calls.append((path,method));return ok([department])
  t=type('T',(),{'admin':staticmethod(admin)})();self.assertEqual(m.exact_department(t)['id'],53)
  for field in ['leader_uid','im_group_enabled']:
   department[field]=1
   with self.assertRaises(m.Rejected):m.exact_department(t)
   department[field]=0
  self.assertTrue(all(method=='GET' for _,method in calls))
 def test_account_create_accepts_new_wrapped_id_and_rejects_root(self):
  self.assertEqual(m.account_id({'id':9001}),9001)
  for value in [1,{'id':1},{},'x',None]:
   with self.assertRaises(m.Rejected):m.account_id(value)
 def test_preflight_rejects_old_or_partial_deployment_before_fixture_writes(self):
  engine,t,_=self.engine();m.preflight(t)
  calls=[]
  def missing(path,value=None,method='POST',session=None):calls.append((method,path));return 404,{}
  with patch.object(t,'public',side_effect=missing):
   with self.assertRaisesRegex(m.Rejected,'deployed_restricted'):m.preflight(t)
  self.assertFalse(any('/account/save' in call[1] or '/role/save' in call[1] for call in t.calls if len(call)>2))
 def test_main_failure_always_attempts_exact_quiescence_and_redacts_exception(self):
  value,_=fixtures();calls=[]
  with tempfile.TemporaryDirectory() as directory:
   state=pathlib.Path(directory)/'state.json';fixture=pathlib.Path(directory)/'fixture.json'
   def create(t,persist):state.write_text('{}');raise RuntimeError('SYNTHETIC SECRET MUST NOT LEAVE')
   with patch.object(m,'STATE',state),patch.object(m,'FIXTURE',fixture),patch.object(m,'Transport',return_value=object()),patch.object(m,'create',side_effect=create),patch.object(m,'preflight'),patch.object(m,'load',return_value=value),patch.object(m,'save'),patch.object(m,'quiesce',side_effect=lambda *a:(calls.append('cleanup') or {'fixtureCount':2})),contextlib.redirect_stdout(io.StringIO()) as output:
    with self.assertRaisesRegex(m.Rejected,'safe_details_withheld'):m.main(['run'])
   self.assertEqual(calls,['cleanup']);self.assertNotIn('SYNTHETIC SECRET',output.getvalue())
 def test_interrupt_during_explicit_resource_pause_still_quiesces_live_fixtures(self):
  value,_=fixtures();calls=[];transport=Mock()
  transport.public.return_value=ok({'epoch':1,'loginAllowed':True})
  engine=Mock();engine.security.return_value={'securityVersion':1};engine.login.return_value={'sessionToken':'synthetic-control'}
  with tempfile.TemporaryDirectory() as directory:
   state=pathlib.Path(directory)/'state.json';source=pathlib.Path(directory)/'fixture.json'
   def create(t,persist):state.write_text('{}');return value
   with patch.object(m,'STATE',state),patch.object(m,'FIXTURE',source),patch.object(m,'Transport',return_value=transport),patch.object(m,'preflight'),patch.object(m,'create',side_effect=create),patch.object(m,'wait_projection'),patch.object(m,'assign_roles'),patch.object(m,'Acceptance',return_value=engine),patch.object(m,'load',return_value=value),patch.object(m,'save'),patch.object(m,'quiesce',side_effect=lambda *a:(calls.append('cleanup') or {'fixtureCount':2})),patch('builtins.input',side_effect=KeyboardInterrupt),contextlib.redirect_stdout(io.StringIO()):
    with self.assertRaises(m.Rejected):m.main(['run','--operator-resource-pause'])
  self.assertEqual(calls,['cleanup'])
  self.assertEqual([call.args[1] for call in engine.login.call_args_list],['untouched-control-J','resource-preview-web','resource-save-pc'])



class CleanupFake(Fake):
 def __init__(self,value):
  super().__init__(value);self.deleted=set();self.roles_live=[{**r,'pid':0} for r in value['roles']]
  self.host_rows=[{'id':a['uid'],'username':a['username'],'name':a['name'],'dept_id':53,'mobile':a['mobile'],'remark':a['remark'],'roleid':[8001,*[r['id'] for r in value['roles']]],'status':0} for a in value['accounts']]
  self.employee_rows=[{'id':a['employeeID'],'user_id':a['uid'],'employee_no':a['username'],'name':a['name'],'dept_id':53,'mobile':a['mobile'],'wecom_userid':'','status':'active'} for a in value['accounts']]
  self.inventory_total=None;self.legacy_closed=set()
 def admin(self,path,method='GET',value=None):
  route=path.split('?')[0]
  if route=='/admin/system/dept/getList':return ok([{'id':53,'pid':0,'business_id':1,'status':0,'leader_uid':0,'im_group_enabled':0,'name':'TEMP QA 20261103','remark':'Temporary synthetic organization acceptance department'}])
  if route=='/admin/system/account/getList':
   found=[r for r in self.host_rows if r['id'] not in self.deleted];return ok({'items':copy.deepcopy(found),'total':self.inventory_total if self.inventory_total is not None else len(found)})
  if route=='/plugins/biz-suite/api/admin/hrm/employees':
   if method=='POST':
    self.calls.append((method,route,copy.deepcopy(value)));found=next(r for r in self.employee_rows if r['id']==value['id']);found.update(value)
   return ok(copy.deepcopy(self.employee_rows))
  if route==m.IM_BASE+'access/employees':return ok([{'employeeID':r['id'],'uid':r['user_id'],'revision':1,
    **{key:r['id'] not in self.legacy_closed for key in ('allowLAN','allowPC','allowMobile','allowInternet','allowInternetDownload')}} for r in self.employee_rows])
  if route==m.IM_BASE+'access/employee':
   self.calls.append((method,route,copy.deepcopy(value)));self.legacy_closed.add(value['employeeID']);return ok({})
  if route==m.IM_BASE+'employees':return ok([{'id':r['id'],'projection':{'enabled':False},'desiredVersion':1,'appliedVersion':1,'error':''} for r in self.employee_rows])
  if route=='/admin/system/role/getList':return ok(copy.deepcopy(self.roles_live))
  if route=='/admin/system/role/del':
   self.calls.append((method,route,copy.deepcopy(value)));self.roles_live=[r for r in self.roles_live if r['id'] not in value['ids']];return ok({})
  if route=='/admin/system/account/upStatus':
   self.calls.append((method,route,copy.deepcopy(value)));next(r for r in self.host_rows if r['id']==value['id'])['status']=1;return ok({})
  if route=='/admin/system/account/del':self.calls.append((method,route,copy.deepcopy(value)));self.deleted.update(value['ids']);return ok({})
  return super().admin(path,method,value)


class CleanupTests(unittest.TestCase):
 def work(self,fixture,transport,hidden_proof=False):
  stack=contextlib.ExitStack();self.addCleanup(stack.close)
  temp=stack.enter_context(tempfile.TemporaryDirectory());home=pathlib.Path(temp);source=home/'fixtures.json';source.write_text(json.dumps(fixture))
  stack.enter_context(patch.object(m,'HOME',home));stack.enter_context(patch.object(m,'FIXTURE',source))
  def save(path,value):path.write_bytes(m.canonical(value))
  stack.enter_context(patch.object(m,'save',side_effect=save))
  helper=type('H',(),{'EMPLOYEE_FIELDS':('id','user_id','wecom_userid','employee_no','name','dept_id','mobile','status')})()
  stack.enter_context(patch.dict(m.sys.modules,{'production_fixture_cleanup':helper}))
  checks=[]
  proof=role_proof(fixture,m.hashlib.sha256(source.read_bytes()).hexdigest()) if hidden_proof else None
  result=m.quiesce(transport,fixture,lambda state:None,lambda name,passed:checks.append((name,passed)),proof)
  return result,json.loads((home/'fixture-cleanup-plan.private.json').read_text()),checks
 def test_full_finally_closes_matrix_legacy_flags_disables_softdeletes_and_uses_reviewed_exact_worker_contract(self):
  fixture,_=fixtures();t=CleanupFake(fixture);result,plan,checks=self.work(fixture,t)
  self.assertEqual(result['employeePhysicalDeletePending'],2);self.assertEqual(result['rolesDeleted'],2);self.assertEqual(t.deleted,{9001,9002});self.assertEqual(t.legacy_closed,{'new-0','new-1'})
  self.assertTrue(all(p for _,p in checks));self.assertTrue(all(r['status']=='inactive' for r in t.employee_rows));self.assertEqual(t.roles_live,[])
  self.assertTrue(all(c['login']=='deny' and c['save']=='deny' for a in t.account.values() for z in a['person'].values() for c in z.values()))
  spec=importlib.util.spec_from_file_location('original_worker',m.ROOT/'deploy/production/fixtures_cleanup.py');worker=importlib.util.module_from_spec(spec);spec.loader.exec_module(worker)
  self.assertEqual(len(worker.validate_plan(plan,m.INSTANCE)),2);predicate=worker.employee_predicate([row['expected'] for row in plan['fixtures']]);self.assertIn("COALESCE(e.wecom_userid,'')=''",predicate);self.assertIn('a.dept_id=53',predicate)
  self.assertFalse(any('/dept/' in call[1] and call[0]!='GET' for call in t.calls if len(call)>2))
 def test_foreign_membership_blocks_recursive_role_delete_without_mutating_foreign_account(self):
  fixture,_=fixtures();t=CleanupFake(fixture);t.host_rows.append({'id':9999,'username':'other-synthetic','roleid':[9101]})
  with self.assertRaisesRegex(m.Rejected,'foreign_member'):self.work(fixture,t)
  self.assertFalse(any(call[1].endswith('/role/del') for call in t.calls if len(call)>2));self.assertNotIn(9999,t.deleted)
 def test_child_role_blocks_recursive_delete(self):
  fixture,_=fixtures();t=CleanupFake(fixture);t.roles_live.append({'id':9999,'pid':9101,'name':'other-synthetic'})
  with self.assertRaisesRegex(m.Rejected,'descendant'):self.work(fixture,t)
  self.assertFalse(any(call[1].endswith('/role/del') for call in t.calls if len(call)>2))
 def test_incomplete_membership_page_refuses_role_deletion(self):
  fixture,_=fixtures();t=CleanupFake(fixture);t.inventory_total=501
  with self.assertRaisesRegex(m.Rejected,'complete_membership'):self.work(fixture,t)
  self.assertFalse(any(call[1].endswith('/role/del') for call in t.calls if len(call)>2))
 def test_creation_reply_loss_recovers_only_exact_nonce_before_quiescence(self):
  fixture,_=fixtures();t=CleanupFake(fixture);fixture['accounts'][0]['createAttempted']=True
  fixture['accounts'][0].pop('uid');fixture['accounts'][0].pop('employeeID');fixture['roles'][0].pop('id')
  result,plan,_=self.work(fixture,t);self.assertEqual(result['fixtureCount'],2);self.assertEqual(fixture['accounts'][0]['uid'],9001);self.assertEqual(fixture['roles'][0]['id'],9101)
 def test_wrong_nonce_refuses_before_first_mutation(self):
  fixture,_=fixtures();t=CleanupFake(fixture);t.host_rows[0]['remark']='another run marker'
  with self.assertRaisesRegex(m.Rejected,'host_binding'):self.work(fixture,t)
  self.assertFalse(any(call[0] in ('POST','DELETE') for call in t.calls))
 def test_parent_sql_proof_allows_only_exact_hidden_roles_with_live_membership_recheck(self):
  fixture,_=fixtures();t=CleanupFake(fixture);t.roles_live=[]
  result,plan,_=self.work(fixture,t,hidden_proof=True)
  self.assertEqual(result['rolesDeleted'],2);self.assertEqual(len(plan['fixtures']),2)
  deletes=[call[2]['ids'] for call in t.calls if len(call)>2 and call[1].endswith('/role/del')]
  self.assertEqual(deletes,[[9101,9102]])
 def test_hidden_proof_never_skips_live_foreign_membership_guard(self):
  fixture,_=fixtures();t=CleanupFake(fixture);t.roles_live=[];t.host_rows.append({'id':9999,'roleid':[9101]})
  with self.assertRaisesRegex(m.Rejected,'foreign_member'):self.work(fixture,t,hidden_proof=True)
  self.assertFalse(any(call[1].endswith('/role/del') for call in t.calls if len(call)>2))
 def test_closed_inactive_pair_can_finish_hidden_role_cleanup_without_reenabling_or_reclosing(self):
  fixture,_=fixtures();t=CleanupFake(fixture);t.roles_live=[]
  for row in t.host_rows:row['status']=1
  for row in t.employee_rows:row['status']='inactive';t.legacy_closed.add(row['id'])
  for item in t.account.values():item['person']=m.matrix('deny')
  result,_,_=self.work(fixture,t,hidden_proof=True);self.assertEqual(result['rolesDeleted'],2)
  writes=[c for c in t.calls if len(c)>2 and c[0] in ('POST','DELETE')]
  self.assertTrue(writes);self.assertTrue(all(c[1] in ('/admin/system/role/del','/admin/system/account/del') for c in writes))


class CaptchaTransportTests(unittest.TestCase):
 def call(self,payload):
  transport=object.__new__(m.Transport);transport.public=Mock(return_value=ok(payload))
  with tempfile.TemporaryDirectory() as tmp,patch.object(m,'HOME',pathlib.Path(tmp)),patch.object(m,'save') as save,patch.dict(m.sys.modules,{'production_backup':type('Backup',(),{'protect':staticmethod(lambda path:None)})()}),patch('getpass.getpass',return_value='synthetic-answer'),contextlib.redirect_stdout(io.StringIO()):
   result=transport.answer('synthetic');self.assertEqual(save.call_count,1);return result
 def test_captcha_id_only_normal_response(self):
  self.assertEqual(self.call({'show':True,'captchaId':'synthetic-id','image':'data:image/png;base64,c3ludGhldGlj'}),{'captchaId':'synthetic-id','captcha':'synthetic-answer'})
 def test_legacy_id_only_normal_response(self):
  self.assertEqual(self.call({'show':True,'id':'legacy-id','img':'data:image/png;base64,c3ludGhldGlj'})['captchaId'],'legacy-id')
 def test_missing_or_empty_identifier_fails_before_saving(self):
  for payload in ({'show':True},{'show':True,'captchaId':''},{'show':True,'captchaId':None}):
   transport=object.__new__(m.Transport);transport.public=Mock(return_value=ok(payload))
   with patch.object(m,'save') as save,self.assertRaisesRegex(m.Rejected,'normal_captcha'):
    try:transport.answer('synthetic')
    finally:save.assert_not_called()


class DurableStateTests(unittest.TestCase):
 def test_nonce_file_synced_before_atomic_replace_and_parent_sync(self):
  with tempfile.TemporaryDirectory() as tmp:
   home=pathlib.Path(tmp).resolve();target=home/'state.json';events=[];real_replace=pathlib.Path.replace;real_fsync=m.os.fsync
   def fsync(descriptor):events.append('file-sync');real_fsync(descriptor)
   def replace(source,destination):events.append('replace');return real_replace(source,destination)
   with patch.object(m,'HOME',home),patch.dict(m.sys.modules,{'production_backup':type('Backup',(),{'protect':staticmethod(lambda path:None)})()}),patch.object(m.os,'fsync',side_effect=fsync),patch.object(pathlib.Path,'replace',new=replace),patch.object(m,'sync_parent',side_effect=lambda path:events.append('parent-sync')):
    m.save(target,{'runID':'synthetic'})
   self.assertEqual(events,['file-sync','replace','parent-sync']);self.assertEqual(json.loads(target.read_text()),{'runID':'synthetic'});self.assertEqual(list(home.glob('*.next-*')),[])
 def test_posix_parent_fsync_always_closes_descriptor(self):
  path=pathlib.Path('synthetic-state.json')
  with patch.object(m.os,'name','posix'),patch.object(m.os,'open',return_value=123) as opened,patch.object(m.os,'fsync',side_effect=OSError('synthetic EIO')),patch.object(m.os,'close') as closed:
   with self.assertRaises(OSError):m.sync_parent(path)
   opened.assert_called_once();closed.assert_called_once_with(123)


def role_proof(value,fixture_sha='b'*64):
 return {'format':'puty-security-hidden-role-proof-v1','namespace':m.NAMESPACE,'instanceID':m.INSTANCE,
  'runID':value['runID'],'fixtureSourceSHA256':fixture_sha,'capturedAt':int(time.time()),
  'roles':[{**r,'pid':0,'deleted':False,'childCount':0,'foreignMemberCount':0,'memberUIDs':[value['accounts'][0]['uid']]} for r in value['roles']]}


class RevocationRetryTests(unittest.TestCase):
 def engine(self,responses):
  value,_=fixtures();t=Fake(value);t.revocation_state=Mock(side_effect=responses);checks=[]
  return m.Acceptance(t,value,record=lambda name,passed:checks.append((name,passed))),t,checks
 def test_exact_503_1006_is_observed_then_explicit_rejection_passes(self):
  engine,t,checks=self.engine([(503,{'errCode':1006,'data':None}),(401,{'errCode':1501,'data':None})])
  with patch.object(m.time,'sleep'):engine.assert_im_revoked({},'old_im_0_rejected')
  self.assertEqual(checks,[('old_im_0_rejected',True)]);self.assertEqual(t.revocation_state.call_count,2)
  seen=engine.state['revocationObservations'];self.assertEqual([(r['status'],r['code']) for r in seen],[(503,1006),(401,1501)])
  self.assertTrue(all(set(r)=={'label','attempt','status','code','elapsedMS','durationMS'} for r in seen))
 def test_200_or_other_error_or_data_never_gets_a_retry_or_pass(self):
  values=[(200,{'code':0,'data':{}}),(200,{'code':401}),(503,{'errCode':1006,'data':{}}),
   (503,{'errCode':1006,'data':None,'imToken':'SYNTHETIC-FORBIDDEN'}),(503,{'errCode':503}),
   (503,{'errCode':'1006','data':None}),(401,{'errCode':1501,'data':{'userID':'SYNTHETIC'}})]
  for response in values:
   engine,t,checks=self.engine([response])
   with self.subTest(response=response),self.assertRaises(m.Rejected):engine.assert_im_revoked({},'old_im_0_rejected')
   self.assertEqual(t.revocation_state.call_count,1);self.assertEqual(checks[-1][1],False)
   self.assertNotIn('SYNTHETIC',json.dumps(engine.state.get('revocationObservations')))
 def test_continuous_dependency_has_a_ten_second_budget_and_never_passes(self):
  engine,t,checks=self.engine(None);t.revocation_state=Mock(return_value=(503,{'errCode':1006,'data':None}));clock=[0.0]
  with patch.object(m.time,'monotonic',side_effect=lambda:clock[0]),patch.object(m.time,'sleep',side_effect=lambda delay:clock.__setitem__(0,clock[0]+delay)):
   with self.assertRaises(m.Rejected):engine.assert_im_revoked({},'old_im_0_rejected')
  self.assertEqual(clock[0],10);self.assertEqual(t.revocation_state.call_count,20);self.assertEqual(checks[-1][1],False)
 def test_transport_error_is_redacted_and_fails_without_retry(self):
  engine,t,checks=self.engine([OSError('SYNTHETIC-TOKEN-AND-URL')])
  with self.assertRaises(m.Rejected):engine.assert_im_revoked({},'old_im_0_rejected')
  self.assertEqual(t.revocation_state.call_count,1);self.assertNotIn('SYNTHETIC',json.dumps(engine.state['revocationObservations']))
 def test_transport_passes_remaining_timeout_and_exact_session_only_in_headers(self):
  t=object.__new__(m.Transport);t.origin=m.ORIGINS['lan'];t.http=Mock(return_value=no())
  self.assertEqual(t.revocation_state({'imToken':'synthetic-im','chatToken':'synthetic-chat','_platform':3},1.25),no())
  self.assertEqual(t.http.call_args.kwargs['timeout'],1.25);self.assertEqual(t.http.call_args.kwargs['headers']['platformID'],'3')


class ResumeTests(unittest.TestCase):
 def prepared(self):
  stack=contextlib.ExitStack();self.addCleanup(stack.close);home=pathlib.Path(stack.enter_context(tempfile.TemporaryDirectory())).resolve()
  source,old=fixtures();state=copy.deepcopy(source)
  for a in state['accounts']:a['baseRoleIDs']=[8001]
  report={'namespace':m.NAMESPACE,'zone':'lan','checks':[{'check':'original_'+str(i),'passed':True} for i in range(50)]+
   [{'check':'old_im_0_rejected','passed':False},{'check':'cleanup_requires_operator_review_accounts_must_remain_disabled','passed':False}]}
  fixture=home/'test-accounts.json';state_path=home/'state.json';report_path=home/'report.json';proof_path=home/'proof.json'
  for p,v in ((fixture,source),(state_path,state),(report_path,report)):p.write_bytes(m.canonical(v))
  h=lambda p:m.hashlib.sha256(p.read_bytes()).hexdigest()
  proof=role_proof(state,h(fixture));proof_path.write_bytes(m.canonical(proof))
  args=type('Args',(),{'fixture_sha256':h(fixture),'state_sha256':h(state_path),'report_sha256':h(report_path),
   'role_proof_sha256':h(proof_path),'role_proof':proof_path,'zone':'lan'})()
  t=CleanupFake(state);t.roles_live=[]
  t.host_rows[1]['roleid']=[8001]
  for row in t.host_rows:row['status']=1
  for row in t.employee_rows:row['status']='inactive'
  for item in t.account.values():item['person']=m.matrix('deny');item['must']=False
  original_admin=t.admin
  def admin(path,method='GET',value=None):
   if path==m.IM_BASE+'access/employees':return ok([{'employeeID':a['employeeID'],'uid':a['uid'],'revision':1,
    **{k:False for k in ('allowLAN','allowPC','allowMobile','allowInternet','allowInternetDownload')}} for a in state['accounts']])
   return original_admin(path,method,value)
  t.admin=admin
  for name,value in (('HOME',home),('FIXTURE',fixture),('STATE',state_path),('REPORT',report_path)):stack.enter_context(patch.object(m,name,value))
  stack.enter_context(patch.object(m,'previous',return_value=old))
  stack.enter_context(patch.dict(m.sys.modules,{'production_backup':type('B',(),{'protect':staticmethod(lambda path:None)})()}))
  return args,t,home,report_path.read_bytes()
 def test_resume_only_validates_gets_then_durably_preserves_exact_original_report(self):
  args,t,home,raw=self.prepared();value,proof,selections=m.prepare_resume(t,args)
  self.assertEqual(len(selections),2);self.assertEqual(proof['fixtureSourceSHA256'],args.fixture_sha256)
  self.assertEqual((home/('original-attempt-'+args.report_sha256+'.json')).read_bytes(),raw)
  self.assertEqual(m.load(m.STATE)['resume']['originalStateSHA256'],args.state_sha256)
  self.assertFalse(any(c[0] in ('POST','DELETE') for c in t.calls));self.assertEqual(m.REPORT.read_bytes(),raw)
 def test_wrong_hash_failed_label_role_proof_or_state_rejects_before_any_mutation(self):
  for kind in ('hash','failure','proof','status','matrix','plan'):
   args,t,home,_=self.prepared()
   if kind=='hash':args.state_sha256='0'*64
   elif kind=='failure':
    v=json.loads(m.REPORT.read_bytes());v['checks'][-2]['check']='some_other_failure';m.REPORT.write_bytes(m.canonical(v));args.report_sha256=m.hashlib.sha256(m.REPORT.read_bytes()).hexdigest()
   elif kind=='proof':
    v=json.loads(args.role_proof.read_bytes());v['roles'][0]['childCount']=1;args.role_proof.write_bytes(m.canonical(v));args.role_proof_sha256=m.hashlib.sha256(args.role_proof.read_bytes()).hexdigest()
   elif kind=='status':t.host_rows[0]['status']=0
   elif kind=='matrix':t.item(9001)['person']=m.matrix('allow')
   else:(home/'fixture-cleanup-plan.private.json').write_text('{}')
   with self.subTest(kind=kind),self.assertRaises(m.Rejected):m.prepare_resume(t,args)
   self.assertFalse(any(c[0] in ('POST','DELETE') for c in t.calls));self.assertEqual(list(home.glob('original-attempt-*')),[])
 def test_hidden_role_proof_is_exact_fresh_and_can_never_include_another_member(self):
  value,_=fixtures();proof=role_proof(value);m.validate_role_proof(proof,value,'b'*64)
  for mutate in (lambda p:p.update(fixtureSourceSHA256='0'*64),lambda p:p.update(capturedAt=int(time.time())-1801),
    lambda p:p['roles'][0].update(pid=1),lambda p:p['roles'][0].update(childCount=1),
    lambda p:p['roles'][0].update(foreignMemberCount=1),lambda p:p['roles'][0].update(memberUIDs=[9002])):
   v=copy.deepcopy(proof);mutate(v)
   with self.assertRaises(m.Rejected):m.validate_role_proof(v,value,'b'*64)
 def test_existing_original_receipt_reflush_failure_blocks_resume_without_report_overwrite(self):
  args,t,home,raw=self.prepared();archive=home/('original-attempt-'+args.report_sha256+'.json');archive.write_bytes(raw)
  for failure in ('file','parent'):
   with self.subTest(failure=failure):
    target=patch.object(m.os,'fsync',side_effect=OSError('SYNTHETIC-FSYNC')) if failure=='file' else patch.object(m,'sync_parent',side_effect=OSError('SYNTHETIC-PARENT-FSYNC'))
    with target,self.assertRaises(OSError):m.prepare_resume(t,args)
    self.assertEqual(m.REPORT.read_bytes(),raw);self.assertEqual(m.hashlib.sha256(m.STATE.read_bytes()).hexdigest(),args.state_sha256)
    self.assertFalse(any(c[0] in ('POST','DELETE') for c in t.calls))
  m.prepare_resume(t,args);self.assertEqual(archive.read_bytes(),raw)
 def test_restoration_never_recreates_accounts_roles_or_passwords(self):
  args,t,home,_=self.prepared();value,proof,selections=m.prepare_resume(t,args);calls=[]
  with patch.object(m,'wait_login_ready'),patch.dict(m.sys.modules,{'production_fixture_cleanup':type('H',(),{'EMPLOYEE_FIELDS':('id','user_id','employee_no','name','dept_id','mobile','status','wecom_userid')})()}):
   m.restore_existing_pair(t,value,selections,lambda v:None,lambda n,p:calls.append((n,p)))
  writes=[c for c in t.calls if c[0] in ('POST','DELETE')]
  allowed={'/admin/system/account/upStatus','/plugins/biz-suite/api/admin/hrm/employees',m.IM_BASE+'access/employee','/admin/system/account/accessPolicy','/admin/system/account/unlock'}
  self.assertTrue(writes);self.assertTrue(all(c[1] in allowed for c in writes));self.assertTrue(all('password' not in (c[2] or {}) for c in writes))
  self.assertEqual({c[2]['id'] for c in writes if c[1].endswith('/upStatus')},{9001,9002});self.assertTrue(all(p for _,p in calls))


class RemainingTests(unittest.TestCase):
 prepared=ResumeTests.prepared
 def remaining(self):
  args,t,home,_=self.prepared();state=m.load(m.STATE);state['quiesced']=True;t.host_rows[0]['roleid']=[8001]
  m.STATE.write_bytes(m.canonical(state));args.state_sha256=m.hashlib.sha256(m.STATE.read_bytes()).hexdigest()
  report={'namespace':m.NAMESPACE,'zone':'lan','checks':[{'check':'old_'+str(i),'passed':True} for i in range(50)]+
   [{'check':'self_change_no_new_session','passed':True},{'check':'old_im_0_rejected','passed':True},{'check':'finally_fixture_quiesced','passed':True}]}
  m.REPORT.write_bytes(m.canonical(report));args.report_sha256=m.hashlib.sha256(m.REPORT.read_bytes()).hexdigest()
  plan={'instanceID':m.INSTANCE,'acceptanceNamespace':m.NAMESPACE,'fixtureSourceSHA256':args.fixture_sha256,
   'fixtures':[{'expected':{'uid':a['uid'],'employeeID':a['employeeID']}} for a in state['accounts']]}
  (home/'fixture-cleanup-plan.private.json').write_bytes(m.canonical(plan));args.cleanup_plan_sha256=m.hashlib.sha256((home/'fixture-cleanup-plan.private.json').read_bytes()).hexdigest()
  progress={'instanceID':m.INSTANCE,'passed':True,'planSHA256':args.cleanup_plan_sha256}
  (home/'fixture-cleanup-progress.private.json').write_bytes(m.canonical(progress));args.cleanup_progress_sha256=m.hashlib.sha256((home/'fixture-cleanup-progress.private.json').read_bytes()).hexdigest()
  proof={'format':'puty-security-host-restore-v1','namespace':m.NAMESPACE,'instanceID':m.INSTANCE,'runID':state['runID'],
   'fixtureSourceSHA256':args.fixture_sha256,'stateSourceSHA256':args.state_sha256,'reportSourceSHA256':args.report_sha256,
   'cleanupPlanSHA256':args.cleanup_plan_sha256,'cleanupProgressSHA256':args.cleanup_progress_sha256,'capturedAt':int(time.time()),
   'restoredRows':2,'hosts':[{**{k:a[k] for k in ('uid','employeeID','username','remark','baseRoleIDs')},'status':1,
     'deletetimeCleared':True,'passwordSaltUnchanged':True,'epochUnchanged':True} for a in state['accounts']],
   'retiredRoleIDs':[r['id'] for r in state['roles']],'retiredRoleRows':0,'retiredMembershipRows':0}
  args.host_restore_proof=home/'host-restored.json';args.host_restore_proof.write_bytes(m.canonical(proof));args.host_restore_proof_sha256=m.hashlib.sha256(args.host_restore_proof.read_bytes()).hexdigest()
  return args,t,home,state,proof
 def test_remaining_hash_chain_preserves_all_four_prior_receipts_and_never_mutates_in_preflight(self):
  args,t,home,state,proof=self.remaining();raws={p:p.read_bytes() for p in (m.STATE,m.REPORT,home/'fixture-cleanup-plan.private.json',home/'fixture-cleanup-progress.private.json')}
  value,actual,selections=m.prepare_remaining(t,args);self.assertEqual(len(selections),2);self.assertEqual(actual,proof);self.assertFalse(value['quiesced'])
  for p,raw in raws.items():self.assertTrue(any(saved.read_bytes()==raw for saved in home.glob('original-*.json')))
  self.assertFalse(any(c[0] in ('POST','DELETE') for c in t.calls));self.assertEqual(len(value['roles']),2)
 def test_remaining_rejects_wrong_receipt_foreign_base_role_and_incomplete_prior_stage_before_enabling(self):
  for kind in ('hash','role','password','report','chain'):
   args,t,home,state,proof=self.remaining()
   if kind=='hash':args.host_restore_proof_sha256='0'*64
   elif kind=='role':t.host_rows[0]['roleid'].append(9999)
   elif kind=='password':state['accounts'][0]['pendingPassword']='SYNTHETIC';m.STATE.write_bytes(m.canonical(state));args.state_sha256=m.hashlib.sha256(m.STATE.read_bytes()).hexdigest()
   elif kind=='report':proof['restoredRows']=3;args.host_restore_proof.write_bytes(m.canonical(proof));args.host_restore_proof_sha256=m.hashlib.sha256(args.host_restore_proof.read_bytes()).hexdigest()
   else:args.cleanup_progress_sha256='0'*64
   with self.subTest(kind=kind),self.assertRaises(m.Rejected):m.prepare_remaining(t,args)
   self.assertFalse(any(c[0] in ('POST','DELETE') for c in t.calls));self.assertEqual(list(home.glob('original-*.json')),[])
 def test_retired_roles_are_never_deleted_again_and_reappearing_membership_is_rejected(self):
  args,t,home,state,proof=self.remaining()
  self.assertEqual(m.isolated_role_inventory(t,state,t.host_rows,proof),set())
  t.host_rows[0]['roleid'].append(state['roles'][0]['id'])
  with self.assertRaisesRegex(m.Rejected,'reappeared'):m.isolated_role_inventory(t,state,t.host_rows,proof)
 def test_remaining_main_never_calls_create_bootstrap_roles_lock_or_self_change(self):
  args,t,home,state,proof=self.remaining();engine=Mock();engine.security.return_value={'securityVersion':1};engine.login.return_value={'sessionToken':'synthetic-control'}
  transport=Mock();transport.public.return_value=ok({'epoch':1,'loginAllowed':True})
  with patch.object(m,'Transport',return_value=transport),patch.object(m,'prepare_remaining',return_value=(state,proof,[])),patch.object(m,'preflight'),patch.object(m,'restore_existing_pair'),patch.object(m,'Acceptance',return_value=engine),patch.object(m,'create') as create,patch.object(m,'assign_roles') as assign,patch.object(m,'save'),patch.object(m,'load',return_value=state),patch.object(m,'quiesce',return_value={'fixtureCount':2}),contextlib.redirect_stdout(io.StringIO()):
   m.main(['resume-remaining'])
  create.assert_not_called();assign.assert_not_called()
  for name in ('bootstrap','roles','lock','password'):getattr(engine,name).assert_not_called()
  engine.admin_reset_remaining.assert_called_once();engine.clients.assert_called_once()
 def test_projection_wait_rejects_old_matching_version_until_the_new_version_is_applied(self):
  value,_=fixtures();target=value['accounts'][0]['employeeID'];other=value['accounts'][1]['employeeID']
  def result(desired,applied):return ok([{'id':target,'projection':{'enabled':True},'desiredVersion':desired,'appliedVersion':applied,'error':''},
    {'id':other,'projection':{'enabled':True},'desiredVersion':1,'appliedVersion':1,'error':''}])
  t=Mock();t.admin.side_effect=[result(1,1),result(2,1),result(2,2)]
  with patch.object(m.time,'sleep'):m.wait_projection(t,value,minimum_versions={target:1})
  self.assertEqual(t.admin.call_count,3)
 def test_failed_sso_response_is_persisted_as_fixed_class_without_body_or_credentials(self):
  value,_=fixtures();t=Fake(value);t.public=Mock(return_value=(403,{'code':403,'msg':'SYNTHETIC-SECRET-RAW','data':None}));engine=m.Acceptance(t,value)
  with self.assertRaisesRegex(m.Rejected,'unexpected_api_decision'):engine.login(value['accounts'][0],'before-admin-reset')
  observation=value['loginObservations'][-1];self.assertEqual(observation['status'],403);self.assertEqual(observation['messageClass'],'unclassified')
  self.assertEqual(set(observation),{'label','client','status','code','durationMS','hasData','messageClass'});self.assertNotIn('SYNTHETIC',json.dumps(observation));self.assertEqual(t.public.call_count,1)
 def test_login_barrier_requires_current_eligible_even_when_the_old_projection_looks_ready(self):
  value,_=fixtures()
  def access(eligible):return ok([{'employeeID':a['employeeID'],'uid':a['uid'],'userID':a['userID'],'eligible':eligible} for a in value['accounts']])
  projected=ok([{'id':a['employeeID'],'projection':{'enabled':True},'desiredVersion':1,'appliedVersion':1,'error':''} for a in value['accounts']])
  t=Mock();t.admin.side_effect=[access(False),projected,access(True),projected]
  with patch.object(m.time,'sleep'):m.wait_login_ready(t,value)
  self.assertEqual(t.admin.call_count,4)


class AdminObservationTests(unittest.TestCase):
 def transport(self,response=None,error=None):
  t=object.__new__(m.Transport);t.admin_token='SYNTHETIC-ADMIN-TOKEN';t.admin_observations=[];t.admin_observation_id='a'*16
  t.host=Mock(return_value=response,side_effect=error);return t
 def test_admin_observations_are_durable_fixed_metadata_without_query_body_or_message(self):
  t=self.transport((200,{'code':1,'msg':'SYNTHETIC-PASSWORD-HASH-ECHO','data':{'password':'SYNTHETIC'}}))
  with patch.object(m,'save') as saved:
   t.admin('/admin/system/account/resetPassword?id=SYNTHETIC-QUERY','POST',{'id':9001,'password':'SYNTHETIC-INPUT'})
  saved.assert_called_once();record=t.admin_observations[0]
  self.assertEqual(record['label'],'account_reset_password');self.assertEqual(record['code'],1);self.assertTrue(record['hasData'])
  self.assertEqual(set(record),{'label','method','status','code','hasData','durationMS','decision'})
  self.assertNotIn('SYNTHETIC',json.dumps(saved.call_args.args[1]));self.assertEqual(record['decision'],'api_rejected')
 def test_admin_transport_failure_is_recorded_and_redacted_before_safe_exception(self):
  t=self.transport(error=OSError('SYNTHETIC-SECRET-URL'))
  with patch.object(m,'save') as saved,self.assertRaisesRegex(m.Rejected,'admin_transport_failure'):
   t.admin('/admin/system/account/security?id=SYNTHETIC')
  saved.assert_called_once();self.assertEqual(t.admin_observations[0]['decision'],'transport_failure')
  self.assertIsNone(t.admin_observations[0]['status']);self.assertNotIn('SYNTHETIC',json.dumps(saved.call_args.args[1]))


if __name__=="__main__":unittest.main()
