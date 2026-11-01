"""Audit actual resolved project-local packages against explicit source manifests."""
from pathlib import Path
import json,urllib.parse,hashlib,yaml
ROOT=Path(__file__).absolute().parents[2]; CLIENT=ROOT/'clients/flutter'
def sha(p):return hashlib.file_digest(p.open('rb'),'sha256').hexdigest()
config=CLIENT/'.dart_tool/package_config.json'
resolved={}
for item in json.loads(config.read_text())['packages']:
 uri=item['rootUri']
 path=Path(urllib.parse.unquote(urllib.parse.urlparse(uri).path.lstrip('/'))) if uri.startswith('file:') else config.parent/urllib.parse.unquote(uri)
 try: relative=path.resolve().relative_to(ROOT.resolve()).as_posix()
 except ValueError:continue
 resolved[item['name']]={'root':path.resolve(),'relative':relative}
snapshot=json.loads((ROOT/'tests/flutter/runtime-source-snapshot-108.json').read_text())
sdk=json.loads((ROOT/'tests/flutter/sdk-source-snapshot-108.json').read_text())
records=[]; edges=[]
for name,item in sorted(resolved.items()):
 rel=item['relative']
 if rel.startswith('tools/flutter-3.32.8/'):
  records.append({'package':name,'root':rel,'coverage':'separate pinned Flutter3.32.8 runtime/engine provenance'});continue
 if rel=='sdks/flutter':
  records.append({'package':name,'root':rel,'coverage':'tests/flutter/sdk-source-snapshot-108.json','files':sdk['fileCount'],'sourceSHA256':sdk['sourceSHA256']});continue
 assert rel=='clients/flutter' or rel.startswith('clients/flutter/'),(name,rel)
 files=[x for x in snapshot['files'] if x.startswith(rel+'/')]
 assert rel+'/pubspec.yaml' in files,(name,rel)
 assert any(x.startswith(rel+'/'+part+'/') for x in files for part in ['lib','android','ios','src']),(name,rel)
 records.append({'package':name,'root':rel,'coverage':'tests/flutter/runtime-source-snapshot-108.json','coveredFiles':len(files),'pubspecSHA256':sha(item['root']/'pubspec.yaml')})
 data=yaml.safe_load((item['root']/'pubspec.yaml').read_text())
 for section in ['dependencies','dev_dependencies','dependency_overrides']:
  for dependency,spec in (data.get(section) or {}).items():
   if not isinstance(spec,dict) or 'path' not in spec:continue
   declared=(item['root']/spec['path']).resolve()
   target=resolved.get(dependency)
   edges.append({'owner':name,'section':section,'dependency':dependency,'declaredPath':str(spec['path']),'resolvedPackageMatches':target is not None and declared==target['root'],'effectiveOverride':target is not None and declared!=target['root']})
   assert target is not None,(name,dependency,'unresolved')
result={'scope':'Resolved package_config roots plus path declarations in every resolved project-local package; dev-only local Flutter packages separately pinned. Each application local root has pubspec and actual Dart/platform-source bytes in runtime source manifest; OpenIM SDK has independent194-file source manifest.','packageConfigSHA256':sha(config),'runtimeManifestSHA256':sha(ROOT/'tests/flutter/runtime-source-snapshot-108.json'),'sdkManifestSHA256':sha(ROOT/'tests/flutter/sdk-source-snapshot-108.json'),'packages':records,'declaredPathEdges':edges,'allCovered':True,'historicalScopeCorrection':'Prior immutable application snapshots through107 omitted openim_working_circle.108 adds existing runtime input coverage; this is not newly introduced product source.','snapshotToolSHA256':sha(ROOT/'tests/flutter/snapshot_runtime_sources.py')}
(ROOT/'tests/flutter/path-dependency-coverage-108.json').write_text(json.dumps(result,indent=2)+'\n')
print(json.dumps({'projectResolvedPackages':len(records),'pathEdges':len(edges),'allCovered':True}))
