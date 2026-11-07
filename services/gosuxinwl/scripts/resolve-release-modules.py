"""Resolve each release module without go.work against an isolated local proxy."""
import os,subprocess,json
from pathlib import Path
root=Path(__file__).resolve().parents[1]
from release_modules import MODULES
mods = [dict(m) for m in MODULES]
# Core first, consumers last; CLI consumes the SQL drivers.
mods.sort(key=lambda m: 0 if m['module'].endswith('/framework') else 2 if m['module'].endswith('/cmd/suxin') else 1)
env={**os.environ,'GOWORK':'off','GOPROXY':(root/'runtime/module-proxy').as_uri()+',https://proxy.golang.org,direct','GONOSUMDB':'github.com/suxinwl/GoSuxin*','GOMODCACHE':str(root/'runtime/final-proxy-cache')}
results=[]
for m in mods+[{'module':'github.com/suxinwl/GoSuxin'}]:
 name=m['module'];rel=name.removeprefix('github.com/suxinwl/GoSuxin').lstrip('/');src=root/rel
 sums=src/'go.sum'
 if sums.exists():sums.write_text('\n'.join(l for l in sums.read_text().splitlines() if not l.startswith('github.com/suxinwl/GoSuxin'))+'\n')
 subprocess.run(['python','scripts/local-module-proxy.py'],cwd=root,check=True,stdout=subprocess.DEVNULL)
 with (root/'runtime'/('tidy-'+(rel or 'app').replace('/','-')+'.log')).open('w') as log:
  p=subprocess.run(['go','mod','tidy'],cwd=src,env=env,stdout=log,stderr=log)
 results.append({'module':name,'tidy':p.returncode});print(name,p.returncode,flush=True)
 if p.returncode:break
(root/'runtime/module-resolution.json').write_text(json.dumps(results,indent=2))
subprocess.run(['python','scripts/local-module-proxy.py'],cwd=root,check=True,stdout=subprocess.DEVNULL)
