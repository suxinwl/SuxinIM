"""Bounded original-UI acceptance using only the already-running test helper."""
from pathlib import Path
from datetime import datetime, timezone, timedelta
import hashlib
import json
import re
import time
import urllib.request

ROOT = Path(__file__).resolve().parents[2]
OUT = ROOT / "tests/web-desktop/android110-group-rtc/watch-attempt2"
OUT.mkdir(parents=True, exist_ok=False)
started = time.monotonic()
utc = datetime.now(timezone.utc)
deadline = started+60


def write(name, data):
    if not isinstance(data, bytes):
        data = (json.dumps(data, ensure_ascii=False, indent=2)+"\n").encode()
    p = OUT/name
    with p.open("xb") as stream:
        stream.write(data)
    return {"path": p.relative_to(ROOT).as_posix(), "size": len(data), "sha256": hashlib.sha256(data).hexdigest()}


def request(path, body=None):
    req = urllib.request.Request("http://127.0.0.1:17788"+path,
        data=None if body is None else json.dumps(body).encode(),
        headers={} if body is None else {"Content-Type":"application/json"},
        method="GET" if body is None else "POST")
    with urllib.request.urlopen(req, timeout=min(5, max(.1, deadline-time.monotonic()))) as response:
        data = response.read()
    return data, json.loads(data)


write("watch-running.json", {"at":utc.isoformat(), "deadline":(utc+timedelta(seconds=60)).isoformat(),
    "running":True,"pollIntervalSeconds":.5,"groupID":"2901057568","caller":"upgrade38_u104"})
accepted = []
samples = []
try:
    while time.monotonic() < deadline:
        raw, state = request("/state")
        ready = {}
        for peer in ["102", "103"]:
            value = next(p for p in state["peers"] if p["peer"] == peer)
            body = value["body"]
            # GroupProfile originalUI text, not the old message sender's name.
            ready[peer] = (value["pageURL"].endswith("/#/chat/sg_2901057568")
                and "升级测试104邀请你进行" in body and "接听" in body
                and re.search(r"\d+人正在通话", body) is not None)
        samples.append({"at":datetime.now(timezone.utc).isoformat(),"incomingOriginalUIMatched":ready})
        wanted = "102" if not accepted else "103"
        if wanted not in accepted and ready[wanted]:
            write("incoming-before-accept-"+wanted+".json", raw)
            response_raw, response = request("/accept", {"peer":wanted})
            write("accepted-"+wanted+".json", {"at":datetime.now(timezone.utc).isoformat(),
                "peer":wanted,"originalUIResponse":response})
            if not response.get("accepted") or response.get("errors"):
                raise RuntimeError("OriginalUI acceptance failed")
            accepted.append(wanted)
            if len(accepted) == 2:
                raw, state = request("/state")
                ref = write("media-sample-0.json", raw)
                write("accepted-ready.json", {"at":datetime.now(timezone.utc).isoformat(),
                    "acceptedOriginalUIInOrder":accepted,"firstMediaState":ref,"errors":state.get("errors",[])})
                break
        time.sleep(.5)
    write("watch-result.json", {"at":datetime.now(timezone.utc).isoformat(),"acceptedOriginalUIInOrder":accepted,
        "elapsedSeconds":time.monotonic()-started,"samples":samples,"result":"accepted"if len(accepted)==2 else"bounded_no_matching_incoming",
        "SDKSignalingIssued":False})
except Exception as error:
    write("watch-result.json", {"at":datetime.now(timezone.utc).isoformat(),"acceptedOriginalUIInOrder":accepted,
        "elapsedSeconds":time.monotonic()-started,"samples":samples,"result":"watch_error","errorType":type(error).__name__,"SDKSignalingIssued":False})
