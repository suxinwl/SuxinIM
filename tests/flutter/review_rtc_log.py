"""Read-only RTC log review. A clean scan is never a device acceptance verdict.

Report metadata and classifications only; never copy arbitrary log messages,
URLs, tokens, SDP, or media content into the report. Counts describe log events,
including propagated errors, rather than distinct root causes.
"""
from __future__ import annotations

import argparse
import collections
import datetime
import hashlib
import json
import pathlib
import re

ROOT = pathlib.Path(__file__).resolve().parents[2]
THREADTIME = re.compile(
    r"^(?P<time>\d\d-\d\d \d\d:\d\d:\d\d\.\d+)\s+"
    r"(?P<pid>\d+)\s+\d+\s+[VDIWEF]\s+[^:]+:\s?(?P<message>.*)$"
)
CLEANUP = re.compile(r"\[OpenIMRtcCleanup\]\s+(?P<stage>[\w./-]+) failed (?P<ms>\d+)ms\b")
CODE = re.compile(r"\b(?:RTC_[A-Z0-9_]+|PlatformException\(([A-Za-z0-9_-]+))")


def classify(message: str) -> tuple[list[str], dict]:
    categories, details = [], {}
    cleanup = CLEANUP.search(message)
    if cleanup:
        categories.append("caughtCleanupFailure")
        details.update(stage=cleanup["stage"][:120], elapsedMs=int(cleanup["ms"]))
    if "RTCVideoRenderer source change failed:" in message:
        categories.append("caughtRendererFailure")
    capture = re.search(r"\[OpenIMCallCapture\] stage=([A-Za-z0-9_.-]+) current=(true|false) error=([A-Za-z0-9_-]+)", message)
    if capture:
        categories.append("caughtCaptureFailure")
        details.update(stage=capture[1][:120], currentCall=capture[2] == "true", safeError=capture[3][:80])
    attachment = re.search(r"\[OpenIMRendererAttachment\] stage=([A-Za-z0-9_.-]+) current=(true|false) error=([A-Za-z0-9_-]+)", message)
    if attachment:
        categories.append("caughtRendererAttachmentFailure")
        details.update(stage=attachment[1][:120], currentSource=attachment[2] == "true", safeError=attachment[3][:80])
    if "Unhandled Exception:" in message:
        categories.append("unhandledException")
    if "FATAL EXCEPTION:" in message or "Fatal signal " in message:
        categories.append("fatal")
    if "SocketException:" in message:
        categories.append("socketExceptionForReview")
    # Native diagnostics are matched as fixed fields, not arbitrary words such
    # as the valid stage name retiredSocketFailure or a success containing code.
    if re.search(r"\bphase=(?:actionEnd|physicalFailure|physicalComplete|physicalOutcome)\b.*\bfailure=", message):
        categories.append("nativePhysicalFailure")
    if re.search(r"\bphase=(?:callerDeadline|callerTimeout)\b", message):
        categories.append("nativeCallerDeadline")
    if re.search(r"\bphase=captureRejected\b", message) or re.search(r"\boperation=local\.admission\b.*\bphase=blocked\b", message):
        categories.append("nativeCaptureRejected")
    if re.search(r"\bphase=(?:rejected|attachRejected)\b", message):
        categories.append("nativeAdmissionRejected")
    if re.search(r"\bphase=(?:quarantine|deadlineOverflow)\b", message):
        categories.append("nativePermanentQuarantine")
    if re.search(r"\bphase=callerError\b", message):
        categories.append("nativeCallerError")
    if "Unable to getUserMedia: Local media cleanup is still pending" in message:
        categories.append("captureCleanupGuard")
    if categories:
        codes = []
        for match in CODE.finditer(message):
            code = match.group(1) or match.group(0)
            if code not in codes:
                codes.append(code)
        if codes:
            details["codes"] = codes
        if "PlatformException" in message:
            details["errorType"] = "PlatformException"
    return categories, details


def review(data: bytes, pid: int) -> dict:
    events, counts = [], collections.Counter()
    ignored_other_pid = 0
    for number, line in enumerate(data.decode("utf-8", errors="replace").splitlines(), 1):
        parsed = THREADTIME.match(line)
        if not parsed:
            continue
        categories, details = classify(parsed["message"])
        if int(parsed["pid"]) != pid:
            ignored_other_pid += bool(categories)
            continue
        if categories:
            counts.update(categories)
            events.append(dict(line=number, deviceTime=parsed["time"], pid=pid,
                               categories=categories, **details))
    return dict(pid=pid, eventCount=len(events), categoryCounts=dict(counts),
                ignoredOtherPIDEvents=ignored_other_pid, events=events)


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--log", type=pathlib.Path, required=True)
    parser.add_argument("--pid", type=int, required=True)
    parser.add_argument("--output", type=pathlib.Path, required=True)
    args = parser.parse_args()
    if args.pid <= 0:
        parser.error("pid must be positive")
    data = args.log.read_bytes()
    report = dict(schema=1, createdAt=datetime.datetime.now(datetime.timezone.utc).isoformat(),
                  logPath=args.log.resolve().relative_to(ROOT).as_posix(), logBytes=len(data),
                  toolPath=pathlib.Path(__file__).resolve().relative_to(ROOT).as_posix(),
                  toolSHA256=hashlib.sha256(pathlib.Path(__file__).read_bytes()).hexdigest(),
                  logSHA256=hashlib.sha256(data).hexdigest(), **review(data, args.pid),
                  boundary="Classified own-PID log events, including caught failures and propagation. "
                  "Not distinct root causes, a complete exception parser, ANR trace review, media proof, "
                  "or an automatic acceptance verdict. Original log and all other evidence require review.")
    with args.output.open("x", encoding="utf-8") as output:
        json.dump(report, output, ensure_ascii=False, indent=2)
        output.write("\n")
    print(json.dumps({key: report[key] for key in (
        "logSHA256", "pid", "eventCount", "categoryCounts", "ignoredOtherPIDEvents")}, ensure_ascii=False))


if __name__ == "__main__":
    main()
