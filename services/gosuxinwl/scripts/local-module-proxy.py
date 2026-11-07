"""Build an isolated file:// module proxy for validating unpublished Suxin modules.
Run from the repository root; generated artifacts stay under ignored runtime/.
"""
import io, json, re, zipfile
from pathlib import Path
ROOT = Path(__file__).resolve().parents[1]
from release_modules import MODULES
modules = [dict(m) for m in MODULES]
PROXY = ROOT/'runtime/module-proxy'
for mod in modules:
    name = mod['module']
    rel = name.removeprefix('github.com/suxinwl/GoSuxin/')
    src = ROOT/rel
    escaped = re.sub('[A-Z]', lambda m: '!'+m[0].lower(), name)
    target = PROXY/escaped/'@v'
    target.mkdir(parents=True, exist_ok=True)
    (target/'v1.0.0.mod').write_bytes((src/'go.mod').read_bytes())
    (target/'v1.0.0.info').write_text(json.dumps({'Version':'v1.0.0','Time':'2026-09-11T00:00:00Z'}))
    (target/'list').write_text('v1.0.0\n')
    nested = [p.parent for p in src.rglob('go.mod') if p.parent != src]
    with zipfile.ZipFile(target/'v1.0.0.zip','w',zipfile.ZIP_DEFLATED) as z:
        for p in sorted(src.rglob('*')):
            if not p.is_file() or any(n in p.parents for n in nested): continue
            r = p.relative_to(src)
            if any(x in {'.git','.github','node_modules','runtime'} for x in r.parts): continue
            z.write(p,name+'@v1.0.0/'+r.as_posix())
print(PROXY.as_uri())
