"""Warm only the immutable images recorded for this isolated project."""
import concurrent.futures,json,pathlib,subprocess,sys
root=pathlib.Path(__file__).resolve().parents[1]
images=json.loads((root/'deploy/images.lock.json').read_text())
logs=root/'logs/pulls';logs.mkdir(parents=True,exist_ok=True)
def pull(item):
 name,value=item
 with (logs/(name+'.log')).open('w') as output:
  code=subprocess.run(['docker','pull',value['image']],stdout=output,stderr=subprocess.STDOUT).returncode
 print(json.dumps({'image':name,'exit':code}),flush=True)
 return code
with concurrent.futures.ThreadPoolExecutor(max_workers=3) as pool:codes=list(pool.map(pull,images.items()))
sys.exit(any(codes))
