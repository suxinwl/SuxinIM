"""Explicit nonmedia regressions; never invokes the entire test directory."""
from pathlib import Path
from datetime import datetime, timezone
import hashlib
import json
import os
import re
import subprocess
import time

ROOT = Path('O:/')
OUT = ROOT / 'tests/flutter'
CLIENT = ROOT / 'clients/flutter'
FILES = [
    'test/release_policy_test.dart',
    'test/persistent_update_gate_test.dart',
    'test/login_navigation_111_test.dart',
    'test/session_epoch_test.dart',
    'test/group_read_adapter_109_test.dart',
    'test/group_read_controller_109_test.dart',
    'test/bugly_error_reporting_110_test.dart',
]
NAMES = [
    'legacy input updates call the pinned conversation input API',
    'captured pair verification never reads a replacement login token',
    'input lookup from an older login never publishes the new session state',
    'quote factory rejects an unsent source before crossing the native boundary',
]
SELECTION = {
    'applicationVersion': '3.8.13+113',
    'reason': 'User explicitly skips video, voice, meeting and call verification.',
    'wholeFiles': FILES,
    'partialFile': 'test/enterprise_transport_test.dart',
    'exactSelectedNames': NAMES,
    'excluded': [
        'Remaining five enterprise_transport tests exercise signaling or RTC endpoints.',
        'All call, RTC cleanup, Room, IO/Signal, renderer, audio/ringtone and wakelock tests.',
        'All native Java suites; unchanged111 results are historical reuse only.',
        'Archived unfinished112 meeting detail/client tests are outside active test sources.',
    ],
    'workingCircle': 'No standalone circle test exists; prior scoped108 original-UI evidence remains historical.',
}
dest = OUT / 'nonmedia-selection-113.json'
assert not dest.exists(), 'Preserve prior selection evidence'
dest.write_text(json.dumps(SELECTION, indent=2) + '\n', encoding='utf-8')
paths = list(json.loads((OUT / 'unit-tests-111-inputs-before.json').read_text())['files'])
paths += ['tests/flutter/run_nonmedia_113.py', 'tests/flutter/nonmedia-selection-113.json']
assert all((ROOT / p).is_file() for p in paths)


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def snapshot():
    return {p: digest(ROOT / p) for p in paths}


before = snapshot()
(OUT / 'nonmedia-113-package-config-before.json').write_bytes((CLIENT / '.dart_tool/package_config.json').read_bytes())
(OUT / 'nonmedia-113-inputs-before.json').write_text(json.dumps({
    'at': datetime.now(timezone.utc).isoformat(), 'files': before, 'fileCount': len(before),
    'cwd': 'O:/clients/flutter', 'PUB_CACHE': 'C:/Users/Administrator/AppData/Local/Pub/Cache',
}, indent=2) + '\n', encoding='utf-8')
env = os.environ.copy()
env.update({
    'FLUTTER_PREBUILT_ENGINE_VERSION': 'ef0cd000916d64fa0c5d09cc809fa7ad244a5767',
    'TEMP': 'D:/OpenIM38-BuildScratch/111-temp-20261101',
    'TMP': 'D:/OpenIM38-BuildScratch/111-temp-20261101',
    'PUB_CACHE': 'C:/Users/Administrator/AppData/Local/Pub/Cache',
    'CI': 'true',
})
flutter = str(ROOT / 'tools/flutter-3.32.8/bin/flutter.bat')
commands = [
    [flutter, 'test', '--no-pub', '--reporter', 'expanded', *FILES],
    [flutter, 'test', '--no-pub', '--reporter', 'expanded', 'test/enterprise_transport_test.dart',
     '--name', '^(?:' + '|'.join(re.escape(x) for x in NAMES) + ')$'],
]
results = []
for index, command in enumerate(commands, 1):
    log = OUT / f'nonmedia-113-part{index}.log'
    started = time.monotonic()
    with log.open('xb') as stream:
        result = subprocess.run(command, cwd=CLIENT, env=env, stdout=stream, stderr=subprocess.STDOUT)
    text = log.read_text(encoding='utf-8', errors='replace')
    match = re.search(r'\+(\d+):\s+All tests passed!', text)
    results.append({'command': command, 'exit': result.returncode, 'elapsedSeconds': time.monotonic() - started,
                    'passed': int(match[1]) if match else None,
                    'log': log.relative_to(ROOT).as_posix(), 'sha256': digest(log)})
    print(json.dumps(results[-1]), flush=True)
    if result.returncode:
        break
after = snapshot()
changed = [p for p in paths if before[p] != after[p]]
(OUT / 'nonmedia-113-package-config-after.json').write_bytes((CLIENT / '.dart_tool/package_config.json').read_bytes())
record = {'at': datetime.now(timezone.utc).isoformat(), 'files': after, 'fileCount': len(after),
          'changed': changed, 'results': results,
          'allSelectedTestsPassed': len(results) == 2 and all(x['exit'] == 0 and x['passed'] is not None for x in results),
          'allInputsUnchanged': not changed, 'skippedMediaAndJava': True}
(OUT / 'nonmedia-113-inputs-after.json').write_text(json.dumps(record, indent=2) + '\n', encoding='utf-8')
print(json.dumps({'passed': sum(x['passed'] or 0 for x in results), 'changed': changed,
                  'allSelectedTestsPassed': record['allSelectedTestsPassed']}), flush=True)
raise SystemExit(0 if record['allSelectedTestsPassed'] and not changed else 1)
