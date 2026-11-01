"""Fingerprint build source files, excluding caches, generators/examples and secrets."""
from pathlib import Path
import hashlib,json,os,sys,re
ROOT=Path(__file__).absolute().parents[2]
CLIENT=ROOT/'clients/flutter'
excluded_cache={'.git','.dart_tool','.gradle','.kotlin','.cxx','build'}
package_only={'example','example_ohos','test','integration_test','pigeons'}
def prune(directory,name):
    parent=Path(directory)
    # These are runtime namespaces too: com/example, lib/test, Classes/example
    # cannot be classified as sample/test applications by their basename.
    parts=parent.relative_to(CLIENT).parts
    runtime=('lib' in parts or 'Classes' in parts or 'Resources' in parts or
             any(parts[i]=='src' and parts[i+1] in ['main','release','development','production'] for i in range(len(parts)-1)))
    if name in excluded_cache:return name!='build' or not runtime
    if name not in package_only or runtime:return False
    if (parent/'pubspec.yaml').is_file():return True
    if name in {'example','example_ohos'} and (parent/name/'pubspec.yaml').is_file():return True
    if name=='test' and parent.name in {'windows','linux','macos'} and (parent/'CMakeLists.txt').is_file():return True
    return name in {'test','integration_test'} and parent.name=='src' and any((parent.parent/x).is_file() for x in ['build.gradle','build.gradle.kts'])
paths=[]
for relative in ['lib','assets','openim_common','openim_live','openim_meeting','openim_working_circle','local_plugin','android','ios','scripts']:
    base=CLIENT/relative
    if not base.is_dir():continue
    for directory,dirs,files in os.walk(base):
        dirs[:]=[d for d in dirs if not prune(directory,d)]
        for name in files:
            p=Path(directory)/name
            if name in ['key.properties','local.properties','PATCH-MANIFEST.json'] or p.suffix in ['.jks','.keystore','.log']:continue
            if p.relative_to(CLIENT).as_posix() in ['android/app/proguard/mapping.txt','android/app/proguard/seeds.txt','android/app/proguard/unused.txt']:continue
            paths.append(p)
paths += [CLIENT/x for x in ['pubspec.yaml','pubspec.lock','tool/validation_main.dart','tool/native_probe.dart']]
mapping={p.relative_to(ROOT).as_posix():hashlib.file_digest(p.open('rb'),'sha256').hexdigest() for p in sorted(set(paths)) if p.is_file()}
digest=hashlib.sha256(json.dumps(mapping,sort_keys=True,separators=(',',':')).encode()).hexdigest()
version=re.search(r'^version:\s*([^+\s]+)\+(\d+)', (CLIENT/'pubspec.yaml').read_text(), re.MULTILINE)
if not version:raise RuntimeError('Expected an explicit application version and build')
result={'applicationVersion':version[1],'applicationBuild':int(version[2]),'sourceScope':'Application/vendored runtime dependency sources, public platform configuration/assets and the separately named private validation entrypoint; official OpenIM SDK/AAR separately pinned by build-provenance.json; no signing secrets or caches','sourceSHA256':digest,'fileCount':len(mapping),'files':mapping}
destination=ROOT/f'tests/flutter/runtime-source-snapshot-{version[2]}.json'
if '--verify' in sys.argv:
    previous=json.loads(destination.read_text())
    changed=[name for name in mapping.keys()|previous['files'].keys() if mapping.get(name)!=previous['files'].get(name)]
    if changed:raise RuntimeError('Runtime build source changed after freeze: '+','.join(changed[:8]))
else:destination.write_text(json.dumps(result,indent=2)+'\n')
print(json.dumps({'sourceSHA256':digest,'fileCount':len(mapping)}))
