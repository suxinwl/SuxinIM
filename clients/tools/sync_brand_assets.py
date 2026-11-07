"""Convert the approved transparent IM PNG into platform icon sizes.

This does no background removal or artwork editing. Import an already reviewed
source with --source; the canonical source thereafter stays in clients/branding.
"""
import argparse
import hashlib
import json
import pathlib
import shutil
from PIL import Image, ImageOps

CLIENTS = pathlib.Path(__file__).resolve().parents[1]

def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()

def synchronize(source=None):
    canonical = CLIENTS / 'branding/IM.png'
    selected = pathlib.Path(source).resolve() if source else canonical
    with Image.open(selected) as image:
        if image.format != 'PNG':
            raise ValueError('Approved logo must be a PNG')
        artwork = image.convert('RGBA')
    alpha = artwork.getchannel('A')
    corners = [artwork.getpixel(point)[3] for point in [(0, 0), (artwork.width-1, 0), (0, artwork.height-1), (artwork.width-1, artwork.height-1)]]
    # The reviewed artwork has one antialiased corner at alpha 1/255.
    # Preserve its pixels verbatim; reject opaque corner backgrounds.
    if alpha.getextrema()[0] != 0 or any(value > 1 for value in corners):
        raise ValueError('Approved logo must have transparent outer corners')
    canonical.parent.mkdir(parents=True, exist_ok=True)
    if selected != canonical:
        shutil.copyfile(selected, canonical)
    targets = {}
    def png(relative, size, opaque=False):
        target = CLIENTS / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        contained = ImageOps.contain(artwork, size, Image.Resampling.LANCZOS)
        canvas = Image.new('RGBA', size, (255, 255, 255, 0))
        canvas.alpha_composite(contained, ((size[0]-contained.width)//2, (size[1]-contained.height)//2))
        if opaque:
            background = Image.new('RGB', size, 'white')
            background.paste(canvas, mask=canvas.getchannel('A'))
            canvas = background
        canvas.save(target, format='PNG', optimize=True)
        targets[str(target.relative_to(CLIENTS))] = {'sha256': digest(target), 'size': list(size), 'opaqueForOS': opaque}
        return target
    for relative, size in {
        'web-desktop/src/assets/images/puty-im.png': 512,
        'web-desktop/src/assets/images/profile/logo.png': 256,
        'web-desktop/public/icons/icon.png': 512,
        'web-desktop/public/icons/tray.png': 32,
        'web-desktop/public/icons/tray@2x.png': 64,
        'web-desktop/public/favicon-16x16.png': 16,
        'web-desktop/public/favicon-32x32.png': 32,
        'web-desktop/public/apple-touch-icon.png': 180,
        'web-desktop/public/android-chrome-192x192.png': 192,
        'web-desktop/public/android-chrome-512x512.png': 512,
        'flutter/openim_common/assets/images/ic_login_logo.png': 512,
        'flutter/openim_common/assets/images/ic_splash_logo.png': 256,
    }.items():
        png(relative, (size, size))
    for density, size in [('mdpi', 48), ('hdpi', 72), ('xhdpi', 96), ('xxhdpi', 144), ('xxxhdpi', 192)]:
        png(f'flutter/android/app/src/main/res/mipmap-{density}/ic_launcher.png', (size, size))
    ios = CLIENTS / 'flutter/ios/Runner/Assets.xcassets/AppIcon.appiconset'
    entries = json.loads((ios / 'Contents.json').read_text(encoding='utf-8'))['images']
    for entry in entries:
        if 'filename' not in entry:
            continue
        width, height = [float(part) for part in entry['size'].split('x')]
        scale = float(entry['scale'].rstrip('x'))
        # Apple launcher icons must be opaque. The approved transparent source
        # is composited over white for this OS resource; in-app PNGs keep alpha.
        png(str((ios / entry['filename']).relative_to(CLIENTS)), (round(width*scale), round(height*scale)), opaque=True)
    for filename, size in [('LaunchImage.png', 96), ('LaunchImage@2x.png', 192), ('LaunchImage@3x.png', 288)]:
        png('flutter/ios/Runner/Assets.xcassets/LaunchImage.imageset/' + filename, (size, size))
    icon_image = Image.open(CLIENTS / 'web-desktop/public/icons/icon.png')
    for relative in ['web-desktop/public/favicon.ico', 'web-desktop/public/icons/icon.ico']:
        target = CLIENTS / relative
        icon_image.save(target, format='ICO', sizes=[(size, size) for size in [16, 24, 32, 48, 64, 128, 256]])
        targets[relative] = {'sha256': digest(target), 'format': 'ICO'}
    # Both references must move before removing the two superseded WebP logos.
    for relative in ['flutter/openim_common/lib/src/res/images.dart', 'flutter/lib/pages/login/login_view.dart']:
        target = CLIENTS / relative
        text = target.read_text(encoding='utf-8').replace('ic_login_logo.webp', 'ic_login_logo.png').replace('ic_splash_logo.webp', 'ic_splash_logo.png')
        target.write_text(text, encoding='utf-8', newline='\n')
    for name in ['ic_login_logo.webp', 'ic_splash_logo.webp']:
        old = CLIENTS / 'flutter/openim_common/assets/images' / name
        if old.exists():
            old.unlink()
    report = {'approvedPNG': str(canonical.relative_to(CLIENTS)), 'sourceSHA256': digest(canonical), 'sourceSize': [artwork.width, artwork.height], 'cornerAlpha': corners, 'outputs': targets, 'defaultAvatarChanged': False}
    output = CLIENTS / 'web-desktop/build/acceptance/brand-assets.json'
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(report, ensure_ascii=False, indent=2)+'\n', encoding='utf-8')
    return report

if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('--source', help='An approved already-transparent IM.png; never modifies this source')
    args = parser.parse_args()
    report = synchronize(args.source)
    print(json.dumps({'sourceSHA256': report['sourceSHA256'], 'PNGSourceTransparent': True, 'outputs': len(report['outputs']), 'defaultAvatarChanged': False}))
