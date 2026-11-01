"""Inspect labels on this task's isolated AVD; omit input field contents."""
import subprocess,re,xml.etree.ElementTree as X,json,sys
from pathlib import Path
A=['E:/AndroidDev/sdk/platform-tools/adb.exe','-s','emulator-5588']
def call(*a):return subprocess.check_output(A+list(a),timeout=60)
call('shell','uiautomator','dump','/sdcard/openim-ui-shape.xml')
r=X.fromstring(call('shell','cat','/sdcard/openim-ui-shape.xml'))
items=[]
for n in r.iter('node'):
 if n.get('class')=='android.widget.EditText':continue
 text=n.get('text') or n.get('content-desc') or ''
 if text: items.append({'text':text,'bounds':n.get('bounds'),'clickable':n.get('clickable')})
if len(sys.argv)>1:
 matches=[x for x in items if (sys.argv[1].casefold()==x['text'].casefold() if '--exact' in sys.argv[2:] else sys.argv[1].casefold() in x['text'].casefold())]
 if len(matches)!=1:raise RuntimeError('Expected one label match; got '+str(len(matches)))
 x1,y1,x2,y2=map(int,re.fullmatch(r'\[(-?\d+),(-?\d+)\]\[(-?\d+),(-?\d+)\]',matches[0]['bounds']).groups());cx,cy=(x1+x2)//2,(y1+y2)//2
 if not (0<=cx<540 and 0<=cy<1170):raise RuntimeError('Label center outside own test display')
 call('shell','input','tap',str(cx),str(cy));print('Tapped label '+sys.argv[1])
else:print(json.dumps(items,ensure_ascii=False))
call('shell','rm','/sdcard/openim-ui-shape.xml')
