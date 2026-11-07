"""Prepare private download candidates; no publication or version API calls."""
import argparse
import hashlib
import html
import json
import os
import pathlib
import re
import stat
import tempfile

ROOT = pathlib.Path(__file__).resolve().parents[1]
ROOT_SHA = '836c3d6c28d5078cc8da6ebeff8e5f9a7fb7f140554e6a7834011d72299c3d69'
ANDROID_SIGNER = '521723f5ecfd7ab4a7cc94f2cad50509e0d3bbc1bd4a565ad44b58c1fc12272f'
RELEASE_NOTES = '安卓 3.8.21 修复 APP 内更新失败，完善安装包打开与失败重试。旧版首次请从下载页覆盖安装，并保留应用数据。包含 3.8.20 的后台返回恢复、相册和文件入口、图片显示及通话修复。后台消息及来电推送需开通厂商服务后启用。'

def require(ok, message):
    if not ok:
        raise RuntimeError(message)

def regular(path):
    require(path.is_file() and not path.is_symlink() and path.resolve() == path
            and stat.S_ISREG(path.stat().st_mode) and path.stat().st_nlink == 1,
            'Canonical regular file required')
    return path

def digest(path):
    with regular(path).open('rb') as source:
        return hashlib.file_digest(source, 'sha256').hexdigest()

def native_artifacts(manifest):
    require(isinstance(manifest, dict) and manifest.get('enterpriseRoot', {}).get('derSHA256') == ROOT_SHA,
            'Pinned enterprise root mismatch')
    artifacts = [entry for entry in manifest.get('artifacts', []) if entry.get('platform') in ('windows', 'android')]
    require(len(artifacts) == 2 and {entry['platform'] for entry in artifacts} == {'windows', 'android'},
            'Exactly one Windows and Android artifact required')
    artifacts.sort(key=lambda entry: entry['platform'] == 'android')
    for entry in artifacts:
        name = entry.get('fileName')
        extension = '.exe' if entry['platform'] == 'windows' else '.apk'
        require(isinstance(name, str) and re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9._-]{0,179}', name)
                and name.endswith(extension), 'Flat native package filename required')
        require(type(entry.get('size')) is int and 0 < entry['size'] < 300 << 20
                and isinstance(entry.get('sha256'), str) and re.fullmatch(r'[0-9a-f]{64}', entry['sha256'])
                and type(entry.get('build')) is int and entry['build'] > 0
                and isinstance(entry.get('version'), str) and re.fullmatch(r'\d+\.\d+\.\d+', entry['version']),
                'Native version, size or digest invalid')
        if entry['platform'] == 'android':
            require(entry.get('applicationID') == 'com.szputy.im' and entry.get('developmentCAIncluded') is False
                    and entry.get('signingCertificateSHA256') == ANDROID_SIGNER,
                    'Production Android identity or signing mismatch')
    return artifacts

def brand_name(root):
    brand = json.loads(regular(root / 'clients/brand.json').read_text(encoding='utf-8'))
    value = brand.get('displayName')
    require(isinstance(value, str) and 0 < len(value.strip()) <= 128
            and not any(ord(char) < 32 or ord(char) == 127 for char in value), 'Invalid client display name')
    return value.strip()

def atomic_write(path, raw):
    require(path.parent.resolve() == path.parent and not path.is_symlink(), 'Private output redirected')
    if path.exists():
        regular(path)
    descriptor, temporary = tempfile.mkstemp(prefix='.downloads-', dir=path.parent)
    try:
        with os.fdopen(descriptor, 'wb') as output:
            output.write(raw)
            output.flush()
            os.fsync(output.fileno())
        os.replace(temporary, path)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)

def prepare(root, manifest_path, out, expected_manifest=None, force_upgrade=False):
    require(type(force_upgrade) is bool, 'Upgrade requirement must be explicit boolean')
    require(root.resolve() == root and manifest_path.resolve() == manifest_path
            and out.resolve() == out and out.is_relative_to(root / '.private'),
            'Canonical input and private output required')
    manifest_raw = regular(manifest_path).read_bytes()
    manifest_sha = hashlib.sha256(manifest_raw).hexdigest()
    require(expected_manifest is None or manifest_sha == expected_manifest, 'Candidate manifest digest mismatch')
    manifest = json.loads(manifest_raw)
    artifacts = native_artifacts(manifest)
    name = brand_name(root)
    for entry in artifacts:
        folder = 'clients/web-desktop/release' if entry['platform'] == 'windows' else 'clients/flutter/build/delivery'
        package = regular(root / folder / entry['fileName'])
        require(package.stat().st_size == entry['size'] and digest(package) == entry['sha256'],
                'Final native artifact digest or size mismatch')
    cards, outputs = [], {}
    for entry in artifacts:
        label = 'Windows 64 位' if entry['platform'] == 'windows' else 'Android 64 位'
        detail = ('安装时自动配置内网 HTTPS 访问，请使用管理员安装。' if entry['platform'] == 'windows'
                  else '企业签名安装包，适用于 ARM64 Android 设备，应用内置信任配置。')
        cards.append(f'<article><h2>{label}</h2><p>{html.escape(name)} {html.escape(entry["version"])} · '
                     f'{entry["size"]/1024/1024:.1f} MB</p><p>{detail}</p><a class="button" '
                     f'href="{html.escape(entry["fileName"])}">下载安装包</a><details><summary>校验值</summary>'
                     f'<code>{entry["sha256"]}</code></details></article>')
        update = {key: entry[key] for key in ('platform', 'version', 'build', 'size', 'sha256')}
        update.update(downloadURL='https://im.szputy.com:9885/downloads/' + entry['fileName'], force=force_upgrade,
                      description='使用 HRM 统一账号登录，支持内网 IP 和公网域名访问。' + RELEASE_NOTES)
        outputs[entry['platform'] + '-version.json'] = (json.dumps(update, ensure_ascii=False, indent=2) + '\n').encode()
    title = html.escape(name + ' 客户端下载')
    page = '''<!doctype html><html lang="zh-CN"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>''' + title + '''</title><link rel="icon" type="image/png" href="/resource/static/brand/im/favicon-32x32.png?v=puty3"><style>body{margin:0;font:16px/1.7 system-ui,"Microsoft YaHei",sans-serif;color:#17304c;background:#f2f6fb}main{max-width:900px;margin:48px auto;padding:0 24px}header{display:flex;align-items:center;gap:20px}img{width:76px;height:76px;object-fit:contain}h1{font-size:30px;margin:0}h2{margin-top:0;font-size:23px}.cards{display:grid;grid-template-columns:repeat(auto-fit,minmax(280px,1fr));gap:22px;margin:30px 0}article{padding:28px;background:#fff;border:1px solid #dde6f1;border-radius:18px;box-shadow:0 6px 24px #254e7b08}.button{display:inline-block;padding:10px 24px;color:#fff;background:#086ee7;border-radius:8px;text-decoration:none;margin:8px 0}a{color:#0864c8}code{display:block;overflow-wrap:anywhere;font-size:12px}details{margin-top:16px;color:#52677f}footer{color:#52677f;font-size:14px}</style><main><header><img src="/resource/static/brand/im/logo.png?v=puty3" alt="''' + html.escape(name) + '''"><div><h1>''' + title + '''</h1><div>HRM 统一账号 · 内外网访问</div></div></header><p>''' + RELEASE_NOTES + '''</p><section class="cards">''' + ''.join(cards) + '''</section><p>内网：<a href="https://192.168.10.11:9880/suxinweb/">HRM 工作台</a> · <a href="https://192.168.10.11:9885/">IM 网页版</a><br>外网：<a href="https://hr.szputy.com:9880/suxinweb/">HRM 工作台</a> · <a href="https://im.szputy.com:9885/">IM 网页版</a></p><footer>请使用 HRM 账号登录。内网首次安装请由管理员分发安装包；Windows 安装后可访问内网浏览器入口。<br>Android 音视频仍需 ARM 设备实机验收；iOS 暂未提供安装包。<br><a href="artifacts-manifest.json">安装包清单</a> · <a href="sha256sums.txt">SHA256 校验清单</a></footer></main></html>'''
    outputs.update({'index.html': page.encode(), 'artifacts-manifest.json': (json.dumps(manifest, ensure_ascii=False, indent=2) + '\n').encode(),
                    'sha256sums.txt': ''.join(entry['sha256'] + '  ' + entry['fileName'] + '\n' for entry in artifacts).encode()})
    publish = {'files': [{'name': entry['fileName'], 'bytes': entry['size'], 'sha256': entry['sha256']} for entry in artifacts]}
    for filename in ('index.html', 'artifacts-manifest.json', 'sha256sums.txt'):
        publish['files'].append({'name': filename, 'bytes': len(outputs[filename]), 'sha256': hashlib.sha256(outputs[filename]).hexdigest()})
    outputs['publish.json'] = (json.dumps(publish, indent=2) + '\n').encode()
    require(manifest_path not in {out / filename for filename in outputs}, 'Output cannot overwrite the source manifest')
    out.mkdir(parents=True, exist_ok=True, mode=0o700)
    for filename, raw in outputs.items():
        atomic_write(out / filename, raw)
    require(digest(manifest_path) == manifest_sha, 'Source manifest changed')
    return {'downloadsPrepared': True, 'packagesVerified': 2, 'directory': str(out), 'displayName': name,
            'sourceManifestSHA256': manifest_sha, 'publishSHA256': hashlib.sha256(outputs['publish.json']).hexdigest(),
            'indexSHA256': hashlib.sha256(outputs['index.html']).hexdigest(),
            'artifactsSHA256': hashlib.sha256(outputs['artifacts-manifest.json']).hexdigest(),
            'sourceManifestUnchanged': True, 'forceUpgrade': force_upgrade, 'published': False}

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--manifest', type=pathlib.Path, default=ROOT / 'clients/artifacts-manifest.json')
    parser.add_argument('--out-dir', type=pathlib.Path, default=ROOT / '.private/production/downloads')
    parser.add_argument('--manifest-sha256')
    parser.add_argument('--force-upgrade', action='store_true', help='Prepare mandatory client update records; no API call')
    args = parser.parse_args()
    print(json.dumps(prepare(ROOT, args.manifest.absolute(), args.out_dir.absolute(), args.manifest_sha256, args.force_upgrade), ensure_ascii=False))

if __name__ == '__main__':
    main()
