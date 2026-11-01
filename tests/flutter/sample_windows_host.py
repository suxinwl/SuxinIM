"""Read-only Windows CPU/RAM samples; no device control or process mutation."""
import argparse
import ctypes
import datetime
import json
import os
import pathlib
import time
from ctypes import wintypes


class MemoryStatus(ctypes.Structure):
    _fields_ = [('length', wintypes.DWORD), ('load', wintypes.DWORD),
                ('totalPhysical', ctypes.c_ulonglong), ('availablePhysical', ctypes.c_ulonglong),
                ('totalPageFile', ctypes.c_ulonglong), ('availablePageFile', ctypes.c_ulonglong),
                ('totalVirtual', ctypes.c_ulonglong), ('availableVirtual', ctypes.c_ulonglong),
                ('availableExtendedVirtual', ctypes.c_ulonglong)]


def ticks(value):
    return (value.dwHighDateTime << 32) | value.dwLowDateTime


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--pid', type=int, required=True)
    parser.add_argument('--seconds', type=int, default=300)
    parser.add_argument('--output', type=pathlib.Path, required=True)
    args = parser.parse_args()
    if os.name != 'nt' or not 1 <= args.pid or not 10 <= args.seconds <= 1800:
        parser.error('Windows, positive PID and duration 10..1800 seconds required')
    if args.output.exists():
        parser.error('Output already exists; preserve earlier evidence')
    kernel = ctypes.WinDLL('kernel32', use_last_error=True)
    kernel.OpenProcess.argtypes = [wintypes.DWORD, wintypes.BOOL, wintypes.DWORD]
    kernel.OpenProcess.restype = wintypes.HANDLE
    kernel.CloseHandle.argtypes = [wintypes.HANDLE]
    kernel.GetSystemTimes.argtypes = [ctypes.POINTER(wintypes.FILETIME)] * 3
    kernel.GetProcessTimes.argtypes = [wintypes.HANDLE] + [ctypes.POINTER(wintypes.FILETIME)] * 4
    kernel.GlobalMemoryStatusEx.argtypes = [ctypes.POINTER(MemoryStatus)]
    handle = kernel.OpenProcess(0x1000, False, args.pid)
    if not handle:
        raise ctypes.WinError(ctypes.get_last_error())
    started = time.monotonic()
    result = {'startedAt': datetime.datetime.now(datetime.timezone.utc).isoformat(),
              'scope': 'Host-wide Windows CPU and memory plus read-only CPU of one existing PID.',
              'pid': args.pid, 'logicalCPUs': os.cpu_count(), 'samples': [],
              'limits': ['Does not measure GPU, device JNI locks, guest CPU or per-thread scheduling.',
                         'Process CPU percentage uses one logical core = 100 percent.',
                         'Pressure correlation does not establish a media failure cause.']}
    previous = None
    try:
        while True:
            idle, kt, ut = (wintypes.FILETIME() for _ in range(3))
            creation, exit_time, pkt, put = (wintypes.FILETIME() for _ in range(4))
            memory = MemoryStatus()
            memory.length = ctypes.sizeof(memory)
            if not kernel.GetSystemTimes(ctypes.byref(idle), ctypes.byref(kt), ctypes.byref(ut)):
                raise ctypes.WinError(ctypes.get_last_error())
            if not kernel.GlobalMemoryStatusEx(ctypes.byref(memory)):
                raise ctypes.WinError(ctypes.get_last_error())
            process_ok = bool(kernel.GetProcessTimes(handle, ctypes.byref(creation), ctypes.byref(exit_time), ctypes.byref(pkt), ctypes.byref(put)))
            now = time.monotonic()
            total, unused = ticks(kt) + ticks(ut), ticks(idle)
            process = ticks(pkt) + ticks(put) if process_ok else None
            sample = {'utc': datetime.datetime.now(datetime.timezone.utc).isoformat(),
                      'elapsedSeconds': round(now - started, 3),
                      'availablePhysicalBytes': memory.availablePhysical,
                      'totalPhysicalBytes': memory.totalPhysical,
                      'memoryLoadPercent': memory.load,
                      'processCreatedFiletime': ticks(creation) if process_ok else None,
                      'processExited': ticks(exit_time) != 0 if process_ok else None,
                      'processQuerySucceeded': process_ok}
            if previous and total > previous[1]:
                sample['hostCPUPercent'] = round(100 * (1 - (unused - previous[2]) / (total - previous[1])), 3)
                if process is not None and previous[3] is not None:
                    sample['processCPUPercentOneCore'] = round((process - previous[3]) / 1e7 / (now - previous[0]) * 100, 3)
            result['samples'].append(sample)
            previous = (now, total, unused, process)
            if now - started >= args.seconds or sample['processExited']:
                break
            time.sleep(min(5, args.seconds - (now - started)))
    finally:
        kernel.CloseHandle(handle)
        result['completedAt'] = datetime.datetime.now(datetime.timezone.utc).isoformat()
        with args.output.open('x', encoding='utf-8') as stream:
            stream.write(json.dumps(result, indent=2) + '\n')
    print(json.dumps({'output': str(args.output), 'samples': len(result['samples'])}))


if __name__ == '__main__':
    main()
