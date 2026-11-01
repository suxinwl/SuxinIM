"""Refresh checked-in digests from the isolated build; never reads signing passwords."""
from pathlib import Path
import hashlib
import json

# Keep the O: subst prefix: Maven artifact names can exceed Windows MAX_PATH.
ROOT = Path(__file__).absolute().parents[2]
CLIENT = ROOT / 'clients/flutter'

def sha(path):
    return hashlib.file_digest(path.open('rb'), 'sha256').hexdigest()

compat = CLIENT / 'local_plugin/compat'
pub = Path.home() / 'AppData/Local/Pub/Cache'
packages = []
previous_manifest = compat / 'PATCH-MANIFEST.json'
previous_packages = {
    item['package']: item
    for item in json.loads(previous_manifest.read_text(encoding='utf-8'))['packages']
} if previous_manifest.is_file() else {}
for package in sorted(compat.iterdir()):
    if not package.is_dir() or package.name == 'camera_android-0.10.8+12':
        continue
    original = pub / 'hosted/pub.dev' / package.name
    if not original.is_dir():
        # Some exact upstream packages are downloaded directly into this
        # project, without installing or changing the global Pub cache.
        if package.name in previous_packages:
            packages.append(previous_packages[package.name])
        continue
    digest = pub / 'hosted-hashes/pub.dev' / (package.name + '.sha256')
    modified = []
    for file in sorted(package.rglob('*')):
        if not file.is_file() or any(x in file.parts for x in ['build', '.dart_tool', '.gradle', '.git']):
            continue
        relative = file.relative_to(package)
        upstream = original / relative
        current = sha(file)
        old = sha(upstream) if upstream.is_file() else None
        if current != old:
            modified.append({'path': relative.as_posix(), 'sha256': current, 'upstreamSHA256': old})
    packages.append({'package': package.name,
        'source': f'https://pub.dev/api/archives/{package.name}.tar.gz',
        'pubArchiveSHA256': digest.read_text().strip() if digest.is_file() else None,
        'licenses': [x.relative_to(package).as_posix() for x in package.glob('LICENSE*')],
        'modifiedFiles': modified})
(compat / 'PATCH-MANIFEST.json').write_text(json.dumps({
    'policy': 'Pinned Pub archives; local compatibility patches only, no changes to global Pub cache. AGP8.7.3/Kotlin2.1.0; numeric compile SDK >=35; WebRTC plugin36. Unused original camera_android excluded.',
    'packages': packages}, indent=2) + '\n', encoding='utf-8')

artifacts = []
cache = ROOT / '.cache/gradle/caches/modules-2/files-2.1'
for aar in sorted(cache.rglob('*.aar')):
    group, name, version = aar.relative_to(cache).parts[:3]
    artifacts.append({'coordinate': f'{group}:{name}:{version}', 'filename': aar.name,
        'sha256': sha(aar), 'size': aar.stat().st_size})
key = [a for a in artifacts if a['coordinate'] in [
    'io.openim:core-sdk:3.8.3-patch15', 'io.github.webrtc-sdk:android:144.7559.09']]
(ROOT / 'tests/flutter/resolved-aar-hashes.json').write_text(json.dumps({
    'note': 'Artifacts resolved in this isolated Gradle cache. App runtime dependency graph is recorded separately; cache may include lint and other build configurations.',
    'keyArtifacts': key, 'resolvedCacheArtifacts': artifacts}, indent=2) + '\n', encoding='utf-8')
provenance_file = ROOT / 'tests/flutter/build-provenance.json'
provenance = json.loads(provenance_file.read_text(encoding='utf-8-sig'))
provenance['pubspecLockSHA256'] = sha(CLIENT / 'pubspec.lock')
provenance['android']['keyArtifacts'] = key
provenance['android']['compileSDK'] = 35
provenance['android']['webrtcPluginCompileSDK'] = 36
provenance['android']['aarManifest'] = 'tests/flutter/resolved-aar-hashes.json'
provenance_file.write_text(json.dumps(provenance, indent=2) + '\n', encoding='utf-8')
print(json.dumps({'compatPackages': len(packages), 'resolvedAARs': len(artifacts), 'keyArtifacts': key}))
