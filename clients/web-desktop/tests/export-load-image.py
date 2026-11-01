import concurrent.futures,hashlib,io,json,pathlib,tarfile,time,urllib.request
root=pathlib.Path.cwd().parents[1];dest=root/'.cache/load-linux-image';dest.mkdir(parents=True,exist_ok=True)
base='https://mcr.microsoft.com/v2/playwright';accept={'Accept':'application/vnd.oci.image.manifest.v1+json,application/vnd.docker.distribution.manifest.v2+json'}
sha='sha256:d65108d1a8095d8b0c4a5e32bf1807a40a5ce90ff0c78473e1116b1c243235e8'
raw=urllib.request.urlopen(urllib.request.Request(base+'/manifests/'+sha,headers=accept),timeout=45).read();assert 'sha256:'+hashlib.sha256(raw).hexdigest()==sha
manifest=json.loads(raw);(dest/'manifest.json').write_bytes(raw)
def download(desc):
 p=dest/desc['digest'].split(':')[1]
 if p.exists() and p.stat().st_size==desc['size']:
  with p.open('rb') as f:
   if 'sha256:'+hashlib.file_digest(f,'sha256').hexdigest()==desc['digest']:return p
 for retry in range(6):
  partial=p.with_suffix('.part');offset=partial.stat().st_size if partial.exists() else 0
  try:
   req=urllib.request.Request(base+'/blobs/'+desc['digest'],headers={'Range':f'bytes={offset}-'} if offset else {})
   with urllib.request.urlopen(req,timeout=90) as r:
    mode='ab' if offset and r.status==206 else 'wb'
    with partial.open(mode) as f:
     while chunk:=r.read(2**20):f.write(chunk)
   with partial.open('rb') as f:actual=hashlib.file_digest(f,'sha256').hexdigest()
   if 'sha256:'+actual!=desc['digest']:raise RuntimeError('sha mismatch')
   partial.replace(p);print(json.dumps({'downloaded':desc['digest'],'bytes':p.stat().st_size}),flush=True);return p
  except Exception as e:print('retry',retry,desc['digest'],str(e),flush=True);time.sleep(2+retry)
 raise RuntimeError('download failed '+desc['digest'])
with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:list(pool.map(download,[manifest['config'],*manifest['layers']]))
tag='mcr.microsoft.com/playwright:v1.64.1-noble';index={'schemaVersion':2,'manifests':[{'mediaType':manifest['mediaType'],'digest':sha,'size':len(raw),'platform':{'os':'linux','architecture':'amd64'},'annotations':{'io.containerd.image.name':tag,'org.opencontainers.image.ref.name':tag}}]}
archive=dest/'playwright-v1.64.1-noble.oci.tar'
with tarfile.open(archive.with_suffix('.partial'),'w') as tar:
 def add(name,data):
  info=tarfile.TarInfo(name);info.size=len(data);tar.addfile(info,io.BytesIO(data))
 add('oci-layout',b'{"imageLayoutVersion":"1.0.0"}');add('index.json',json.dumps(index).encode());add('blobs/sha256/'+sha.split(':')[1],raw)
 for desc in [manifest['config'],*manifest['layers']]:tar.add(dest/desc['digest'].split(':')[1],arcname='blobs/sha256/'+desc['digest'].split(':')[1])
archive.with_suffix('.partial').replace(archive)
with archive.open('rb') as f:digest=hashlib.file_digest(f,'sha256').hexdigest()
(dest/'archive-sha256.json').write_text(json.dumps({'path':str(archive),'sha256':digest,'manifest':sha,'tag':tag},indent=2))
print(json.dumps({'archive':str(archive),'bytes':archive.stat().st_size,'sha256':digest}),flush=True)
