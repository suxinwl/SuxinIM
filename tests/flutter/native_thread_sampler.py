"""Opt-in, bounded, read-only sampling for one fixed owned Android test app.

This file's preparation/tests do not run adb. Device use belongs to the AVD owner.
No stacks, SIGQUIT, JNI calls, application storage, or application APIs are read.
"""
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path
import argparse
import json
import re
import subprocess
import time

ROOT = Path(__file__).resolve().parents[2]
ADB = Path("E:/AndroidDev/sdk/platform-tools/adb.exe")
SERIAL = "emulator-5588"
AVD = "openim38_api30"
PACKAGE = "io.openim.upgrade38.test.dev"


@dataclass(frozen=True)
class Config:
    label: str
    duration: float = 180.0
    interval: float = 2.0

    def validate(self):
        if not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9_-]{0,63}", self.label):
            raise ValueError("Label must be a new safe 1-64 character identifier")
        if not 1 <= self.duration <= 180 or not 1 <= self.interval <= 2:
            raise ValueError("Duration must be1-180s; interval must be1-2s")


def parse_pid(text):
    text = text.strip()
    if not re.fullmatch(r"[1-9][0-9]*", text):
        raise ValueError("Expected exactly one positive main-package PID")
    return int(text)


def parse_stat(text, expected_tid):
    """Linux /proc stat: comm can contain spaces and ')' characters."""
    close = text.rfind(")")
    open_at = text.find("(")
    if open_at < 1 or close <= open_at:
        raise ValueError("Malformed task stat")
    tid = parse_pid(text[:open_at])
    fields = text[close+1:].split()
    if tid != expected_tid or len(fields) < 20 or not re.fullmatch(r"[A-Za-z]", fields[0]):
        raise ValueError("Wrong task identity or truncated stat")
    # fields[0] is kernel stat field3(state), not field1(pid).
    utime, stime, start = (int(fields[i]) for i in (11, 12, 19))
    if min(utime, stime, start) < 0:
        raise ValueError("Negative unsigned kernel tick field")
    return {"tid": tid, "statComm": text[open_at+1:close], "state": fields[0],
            "utimeTicks": utime, "stimeTicks": stime, "starttimeTicks": start}


def batch_script(pid):
    if not isinstance(pid, int) or pid <= 0:
        raise ValueError("Only a validated numeric PID may enter the fixed shell script")
    # One fixed shell batch. Only numeric PID is substituted. Per-thread proc
    # reads use shell builtins, not command substitution or external cat/tr.
    # Full NUL-separated cmdline identity stays unchanged (two tr per batch).
    return r'''# Exactly two fixed initialization subshells, never one per thread.
TAB="$(printf '\t')"
CR="$(printf '\r')"
# Global proc_value: invoking this function in $(...) would fork per read.
# Normalize embedded line delimiters and tabs/CR to spaces for the line protocol.
read_proc() {
  proc_value=
  proc_first=1
  proc_line=
  while IFS= read -r proc_line || [ -n "$proc_line" ]; do
    if [ "$proc_first" = 0 ]; then proc_value="$proc_value "; fi
    proc_value="$proc_value$proc_line"
    proc_first=0
  done 2>/dev/null < "$1"
  while :; do
    case "$proc_value" in
      *"$TAB"*) proc_value="${proc_value%%"$TAB"*} ${proc_value#*"$TAB"}" ;;
      *"$CR"*) proc_value="${proc_value%%"$CR"*} ${proc_value#*"$CR"}" ;;
      *) break ;;
    esac
  done
}
current="$(pidof io.openim.upgrade38.test.dev 2>/dev/null)"
printf 'PIDOF\t%s\n' "$current"
if [ "$current" != "__PID__" ]; then printf 'STOP\tpid_changed\n'; exit 0; fi
printf 'CMDLINE\t'
tr '\000\n\t' '   ' < /proc/__PID__/cmdline 2>/dev/null
printf '\n'
read_proc /proc/uptime
printf 'UPTIME\t%s\n' "$proc_value"
read_proc /proc/__PID__/stat
printf 'PSTAT\t%s\n' "$proc_value"
for task in /proc/__PID__/task/*; do
  [ -d "$task" ] || continue
  tid="${task##*/}"
  case "$tid" in ''|*[!0-9]*) continue;; esac
  read_proc "$task/comm"
  printf 'COMM\t%s\t%s\n' "$tid" "$proc_value"
  read_proc "$task/stat"
  printf 'STAT\t%s\t%s\n' "$tid" "$proc_value"
  read_proc "$task/wchan"
  printf 'WCHAN\t%s\t%s\n' "$tid" "$proc_value"
done
current_end="$(pidof io.openim.upgrade38.test.dev 2>/dev/null)"
printf 'PIDOF_END\t%s\n' "$current_end"
if [ "$current_end" != "__PID__" ]; then printf 'STOP\tpid_changed_during_batch\n'; exit 0; fi
printf 'CMDLINE_END\t'
tr '\000\n\t' '   ' < /proc/__PID__/cmdline 2>/dev/null
printf '\n'
read_proc /proc/__PID__/stat
printf 'PSTAT_END\t%s\n' "$proc_value"
printf 'END\n'
'''.replace("__PID__", str(pid))


def parse_batch(text, pid):
    result = {"threads": [], "parseErrors": []}
    threads = {}
    for line in text.replace("\r\n", "\n").splitlines():
        if not line:
            continue
        kind, _, value = line.partition("\t")
        if kind in ("COMM", "STAT", "WCHAN"):
            tid_text, sep, raw = value.partition("\t")
            try:
                tid = parse_pid(tid_text)
                if not sep:
                    raise ValueError("Missing field delimiter")
                row = threads.setdefault(tid, {"tid": tid})
                if kind == "COMM":
                    row["comm"] = raw.rstrip()
                elif kind == "STAT":
                    if raw.strip():
                        row.update(parse_stat(raw, tid))
                    else:
                        row["statUnavailable"] = True
                else:
                    row["wchan"] = raw.strip()
                    row["wchanKnown"] = bool(raw.strip() and raw.strip() != "0")
                    if not row["wchanKnown"]:
                        row["wchanMeaning"] = "unknown; not evidence of CPU busy/running"
            except ValueError as error:
                result["parseErrors"].append({"field": kind, "reason": str(error)})
        elif kind in ("PIDOF", "PIDOF_END"):
            try:
                result["observedPID" if kind == "PIDOF" else "endPID"] = parse_pid(value)
            except ValueError:
                result["identityFailure"] = "main PID missing or not unique"
        elif kind in ("CMDLINE", "CMDLINE_END"):
            result["cmdline" if kind == "CMDLINE" else "endCmdline"] = value.strip(" \t\0")
        elif kind == "UPTIME":
            values = value.split()
            if values and re.fullmatch(r"[0-9]+(?:\.[0-9]+)?", values[0]):
                result["deviceUptimeSecondsRaw"] = values[0]
            else:
                result["parseErrors"].append({"field": "UPTIME", "reason": "missing/invalid"})
        elif kind in ("PSTAT", "PSTAT_END"):
            try:
                result["process" if kind == "PSTAT" else "endProcess"] = parse_stat(value, pid)
            except ValueError as error:
                result["identityFailure"] = "process stat unreadable: " + str(error)
        elif kind == "STOP":
            result["stop"] = value
        elif kind == "END":
            result["batchEnded"] = True
        else:
            # Do not retain arbitrary command output in a report.
            result["parseErrors"].append({"field": "unexpected", "reason": "unrecognized batch line"})
    result["threads"] = sorted(threads.values(), key=lambda row: row["tid"])
    return result


def adb_runner(arguments, timeout):
    result = subprocess.run([str(ADB), "-s", SERIAL, *arguments], capture_output=True,
                            timeout=timeout, check=False)
    if result.returncode != 0:
        # No stderr body is copied; this sampler needs no arbitrary device log.
        raise RuntimeError("Fixed read-only adb command failed, exit=" + str(result.returncode))
    return result.stdout.decode("utf-8", errors="replace")


def capture(config, runner=adb_runner, clock=time.monotonic, sleep=time.sleep):
    config.validate()
    started = clock()
    deadline = started + config.duration
    report = {"schema": 1, "label": config.label, "recordedAt": datetime.now(timezone.utc).isoformat(),
        "serial": SERIAL, "requiredAVD": AVD, "package": PACKAGE,
        "durationLimitSeconds": config.duration, "intervalSeconds": config.interval,
        "samples": [], "CPUPercentCalculated": False, "CLK_TCKVerified": False,
        "stackOrSIGQUITOrJNIOrApplicationDataRead": False,
        "instrumentationOverheadMeasured": False,
        "limits": ["Raw cumulative ticks are not CPU percentages or a window busy-loop proof.",
                   "wchan0/empty means unknown, not running/busy.",
                   "Threads can exit/rename between reads; unavailable/parse errors are preserved.",
                   "No lock owner or exact unsymbolized native task is identified by this sampling alone."]}

    def run(args):
        remaining = deadline-clock()
        if remaining <= 0:
            raise TimeoutError("bounded sampling window completed")
        return runner(args, min(15.0, remaining))

    try:
        name = run(["emu", "avd", "name"]).replace("\r", "").splitlines()
        if not name or name[0].strip() != AVD or any(x.strip() not in (AVD, "OK", "") for x in name):
            report["stopReason"] = "refused_wrong_avd"
            return report
        pid = parse_pid(run(["shell", "pidof", PACKAGE]))
        report["fixedPID"] = pid
        starttime = None
        while clock() < deadline:
            at = clock()
            batch = parse_batch(run(["shell", batch_script(pid)]), pid)
            identity_ok = (batch.get("observedPID") == pid and batch.get("cmdline") == PACKAGE
                           and batch.get("endPID") == pid and batch.get("endCmdline") == PACKAGE
                           and not batch.get("identityFailure") and batch.get("process") is not None
                           and batch.get("endProcess") is not None and batch.get("batchEnded"))
            if batch.get("stop") or not identity_ok:
                report["stopReason"] = "pid_or_package_identity_changed_or_unreadable"
                report["identityStop"] = {"observedPID": batch.get("observedPID"),
                    "reason": batch.get("stop") or batch.get("identityFailure") or "package identity mismatch"}
                break
            current_start = batch["process"]["starttimeTicks"]
            if batch["endProcess"]["starttimeTicks"] != current_start:
                report["stopReason"] = "pid_reused_during_batch"
                break
            if starttime is None:
                starttime = current_start
                report["fixedProcessStarttimeTicks"] = starttime
            elif starttime != current_start:
                report["stopReason"] = "pid_reused_with_new_process_starttime"
                break
            report["samples"].append({"hostSecondsSinceStart": round(at-started, 6),
                "hostBatchElapsedSeconds": round(clock()-at, 6), **batch})
            remaining = deadline-clock()
            if remaining <= 0:
                break
            # No catch-up bursts when reads take longer than the interval.
            sleep(min(config.interval, remaining))
        report.setdefault("stopReason", "bounded_window_completed")
    except KeyboardInterrupt:
        report["stopReason"] = "operator_interrupted"
    except (ValueError, RuntimeError, TimeoutError, OSError, subprocess.TimeoutExpired) as error:
        report["stopReason"] = "bounded_window_completed" if clock() >= deadline else "read_or_identity_failure"
        report["errorType"] = type(error).__name__
        # Fixed diagnostic text only, never arbitrary subprocess/device output.
    finally:
        report["hostElapsedSeconds"] = round(clock()-started, 6)
        report["sampleCount"] = len(report["samples"])
    return report


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--label", required=True)
    parser.add_argument("--duration", type=float, default=180)
    parser.add_argument("--interval", type=float, default=2)
    parser.add_argument("--start-owned-avd", action="store_true", help="Only the current AVD owner may opt in")
    args = parser.parse_args()
    config = Config(args.label, args.duration, args.interval)
    config.validate()
    if not args.start_owned_avd:
        parser.error("No adb call made: explicit --start-owned-avd is required")
    path = ROOT / "tests/flutter/diagnostics/native-thread-samples" / (config.label+".json")
    path.parent.mkdir(parents=True, exist_ok=True)
    # Reserve before even validation adb reads. Refuse any existing label.
    with path.open("x", encoding="utf-8") as output:
        report = capture(config)
        json.dump(report, output, ensure_ascii=False, indent=2)
        output.write("\n")
    print(json.dumps({"path": path.relative_to(ROOT).as_posix(),
        "samples": report["sampleCount"], "stopReason": report["stopReason"]}))


if __name__ == "__main__":
    main()
