"""Fetch pinned production image blobs on Windows and export verified OCI archives.
Never updates the existing deployment lock or invokes a remote server.
"""
from __future__ import annotations
import argparse,concurrent.futures,hashlib,io,json,os,pathlib,re,tarfile,time,urllib.request
ROOT=pathlib.Path(__file__).resolve().parents[1]
SOURCE=pathlib.Path('E:/PUTY_AI/OpenIM3.4-3.8/OpenIM3.8.3patch.18_1.8.4patch.5')
CACHE=SOURCE/'.cache/image-blobs'
OUT=ROOT/'dist/production-images'
ACCEPT='application/vnd.oci.image.index.v1+json, application/vnd.docker.distribution.manifest.list.v2+json, application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json'
def digest(path):
    with path.open('rb') as stream:return 'sha256:'+hashlib.file_digest(stream,'sha256').hexdigest()
def read_url(url,headers=None):
    with urllib.request.urlopen(urllib.request.Request(url,headers=headers or {}),timeout=45) as response:return response.read()
def manifest(repo,reference,headers):
    raw=read_url(f'https://registry-1.docker.io/v2/{repo}/manifests/{reference}',headers)
    actual='sha256:'+hashlib.sha256(raw).hexdigest()
    if reference.startswith('sha256:') and actual!=reference:raise ValueError('manifest digest mismatch')
    return json.loads(raw),raw,actual
def blob(repo,descriptor,headers):
    sha=descriptor['digest']
    if not re.fullmatch(r'sha256:[0-9a-f]{64}',sha) or not isinstance(descriptor['size'],int) or descriptor['size']<0:raise ValueError('invalid blob descriptor')
    path=CACHE/sha.split(':')[1]
    if path.exists() and path.stat().st_size==descriptor['size'] and digest(path)==sha:return path
    partial=path.with_suffix('.production-partial')
    for attempt in range(5):
        try:
            offset=partial.stat().st_size if partial.exists() else 0
            if offset>=descriptor['size']:
                if offset==descriptor['size'] and digest(partial)==sha:partial.replace(path);return path
                partial.write_bytes(b'');offset=0
            request_headers=dict(headers)
            if offset:request_headers['Range']=f'bytes={offset}-'
            with urllib.request.urlopen(urllib.request.Request(f'https://registry-1.docker.io/v2/{repo}/blobs/{sha}',headers=request_headers),timeout=45) as response:
                append=offset>0 and response.status==206
                with partial.open('ab' if append else 'wb') as stream:
                    while chunk:=response.read(1024*1024):stream.write(chunk)
            if partial.stat().st_size!=descriptor['size'] or digest(partial)!=sha:raise ValueError('blob size/digest mismatch')
            partial.replace(path)
            print(json.dumps({'blobVerified':sha,'bytes':path.stat().st_size}),flush=True)
            return path
        except Exception as error:
            print(json.dumps({'blobRetry':sha,'attempt':attempt+1,'errorType':type(error).__name__}),flush=True)
            if attempt==4:raise
            time.sleep(min(2**attempt,8))
def export(name,image):
    repo,reference=image.removeprefix('docker.io/').split('@')
    token=json.loads(read_url(f'https://auth.docker.io/token?service=registry.docker.io&scope=repository:{repo}:pull'))['token']
    headers={'Authorization':'Bearer '+token,'Accept':ACCEPT,'User-Agent':'GoSuxin-production-image-fetch/1'}
    doc,raw,selected=manifest(repo,reference,headers)
    original=(raw,selected)
    if 'manifests' in doc:
        candidates=[item for item in doc['manifests'] if item.get('platform',{}).get('os')=='linux' and item.get('platform',{}).get('architecture')=='amd64']
        if len(candidates)!=1:raise ValueError('linux/amd64 manifest is not unique')
        doc,raw,selected=manifest(repo,candidates[0]['digest'],headers)
    print(json.dumps({'manifestVerified':name,'requested':reference,'selected':selected,'blobs':len(doc['layers'])+1}),flush=True)
    descriptors=[doc['config'],*doc['layers']]
    with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:list(pool.map(lambda item:blob(repo,item,headers),descriptors))
    config=json.loads((CACHE/doc['config']['digest'].split(':')[1]).read_bytes())
    if config.get('os')!='linux' or config.get('architecture')!='amd64':raise ValueError('downloaded image is not linux/amd64')
    tag=f'docker.io/{repo}:openim38-pinned'
    index={'schemaVersion':2,'manifests':[{'mediaType':doc['mediaType'],'digest':selected,'size':len(raw),'platform':{'os':'linux','architecture':'amd64'},'annotations':{'io.containerd.image.name':tag,'org.opencontainers.image.ref.name':tag}}]}
    path=OUT/(name+'.oci.tar');partial=path.with_suffix('.partial')
    with tarfile.open(partial,'w') as archive:
        def add_bytes(name,value):
            info=tarfile.TarInfo(name);info.size=len(value);info.mode=0o644;archive.addfile(info,io.BytesIO(value))
        add_bytes('oci-layout',b'{"imageLayoutVersion":"1.0.0"}')
        add_bytes('index.json',json.dumps(index).encode())
        add_bytes('blobs/sha256/'+selected.split(':')[1],raw)
        if original[1]!=selected:add_bytes('blobs/sha256/'+original[1].split(':')[1],original[0])
        for descriptor in descriptors:
            cached=CACHE/descriptor['digest'].split(':')[1]
            if digest(cached)!=descriptor['digest']:raise ValueError('cached blob changed during OCI export')
            archive.add(cached,arcname='blobs/sha256/'+cached.name)
    partial.replace(path)
    result={'name':name,'requestedImage':image,'manifestDigest':selected,'tag':tag,'archive':str(path),'sha256':digest(path).removeprefix('sha256:'),'bytes':path.stat().st_size}
    (OUT/(name+'.json')).write_text(json.dumps(result,indent=2),encoding='utf-8')
    print(json.dumps({'imageReady':result}),flush=True)
    return result
if __name__=='__main__':
    parser=argparse.ArgumentParser();parser.add_argument('--images',nargs='+',choices=['mysql','kafka','runtime'],default=['mysql','kafka','runtime']);args=parser.parse_args()
    locked=json.loads((SOURCE/'deploy/images.lock.json').read_text())
    images={'mysql':'docker.io/library/mysql@sha256:62fb722c78b24245ddff1796a0fcee4a49cc5b87e0aaaf20c92d1da9e0a2497b','kafka':locked['kafka']['image'],'runtime':locked['runtime']['image']}
    CACHE.mkdir(parents=True,exist_ok=True);OUT.mkdir(parents=True,exist_ok=True)
    with concurrent.futures.ThreadPoolExecutor(max_workers=3) as pool:
        failures=0
        tasks={pool.submit(export,name,images[name]):name for name in args.images}
        for task in concurrent.futures.as_completed(tasks):
            try:task.result()
            except Exception as error:failures+=1;print(json.dumps({'imageFailed':tasks[task],'errorType':type(error).__name__,'message':str(error)}),flush=True)
    raise SystemExit(1 if failures else 0)
