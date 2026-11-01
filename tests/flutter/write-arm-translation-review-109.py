"""Read-only binary/log review. Does not invoke adb, Gradle, or the application."""
from pathlib import Path
from datetime import datetime, timezone
import hashlib
import json
import re
import struct
import subprocess
import urllib.request
import zipfile

ROOT = Path(__file__).resolve().parents[2]
OUT = ROOT / "tests/flutter/diagnostics/109/arm-translation"
OUT.mkdir(parents=True, exist_ok=True)
NDK = Path("E:/AndroidDev/sdk/ndk/26.3.11579264/toolchains/llvm/prebuilt/windows-x86_64/bin")
APK = ROOT / ".private/android-validation/OpenIM38-3.8.9+109-main-arm64-candidate.apk"
TOMB = ROOT / "tests/flutter/android-109-arm-translation-tombstone_00.txt"
LOG = ROOT / "tests/flutter/android-109-clean-first-start-logcat.txt"


def sha(data):
    return hashlib.sha256(data).hexdigest()


def record(path):
    return {"path": path.relative_to(ROOT).as_posix(), "size": path.stat().st_size,
            "sha256": sha(path.read_bytes())}


def save(path, data):
    if isinstance(data, str):
        data = data.encode("utf-8")
    if path.exists():
        assert path.read_bytes() == data, f"Refusing to overwrite differing evidence: {path}"
    else:
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(data)
    return record(path)


def run(name, args):
    result = subprocess.run([str(v) for v in args], capture_output=True, check=False)
    assert result.returncode == 0, (args, result.returncode, result.stderr.decode(errors="replace"))
    return save(OUT / name, result.stdout + result.stderr), result.stdout.decode("utf-8", errors="replace")


def elf_info(data):
    assert data[:6] == b"\x7fELF\x02\x01"
    phoff = struct.unpack_from("<Q", data, 32)[0]
    shoff = struct.unpack_from("<Q", data, 40)[0]
    phsize, phnum, shsize, shnum, shstridx = struct.unpack_from("<5H", data, 54)
    ph = [struct.unpack_from("<IIQQQQQQ", data, phoff+i*phsize) for i in range(phnum)]
    sh = [struct.unpack_from("<IIQQQQIIQQ", data, shoff+i*shsize) for i in range(shnum)]
    string_header = sh[shstridx]
    strings = data[string_header[4]:string_header[4]+string_header[5]]
    sections = {}
    for h in sh:
        end = strings.index(b"\0", h[0])
        sections[strings[h[0]:end].decode()] = h
    return ph, sections


def file_to_va(program_headers, offset):
    for p in program_headers:
        if p[0] == 1 and p[2] <= offset < p[2]+p[5]:
            return p[3]+offset-p[2]
    raise AssertionError("File offset not covered by PT_LOAD")


apk_data = APK.read_bytes()
assert sha(apk_data) == "141ef706479719a644a6676d244ec85412e5c1fb09af3df16d3a6d30e06cd7c1"
build_report = ROOT / "tests/flutter/android-main-arm64-109.json"
payload_report = ROOT / "tests/flutter/android-main-arm64-109-payload-verification.json"
build = json.loads(build_report.read_text(encoding="utf-8-sig"))
payload = json.loads(payload_report.read_text(encoding="utf-8-sig"))
assert build["sha256"] == sha(apk_data) == payload["apkSHA256"]
tomb = TOMB.read_text(encoding="utf-8", errors="replace")
log = LOG.read_text(encoding="utf-8", errors="replace")
inputs = [record(p) for p in [APK, TOMB, LOG, build_report, payload_report,
    ROOT / "tests/flutter/android-109-clean-arm-startup-failure.json"]]
mapping = []
targets = [{"kind": "primary", "guestPC": 0x76fe1f5e1c50, "word": 0xd5380000},
           {"kind": "secondaryAfterPrimaryFatal", "guestPC": 0x76fe33cf3c64, "word": 0x5ee1b900}]
zip_entries = []
with zipfile.ZipFile(APK) as z:
    for entry in z.infolist():
        if not entry.filename.startswith("lib/"):
            continue
        nlen, xlen = struct.unpack_from("<HH", apk_data, entry.header_offset+26)
        start = entry.header_offset+30+nlen+xlen
        data = z.read(entry)
        expected = build["nativeLibraries"][entry.filename]
        assert sha(data) == expected["sha256"] and len(data) == expected["size"]
        zip_entries.append({"entry": entry.filename, "dataOffset": hex(start),
                            "size": len(data), "sha256": sha(data), "method": entry.compress_type})
        for target in targets:
            for line_no, line in enumerate(tomb.splitlines(), 1):
                m = re.search(r"([0-9a-fA-F']{8,})-([0-9a-fA-F']{8,})\s+([rwx-]{3})\s+([0-9a-fA-F]+)", line)
                if not m:
                    continue
                lo, hi = int(m[1].replace("'", ""), 16), int(m[2].replace("'", ""), 16)+1
                if not lo <= target["guestPC"] < hi:
                    continue
                apk_offset = int(m[4], 16)+target["guestPC"]-lo
                if not start <= apk_offset < start+entry.compress_size:
                    continue
                assert entry.compress_type == zipfile.ZIP_STORED
                elf_offset = apk_offset-start
                word = struct.unpack_from("<I", data, elf_offset)[0]
                assert word == target["word"]
                binary = OUT / Path(entry.filename).name
                save(binary, data)
                ph, sections = elf_info(data)
                va = file_to_va(ph, elf_offset)
                mapping.append({**target, "guestPC": hex(target["guestPC"]), "word": hex(word),
                    "mapLine": line_no, "map": line.strip(), "mapStart": hex(lo), "mapEndExclusive": hex(hi),
                    "apkFileOffset": hex(apk_offset), "zipDataOffset": hex(start), "entry": entry.filename,
                    "elfFileOffset": hex(elf_offset), "elfVirtualAddress": hex(va),
                    "littleEndianBytes": data[elf_offset:elf_offset+4].hex(), "binary": record(binary)})
assert len(zip_entries) == 13 and len(mapping) == 2
evidence = []
go_binary = OUT / "libgojni.so"
evidence.append(run("gojni-elf-headers.txt", [NDK/"llvm-readelf.exe", "-l", "-n", go_binary])[0])
evidence.append(run("gojni-disassembly.txt", [NDK/"llvm-objdump.exe", "-d", "--start-address=0x3c9a60",
    "--stop-address=0x3c9c60", go_binary])[0])
buildinfo_record, buildinfo = run("gojni-buildinfo.txt", [ROOT/"tools/go1.25.14/bin/go.exe", "version", "-m", go_binary])
evidence.append(buildinfo_record)
assert ": go1.25.6" in buildinfo
secondary = next(m for m in mapping if m["kind"] != "primary")
va = int(secondary["elfVirtualAddress"], 16)
evidence.append(run("flutter-secondary-disassembly.txt", [NDK/"llvm-objdump.exe", "-d",
    f"--start-address={hex(va-16)}", f"--stop-address={hex(va+20)}", OUT/"libflutter.so"])[0])

# Recover the exact Go function from this binary's own pclntab and its ELF relocation.
b = go_binary.read_bytes()
ph, sections = elf_info(b)
magic = bytes.fromhex("f1ffffff00000408")
pos = b.index(magic)
assert b.find(magic, pos+1) == -1
nfunc, nfile, disk_text, fn, cu, ft, pt, pcln = struct.unpack_from("<8Q", b, pos+8)
header_va = file_to_va(ph, pos)
rel = sections[".rela.dyn"]
text_relocations = []
for a in range(rel[4], rel[4]+rel[5], rel[9]):
    offset, info, addend = struct.unpack_from("<QQq", b, a)
    if offset == header_va+24:
        text_relocations.append((offset, info, addend))
assert len(text_relocations) == 1 and text_relocations[0][1] == 0x403
text_start = text_relocations[0][2]
primary = next(m for m in mapping if m["kind"] == "primary")
target_va = int(primary["elfVirtualAddress"], 16)
function = None
for i in range(nfunc):
    entry, func_offset = struct.unpack_from("<II", b, pos+pcln+i*8)
    next_entry = struct.unpack_from("<I", b, pos+pcln+(i+1)*8)[0]
    if text_start+entry <= target_va < text_start+next_entry:
        encoded_entry, nameoff = struct.unpack_from("<Ii", b, pos+pcln+func_offset)
        namepos = pos+fn+nameoff
        name = b[namepos:b.index(b"\0", namepos)].decode()
        function = {"name": name, "functionIndex": i, "entryOffset": hex(entry),
            "textStartRELATIVEAddend": hex(text_start), "textStartRelocationAddress": hex(header_va+24),
            "relocationType": "R_AARCH64_RELATIVE (1027)", "pclntabFileOffset": hex(pos),
            "functionVirtualAddress": hex(text_start+entry), "functionEndVirtualAddress": hex(text_start+next_entry),
            "nfunc": nfunc, "method": "Actual binary pclntab function table plus actual ELF RELA; no nearest-symbol guess."}
assert function and function["name"] == "internal/cpu.getMIDR"
assert function["functionVirtualAddress"] == primary["elfVirtualAddress"]
evidence.append(save(OUT/"gojni-pclntab-function.json", json.dumps(function, indent=2)+"\n"))

primary_sources = []
for filename in ["src/internal/cpu/cpu_arm64.s", "src/internal/cpu/cpu_arm64_hwcap.go", "LICENSE"]:
    url = f"https://raw.githubusercontent.com/golang/go/go1.25.6/{filename}"
    data = urllib.request.urlopen(url, timeout=30).read()
    archived = save(OUT/"primary-go1.25.6"/Path(filename).name, data)
    primary_sources.append({**archived, "url": url, "tag": "go1.25.6"})

# Only the binary artifact may live in .private; no credential fixtures are opened.
current_inputs = [record(ROOT/p["path"]) for p in inputs]
assert inputs == current_inputs
report = {
    "recordedAt": datetime.now(timezone.utc).isoformat(), "scope": "Independent read-only 109 ARM64 translated-emulator startup SIGILL analysis",
    "actions": {"adb": False, "AVD": False, "browserOrHelper": False, "server": False,
        "productSourceChanged": False, "buildOrToolchainChanged": False, "normalAppExecuted": False,
        "diagnosticCopiesAndToolOutputOnly": True, "credentialFilesRead": False},
    "inputs": inputs, "inputsPrePostIdentical": True,
    "apk": {"sha256": sha(apk_data), "entrypoint": "lib/main.dart", "version": "3.8.9+109",
        "nativeLibrariesIndependentlyHashed": zip_entries, "all13MatchBuildReport": True,
        "installedPackageIdentityBoundary": "APK SHA matches Flutter's public installation/startup record; no independent adb pull performed."},
    "runtime": {"PID": 7245, "TID": 7547, "thread": "pool-2-thread-1", "hostABI": "x86_64",
        "guestNativeABI": "arm64-v8a", "translator": "NDK translation aarch64 version 0.2.2",
        "translatorBuildID": "8d5bb5ddeee9364830ea3d7d46ef98b9", "fatalSignal": "SIGILL / SI_TKILL",
        "firstUndefinedInstructionAt": "2026-11-01T08:52:29.275Z", "decoderStack": ["DecodeSystemClass", "Decoder.Decode", "InterpretInsn"],
        "hostTopFrameBoundary": "Host tgkill frame is delivery of the translator's SIGILL, not the guest instruction's library."},
    "addressMapping": mapping, "exactGoFunction": function,
    "primaryInstruction": {"word": "0xd5380000", "decoded": "mrs x0, MIDR_EL1", "actualLibrary": "libgojni.so",
        "libraryBuildID": "60452166e31d020bd04bcf991e653134c7b597ae", "embeddedGoVersion": "go1.25.6",
        "origin": "Fixed io.openim:core-sdk:3.8.3-patch15 AAR per existing payload-verification report; its original bytes are not replaced.",
        "sourceMeaning": "Go's arm64 CPU initialization reads MIDR only when HWCAP_CPUID is advertised. This is CPU identification, before normal SDK use, not a messaging/media operation.",
        "HWCAPBoundary": "No guest auxv capture was performed. The source guard and exact executed function support an ABI/translator-compatibility inference, not a directly measured HWCAP bit."},
    "secondaryInstruction": {"at": "2026-11-01T08:52:30.238Z", "thread": 7280, "word": "0x5ee1b900",
        "library": "libflutter.so", "boundary": "Reported after primary fatal SIGILL; mapped and preserved separately, not substituted as primary cause or proof of independent normal-execution failure."},
    "conclusions": [
        "The first observed failure is the actual NDK translator rejecting the Go runtime MIDR_EL1 read inside the fixed OpenIM ARM64 library.",
        "It is not a WebRTC renderer/teardown frame, nor an HRM or group-read adapter failure.",
        "Linux exposes CPU identification registers through a kernel MRS-emulation ABI advertised by HWCAP_CPUID; this x86_64 native bridge does not successfully execute the observed instruction.",
        "This ARM64 candidate cannot pass startup in this observed translated AVD. Physical ARM64 startup, certificate trust, messaging, and media remain unverified; translation failure neither passes nor fails physical hardware."
    ],
    "nextMinimalValidation": "Keep the fixed candidate unchanged. Use a physical ARM64 device for ARM64 runtime acceptance; a same-source normal-main x86_64 package can separately exercise this existing x86_64 AVD without claiming ARM validation.",
    "notPerformed": ["No patched AAR or binary instruction bypass", "No GODEBUG or CPU-feature override", "No emulator/guest changes", "No ARM device validation", "No group entered or marked read"],
    "primarySources": primary_sources,
    "linuxABISource": {"url": "https://docs.kernel.org/arch/arm64/cpu-feature-registers.html",
        "supports": "HWCAP_CPUID advertises userspace register access; MRS is trapped/emulated, including MIDR_EL1. This is ABI context, not proof of the vendor translator implementation."},
    "diagnosticEvidence": evidence,
}
report_path = ROOT / "tests/flutter/android-arm-translation-independent-review-109.json"
saved = save(report_path, json.dumps(report, indent=2, ensure_ascii=False)+"\n")
print(json.dumps(saved))
