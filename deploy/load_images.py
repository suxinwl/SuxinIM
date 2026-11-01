"""Load only digest-verified project image archives. Existing services untouched."""
import hashlib,json,pathlib,subprocess,sys
root=pathlib.Path(__file__).resolve().parents[1]
lock=json.loads((root/'deploy/images.lock.json').read_text())
logs=root/'logs/image-loads';logs.mkdir(parents=True,exist_ok=True)
for name,value in lock.items():
 image_id=value['image']
 if subprocess.run(['docker','image','inspect',image_id],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL).returncode==0:continue
 archive=root/'image-archives'/(name+'.oci.tar')
 with archive.open('rb') as stream:sha=hashlib.file_digest(stream,'sha256').hexdigest()
 if sha!=value['ociArchive']['sha256']:raise RuntimeError('Image archive checksum mismatch: '+name)
 with (logs/(name+'.log')).open('w') as output:
  subprocess.run(['docker','load','-i',str(archive)],stdout=output,stderr=subprocess.STDOUT,check=True)
 subprocess.run(['docker','image','inspect',image_id],stdout=subprocess.DEVNULL,check=True)
 print(json.dumps({'imageLoaded':name,'imageId':image_id}),flush=True)
