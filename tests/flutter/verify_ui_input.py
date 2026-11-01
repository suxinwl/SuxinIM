"""Compare synthetic UI input without persisting or printing its plaintext."""
import difflib
import json
from pathlib import Path
import subprocess
import xml.etree.ElementTree as X
ROOT=Path(__file__).absolute().parents[2]
expected=json.loads((ROOT/'.private/test-accounts.json').read_text(encoding='utf-8-sig'))['password']
a=['E:/AndroidDev/sdk/platform-tools/adb.exe','-s','emulator-5588']
def run(args): return subprocess.check_output(a+args,timeout=45)
run(['shell','input','tap','465','510'])
try:
    run(['shell','uiautomator','dump','/sdcard/check-ui.xml'])
    root=X.fromstring(run(['shell','cat','/sdcard/check-ui.xml']))
    fields=[n.get('text','') for n in root.iter('node') if n.get('class')=='android.widget.EditText']
    actual=fields[-1]
    print(json.dumps({'expectedLength':len(expected),'actualLength':len(actual),'equal':actual==expected,
      'differences':[{'kind':t,'expectedRange':[i,j],'actualRange':[k,l]} for t,i,j,k,l in difflib.SequenceMatcher(None,expected,actual).get_opcodes() if t!='equal']}))
finally:
    run(['shell','rm','-f','/sdcard/check-ui.xml'])
    run(['shell','input','tap','465','510'])
