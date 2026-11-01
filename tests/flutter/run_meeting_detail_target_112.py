"""Run the actual112 detail fixture with fixed O: inputs and preserved evidence."""
from pathlib import Path
from datetime import datetime, timezone
import hashlib
import json
import os
import re
import subprocess
import sys
import time

root = Path('O:/')
client = root / 'clients/flutter'
folder = root / 'tests/flutter'
label = sys.argv[1]
if not re.fullmatch(r'meeting-detail-112-[a-z0-9-]+', label):
    raise RuntimeError('A fresh112 detail evidence label is required')
paths = json.loads((folder / '112-working-scope.json').read_text())['productFiles']
paths += ['clients/flutter/test/meeting_detail_112_test.dart',
          'clients/flutter/pubspec.yaml', 'clients/flutter/pubspec.lock',
          'clients/flutter/.dart_tool/package_config.json',
          'tools/flutter-3.32.8/engine/src/flutter/txt/third_party/fonts/Roboto-Regular.ttf']
destinations = [folder / (label + suffix) for suffix in [
    '-before.json', '-after.json', '.log',
    '-package-config-before.json', '-package-config-after.json']]
if any(path.exists() for path in destinations):
    raise RuntimeError('Preserve prior target evidence')


def snapshot():
    return {name: hashlib.sha256((root / name).read_bytes()).hexdigest() for name in paths}


before = snapshot()
destinations[3].write_bytes((client / '.dart_tool/package_config.json').read_bytes())
destinations[0].write_text(json.dumps({
    'at': datetime.now(timezone.utc).isoformat(), 'files': before,
    'cwd': 'O:/clients/flutter', 'flutter': 'O:/tools/flutter-3.32.8/bin/flutter.bat',
    'PUB_CACHE': 'C:/Users/Administrator/AppData/Local/Pub/Cache',
    'boundary': '112working code; no APK or device acceptance inferred',
}, indent=2) + '\n', encoding='utf-8')
env = os.environ.copy()
env['FLUTTER_PREBUILT_ENGINE_VERSION'] = 'ef0cd000916d64fa0c5d09cc809fa7ad244a5767'
env['TEMP'] = env['TMP'] = 'D:/OpenIM38-BuildScratch/111-temp-20261101'
env['PUB_CACHE'] = 'C:/Users/Administrator/AppData/Local/Pub/Cache'
env['CI'] = 'true'
command = [str(root / 'tools/flutter-3.32.8/bin/flutter.bat'), 'test', '--no-pub',
           '--reporter', 'expanded', 'test/meeting_detail_112_test.dart']
started = time.monotonic()
with destinations[2].open('xb') as stream:
    result = subprocess.run(command, cwd=client, env=env, stdout=stream,
                            stderr=subprocess.STDOUT)
after = snapshot()
changed = [path for path in paths if before[path] != after[path]]
destinations[4].write_bytes((client / '.dart_tool/package_config.json').read_bytes())
record = {'at': datetime.now(timezone.utc).isoformat(), 'exitCode': result.returncode,
          'elapsedSeconds': time.monotonic() - started, 'files': after,
          'changed': changed, 'command': command,
          'logSHA256': hashlib.sha256(destinations[2].read_bytes()).hexdigest()}
destinations[1].write_text(json.dumps(record, indent=2) + '\n', encoding='utf-8')
print(json.dumps({'exitCode': result.returncode, 'elapsedSeconds': record['elapsedSeconds'],
                  'changed': changed, 'log': destinations[2].relative_to(root).as_posix(),
                  'tail': destinations[2].read_text(encoding='utf-8', errors='replace')[-3200:]}))
sys.exit(result.returncode or bool(changed))
