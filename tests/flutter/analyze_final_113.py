"""Analyze the same19 static-analysis targets; user-skipped media tests are never executed."""
from pathlib import Path
import json
import re
import subprocess

root = Path(__file__).absolute().parents[2]
client = root / 'clients/flutter'
script = (client / 'scripts/build-android.ps1').read_text(encoding='utf-8')
section = re.search(r'\$analysisTargets\s*=\s*@\((.*?)\n  \)', script, re.S).group(1)
targets = re.findall(r"'([^']+)'", section)
assert len(targets) == 19
assert targets[-1] == 'local_plugin/compat/flutter_bugly-1.1.1/lib/src/flutter_bugly.dart'
log = root / 'tests/flutter/analyze-113-final.log'
scope = root / 'tests/flutter/analysis-scope-113.json'
if log.exists() or scope.exists():
    raise RuntimeError('Do not overwrite analysis evidence')
result = subprocess.run([str(root / 'tools/flutter-3.32.8/bin/cache/dart-sdk/bin/dart.exe'),
                         'analyze', '--format', 'machine', *targets], cwd=client,
                        stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
log.write_bytes(result.stdout)
text = result.stdout.decode('utf-8', errors='replace')
counts = {key.lower(): len(re.findall(r'^' + key + r'\|', text, re.M))
          for key in ['ERROR', 'WARNING', 'INFO']}
scope.write_text(json.dumps({'analysisExit': result.returncode, 'analysisTargets': targets,
                             'counts': counts,
                             'scopeNote': 'Same19static targets as111; no runtime or test execution for user-skipped media. No analysis-scope reduction.'}, indent=2) + '\n', encoding='utf-8')
print(json.dumps({'exit': result.returncode, 'targetCount': len(targets), 'counts': counts}))
if counts['error']:
    raise SystemExit(1)
