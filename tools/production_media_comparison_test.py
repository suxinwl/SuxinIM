"""Pure offline metadata classification/guard checks; no production reads."""
import copy
import importlib.util
import json
import pathlib
import re
import unittest
from unittest import mock

ROOT=pathlib.Path(__file__).resolve().parents[1]
spec=importlib.util.spec_from_file_location('media_comparison_tested',ROOT/'deploy/production/inspect_media_comparison.py')
worker=importlib.util.module_from_spec(spec);spec.loader.exec_module(worker)


def subjects():
    return [{'label':label,'employeeID':'synthetic-'+label,'userID':'emp_'+worker.digest('synthetic-'+label)[:32],
             'employeeNo':number} for label,number in [('affected','1234'),('reference','5678')]]


class MediaComparisonTests(unittest.TestCase):
    def test_exact_two_subject_mappings_and_no_other_input_fields(self):
        source={'readOnly':True,'subjects':subjects()}
        worker.validate_subjects(source)
        for change in ('mapping','label','field','number','duplicate'):
            value=copy.deepcopy(source)
            if change=='mapping':value['subjects'][0]['userID']='emp_other'
            if change=='label':value['subjects'][0]['label']='some-person-name'
            if change=='field':value['subjects'][0]['password']='not-permitted'
            if change=='number':value['subjects'][0]['employeeNo']='anything'
            if change=='duplicate':value['subjects'][1]=value['subjects'][0]
            with self.subTest(change=change),self.assertRaises(RuntimeError):worker.validate_subjects(value)

    def test_origin_path_and_broken_second_query_are_only_categories(self):
        raw='https://im.szputy.com:9885/attachments/'+'a'*64+'/openim38/private-file?X-Amz-Date=20200101T000000Z&X-Amz-Expires=60&X-Amz-Signature=not-a-live-credential?type=image'
        result=worker.classify(raw,'synthetic-viewer',{}, {},10000000000)
        self.assertEqual(result['origin'],'im-wan')
        self.assertEqual(result['pathKind'],'grant')
        self.assertEqual(result['secondQuestionMark'],'true')
        self.assertEqual(result['querySignatureExpiry'],'expired')
        self.assertEqual(result['grantRegistry'],'missing-or-TTL-expired')
        self.assertNotIn('private-file',json.dumps(result))
        self.assertEqual(worker.classify('http://legacy.invalid:10002/object/x','',{}, {},0)['origin'],'non-https-http')
        self.assertEqual(worker.classify('https://untrusted.invalid/api/object/x','',{}, {},0)['origin'],'unconfigured-origin')

    def test_live_grant_requires_original_sid_epoch_current_projection_and_platform(self):
        token='b'*64; sid='synthetic-noncredential-sid'; user='emp_synthetic'
        grant={'userID':user,'sessionID':sid,'platform':3,'epoch':2,'expiresAt':200}
        state={'HRM38:SESSION:'+worker.digest(sid):json.dumps({'userID':user,'platform':3,'epoch':2,'expiresAt':200,'revoked':False}),
               'HRM38:USER:'+user:json.dumps({'enabled':True}), 'HRM38:EPOCH:'+user:'2',
               'HRM38:CURRENT:'+user+':3':sid}
        url='https://192.168.10.11:9885/attachments/'+token+'/openim38/hash-key'
        self.assertEqual(worker.classify(url,user,{token:grant},state,100)['grantSourceSID'],'live-registry')
        self.assertEqual(worker.classify(url,'other-viewer',{token:grant},state,100)['grantOwnerRelation'],'other-user')
        for change in ('revoked','expired','epoch','current','projection','missing'):
            changed=copy.deepcopy(state)
            key='HRM38:SESSION:'+worker.digest(sid)
            session=json.loads(changed[key])
            if change=='revoked':session['revoked']=True
            if change=='expired':session['expiresAt']=99
            if change=='epoch':changed['HRM38:EPOCH:'+user]='3'
            if change=='current':changed['HRM38:CURRENT:'+user+':3']='replacement'
            if change=='projection':changed['HRM38:USER:'+user]=json.dumps({'enabled':False})
            changed[key]=json.dumps(session)
            if change=='missing':del changed[key]
            with self.subTest(change=change):self.assertEqual(worker.classify(url,user,{token:grant},changed,100)['grantSourceSID'],'not-live-registry')

    def test_summary_contains_no_url_token_user_or_message_body(self):
        people=subjects();secret_url='https://private-origin.invalid/openim38/private-filename?signature=not-a-real-credential'
        metadata={'records':[{'label':'affected','channel':'moment','self':False,'urls':[{'field':'original','url':secret_url}]}],
                  'stats':{'affected':{'momentCandidates':1},'reference':{'momentCandidates':0}}}
        result=worker.summarize(metadata,people,{}, {},100)
        output=json.dumps(result)
        for private in ['private-origin','private-filename','not-a-real-credential',people[0]['userID'],people[1]['employeeID']]:self.assertNotIn(private,output)
        self.assertEqual(result['affected']['urlCategoryCounts']['moment/other/origin/unconfigured-origin'],1)

    def test_mysql_is_one_read_only_transaction_and_both_mappings_must_stay_ready(self):
        people=subjects();calls=[]
        rows=[{'employeeID':r['employeeID'],'employeeNo':r['employeeNo'],'userID':r['userID'],'sourceUID':10+i,'uid':10+i,
               'sourceStatus':'active','status':'ready','enabled':1,'desired':4,'applied':4,'errorEmpty':1} for i,r in enumerate(people)]
        def execute(argv,data=None,timeout=60):calls.append((argv,data));return json.dumps(rows).encode()
        with mock.patch.object(worker,'execute',execute):
            worker.verify_mysql('mock-container',people)
            sql=calls[0][1].decode()
            self.assertIn('START TRANSACTION READ ONLY',sql);self.assertTrue(sql.endswith('ROLLBACK;\n'))
            self.assertIsNone(re.search(r'\b(DELETE|INSERT|UPDATE|CREATE|DROP)\b',sql))
            self.assertNotIn(people[0]['employeeID'],sql)
            self.assertIn('MYSQL_PWD="$MYSQL_ROOT_PASSWORD"',calls[0][0][-1])
            rows[0]['enabled']=0
            with self.assertRaises(RuntimeError):worker.verify_mysql('mock-container',people)

    def test_mongo_only_bounded_reads_and_does_not_print_bodies(self):
        source=worker.MONGO_SCRIPT
        self.assertIn('.limit(50)',source);self.assertIn('{$limit:50}',source)
        self.assertIn("'msgs.msg.content_type':{$in:[102,104,105]}",source)
        self.assertIn("'msgs.revoke':{$eq:null}",source)
        self.assertIn('urls:urls(data)',source)
        self.assertIn('print(JSON.stringify({records,stats}))',source)
        self.assertIsNone(re.search(r'\.(insert|update|delete|drop|save|create)\w*\s*\(',source))
        self.assertNotIn('$out',source);self.assertNotIn('$merge',source)

    def test_registry_only_exact_bounded_mget_no_scan_or_secret_argv(self):
        calls=[]
        def execute(argv,data=None,timeout=60):calls.append((argv,data));return b'[null]'
        with mock.patch.object(worker,'execute',execute):
            key='HRM38:ATTACH:'+'a'*64
            self.assertEqual(worker.redis_mget('mock-redis',[key]),{key:None})
            self.assertEqual(calls[0][1],('MGET '+key+'\n').encode())
            self.assertNotIn('-a',calls[0][0]);self.assertIn('REDISCLI_AUTH="$REDIS_PASSWORD"',calls[0][0][-1])
            with self.assertRaises(RuntimeError):worker.redis_mget('mock-redis',['SCAN 0'])


if __name__=='__main__':unittest.main()
