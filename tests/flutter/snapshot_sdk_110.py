"""Re-read all fixed SDK194 files without changing its repository or outputs."""
from pathlib import Path
import hashlib
import json
import subprocess

root = Path(__file__).absolute().parents[2]
prior = root / 'tests/flutter/sdk-source-snapshot-109.json'
destination = root / 'tests/flutter/sdk-source-snapshot-110.json'
if destination.exists():
    raise RuntimeError('Preserve SDK freeze manifest')
data = json.loads(prior.read_text(encoding='utf-8'))
actual = {}
for name, expected in data['files'].items():
    path = root / name
    digest = hashlib.sha256(path.read_bytes()).hexdigest()
    if digest != expected:
        raise RuntimeError('Fixed SDK byte changed: ' + name)
    actual[name] = digest
status = subprocess.check_output(['git', 'status', '--porcelain', '--untracked-files=no'],
                                 cwd=root / 'sdks/flutter').decode('utf-8').strip()
if status:
    raise RuntimeError('Fixed SDK has tracked edits')
tracked = subprocess.check_output(['git', 'ls-files'], cwd=root / 'sdks/flutter').decode('utf-8').splitlines()
if {'sdks/flutter/' + name for name in tracked} != set(actual):
    raise RuntimeError('Fixed SDK tracked-file scope changed')
digest = hashlib.sha256(json.dumps(actual, sort_keys=True, separators=(',', ':')).encode()).hexdigest()
if digest != data['sourceSHA256'] or len(actual) != 194:
    raise RuntimeError('SDK aggregate/scope differs')
destination.write_bytes(prior.read_bytes())
print(json.dumps({'sdkFiles': len(actual), 'sourceSHA256': digest, 'trackedChanges': 0}))
