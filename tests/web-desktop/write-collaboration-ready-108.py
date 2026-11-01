"""Validate the dedicated idle Web102/103 runtime and record public readiness."""
import hashlib
import json
import re
import subprocess
from datetime import datetime, timezone
from pathlib import Path
from urllib.request import urlopen

ROOT=Path(__file__).resolve().parents[2]
ENDPOINT="http://127.0.0.1:17788"
ARCHIVE="tests/web-desktop/helper-history/108-before-refresh-20261101T063929Z/"
NODE=ROOT/"tools/node-v22.23.4-win-x64/node.exe"

def file(name):
    data=(ROOT/name).read_bytes()
    return {"path":name,"bytes":len(data),"sha256":hashlib.sha256(data).hexdigest()}

state=json.load(urlopen(ENDPOINT+"/state",timeout=20))
assert len(state["peers"])==2 and not state["groups"] and not state["createdMeetingRoomIDs"] and not state["errors"]
for peer in state["peers"]:
    h=peer["health"]
    assert h["ready"] and h["loginStatus"]==3 and h["userID"]==h["expectedUserID"]=="upgrade38_u"+peer["peer"]
    assert h["wasmCallbackReady"] and h["sqliteConversationRead"] and h["observerInstalled"] and peer["transport"]["available"]
    assert not peer["videos"] and not peer["audios"] and not peer["meetingVisible"] and not peer["transport"]["peers"]
listener=subprocess.run(["netstat.exe","-ano"],check=True,capture_output=True,text=True,errors="replace").stdout
matches=re.findall(r"^\s*TCP\s+127\.0\.0\.1:17788\s+\S+\s+LISTENING\s+(\d+)\s*$",listener,re.M)
assert len(matches)==1
pid=int(matches[0])
command=("$ErrorActionPreference='Stop'; $p=Get-CimInstance Win32_Process -Filter 'ProcessId="+str(pid)+"'; "
"$protected=@(Get-Process -Id 39028,41096 | ForEach-Object {@{pid=$_.Id;name=$_.ProcessName;startedAtUTC=$_.StartTime.ToUniversalTime().ToString('o')}}); "
"@{runtime=@{pid=$p.ProcessId;parentPid=$p.ParentProcessId;name=$p.Name;commandLine=$p.CommandLine;startedAtUTC=(Get-Process -Id $p.ProcessId).StartTime.ToUniversalTime().ToString('o')};protected=$protected;oldPidsRemaining=@(Get-CimInstance Win32_Process -Filter 'ProcessId=44296 OR ProcessId=4204' | ForEach-Object {$_.ProcessId})}|ConvertTo-Json -Depth6")
meta=json.loads(subprocess.run(["powershell.exe","-NoProfile","-Command",command.replace("-Depth6","-Depth 6")],check=True,capture_output=True,text=True,errors="replace").stdout)
assert not meta["oldPidsRemaining"] and meta["runtime"]["name"]=="node.exe"
assert "tests/android-collaboration.mjs" in meta["runtime"]["commandLine"].replace("\\","/") and "--start-confirmed" in meta["runtime"]["commandLine"]
old=json.loads((ROOT/ARCHIVE/"manifest.json").read_text(encoding="utf-8-sig"))
assert sorted(meta["protected"],key=lambda x:x["pid"])==sorted(old["protectedProcesses"],key=lambda x:x["pid"])
scripts=["clients/web-desktop/tests/"+name for name in ("android-collaboration.mjs","rtc-media-state.mjs","video-frame-state.mjs")]
for name in scripts:
    subprocess.run([str(NODE),"--check",str(ROOT/name)],check=True,capture_output=True)
prepared=file("tests/web-desktop/android-collaboration-prepared-107.json")
assert prepared["sha256"]=="e036a057f3e21028814f48acc7b252cb85b724aad0dadfd5cf77069c2d044a17"
journal="clients/web-desktop/test-results/android-collaboration/operations.json"
operations=json.loads((ROOT/journal).read_text())
assert not operations["operations"] and not operations["errors"]
serialized=json.dumps(state,ensure_ascii=False,indent=2)+"\n"
assert not re.search(r"eyJ[A-Za-z0-9_-]{12,}\.|X-Amz-Signature",serialized)
state_path="tests/web-desktop/android-collaboration-state-108-ready.json"
assert not (ROOT/state_path).exists()
(ROOT/state_path).write_text(serialized,encoding="utf-8")
summary=[]
for peer in state["peers"]:
    summary.append({key:peer[key] for key in ("peer","sampledAt","health","pageURL","meetingVisible","received","transport")}|{"videos":len(peer["videos"]),"audios":len(peer["audios"]),"meetingControls":len(peer["meetingControls"])})
report={
    "schema":1,"status":"READY108_ACTUAL_RUNTIME_IDLE_NOT_ANDROID_FEATURE_ACCEPTANCE","verifiedAtUTC":datetime.now(timezone.utc).isoformat(),
    "endpoint":ENDPOINT,"sessionID":1074,"nodeProcess":meta["runtime"],"pageOrigin":"http://127.0.0.1:7777","independentTLSStack":"https://192.168.10.11:29880",
    "tls":{"ignoreHTTPSErrors":False,"ignoreCertificateErrorsFlag":False,"evidence":"Original UI Chat login succeeded under default browser TLS validation; only original UI initiated SDK login"},
    "scripts":[file(name) for name in scripts],"syntaxChecks":"node --check passed all3 runtime scripts","peers":summary,
    "currentErrors":state["errors"],"groupsCreatedByThisHelper":state["groups"],"createdMeetingRoomIDs":state["createdMeetingRoomIDs"],"currentGroupID":None,
    "startupJournal":file(journal)|{"operationCount":0},"state":file(state_path),"protectedProcesses":[row|{"unchanged":True} for row in meta["protected"]],
    "oldHelper":{"pid":44296,"sessionID":3255,"closedViaOwnEndpoint":True,"processExited":True,"priorManifest":file(ARCHIVE+"manifest.json")},
    "prepared107":prepared|{"preserved":True,"sourceArchive":file(ARCHIVE+"prepared107-source/manifest.json")},
    "startupProbeFailures":file(ARCHIVE+"startup-attempt-failures-108.json")|{"count":2,"notFunctionalAcceptance":True},
    "usage":file("clients/web-desktop/tests/android-collaboration-usage.md"),
    "boundaries":[
        "Dedicated test browser102/103 only; no user Chrome profile.101 helper and Vite PID/start-time unchanged.",
        "Audio/native RTP observer actually installed but no media present; real audio/RTP receipt/playback remains untested by this preparation.",
        "Browser transport/playback/energy evidence is distinct from physical speaker audibility.",
        "New helper operation journal is empty; no group/meeting/invitation/message/call/composer operation executed.",
        "Android original UI/AVD and frozen product/native sources were not operated or modified.",
        "Old startup probe failures and intermediate health-serialization adjustment are test-tool preparation, not Android or product functionality acceptance. Current readiness is based on direct real SDK queries sampled after startup.",
    ],
}
target="tests/web-desktop/android-collaboration-ready-108.json"
assert not (ROOT/target).exists()
(ROOT/target).write_text(json.dumps(report,ensure_ascii=False,indent=2)+"\n",encoding="utf-8")
print(json.dumps(file(target)|{"pid":pid,"sessionID":1074,"peers":[row["health"] for row in summary]},ensure_ascii=False))
