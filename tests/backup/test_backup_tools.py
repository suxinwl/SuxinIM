"""No production/Docker operations: guards, archive paths and encryption checks."""
import copy,hashlib,importlib.util,io,json,pathlib,shutil,sys,tarfile,tempfile,unittest
from unittest.mock import patch

SUITE=pathlib.Path(__file__).resolve().parents[2]
sys.path.insert(0,str(SUITE/'deploy'))
import stack_guard
spec=importlib.util.spec_from_file_location('backup_tool',SUITE/'deploy/backup-tool.py')
backup=importlib.util.module_from_spec(spec);spec.loader.exec_module(backup)

class BackupToolTests(unittest.TestCase):
 def setUp(self):
  self.temporary=tempfile.TemporaryDirectory();self.addCleanup(self.temporary.cleanup)
  self.root=pathlib.Path(self.temporary.name).resolve()
  self.spec={'name':'openim38-upgrade','volumes':{'mongo-data':{'name':'openim38-upgrade_mongo-data'}},'networks':{'openim38':{'name':'openim38-upgrade-net'}},'services':{'mongo':{'volumes':[{'type':'volume','source':'mongo-data','target':'/data/db'},{'type':'bind','source':str(self.root/'config'),'target':'/config'}]}}}
 def test_valid_isolated_spec(self):stack_guard.validate_spec(self.spec,self.root)
 def test_foreign_resources_and_bind_mounts_refused(self):
  for mutation in ('project','external volume','foreign volume','host-backed volume','foreign network','outside bind','host network','container network','public port','privileged'):
   with self.subTest(mutation=mutation):
    spec=copy.deepcopy(self.spec)
    if mutation=='project':spec['name']='production'
    if mutation=='external volume':spec['volumes']['mongo-data']['external']=True
    if mutation=='foreign volume':spec['volumes']['mongo-data']['name']='gosuxin-suite_mongo-data'
    if mutation=='host-backed volume':spec['volumes']['mongo-data']['driver_opts']={'type':'none','o':'bind','device':'/production'}
    if mutation=='foreign network':spec['networks']['openim38']['name']='production-net'
    if mutation=='outside bind':spec['services']['mongo']['volumes'][1]['source']=str(self.root.parent/'production')
    if mutation=='host network':spec['services']['mongo']['network_mode']='host'
    if mutation=='container network':spec['services']['mongo']['network_mode']='container:production'
    if mutation=='public port':spec['services']['mongo']['ports']=[{'host_ip':'0.0.0.0','published':'27017'}]
    if mutation=='privileged':spec['services']['mongo']['privileged']=True
    with self.assertRaises(RuntimeError):stack_guard.validate_spec(spec,self.root)
 def test_archive_traversal_and_links_refused(self):
  for name,kind in (('../outside',tarfile.REGTYPE),('/absolute',tarfile.REGTYPE),('link',tarfile.SYMTYPE),('hardlink',tarfile.LNKTYPE),('device',tarfile.CHRTYPE)):
   with self.subTest(name=name):
    member=tarfile.TarInfo(name);member.type=kind
    with self.assertRaises(RuntimeError):backup.safe_member(member)
 def test_hmac_rejects_tamper_before_decrypting(self):
  key=self.root/'backup.key';key.write_text('a'*96+'\n')
  archive=self.root/'backup.tar.enc';archive.write_bytes(b'original ciphertext')
  archive.with_suffix(archive.suffix+'.hmac.json').write_text(json.dumps({'format':backup.FORMAT,'value':backup.mac(archive,key)}))
  archive.write_bytes(b'tampered ciphertext')
  with patch.object(backup,'run') as run:
   with self.assertRaisesRegex(RuntimeError,'authentication failed'):backup.extract_verified(archive,key,self.root)
   run.assert_not_called()
 def test_attachment_manifest_compares_content_not_tar_encoding(self):
  archives=[]
  for index in range(2):
   archive=self.root/f'volume{index}.tar.gz'
   with tarfile.open(archive,'w:gz') as tar:
    member=tarfile.TarInfo('./bucket/test.bin');member.size=5;member.mtime=index
    tar.addfile(member,io.BytesIO(b'hello'))
   archives.append(archive)
  self.assertEqual(backup.tar_inventory(archives[0]),backup.tar_inventory(archives[1]))
 @unittest.skipUnless(shutil.which('openssl'),'OpenSSL unavailable on this host; server integration also covers encryption')
 def test_authenticated_roundtrip_checks_nested_files(self):
  stage=self.root/'source';stage.mkdir();destination=self.root/'restored';destination.mkdir()
  key=self.root/'backup.key';key.write_text('a'*96+'\n')
  for name in ('runtime.tar.gz','minio-data.tar.gz'):
   with tarfile.open(stage/name,'w:gz') as tar:
    member=tarfile.TarInfo('test.txt');member.size=5;tar.addfile(member,io.BytesIO(b'hello'))
  files=[{'path':p.name,'bytes':p.stat().st_size,'sha256':backup.sha(p)} for p in stage.iterdir()]
  manifest={'format':backup.FORMAT,'syntheticOnly':True,'databases':list(backup.DATABASES),'files':files}
  (stage/'manifest.json').write_text(json.dumps(manifest))
  payload=self.root/'payload.tar'
  with tarfile.open(payload,'w') as tar:
   for path in stage.iterdir():tar.add(path,arcname=path.name)
  encrypted=self.root/'backup.tar.enc'
  backup.run(['openssl','enc','-aes-256-cbc','-salt','-pbkdf2','-iter','200000','-pass','file:'+str(key),'-in',str(payload),'-out',str(encrypted)])
  encrypted.with_suffix(encrypted.suffix+'.hmac.json').write_text(json.dumps({'format':backup.FORMAT,'value':backup.mac(encrypted,key)}))
  restored=backup.extract_verified(encrypted,key,destination)
  self.assertEqual(restored['files'],files)
  self.assertEqual(backup.sha(destination/'runtime.tar.gz'),backup.sha(stage/'runtime.tar.gz'))

if __name__=='__main__':unittest.main(verbosity=2)
