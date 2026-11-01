"""Read real Web UI frame counters and own-AVD screenshots during a call."""
from pathlib import Path
import datetime,hashlib,json,subprocess,sys,time,urllib.request,re
from PIL import Image
import io
ROOT=Path(__file__).absolute().parents[2]
label=sys.argv[1]
if not re.fullmatch(r'android-(10[3-9]|110)-call[12]',label):raise ValueError('Explicit own call evidence label required')
ADB=['E:/AndroidDev/sdk/platform-tools/adb.exe','-s','emulator-5588']
if any((ROOT/f'tests/flutter/{label}{suffix}').exists() for suffix in ['-samples.json',*[f'-{n:03d}.png' for n in [0,30,60,90,120,150]]]):raise RuntimeError('Preserve existing call evidence; use a new explicit test label')
if subprocess.check_output(ADB+['emu','avd','name'],timeout=15).splitlines()[0]!=b'openim38_api30':raise RuntimeError('Owned original API30 AVD only')
installed=subprocess.check_output(ADB+['shell','dumpsys','package','io.openim.upgrade38.test.dev'],timeout=20).decode(errors='replace')
if int(re.search(r'versionCode=(\d+)',installed)[1])!=int(label.split('-')[1]):raise RuntimeError('Evidence label must match installed own build')
samples=[];start=time.monotonic()
device_start_epoch=int(subprocess.check_output(ADB+['shell','date','+%s'],text=True,timeout=15).strip())
for target in [0,30,60,90,120,150]:
    while time.monotonic()-start<target:time.sleep(min(1,target-(time.monotonic()-start)))
    with urllib.request.urlopen('http://127.0.0.1:17787/state',timeout=15) as response:state=json.load(response)
    png=subprocess.check_output(ADB+['exec-out','screencap','-p'],timeout=30)
    shot=ROOT/f'tests/flutter/{label}-{target:03d}.png';shot.write_bytes(png)
    image=Image.open(io.BytesIO(png)).convert('RGB')
    crop=image.crop((20,200,330,820))
    pid=subprocess.check_output(ADB+['shell','pidof','io.openim.upgrade38.test.dev'],text=True,timeout=15).strip()
    if not pid.isdigit():raise RuntimeError('Expected one own-app process')
    # One toybox process: spawning cat once per thread can itself take tens of
    # seconds on a software emulator and distort the sampling window.
    names=subprocess.run(ADB+['shell',f'cat /proc/{pid}/task/*/comm 2>/dev/null'],
        capture_output=True,text=True,timeout=20).stdout.splitlines()
    # Inspect only the explicit safe development-counter line. All other logcat
    # content stays out of this report; full raw evidence uses the private log
    # capture/redaction workflow separately.
    counter_log=subprocess.check_output(ADB+['logcat','-d','-v','epoch','-s','flutter:I'],
        text=True,encoding='utf-8',errors='replace',timeout=20)
    receiver_counters={};receiver_states={}
    for line in counter_log.splitlines():
        state_match=re.match(r'^\s*(\d+\.\d+)\s+(\d+)\s+\d+\s+I\s+flutter\s*:\s*\[OpenIMRtcReceiveState\] track=([A-Za-z0-9_.:/-]{1,160}) state=([A-Za-z0-9_]{1,100})$',line)
        if state_match and int(state_match[2])==int(pid) and float(state_match[1])>=device_start_epoch:
            receiver_states[state_match[3]]={'deviceEpoch':float(state_match[1]),'state':state_match[4]}
        match=re.match(r'^\s*(\d+\.\d+)\s+(\d+)\s+\d+\s+I\s+flutter\s*:\s*\[OpenIMRtcReceive\] track=([A-Za-z0-9_.:/-]{1,160}) framesDecoded=(\d+(?:\.\d+)?|null) bytesReceived=(\d+(?:\.\d+)?|null)$',line)
        if not match or int(match[2])!=int(pid) or float(match[1])<device_start_epoch:continue
        receiver_counters[match[3]]={'deviceEpoch':float(match[1]),
            'framesDecoded':None if match[4]=='null' else float(match[4]),
            'bytesReceived':None if match[5]=='null' else float(match[5])}
    sample={'elapsedSeconds':round(time.monotonic()-start,1),'capturedAt':datetime.datetime.now(datetime.timezone.utc).isoformat(),
        'callDeviceEpochStart':device_start_epoch,
        'webSampledAt':state.get('sampledAt'),'videos':state.get('videos'),
        'webErrors':state.get('errors'),'webCallUIOpen':'挂断' in state.get('body',''),
        'androidScreenshot':shot.relative_to(ROOT).as_posix(),'androidPID':int(pid),
        'androidRemoteVideoCropSHA256':hashlib.sha256(crop.tobytes()).hexdigest(),
        'nativeRemoteReceiverCounters':receiver_counters,
        'nativeRemoteReceiverStates':receiver_states,
        'nativeStatsWorkers':sum(n.startswith('OpenIMRtcStats') for n in names),
        'nativeCleanupWorkers':sum(n.startswith('OpenIMRtcCleanu') for n in names),
        'nativeMediaWorkers':sum(n.startswith('OpenIMRtcMedia-') for n in names),
        'nativeMediaDeadlineThreads':sum(n.startswith('OpenIMRtcMediaD') for n in names),
        'threadCount':len(names)}
    samples.append(sample)
    (ROOT/f'tests/flutter/{label}-samples.json').write_text(json.dumps(samples,indent=2)+'\n')
    print(json.dumps({'elapsed':sample['elapsedSeconds'],'videos':sample['videos'],'statsWorkers':sample['nativeStatsWorkers'],'cleanupWorkers':sample['nativeCleanupWorkers'],'mediaWorkers':sample['nativeMediaWorkers'],'mediaDeadlineThreads':sample['nativeMediaDeadlineThreads'],'nativeRemoteReceiverCounters':receiver_counters}),flush=True)
print('Completed150-second real original UI frame sampling',flush=True)
