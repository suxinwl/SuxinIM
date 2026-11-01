"""Bind the separately intrusive recapture stack to exact APK/native evidence."""
from pathlib import Path
from datetime import datetime, timezone
import hashlib
import json
import re
import subprocess
import zipfile

ROOT = Path(__file__).resolve().parents[2]
SHORT_ROOT = Path('O:/')
OUT = ROOT / 'tests/flutter/native-detach-active-stack-review-108.json'
EVIDENCE = ROOT / 'tests/flutter/diagnostics/108/detach-native-symbols'
LLVM = Path('E:/AndroidDev/sdk/ndk/26.3.11579264/toolchains/llvm/prebuilt/windows-x86_64/bin')


def record(path):
    path = Path(path)
    if not path.is_absolute():
        path = ROOT / path
    raw = path.read_bytes()
    return {'path': path.relative_to(ROOT).as_posix(), 'sha256': hashlib.sha256(raw).hexdigest(), 'bytes': len(raw)}


def main():
    if OUT.exists():
        raise SystemExit('Refuse to overwrite prior report')
    trace = Path('tests/flutter/android-108-recapture-diagnostic-manual-sigquit-0.txt')
    text = (ROOT / trace).read_text(encoding='utf-8', errors='replace')
    selected = []
    for section in re.split(r'(?m)(?=^")', text):
        name = section.splitlines()[0] if section else ''
        if any(key in name.lower() for key in ['"main"', 'openimrtcmedia-', 'worker_thread -', 'signaling_threa', 'network_thread', 'main_texture_camera_thread', '"eglthread"']):
            selected.append({'thread': name, 'stack': section.rstrip()})
    lib = EVIDENCE / 'libjingle_peerconnection_so.so'
    commands = [
        ('readelf-sections-notes.txt', ['llvm-readelf.exe', '--sections', '--notes', str(lib)]),
        ('dynamic-symbols.txt', ['llvm-nm.exe', '--dynamic', '--defined-only', str(lib)]),
        ('symbolizer-selected-offsets.txt', ['llvm-symbolizer.exe', '--obj=' + str(lib), '0x51f6fa', '0xc26843', '0x51446b', '0x51456c', '0x51c200', '0x71e1ef', '0x71e468']),
        ('disassembly-selected-offsets.txt', ['llvm-objdump.exe', '-d', '--start-address=0xc267d0', '--stop-address=0xc268a0', str(lib)]),
    ]
    for name, args in commands:
        target = EVIDENCE / name
        if target.exists():
            raise SystemExit('Refuse to overwrite ' + str(target))
        args[0] = str(LLVM / args[0])
        result = subprocess.run(args, check=True, capture_output=True, text=True)
        target.write_text(result.stdout + result.stderr, encoding='utf-8')
    apk = SHORT_ROOT / 'clients/flutter/build/app/outputs/flutter-apk/app-development-release.apk'
    with zipfile.ZipFile(apk) as z:
        native_apk = z.read('lib/x86_64/libjingle_peerconnection_so.so')
    aar_native = lib.read_bytes()
    if native_apk != aar_native:
        raise SystemExit('AAR/native APK bytes differ')
    apk_sha = hashlib.file_digest(apk.open('rb'), 'sha256').hexdigest()
    diagnostic = json.loads((ROOT / 'tests/flutter/android-108-recapture-diagnostic-report.json').read_text(encoding='utf-8'))
    if apk_sha != diagnostic['apkSHA256']:
        raise SystemExit('APK differs from the running diagnostic APK report')
    embedding = next((SHORT_ROOT / '.cache/gradle/caches/modules-2/files-2.1/io.flutter/flutter_embedding_release/1.0.0-ef0cd000916d64fa0c5d09cc809fa7ad244a5767').rglob('*.jar'))
    embedding_raw = embedding.read_bytes()
    inputs = [record(p) for p in [trace,
        'tests/flutter/android-108-recapture-diagnostic-complete-logcat.txt',
        'tests/flutter/android-108-recapture-diagnostic-log-event-review.json',
        'tests/flutter/android-108-recapture-diagnostic-manual-diagnostic.json',
        'tests/flutter/android-108-recapture-diagnostic-report.json',
        'tests/flutter/native-detach-timing-review-108.json',
        'tests/flutter/android-108-recapture-host-pressure.json']]
    for item in json.loads((ROOT / 'tests/flutter/native-detach-timing-review-108.json').read_text(encoding='utf-8'))['sourceAndEvidence']:
        if item['path'].startswith('clients/'):
            if record(item['path']) != item:
                raise SystemExit('Frozen source changed: ' + item['path'])
            inputs.append(item)
    result = {
        'schema': 1, 'recordedAtUTC': datetime.now(timezone.utc).isoformat(),
        'version': '3.8.8+108 private candidate',
        'status': 'Both callers blocked inside nativeRemoveSink established; exact target-worker internal operation remains unsymbolized',
        'diagnosticOnly': True, 'normalLatencyAcceptance': False,
        'manualSignal': {'pid': 4997, 'signal': 'SIGQUIT', 'harnessUTC': '2026-11-01T07:28:09.888403+00:00',
            'dumpHeaderGuestTime': '2026-11-01 07:28:08', 'systemANR': False,
            'timingBoundary': 'Clock fields are recorded as supplied; the signal and dump itself perturb scheduling and are not a normal-call latency sample.'},
        'sourceAndEvidence': inputs,
        'findings': [
            'Both OpenIMRtcMedia workers have entered VideoTrack.nativeRemoveSink and are waiting in native pthread_cond_wait at the same library offsets 0x51f6fa / 0xc26843. This narrows the Java action from the earlier log-only review: the sampled wait is not nativeFreeSink, project queue admission, or post-detach output.release.',
            'The remote worker holds its own RtcOperationGate nativeCall lock while in JNI; the local worker is also in JNI without that gate. The remote gate being held is an effect of executing its owner action, not evidence that the local worker waits for that Java lock.',
            'WebRTC worker_thread TID5298 is sampled state R, clock_gettime on top of unsymbolized 0x51446b/0x51456c/0x51c200/0x71e1ef/0x71e468. The stack does not name a VideoBroadcaster lock or EGL callback. One running snapshot cannot distinguish an ordinary task-loop sampling point, busy repeated work, delayed scheduling, or the cause of the pending proxy requests.',
            'Both EglThread stacks are Looper.nativePollOnce; the camera handler also shows its Looper with an ART JNI/safepoint transition. No sampled Java stack shows SurfaceTextureRenderer.onFrame, ImageReaderSurfaceProducer.getSurface, or producer-setSize holding a frame lock. This does not prove those paths never blocked before or after the snapshot.',
            'The main stack is an Android input-event/Looper/ART condition transition during the manual dump, not VideoTrack.nativeRemoveSink. This diagnostic is not a fresh system input-ANR report.',
        ],
        'exactNativeArtifact': {
            'dependency': 'io.github.webrtc-sdk:android:144.7559.09',
            'apkPath': 'clients/flutter/build/app/outputs/flutter-apk/app-development-release.apk', 'apkSHA256': apk_sha,
            'apkEntry': 'lib/x86_64/libjingle_peerconnection_so.so', 'sameBytesAsDependencyAAR': True,
            'library': record(lib), 'gnuBuildID': '947d191334d21615',
            'symbolBoundary': 'ELF contains .dynsym but no .symtab or DWARF/debug sections. Exact local llvm-symbolizer returns unknown for the selected offsets. The nearest exported-symbol labels printed by objdump are not valid function names for stripped internal code.',
            'limitedDisassembly': 'At return PC0xc26843 the exact binary calls0x51f530 with0x7fffffffffffffff and then performs event cleanup, consistent with the primary proxy forever-wait mechanism. This does not recover the unexported target-worker function name or prove a particular lock cycle.',
            'officialReleaseAssets': {
                'url': 'https://github.com/webrtc-sdk/android/releases/expanded_assets/v144.7559.09',
                'checkedAtUTC': datetime.now(timezone.utc).isoformat(),
                'listed': ['libwebrtc.aar', 'libwebrtc_prefixed.aar', 'libwebrtc_prefixed_stripped.aar', 'Source code (zip)', 'Source code (tar.gz)'],
                'originalAARSHA256': '34cf91dd7497e5fe88adb76ba29ccae35db42dd6614ce548b79ce037b6d634d5',
                'boundary': 'No separate symbols/debug-symbols asset is listed here; this is not a claim that no symbols exist elsewhere. A prefixed/different build is not a valid address map for this binary.',
            },
        },
        'embeddingReadOnlyCheck': {
            'engineRevision': 'ef0cd000916d64fa0c5d09cc809fa7ad244a5767',
            'jarSHA256': hashlib.sha256(embedding_raw).hexdigest(),
            'jarBytes': len(embedding_raw),
            'inspection': 'javap of the actual cached release embedding JAR, not an assumed current upstream source',
            'findings': [
                'createSurfaceProducer selects ImageReaderSurfaceProducer for API>=29 unless debugForceSurfaceProducerGlTextures is enabled, otherwise SurfaceTextureSurfaceProducer.',
                'ImageReaderSurfaceProducer.setSize updates requested dimensions/createNewReader. getSurface calls getActiveReader under the producer lock, may create ImageReader.newInstance on API30, then calls ImageReader.getSurface. The inspected method bodies contain no synchronous post-and-wait hop to the main thread or checkIsOnMainThread call.',
                'SurfaceTextureSurfaceProducer.setSize directly calls setDefaultBufferSize; getSurface lazily constructs a Surface. Those inspected method bodies likewise contain no synchronous main-thread hop.',
                'These are method-body observations, not a general Android thread-safety guarantee. Native ImageReader/Surface internals and other producer methods can have their own constraints. The snapshot does not establish this candidate as the stalled path and does not justify a SurfaceTexture fallback change.',
            ],
        },
        'nextMinimumEvidence': [
            'Prefer matching vendor debug symbols for BuildID947d191334d21615. Do not map these offsets to another WebRTC build.',
            'If exact symbols are unavailable, a separately coordinated short own-process native sampling trace can show whether the common worker makes progress and identify repeated PC locations/queue starvation. Any such sampling must remain diagnostic, not the normal two-call acceptance run.',
            'No timeout increase, unknown-JNI retry, extra worker, or frozen product change is justified by these sampled addresses alone.',
        ],
        'proposedSingleVariableExperimentNotExecuted': {
            'first': 'With explicit coordination, terminate/relaunch only this own Android app process on the same already-running AVD, install nothing and retain the exact34f0... APK/account/CA/Web101/720p settings. Establish baseline idle admission, then one original-UI video call and hangup with the same safe logs; no SIGQUIT in the timing window. Compare action durations and prior stats STOP/quarantine state against the warm-process failed call.',
            'controlled': ['same APK bytes', 'same AVD boot/engine', 'same peer and original UI', 'same video settings', 'same caller budget', 'no native retry/change', 'no diagnostic signal during latency sample'],
            'interpretation': 'A cold-process improvement isolates accumulated per-process state as a candidate; it does not prove which state caused the stall or satisfy two150s normal-call acceptance.',
            'notCurrentlyEquivalent': 'Disabling LiveKit monitoring or lowering camera capture settings has no established original-UI/runtime-only switch in this frozen candidate. Do not silently edit product code or combine those changes with the cold-process experiment. They require a separately labelled single-variable diagnostic build or a demonstrated existing setting, if later authorized.',
        },
        'hostPressureBoundary': 'Parent captured61 read-only host samples over07:27:10-07:32:10. Around07:28:00/05/10/15 total hostCPU is28.77/22.83/20.31/21.01%, qemu uses roughly5.0-6.1core-equivalents on40logicalCPUs, freeRAM3.42-3.60GiB. This does not show sustained whole-host saturation, and cannot exclude a guest worker, GPU, emulator, or scheduling bottleneck. After-the-fact/per-window correlations are not a native lock diagnosis.',
        'threadEvidence': selected,
        'diagnosticArtifacts': [record(p) for p in sorted(EVIDENCE.iterdir()) if p.is_file()],
        'unchanged': ['frozen product/native/SDK', '12s budget', 'helpers', 'AVD', 'server'],
    }
    OUT.write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps(record(OUT)))


if __name__ == '__main__':
    main()
