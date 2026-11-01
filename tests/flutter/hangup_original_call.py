"""Operate the original hang-up and test UI input while cleanup completes."""

from pathlib import Path

import datetime, hashlib, json, re, subprocess, sys, threading, time, urllib.request



ROOT=Path(__file__).absolute().parents[2]

label=sys.argv[1]

if not re.fullmatch(r'android-(104|105|106|107|108|109|110)-call[12]|android-108-(?:recapture|cold-video|voice)-diagnostic',label):raise ValueError('Explicit own test label required')

if (ROOT/f'tests/flutter/{label}-hangup.json').exists():raise RuntimeError('Preserve existing hangup evidence; use a new explicit test label')

ADB=['E:/AndroidDev/sdk/platform-tools/adb.exe','-s','emulator-5588']

def adb(*args):return subprocess.check_output(ADB+list(args),timeout=30)

if adb('emu','avd','name').splitlines()[0] not in [b'openim38_api30', b'openim38_api30_arm109']:raise RuntimeError('Own AVD only')

def state():

    with urllib.request.urlopen('http://127.0.0.1:17787/state',timeout=15) as response:return json.load(response)

def shot(name):

    p=ROOT/f'tests/flutter/{label}-{name}.png';p.write_bytes(adb('exec-out','screencap','-p'));return p.relative_to(ROOT).as_posix()

began=datetime.datetime.now(datetime.timezone.utc).isoformat();start=time.monotonic()

device_start_epoch=int(adb('shell','date','+%s').decode().strip())

hangup_device_start_epoch=device_start_epoch

started_pid=adb('shell','pidof','io.openim.upgrade38.test.dev').decode().strip()

if not started_pid.isdigit():raise RuntimeError('Expected one own-app process before hangup')

call_samples=ROOT/f'tests/flutter/{label}-samples.json'

if call_samples.is_file():

    prior_samples=json.loads(call_samples.read_text())

    if prior_samples and prior_samples[0].get('androidPID')==int(started_pid):

        device_start_epoch=prior_samples[0].get('callDeviceEpochStart',device_start_epoch)

package=adb('shell','dumpsys','package','io.openim.upgrade38.test.dev').decode(errors='replace')

version=re.search(r'versionName=([^\s]+)',package)[1]

build=int(re.search(r'versionCode=(\d+)',package)[1])

if int(label.split('-')[1]) != build:raise RuntimeError('Label does not match installed own app build')

before_anr=adb('shell','dumpsys','activity','lastanr')

before_anr_sha=hashlib.sha256(before_anr).hexdigest()

diagnostic_records=[]

diagnostic_errors=[]

diagnostic_thread=None

if '--diagnostic-sigquit' in sys.argv:

    if build!=105:raise RuntimeError('This diagnostic is restricted to candidate105')

    old_traces=set(adb('shell','find','/data/anr','-maxdepth','1','-type','f').decode().splitlines())

    tap_clock=time.monotonic()

    def dump_native_threads():

        try:

            for second in [5,10]:

                while time.monotonic()-tap_clock<second:time.sleep(.05)

                current=adb('shell','pidof','io.openim.upgrade38.test.dev').decode().strip()

                if current!=started_pid:raise RuntimeError('Own app PID changed before requested diagnostic')

                adb('shell','kill','-3',started_pid)

                diagnostic_records.append({'requestedSecondsAfterHangup':second,

                    'hostAt':datetime.datetime.now(datetime.timezone.utc).isoformat(),

                    'deviceEpoch':int(adb('shell','date','+%s').decode().strip()),

                    'pid':int(started_pid),'signal':'SIGQUIT','anr':False})

            time.sleep(3)

            new_traces=set(adb('shell','find','/data/anr','-maxdepth','1','-type','f').decode().splitlines())-old_traces

            saved=[]

            for index,trace in enumerate(sorted(new_traces)):

                if not re.fullmatch(r'/data/anr/[A-Za-z0-9_.:-]+',trace):raise RuntimeError('Unexpected diagnostic trace path')

                raw=adb('exec-out','cat',trace)

                if f'----- pid {started_pid} '.encode() not in raw:continue

                private=ROOT/f'.private/android-raw-evidence/{label}-manual-sigquit-{index}.txt'

                if private.exists():raise RuntimeError('Do not overwrite manual trace history')

                private.write_bytes(raw)

                public,count=re.subn(rb'eyJ[A-Za-z0-9_-]{15,}\.[A-Za-z0-9_-]{20,}\.[A-Za-z0-9_-]{15,}',b'[REDACTED_JWT]',raw)

                target=ROOT/f'tests/flutter/{label}-manual-sigquit-{index}.txt';target.write_bytes(public)

                saved.append({'source':trace,'privateRaw':private.relative_to(ROOT).as_posix(),

                    'public':target.relative_to(ROOT).as_posix(),'rawSHA256':hashlib.sha256(raw).hexdigest(),

                    'publicSHA256':hashlib.sha256(public).hexdigest(),'jwtRedactions':count})

            diagnostic_records.append({'manualDumpFiles':saved,'note':'Actively requested SIGQUIT thread snapshots; never classified as system ANR.'})

        except BaseException as error:

            diagnostic_errors.append(type(error).__name__)

    diagnostic_thread=threading.Thread(target=dump_native_threads,name='own105-manual-thread-dumps')

    diagnostic_thread.start()

adb('shell','input','tap','282','970')

hangupAt=time.monotonic()-start

# These are original UI controls, with no direct SDK/message injection.

adb('shell','input','tap','200','1057')

marker={'android-108-recapture-diagnostic':'a108rd','android-108-cold-video-diagnostic':'a108cv','android-108-voice-diagnostic':'a108v'}.get(label,f'a{build}c{label[-1]}')

adb('shell','input','text',marker)

adb('shell','input','tap','499','662')

inputDone=time.monotonic()-start

screen=shot('after-hangup-input')

samples=[];firstSeen=None;released=None

for index in range(21):

    web=state()

    if marker in web.get('body','') and firstSeen is None:firstSeen=round(time.monotonic()-start,3)

    camera=adb('shell','dumpsys','media.camera').decode(errors='replace')

    section=re.search(r'Active Camera Clients:\s*(\[[\s\S]*?\])',camera)

    active=section.group(1).strip() if section else None

    if active is not None and not active[1:-1].strip():active='[]'

    pid=adb('shell','pidof','io.openim.upgrade38.test.dev').decode().strip()

    if pid != started_pid:raise RuntimeError('Own app process changed during cleanup sampling')

    # Individual tasks can exit between glob expansion and cat; preserve partial

    # samples and the return code instead of aborting the entire evidence run.

    task_sample=subprocess.run(ADB+['shell',f'cat /proc/{pid}/task/*/comm 2>/dev/null'],capture_output=True,timeout=30)

    names=task_sample.stdout.decode(errors='replace').splitlines()

    item={'elapsedSeconds':round(time.monotonic()-start,3),'activeCameraClients':active,

          'webVideos':len(web.get('videos',[])),'webErrors':web.get('errors',[]),

          'uiMessageReceived':marker in web.get('body',''),'nativeStatsWorkers':sum(x.startswith('OpenIMRtcStats') for x in names),

          'nativeCleanupWorkers':sum(x.startswith('OpenIMRtcCleanu') for x in names),

          'nativeMediaWorkers':sum(x.startswith('OpenIMRtcMedia-') for x in names),

          'nativeMediaDeadlineThreads':sum(x.startswith('OpenIMRtcMediaD') for x in names),'threadCount':len(names),'threadSampleReturnCode':task_sample.returncode}

    samples.append(item)

    if active=='[]' and not web.get('videos') and released is None:

        released=item['elapsedSeconds']

    if released is not None and firstSeen is not None:break

    time.sleep(3)

if diagnostic_thread is not None:diagnostic_thread.join(timeout=45)

with urllib.request.urlopen('http://127.0.0.1:17787/messages',timeout=15) as response:

    received_messages=json.load(response)

if isinstance(received_messages,dict):received_messages=list(received_messages.values())

message_proof=next(({'clientMsgID':m.get('clientMsgID'),'sendID':m.get('sendID'),

    'contentType':m.get('contentType'),'text':m.get('text')}

    for m in received_messages if m.get('sendID')=='upgrade38_u104' and

    m.get('contentType')==101 and m.get('text')==marker),None)

after_anr=adb('shell','dumpsys','activity','lastanr')

anr=after_anr.decode(errors='replace')

(ROOT/f'tests/flutter/{label}-lastanr.txt').write_text(anr)

# lastanr is global and another process can replace it. Independently inspect

# ActivityManager's time-windowed ANR records for this exact test package/PID.

device_end_epoch=int(adb('shell','date','+%s').decode().strip())

manager_log=adb('logcat','-d','-v','epoch','-b','system','-b','main',

                'ActivityManager:I','ActivityTaskManager:I','InputDispatcher:I','*:S').decode(errors='replace')

manager_lines=[]

for line in manager_log.splitlines():

    match=re.match(r'^\s*(\d+\.\d+)\s+\d+\s+\d+\s+[VDIWEFAS]\s+([^:]+):\s?(.*)$',line)

    if match and device_start_epoch-1<=float(match[1])<=device_end_epoch+1:

        manager_lines.append({'epoch':float(match[1]),'tag':match[2].strip(),'message':match[3]})

anr_events=[]

for index,item in enumerate(manager_lines):

    if 'ANR in io.openim.upgrade38.test.dev' not in item['message']:continue

    block=manager_lines[index:index+16]

    pid_line=next((entry['message'] for entry in block if re.search(r'\bPID:\s*\d+',entry['message'])),None)

    event_pid=re.search(r'\bPID:\s*(\d+)',pid_line)[1] if pid_line else None

    anr_events.append({'epoch':item['epoch'],'package':'io.openim.upgrade38.test.dev',

        'pid':int(event_pid) if event_pid else None,'matchesStartedPID':event_pid==started_pid,

        'metadata':block})

(ROOT/f'tests/flutter/{label}-activitymanager-anr.json').write_text(json.dumps({

    'deviceEpochStart':device_start_epoch,'hangupDeviceEpochStart':hangup_device_start_epoch,'deviceEpochEnd':device_end_epoch,

    'ownPID':int(started_pid),'events':anr_events,

    'scope':'Only ActivityManager/ActivityTaskManager/InputDispatcher ANR metadata in this hangup window; no IM token logs'},indent=2)+'\n')

record={'applicationVersion':f'{version}+{build}','startedAt':began,'hangupInputCompletedAtSeconds':round(hangupAt,3),

        'requestedMarker':marker,'scriptSHA256':hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),

        'composerInputCompletedAtSeconds':round(inputDone,3),'messageReceivedAtSeconds':firstSeen,

        'exactSDKMessageMatched':message_proof is not None,'actualSDKMessageProof':message_proof,

        'cameraAndWebReleasedAtSampleSeconds':released,'screenshot':screen,'samples':samples,

        'priorANRDumpSHA256':before_anr_sha,

        'afterANRDumpSHA256':hashlib.sha256(after_anr).hexdigest(),

        'priorANRExisted':'<no ANR has occurred since boot>' not in before_anr.decode(errors='replace'),

        'lastANRPresent':hashlib.sha256(after_anr).hexdigest()!=before_anr_sha and 'io.openim.upgrade38.test.dev' in anr.split('mLastHomeActivity')[0],

        'ownPID':int(started_pid),'activityManagerANRPresent':bool(anr_events),

        'activityManagerANREvidence':f'tests/flutter/{label}-activitymanager-anr.json',

        'manualSIGQUITDiagnostics':diagnostic_records,'manualSIGQUITErrorTypes':diagnostic_errors,

        'note':'ADB commands each take host time; media release time is sampled upper bound, exact native stages remain in redacted logcat.'}

(ROOT/f'tests/flutter/{label}-hangup.json').write_text(json.dumps(record,indent=2)+'\n')

print(json.dumps(record))

