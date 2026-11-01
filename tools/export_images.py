"""Verified Docker save archives for hosts whose configured registry proxy fails.

Registry manifests, configs, compressed layers AND uncompressed diff IDs are
verified. Unique project tags never overwrite production image tags.
"""
import concurrent.futures,gzip,hashlib,io,json,pathlib,shutil,tarfile,urllib.request
ROOT=pathlib.Path(__file__).resolve().parents[1]
CACHE=ROOT/'.cache/image-blobs';CACHE.mkdir(parents=True,exist_ok=True)
LOCK=ROOT/'deploy/images.lock.json'
IMAGES=json.loads(LOCK.read_text())
ACCEPT='application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json'
def digest(path):
 with path.open('rb') as stream:return 'sha256:'+hashlib.file_digest(stream,'sha256').hexdigest()
def get(url,headers):return urllib.request.urlopen(urllib.request.Request(url,headers=headers),timeout=120)
def export(item):
 name,value=item;image=value['image'];repository,manifest_digest=image.split('@');repo=repository.removeprefix('docker.io/')
 with get(f'https://auth.docker.io/token?service=registry.docker.io&scope=repository:{repo}:pull',{}) as response:token=json.load(response)['token']
 headers={'Authorization':'Bearer '+token,'Accept':ACCEPT}
 with get(f'https://registry-1.docker.io/v2/{repo}/manifests/{manifest_digest}',headers) as response:raw=response.read()
 if 'sha256:'+hashlib.sha256(raw).hexdigest()!=manifest_digest:raise RuntimeError('Manifest digest mismatch')
 manifest=json.loads(raw)
 def blob(descriptor):
  sha=descriptor['digest'];path=CACHE/sha.split(':')[1]
  if not path.exists():
   temporary=path.with_suffix('.partial')
   with get(f'https://registry-1.docker.io/v2/{repo}/blobs/{sha}',headers) as source,temporary.open('wb') as output:shutil.copyfileobj(source,output)
   if digest(temporary)!=sha:raise RuntimeError('Compressed image blob mismatch')
   temporary.replace(path)
  elif digest(path)!=sha:raise RuntimeError('Cached blob mismatch')
  return path
 config_path=blob(manifest['config']);config=json.loads(config_path.read_text());value['localImageId']=manifest['config']['digest']
 if name in ['kafka','runtime']:
  return name,value
 diff_ids=config['rootfs']['diff_ids'];layers=[]
 for descriptor,diff_id in zip(manifest['layers'],diff_ids,strict=True):
  compressed=blob(descriptor);unpacked=CACHE/(diff_id.split(':')[1]+'.tar')
  if not unpacked.exists():
   with gzip.open(compressed,'rb') as source,unpacked.with_suffix('.partial').open('wb') as output:shutil.copyfileobj(source,output)
   if digest(unpacked.with_suffix('.partial'))!=diff_id:raise RuntimeError('Image diff ID mismatch')
   unpacked.with_suffix('.partial').replace(unpacked)
  elif digest(unpacked)!=diff_id:raise RuntimeError('Cached diff ID mismatch')
  layers.append(unpacked)
 output=ROOT/'.private/image-archives'/(name+'.tar.gz');output.parent.mkdir(parents=True,exist_ok=True)
 unique_tag=repo+':openim38-'+value['localImageId'].split(':')[1][:12]
 if not output.exists():
  with tarfile.open(output.with_suffix('.partial'),'w:gz',compresslevel=1) as tar:
   config_name=value['localImageId'].split(':')[1]+'.json';tar.add(config_path,arcname=config_name)
   layer_names=[]
   for layer in layers:
    layer_name=layer.stem+'/layer.tar';tar.add(layer,arcname=layer_name);layer_names.append(layer_name)
   content=json.dumps([{'Config':config_name,'RepoTags':[unique_tag],'Layers':layer_names}]).encode()
   info=tarfile.TarInfo('manifest.json');info.size=len(content);tar.addfile(info,io.BytesIO(content))
  output.with_suffix('.partial').replace(output)
 value['offlineArchive']={'path':str(output.relative_to(ROOT)),'sha256':digest(output).split(':')[1],'tag':unique_tag}
 print(json.dumps({'exported':name,'imageId':value['localImageId'],'bytes':output.stat().st_size}),flush=True)
 return name,value
if __name__=='__main__':
 with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:result=dict(pool.map(export,IMAGES.items()))
 LOCK.write_text(json.dumps(result,indent=2)+'\n')
