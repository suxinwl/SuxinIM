"""Compare existing single-call screenshots; does not operate a device.

Inspect the call layout before using this ROI. A changed region proves sampled
pixel changes only; an unchanged region does not prove a frozen decoder.
"""
import argparse
import datetime
import hashlib
import json
import pathlib

from PIL import Image, ImageChops, ImageStat

ROOT = pathlib.Path(__file__).resolve().parents[2]
ROI = (40, 400, 330, 800)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--build', required=True, type=int)
    parser.add_argument('--call', required=True, type=int, choices=(1, 2))
    parser.add_argument('--normal-main', action='store_true',
                        help='Use actual normal-main x64 artifact metadata instead of the validation entrypoint')
    args = parser.parse_args()
    if not 106 <= args.build <= 9999:
        parser.error('Only the isolated upgraded test builds are supported')
    stem = f'android-{args.build}-call{args.call}'
    metadata_stem = 'android-main-x64' if args.normal_main else 'android-validation-x64'
    meta_path = ROOT / f'tests/flutter/{metadata_stem}-{args.build}.json'
    meta = json.loads(meta_path.read_text('utf-8'))
    if meta['applicationBuild'] != args.build:
        raise ValueError('Artifact metadata build differs')
    if args.normal_main and (meta.get('entrypoint') != 'lib/main.dart' or
                             meta.get('abis') != ['x86_64']):
        raise ValueError('Normal-main x64 artifact identity differs')
    frames, pairs, previous = [], [], None
    for second in (30, 60, 90, 120, 150):
        path = ROOT / f'tests/flutter/{stem}-{second:03}.png'
        raw = path.read_bytes()
        with Image.open(path) as source:
            image = source.convert('RGB')
        if image.size != (540, 1170):
            raise ValueError('Call layout dimensions require a new ROI review')
        crop = image.crop(ROI)
        frames.append({'nominalSecond': second,
                       'path': path.relative_to(ROOT).as_posix(),
                       'pngSHA256': hashlib.sha256(raw).hexdigest(),
                       'remoteROIPixelSHA256': hashlib.sha256(crop.tobytes()).hexdigest()})
        if previous is not None:
            diff = ImageChops.difference(previous[1], crop)
            pixels = diff.tobytes()
            changed = sum(max(pixels[i:i + 3]) > 8 for i in range(0, len(pixels), 3))
            total = crop.width * crop.height
            pairs.append({'fromSecond': previous[0], 'toSecond': second,
                          'changedPixelsThreshold8': changed, 'totalPixels': total,
                          'changedFraction': changed / total,
                          'rgbRMSDifference': ImageStat.Stat(diff).rms})
        previous = (second, crop)
    report = {
        'recordedAt': datetime.datetime.now(datetime.timezone.utc).isoformat(),
        'tool': 'tests/flutter/review_remote_pixels.py',
        'scope': 'Read-only analysis of existing screenshots; no device or source operation',
        'artifactMetadata': meta_path.relative_to(ROOT).as_posix(),
        'artifactSHA256': meta['sha256'], 'sourceSHA256': meta['sourceSHA256'],
        'remoteOnlyROI': list(ROI),
        'roiBasis': 'Intended fullscreen Web synthetic camera area, excluding top-right local preview, status bar, timer and controls; requires manual layout inspection',
        'frames': frames, 'comparisons': pairs,
        'allConsecutiveIntervalsHaveRemotePixelChanges': all(p['changedFraction'] > .01 for p in pairs),
        'limits': ['Sampled pixel changes do not measure FPS, latency or uninterrupted playback.',
                   'No conclusion about physical cameras, audio, screen sharing or native stats counters.'],
    }
    output = ROOT / f'tests/flutter/{stem}-remote-pixel-review.json'
    with output.open('x', encoding='utf-8') as stream:
        stream.write(json.dumps(report, indent=2) + '\n')
    print(json.dumps({'report': output.relative_to(ROOT).as_posix(),
                      'allIntervalsChanged': report['allConsecutiveIntervalsHaveRemotePixelChanges'],
                      'changedFractions': [p['changedFraction'] for p in pairs]}))


if __name__ == '__main__':
    main()
