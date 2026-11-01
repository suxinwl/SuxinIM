from pathlib import Path
import json,re,subprocess
root=Path('O:/');client=root/'clients/flutter';s=(client/'scripts/build-android.ps1').read_text()
section=re.search(r'\$analysisTargets\s*=\s*@\((.*?)\n  \)',s,re.S).group(1)
targets=re.findall(r"'([^']+)'",section)
result=subprocess.run([str(root/'tools/flutter-3.32.8/bin/cache/dart-sdk/bin/dart.exe'),'analyze','--format','machine',*targets],cwd=client,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
log=root/'tests/flutter/analyze-108-final.log';log.write_bytes(result.stdout)
text=result.stdout.decode('utf-8',errors='replace');counts={x.lower():len(re.findall(r'^'+x+r'\|',text,re.M)) for x in ['ERROR','WARNING','INFO']}
(root/'tests/flutter/analysis-scope-108.json').write_text(json.dumps({'testExit':0,'analysisExit':result.returncode,'analysisTargets':targets,'counts':counts,'scopeNote':'108 adds actual media devices leaf to the107 scope; counts must not be compared as same-scope trends.'},indent=2)+'\n')
print(json.dumps({'exit':result.returncode,'targetCount':len(targets),'counts':counts}))
if counts['error']: raise SystemExit(1)
