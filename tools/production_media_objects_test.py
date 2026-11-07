"""Offline exact object lookup/HEAD guards, with synthetic values only."""
import datetime
import importlib.util
import json
import pathlib
import re
import unittest
from unittest import mock

ROOT=pathlib.Path(__file__).resolve().parents[1]
spec=importlib.util.spec_from_file_location('media_objects_tested',ROOT/'deploy/production/inspect_media_objects.py')
worker=importlib.util.module_from_spec(spec);spec.loader.exec_module(worker)


class ObjectChecks(unittest.TestCase):
    def test_collection_matches_openim_s3_persistence_and_exact_fields(self):
        constants=(ROOT/'services/openim-server/pkg/common/storage/database/name.go').read_text()
        collection=re.search(r'\bObjectName\s*=\s*"([^"]+)"',constants).group(1)
        self.assertEqual(collection,'s3')
        self.assertIn("getDB('openim38').getCollection('"+collection+"')",worker.MAPPING_SCRIPT)
        self.assertNotIn("getDB('openim38').object",worker.MAPPING_SCRIPT)
        persistence=(ROOT/'services/openim-server/pkg/common/storage/database/mgo/object.go').read_text()
        self.assertIn('db.Collection(database.ObjectName)',persistence)
        self.assertIn('bson.M{"name": obj.Name, "engine": obj.Engine}',persistence)
        self.assertIn("filter={engine:'minio'}",worker.MAPPING_SCRIPT)
        self.assertIn("item.kind==='alias'?'name':'key'",worker.MAPPING_SCRIPT)

    def test_descriptor_preserves_exact_decoding_and_rejects_unconfigured_or_traversal(self):
        self.assertEqual(worker.descriptor('https://im.szputy.com:9885/api/object/emp_fake/photo%20100%25.png?type=image'),('alias','emp_fake/photo 100%.png'))
        self.assertEqual(worker.descriptor('https://192.168.10.11:9885/attachments/'+'a'*64+'/openim38/openim/data/hash?signature=fixture'),('key','openim/data/hash'))
        for url in ['http://im.szputy.com:9885/openim38/key','https://other.invalid/openim38/key',
                    'https://im.szputy.com:9885/openim38/a/%2e%2e/key','https://im.szputy.com:9885/openim38/a//key',
                    'https://im.szputy.com:9885/openim38/a%00key','https://im.szputy.com:9885/openim38/bad%',
                    'https://im.szputy.com:9885/attachments/not-a-grant/openim38/key']:
            with self.subTest(url=url):self.assertIsNone(worker.descriptor(url))

    def test_exact_head_signing_keeps_path_and_never_contains_raw_secret(self):
        secret='a'*64;now=datetime.datetime(2026,11,3,12,0,tzinfo=datetime.timezone.utc)
        path,headers=worker.head_headers('folder/photo 100%.png','fixture-access',secret,now)
        self.assertEqual(path,'/openim38/folder/photo%20100%25.png')
        self.assertEqual(headers['Host'],'127.0.0.1:19000')
        self.assertEqual(headers['X-Amz-Date'],'20261103T120000Z')
        self.assertIn('SignedHeaders=host;x-amz-content-sha256;x-amz-date',headers['Authorization'])
        self.assertNotIn(secret,json.dumps(headers))
        with self.assertRaises(RuntimeError):worker.head_headers('../unsafe','x',secret,now)

    def test_head_only_no_redirect_no_body_and_error_categories_are_inconclusive(self):
        requests=[]
        class Response:
            status=200
            def getheader(self,name):return '123'
            def read(self,*args):raise AssertionError('Pixels must not be read')
        class Connection:
            def __init__(self,*args,**kwargs):requests.append((args,kwargs))
            def request(self,method,path,headers):requests.append((method,path,headers))
            def getresponse(self):return Response()
            def close(self):pass
        with mock.patch.object(worker.http.client,'HTTPConnection',Connection):
            for status,expected in [(200,{'status':200,'size':123}),(404,{'status':404}),(403,{'status':'inconclusive'}),(302,{'status':'inconclusive'}),(503,{'status':'inconclusive'})]:
                Response.status=status
                self.assertEqual(worker.head_object('exact-key','fixture-access','b'*64),expected)
        self.assertTrue(all(r[0]=='HEAD' for r in requests[1::2]))
        self.assertTrue(all(r[0]==('127.0.0.1',19000) for r in requests[0::2]))

    def test_mapping_is_exact_bounded_and_unknown_schema_has_no_fallback(self):
        calls=[]
        rows=[{'kind':'alias','value':'emp_fake/photo.png','state':'mapped','engine':'minio','name':'emp_fake/photo.png','key':'exact-key','size':'123'}]
        def execute(argv,data=None,timeout=60):calls.append((argv,data));return json.dumps(rows).encode()
        with mock.patch.object(worker.probe,'execute',execute):
            found=worker.mappings('mock-mongo',{('alias','emp_fake/photo.png')})
            self.assertEqual(found[('alias','emp_fake/photo.png')]['key'],'exact-key')
            self.assertNotIn('$regex',calls[0][1].decode())
            rows[0]['engine']='other'
            self.assertEqual(worker.mappings('mock-mongo',{('alias','emp_fake/photo.png')})[('alias','emp_fake/photo.png')]['state'],'unsupported-schema')
            with self.assertRaises(RuntimeError):worker.mappings('mock-mongo',{('alias',str(i)) for i in range(129)})

    def test_minio_endpoint_requires_owned_exact_loopback_binding_before_env_read(self):
        calls=[]
        def execute(argv,data=None,timeout=60):
            calls.append(argv)
            return json.dumps({'9000/tcp':[{'HostIp':'0.0.0.0','HostPort':'19000'}]}).encode()
        with mock.patch.object(worker.probe,'execute',execute),self.assertRaises(RuntimeError):worker.minio_credentials('mock-minio')
        self.assertEqual(len(calls),1)

    def test_summary_never_exposes_alias_key_url_or_service_credentials(self):
        alias='emp_private/private-image.png';key='private-exact-key'
        metadata={'records':[{'label':'affected','channel':'chat-102','self':False,'urls':[{'url':'https://im.szputy.com:9885/api/object/'+alias}]}],
                  'stats':{'affected':{},'reference':{}}}
        mapped={('alias',alias):{'state':'mapped','key':key,'size':'123'}}
        report=worker.summarize(metadata,mapped,{key:{'status':200,'size':123}})
        output=json.dumps(report)
        for private in [alias,key,'https://','Authorization','Credential=']:self.assertNotIn(private,output)
        self.assertEqual(report['affected']['exactObjectCounts']['chat-102/other/HEAD/200'],1)


if __name__=='__main__':unittest.main()
