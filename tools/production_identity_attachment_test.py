"""Offline guards for the explicit synthetic-object inventory and cleanup scope."""
import base64
import contextlib
import copy
import hashlib
import io
import json
import pathlib
import tempfile
import time
import types
import unittest
from unittest import mock

import production_identity_acceptance_server as worker


class ExactAttachmentTest(unittest.TestCase):
    def setUp(self):
        self.owner = 'emp_'+worker.digest('fixture-a')[:32]
        self.users = [self.owner, 'emp_'+worker.digest('fixture-b')[:32]]
        self.now = int(time.time())
        self.canonical = bytes(i % 251 for i in range(4096))

    def item(self, mode, body=None):
        raw = self.canonical if body is None else body
        source = hashlib.md5(hashlib.md5(raw).hexdigest().encode()).hexdigest()
        item = {'userID': self.owner, 'name': self.owner+'/puty-client-acceptance-11111111-2222-3333-4444-555555555555.bin', 'size': len(raw), 'bodySHA256': hashlib.sha256(raw).hexdigest(), 'uploaded': True, 'createdAt': self.now, 'platform': 3, 'bodyMode': mode, 'objectKey': 'openim/data/hash/'+source, 'expectedSourceUploadHash': source, 'contentType': 'application/octet-stream'}
        if mode == 'android-main':
            item.update(platform=2, clientMsgID='a'*32, name=self.owner+'/msg_file_'+'a'*32+'/puty-android-main-'+'b'*24+'.txt', contentType='text/plain')
        if mode == 'identity-form':
            item.update(platform=2, name=self.owner+'/production-acceptance-'+'c'*24+'.txt', objectKey='openim/direct/20261103/'+self.owner+'/'+'d'*32+'.txt', contentType='text/plain', bodyMD5=hashlib.md5(raw).hexdigest(), bodyBase64=base64.b64encode(raw).decode())
            item.pop('expectedSourceUploadHash')
        return item

    def valid(self, item):
        return worker.exact_fixture_descriptor(item, self.users, self.now-1, self.now+1)

    def test_windows_formats_stay_exact(self):
        item = self.item('deterministic')
        self.assertTrue(self.valid(item))
        self.assertTrue(worker.exact_fixture_body(item, self.canonical))
        self.assertFalse(self.valid({**item, 'name': self.owner+'/other.bin'}))
        self.assertFalse(self.valid({**item, 'size': 436}))
        self.assertFalse(self.valid({**item, 'bodyMode': 'arbitrary'}))
        self.assertFalse(self.valid({**item, 'contentType': 'text/plain'}))
        missing_mime = dict(item)
        missing_mime.pop('contentType')
        self.assertFalse(self.valid(missing_mime))
        raw = b'x'*32+self.canonical[32:]
        nonce = self.item('nonce32', raw)
        self.assertTrue(self.valid(nonce))
        self.assertTrue(worker.exact_fixture_body(nonce, raw))
        self.assertFalse(worker.exact_fixture_body(nonce, self.canonical))

    def test_android_exact_msg_owner_and_sha(self):
        item = self.item('android-main', b'x'*4096)
        self.assertTrue(self.valid(item))
        self.assertTrue(worker.exact_fixture_body(item, b'x'*4096))
        self.assertFalse(worker.exact_fixture_body(item, b'y'*4096))
        for delta in ({'platform': 3}, {'contentType': 'application/octet-stream'}, {'clientMsgID': 'f'*32}, {'objectKey': 'openim/data/hash/'+'0'*32}, {'name': self.users[1]+'/msg_file_'+'a'*32+'/puty-android-main-'+'b'*24+'.txt'}):
            self.assertFalse(self.valid({**item, **delta}))

    def test_form_fixed_marker_bytes_and_owner_key(self):
        raw = (b'PUTY production identity acceptance fixture\n'+b'a'*64+b'\n')*4
        self.assertEqual(len(raw), 436)
        item = self.item('identity-form', raw)
        self.assertTrue(self.valid(item))
        self.assertTrue(worker.exact_fixture_body(item, raw))
        for delta in ({'objectKey': 'openim/direct/20261103/'+self.users[1]+'/'+'d'*32+'.txt'}, {'bodySHA256': '0'*64}, {'bodyBase64': '???'}, {'bodyMD5': '0'*32}, {'contentType': 'application/octet-stream'}):
            self.assertFalse(self.valid({**item, **delta}))
        invalid = self.item('identity-form', b'x'*436)
        self.assertFalse(self.valid(invalid))

    def test_paths_and_unknown_modes_rejected(self):
        item = self.item('android-main')
        for name in (self.owner+'/../x', self.owner+'//x', self.owner+'/msg_file_'+'a'*32+'/puty-android-main-'+'b'*24+'.txt\n', item['name'].replace('/', '\\')):
            self.assertFalse(self.valid({**item, 'name': name}))
        self.assertFalse(self.valid({**item, 'bodyMode': 'exact-sha256'}))

    def inventory_case(self, outside=False):
        raw = (b'PUTY production identity acceptance fixture\n'+b'a'*64+b'\n')*4
        items = [self.item('deterministic'), self.item('android-main', b'x'*4096), self.item('identity-form', raw)]
        alias = {**items[0], 'name': items[0]['name'].replace('11111111', '99999999')}
        items.insert(1, alias)
        bodies = {items[0]['objectKey']: self.canonical, items[2]['objectKey']: b'x'*4096, items[3]['objectKey']: raw}
        rows = [{'_id': format(i+1, '024x'), 'name': item['name'], 'engine': 'minio', 'key': item['objectKey'], 'size': item['size'], 'content_type': item.get('contentType', 'application/octet-stream'), 'createdAt': self.now, 'references': (1 if i < 2 else 0)+(1 if outside and i == 0 else 0), 'group': 'acceptance' if item['bodyMode'] == 'identity-form' else '', 'rawDocument': '{}'} for i, item in enumerate(items)]
        payload = {'fixtureEmployeeIDs': ['fixture-a', 'fixture-b'], 'fixtureIMUserIDs': self.users, 'createdAfter': self.now-10, 'createdBefore': self.now+1, 'clientUploads': items}
        def storage(env, port, method, key, body_limit=1024):
            self.assertIn(method, ('GET', 'HEAD'))
            body = bodies[key]
            self.assertLessEqual(len(body), body_limit if method == 'GET' else 4096)
            return 200, body if method == 'GET' else b'', {'content-length': str(len(body)), 'etag': '"'+hashlib.md5(body).hexdigest()+'"'}
        with tempfile.TemporaryDirectory() as folder:
            path = pathlib.Path(folder)/'plan.json'
            args = types.SimpleNamespace(action='client-attachment-inventory', inventory_file=str(path), mongo_container='mock', minio_port=19000)
            with mock.patch.object(worker, 'mongo_json', return_value=rows) as mongo, mock.patch.object(worker, 'storage', side_effect=storage), contextlib.redirect_stdout(io.StringIO()):
                if outside:
                    with self.assertRaises(worker.Failure):
                        worker.attachment_cleanup(args, {}, payload)
                    self.assertFalse(path.exists())
                else:
                    worker.attachment_cleanup(args, {}, payload)
                    self.assertEqual(len(json.loads(path.read_text())['rows']), 4)
                    script = mongo.call_args.args[1]
                    self.assertIn('relaxed:false', script)
                    self.assertNotIn('$regex', script)
                    self.assertIn('$or', script)

    def test_mixed_inventory_reads_only_exact_names_and_preserves_shared_aliases(self):
        self.inventory_case()

    def test_any_reference_outside_approved_exact_aliases_aborts(self):
        self.inventory_case(outside=True)

    def test_apply_keeps_large_BSON_Long_in_canonical_CAS_and_verifies_absence(self):
        raw = b'x'*4096
        item = self.item('android-main', raw)
        row = {'_id': '1'*24, 'name': item['name'], 'engine': 'minio', 'key': item['objectKey'], 'size': 4096, 'content_type': 'text/plain', 'createdAt': self.now, 'references': 0, 'group': '', 'rawDocument': '{"_id":{"$oid":"'+'1'*24+'"},"revision":{"$numberLong":"9223372036854775807"}}', 'sha256': hashlib.sha256(raw).hexdigest()}
        payload = {'fixtureEmployeeIDs': ['fixture-a', 'fixture-b'], 'fixtureIMUserIDs': self.users, 'createdAfter': self.now-1, 'createdBefore': self.now+1, 'clientUploads': [item]}
        gone = False
        def storage(env, port, method, key, body_limit=1024):
            nonlocal gone
            self.assertEqual(key, item['objectKey'])
            if method == 'DELETE':
                gone = True
                return 204, b'', {}
            if gone:
                self.assertEqual(method, 'HEAD')
                return 404, b'', {}
            return 200, raw if method == 'GET' else b'', {'content-length': '4096', 'etag': '"'+hashlib.md5(raw).hexdigest()+'"'}
        replies = [{'matches': 1, 'references': 0, 'approvedExact': 1, 'outsideApproved': 0}, {'matches': 1, 'references': 0}, {'deleted': 1}, {'remaining': 0}]
        with tempfile.TemporaryDirectory() as folder:
            path = pathlib.Path(folder)/'plan.json'
            path.write_text(json.dumps({'scope': payload, 'rows': [row]}))
            args = types.SimpleNamespace(action='client-attachment-cleanup', inventory_file=str(path), mongo_container='mock', minio_port=19000, apply=True)
            with mock.patch.object(worker, 'mongo_json', side_effect=replies) as mongo, mock.patch.object(worker, 'storage', side_effect=storage), contextlib.redirect_stdout(io.StringIO()):
                worker.attachment_cleanup(args, {}, payload)
            scripts = [call.args[1] for call in mongo.call_args_list]
            self.assertTrue(gone)
            self.assertEqual(len(scripts), 4)
            self.assertTrue(all('9223372036854775807' in script and ',{relaxed:false})' in script for script in scripts))
            self.assertEqual(scripts[0].count(',{relaxed:false})'), 2)
            self.assertTrue(json.loads(path.read_text())['rows'][0]['metadataDeletedAt'])


if __name__ == '__main__':
    unittest.main()
