"""Load only verified archives or immutable image references for PUTY production."""
import concurrent.futures
import hashlib
import json
import pathlib
import subprocess

BASE = pathlib.Path('/srv/puty-suite')


def load(item):
    name, definition = item
    image = definition['image']
    log_file = BASE / 'staging' / (name + '-image.log')
    if subprocess.run(['docker', 'image', 'inspect', image], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL).returncode == 0:
        return {'image': name, 'ready': True, 'existing': True}
    archive = BASE / 'staging' / (name + '.oci.tar')
    with log_file.open('w') as log:
        if archive.exists():
            with archive.open('rb') as stream:
                digest = hashlib.file_digest(stream, 'sha256').hexdigest()
            if digest != definition['ociArchive']['sha256']:
                raise RuntimeError('Archive checksum mismatch: ' + name)
            subprocess.run(['docker', 'load', '-i', str(archive)], stdout=log, stderr=subprocess.STDOUT, check=True, timeout=1200)
        else:
            subprocess.run(['docker', 'pull', image], stdout=log, stderr=subprocess.STDOUT, check=True, timeout=600)
    subprocess.run(['docker', 'image', 'inspect', image], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=True)
    result = {'image': name, 'ready': True}
    print(json.dumps(result), flush=True)
    return result


if __name__ == '__main__':
    if not BASE.is_dir() or str(BASE.resolve()) != '/srv/puty-suite':
        raise RuntimeError('Unexpected production root')
    definitions = json.loads((BASE / 'staging/images.lock.json').read_text())
    definitions.pop('nginx', None)
    definitions.pop('livekit', None)
    definitions['mysql'] = {'image': 'mysql:8.0.46@sha256:62fb722c78b24245ddff1796a0fcee4a49cc5b87e0aaaf20c92d1da9e0a2497b'}
    with concurrent.futures.ThreadPoolExecutor(max_workers=3) as pool:
        results = list(pool.map(load, definitions.items()))
    (BASE / 'staging/dependency-results.json').write_text(json.dumps(results, indent=2) + '\n')
