"""Copy approved PNG sources and derive transparent PNG/ICO sizes, without redraw.

Default is a read-only plan. Supply --apply --plan-sha256 <reported SHA> to write
only the fixed branding targets. LOGO2 and default-avatar.png are never targets.
Requires Pillow; production runtime has no dependency on the source drive.
"""
import argparse
import hashlib
import io
import json
import os
from pathlib import Path
import stat
import tempfile

from PIL import Image, __version__ as PILLOW_VERSION

ROOT = Path(__file__).resolve().parents[1]
SOURCES = {'suxin': 'Suxin.png', 'hr': 'HR.png', 'im': 'IM.png', 'oa': 'OA.png', 'tools': '小工具.png'}
SIZES = {'favicon-16x16.png': 16, 'favicon-32x32.png': 32, 'apple-touch-icon.png': 180,
         'android-chrome-192x192.png': 192, 'android-chrome-512x512.png': 512}
ICO_SIZES = (16, 32, 48, 64, 128, 256)


def require(ok, message):
    if not ok:
        raise RuntimeError(message)


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def canonical(value):
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(',', ':')).encode()


def regular(path):
    require(path.is_file() and path.resolve() == path and not path.is_symlink()
            and stat.S_ISREG(path.stat().st_mode) and path.stat().st_nlink == 1,
            'regular_canonical_unlinked_file_required')
    return path


def alpha_image(raw):
    require(len(raw) <= 32 << 20, 'source_PNG_too_large')
    image = Image.open(io.BytesIO(raw))
    require(image.format == 'PNG' and image.width == image.height and 256 <= image.width <= 8192
            and getattr(image, 'n_frames', 1) == 1, 'single_square_PNG_required')
    image.load()
    rgba = image.convert('RGBA')
    require(rgba.getchannel('A').getextrema()[0] < 255, 'approved_transparent_PNG_required')
    return rgba


def derive(raw):
    image = alpha_image(raw)
    result = {'logo.png': raw}  # The main logo is the exact approved PNG, including alpha and metadata.
    for name, size in SIZES.items():
        out = io.BytesIO()
        image.resize((size, size), Image.Resampling.LANCZOS).save(out, 'PNG', compress_level=9)
        result[name] = out.getvalue()
    out = io.BytesIO()
    image.save(out, 'ICO', sizes=[(size, size) for size in ICO_SIZES])
    result['favicon.ico'] = out.getvalue()
    return result


def preserve_avatar(project):
    result = {}
    for name in ('resource/static/brand/default-avatar.png', 'web/public/default-avatar.png'):
        path = project / name
        if path.exists():
            result[name] = sha(regular(path).read_bytes())
    return result


def destination(project, name):
    path = project / name
    require(path.resolve() == path and path.is_relative_to(project) and not path.is_symlink(),
            'branding_destination_redirected')
    if path.exists():
        regular(path)
    return path


def create_plan(project, source):
    require(project.is_dir() and project.resolve() == project and source.is_dir() and source.resolve() == source,
            'canonical_project_and_source_directories_required')
    source_rows, outputs = {}, {}
    for brand, filename in SOURCES.items():
        path = regular(source / filename)
        raw = path.read_bytes()
        image = alpha_image(raw)
        source_rows[filename] = {'sha256': sha(raw), 'bytes': len(raw), 'width': image.width,
                                 'height': image.height, 'alphaExtrema': list(image.getchannel('A').getextrema())}
        outputs['devsource/branding/' + filename] = raw
        if brand not in ('suxin', 'hr', 'im'):
            continue
        directories = ['resource/static/brand/' + brand, 'web/public/brand/' + brand]
        if brand == 'suxin':
            directories.extend(['resource/static/brand', 'web/public'])
        for name, value in derive(raw).items():
            for directory in directories:
                outputs[directory + '/' + name] = value
    manifest = {'format': 'puty-brand-sources-v1', 'pillow': PILLOW_VERSION,
                'sources': source_rows, 'transformation': 'exact logo/source PNG copy; RGBA LANCZOS sizes; ICO container',
                'pngSizes': SIZES, 'icoSizes': list(ICO_SIZES), 'defaultAvatarChanged': False,
                'targets': {name: {'sha256': sha(raw), 'bytes': len(raw)} for name, raw in sorted(outputs.items())}}
    outputs['devsource/branding/manifest.json'] = canonical(manifest) + b'\n'
    for name in outputs:
        require('default-avatar' not in name and 'LOGO2' not in name, 'avatar_is_not_a_branding_target')
        destination(project, name)
    plan = {'format': 'puty-brand-generation-plan-v1', 'project': str(project), 'sourceDirectory': str(source),
            'sources': source_rows, 'pillow': PILLOW_VERSION, 'staticTargetCount': 56, 'sourceCopyCount': 5,
            'avatarPreserved': preserve_avatar(project),
            'outputs': {name: {'sha256': sha(raw), 'bytes': len(raw)} for name, raw in sorted(outputs.items())}}
    require(sum(name.startswith(('resource/static/brand/', 'web/public/')) for name in outputs) == 56,
            'unexpected_static_brand_target_count')
    return plan, outputs


def atomic_write(path, raw):
    path.parent.mkdir(parents=True, exist_ok=True)
    require(path.parent.resolve() == path.parent and not path.is_symlink(), 'branding_destination_redirected')
    descriptor, temporary = tempfile.mkstemp(prefix='.brand-', dir=path.parent)
    try:
        with os.fdopen(descriptor, 'wb') as handle:
            handle.write(raw)
            handle.flush()
            os.fsync(handle.fileno())
        os.replace(temporary, path)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


def apply(project, source, reviewed_sha):
    plan, outputs = create_plan(project, source)
    require(sha(canonical(plan)) == reviewed_sha, 'exact_reviewed_plan_SHA256_required')
    previous = {name: regular(project / name).read_bytes() if (project / name).exists() else None for name in outputs}
    changed = []
    try:
        for name, raw in outputs.items():
            path = destination(project, name)
            changed.append(name)
            atomic_write(path, raw)
            require(sha(regular(path).read_bytes()) == plan['outputs'][name]['sha256'], 'generated_asset_hash_mismatch')
        require(preserve_avatar(project) == plan['avatarPreserved'], 'default_avatar_changed')
        for filename, row in plan['sources'].items():
            require(sha(regular(source / filename).read_bytes()) == row['sha256'], 'approved_source_changed')
    except BaseException:
        for name in reversed(changed):
            path = destination(project, name)
            if previous[name] is None:
                if path.exists():
                    regular(path).unlink()
            else:
                atomic_write(path, previous[name])
        raise
    return {'generated': True, 'staticTargets': 56, 'sourceCopies': 5, 'reviewedPlanSHA256': reviewed_sha,
            'defaultAvatarUnchanged': True, 'sourcePNGsUnchanged': True,
            'manifest': str(project / 'devsource/branding/manifest.json')}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--project-root', type=Path, default=ROOT)
    parser.add_argument('--source-root', type=Path)
    parser.add_argument('--apply', action='store_true')
    parser.add_argument('--plan-sha256')
    args = parser.parse_args()
    project = args.project_root.absolute()
    source = args.source_root.absolute() if args.source_root else project / 'devsource/branding'
    if args.apply:
        print(json.dumps(apply(project, source, args.plan_sha256), ensure_ascii=False))
    else:
        plan, _ = create_plan(project, source)
        print(json.dumps({'readOnlyPlan': True, 'planSHA256': sha(canonical(plan)), **plan}, ensure_ascii=False))


if __name__ == '__main__':
    main()
