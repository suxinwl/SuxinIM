"""Preserve official manifests byte-for-byte for Docker's containerd image store."""
import concurrent.futures,hashlib,io,json,pathlib,tarfile,urllib.request
ROOT=pathlib.Path(__file__).resolve().parents[1];LOCK=ROOT/'deploy/images.lock.json';images=json.loads(LOCK.read_text());CACHE=ROOT/'.cache/image-blobs'
def export(item):
 name,value=item
 if name in ['kafka','runtime']:return name,value
 repository,sha=value['image'].split('@');repo=repository.removeprefix('docker.io/')
 with urllib.request.urlopen(f'https://auth.docker.io/token?service=registry.docker.io&scope=repository:{repo}:pull',timeout=45) as response:token=json.load(response)['token']
 headers={'Authorization':'Bearer '+token,'Accept':'application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json'}
 with urllib.request.urlopen(urllib.request.Request(f'https://registry-1.docker.io/v2/{repo}/manifests/{sha}',headers=headers),timeout=60) as response:raw=response.read()
 if 'sha256:'+hashlib.sha256(raw).hexdigest()!=sha:raise RuntimeError('Manifest mismatch')
 manifest=json.loads(raw);tag='docker.io/'+repo+':openim38-pinned'
 index={'schemaVersion':2,'manifests':[{'mediaType':manifest['mediaType'],'digest':sha,'size':len(raw),'platform':{'os':'linux','architecture':'amd64'},'annotations':{'io.containerd.image.name':tag,'org.opencontainers.image.ref.name':tag}}]}
 path=ROOT/'.private/image-archives'/(name+'.oci.tar')
 if not path.exists():
  with tarfile.open(path.with_suffix('.partial'),'w') as tar:
   def add_bytes(name,content):
    info=tarfile.TarInfo(name);info.size=len(content);tar.addfile(info,io.BytesIO(content))
   add_bytes('oci-layout',b'{"imageLayoutVersion":"1.0.0"}')
   add_bytes('index.json',json.dumps(index).encode())
   add_bytes('blobs/sha256/'+sha.split(':')[1],raw)
   for descriptor in [manifest['config'],*manifest['layers']]:
    blob=CACHE/descriptor['digest'].split(':')[1]
    with blob.open('rb') as stream:
     if 'sha256:'+hashlib.file_digest(stream,'sha256').hexdigest()!=descriptor['digest']:raise RuntimeError('Blob mismatch')
    tar.add(blob,arcname='blobs/sha256/'+blob.name)
  path.with_suffix('.partial').replace(path)
 with path.open('rb') as stream:digest=hashlib.file_digest(stream,'sha256').hexdigest()
 value['ociArchive']={'path':str(path.relative_to(ROOT)),'sha256':digest,'tag':tag}
 print(json.dumps({'ociExported':name,'bytes':path.stat().st_size}),flush=True);return name,value
with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:values=dict(pool.map(export,images.items()))
LOCK.write_text(json.dumps(values,indent=2)+'\n')
