from pathlib import Path
import json,subprocess,sys,re,xml.etree.ElementTree as X,time,datetime
R=Path('O:/');D=R/'tests/flutter';A=['E:/AndroidDev/sdk/platform-tools/adb.exe','-s','emulator-5588'];mode=sys.argv[1];spec={'public':('android108-circle-public',None),'private':('android108-circle-private','Private\n'),'exclude102':('android108-circle-exclude102','Not visible to some\n')}
if mode not in spec:raise ValueError('Explicit synthetic range case only')
marker,choice=spec[mode];out=D/f'android-108-circle-{mode}-original-ui.json'
if out.exists():raise RuntimeError('Preserve case evidence')
def adb(*x):return subprocess.check_output(A+list(x),timeout=30)
if adb('shell','pidof','io.openim.upgrade38.test.dev').strip()!=b'7825':raise RuntimeError('Keep tested108 PID')
ops=[]
def tree():
 adb('shell','uiautomator','dump','/sdcard/openim108-range-ui.xml');return X.fromstring(adb('exec-out','cat','/sdcard/openim108-range-ui.xml'))
def text(n):return n.get('text') or n.get('content-desc') or ''
def tap(pred,desc):
 matches=[n for n in tree().iter('node') if pred(n)]
 if len(matches)!=1:raise RuntimeError(desc+' expected1 got'+str(len(matches)))
 x1,y1,x2,y2=map(int,re.findall(r'\d+',matches[0].get('bounds')));adb('shell','input','tap',str((x1+x2)//2),str((y1+y2)//2));ops.append({'at':datetime.datetime.now(datetime.timezone.utc).isoformat(),'action':desc})
def label(s):tap(lambda n:text(n)==s,'tap original '+s)
def shot(s):
 name=f'android-108-circle-{mode}-{s}.png';(D/name).write_bytes(adb('exec-out','screencap','-p'));return 'tests/flutter/'+name
if any(text(n)=='Contacts' and n.get('clickable')=='false' for n in tree().iter('node')):label('Work Circle')
if not any(text(n)=='Work Circle' for n in tree().iter('node')):raise RuntimeError('Expected original Work Circle')
adb('shell','input','tap','505','87');label('Publish Picture')
tap(lambda n:n.get('class')=='android.widget.EditText','focus original description');adb('shell','input','text',marker);adb('shell','input','keyevent','4')
if choice:
 tap(lambda n:text(n).startswith('Who can watch\n'),'open original audience selector');tap(lambda n:text(n).startswith(choice),'choose original '+choice.strip())
 if mode=='exclude102':
  label('My Friends');tap(lambda n:'鍗囩骇娴嬭瘯102' in text(n),'select only102');tap(lambda n:text(n).startswith('Confirm ('),'confirm selected102')
 label('OK')
before=shot('before-publish');label('Publish');time.sleep(1)
post=tree();visible=[text(n) for n in post.iter('node') if marker in text(n)]
if not visible:raise RuntimeError('Published marker not present in original feed')
after=shot('after-publish');record={'at':datetime.datetime.now(datetime.timezone.utc).isoformat(),'applicationVersion':'3.8.8+108','apkSHA256':'34f0fb2bbf605930e02c9409ed76ab214eff1b26f946cafc1946893e92f81626','installedSourceSHA256':'b7f9b666b9b982bd10b0f457d6b802ccde042cad5b0aac57253ffc6036aafcaf','currentWorkingTreeMayContain109Edits':True,'ownPID':7825,'marker':marker,'permissionMode':mode,'originalAndroidPublishCompleted':True,'operations':ops,'screenshots':[before,after],'visibleOriginalFeed':visible,'expectedRecipientVisibility':{'102':mode=='public','103':mode!='private'},'webRecipientEvidence':'pending separate original Web102/103 observation','overallUpgradeAccepted':False};out.write_text(json.dumps(record,indent=2)+'\n',encoding='utf-8');print(json.dumps({'marker':marker,'mode':mode,'published':True,'evidence':out.name}),flush=True)
