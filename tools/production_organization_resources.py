"""Read-only final organization resource evidence. Never query employee data."""
import base64
import argparse
import concurrent.futures
import datetime
import hashlib
import io
import json
import pathlib
import re
import time
import urllib.error
import urllib.request

ROOT = pathlib.Path(__file__).resolve().parents[1]
HRM = pathlib.Path('E:/PUTY_AI/kaoqinstyem/gosuxinwl')
PRIVATE = ROOT / '.private/production/acceptance'
HRM_WEB = HRM / '.cache/department-im-final-png-20261103'
IM_WEB = ROOT / 'clients/web-desktop/dist-web'
PLUGIN_SOURCES = [HRM / 'plugins/biz-suite/admin', HRM / '.cache/hrm-brand-puty3-png-release-20261103']
HRM_ORIGIN, IM_ORIGIN = 'https://192.168.10.11:9880', 'https://192.168.10.11:9885'
EXPECTED_HRM = '64397b487ed3a7f5ed510387a166873af238d740d8fbc4eec3df097be2fd3c16'
EXPECTED_IM = '75a25a4cadab9358b3f9f984dc7a18ebcc54b7caebfab43de435da1169f30cd6'
EXPECTED_AVATAR = '077ce3cbccbe09cef0eee11f760aaa801b93ea09ecb29cd89b958012a561ad78'
EXPECTED_PLUGIN = 'dba1dfb94a3f31b7dae712e027fac72f99cf60061741d9f0fe2aa2debcb8a2a6'


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *args):
        return None


def main():
    import production_hrm_acceptance as hrm
    from PIL import Image
    from production_backup import protect
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', default='organization-final-resources.json')
    args = parser.parse_args()
    if not re.fullmatch(r'organization-final-resources(?:-retry[0-9]+)?\.json', args.output):
        raise RuntimeError('Independent resource report filename required')
    output = PRIVATE / args.output
    if output.exists():
        raise RuntimeError('Preserve the existing final resource evidence')
    session = PRIVATE / 'hrm-session.json'
    session_before = sha(session.read_bytes())
    token = json.loads(session.read_text(encoding='utf-8'))['token']
    checks, assets = [], []
    def check(name, passed, **detail):
        checks.append({'name': name, 'passed': bool(passed), **detail})
    def get(origin, path, auth=False):
        if origin not in (HRM_ORIGIN, IM_ORIGIN) or not path.startswith('/') or path.startswith('//'):
            raise RuntimeError('Resource destination refused')
        headers = {'Cache-Control': 'no-cache', 'Pragma': 'no-cache'}
        if auth:
            if origin != HRM_ORIGIN or not path.startswith('/plugins/biz-suite/admin/'):
                raise RuntimeError('Authenticated resource scope refused')
            stamp = str(int(time.time()))
            verify = hashlib.md5((hrm.VERIFY + stamp).encode()).hexdigest()
            headers.update(apiverify=base64.b64encode((verify + '#' + stamp).encode()).decode(),
                           Authorization=hrm.crypt(token + '#' + stamp), locale='zh-CN')
            headers.update({'X-Requested-With': 'XMLHttpRequest', 'Accept': 'text/html,*/*',
                            'Referer': HRM_ORIGIN + '/suxinweb/'})
        opener = urllib.request.build_opener(urllib.request.ProxyHandler({}),
            urllib.request.HTTPSHandler(context=hrm.context), NoRedirect())
        try:
            with opener.open(urllib.request.Request(origin + path, headers=headers, method='GET'), timeout=30) as response:
                raw = response.read(16 << 20)
                return response.status, raw, response.headers.get('Content-Type', '')
        except urllib.error.HTTPError as error:
            return error.code, b'', ''
        except Exception as error:
            return 0, b'', type(error).__name__
    page_rows = []
    for name, origin, path, local, expected in [
            ('HRM', HRM_ORIGIN, '/suxinweb/', HRM_WEB, EXPECTED_HRM),
            ('IM', IM_ORIGIN, '/', IM_WEB, EXPECTED_IM)]:
        status, raw, _ = get(origin, path)
        expected_local = sha((local / 'index.html').read_bytes())
        check(name + '_index', status == 200 and sha(raw) == expected == expected_local,
              status=status, expectedSHA256=expected, actualSHA256=sha(raw), localSHA256=expected_local)
        html = raw.decode('utf-8', 'replace')
        icons = re.findall(r'<link[^>]*rel=["\']icon["\'][^>]*>', html)
        check(name + '_PNG_favicon_reference', bool(icons) and all('.png?' in icon or '.png"' in icon for icon in icons)
              and not any('.ico' in icon for icon in icons))
        page_rows.append((name, origin, path, local, html))
    # Go's FileServer redirects an explicit index.html to ./ before serving it.
    status, plugin_raw, plugin_type = get(HRM_ORIGIN, '/plugins/biz-suite/admin/', True)
    plugin_local = next((p for p in PLUGIN_SOURCES if sha((p / 'index.html').read_bytes()) == sha(plugin_raw)), None)
    check('plugin_authenticated_index_matches_source', status == 200 and plugin_local is not None,
          status=status, contentType=plugin_type, actualSHA256=sha(plugin_raw), localSource=str(plugin_local) if plugin_local else None)
    plugin_html = plugin_raw.decode('utf-8', 'replace')
    check('plugin_PNG_favicon_puty3', bool(re.search(r'rel=["\']icon["\'][^>]*\.png\?v=puty3', plugin_html)))
    if plugin_local:
        page_rows.append(('plugin', HRM_ORIGIN, '/plugins/biz-suite/admin/', plugin_local, plugin_html))
    # Only plugin catalog metadata; no account or employee API.
    status, reply = hrm.request('/admin/developer/packinstall/getInstallPack?catalog=1', method='GET')
    catalog = reply.get('data') if isinstance(reply, dict) else None
    plugin = [row for row in catalog if row.get('name') == 'biz-suite'] if isinstance(catalog, list) else []
    check('plugin_runtime_active', status == 200 and reply.get('code') == 0 and len(plugin) == 1
          and plugin[0].get('active') is True and plugin[0].get('installed') is True,
          packageIdentityProvidedByRoot=EXPECTED_PLUGIN, packageIdentityIndependentHTTPProof=False)
    requests = {}
    def add(label, origin, path, local, auth=False):
        if local.is_file():
            requests[(origin, path, auth)] = (label, local)
    for name, origin, base_path, local, html in page_rows:
        for path in re.findall(r'(?:src|href)=["\']([^"\']+)["\']', html):
            if path.startswith('//') or path.startswith('http') or path.startswith('data:'):
                continue
            plain = path.split('?', 1)[0]
            if name == 'HRM' and plain.startswith('/suxinweb/'):
                source = local / plain.removeprefix('/suxinweb/')
            elif name == 'IM' and plain.startswith('./'):
                source, path = local / plain[2:], '/' + path[2:]
            elif name == 'plugin' and plain.startswith('/plugins/biz-suite/admin/'):
                source = local / plain.removeprefix('/plugins/biz-suite/admin/')
            else:
                continue
            add(name + '_HTML_dependency', origin, path, source, name == 'plugin')
    brand_manifest = json.loads((HRM / '.cache/public-brand-puty3-20261103.manifest.json').read_text())
    brand_local = HRM / 'resource/static/brand'
    for relative in brand_manifest['files']:
        for origin, label in ((HRM_ORIGIN, 'HRM'), (IM_ORIGIN, 'IM')):
            add(label + '_public_brand', origin, '/resource/static/brand/' + relative + '?v=puty3', brand_local / relative)
        add('HRM_web_brand', HRM_ORIGIN, '/suxinweb/brand/' + relative + '?v=puty3', HRM_WEB / 'brand' / relative)
    for relative in ('logo.png', 'favicon.ico', 'favicon-16x16.png', 'favicon-32x32.png', 'apple-touch-icon.png',
                     'android-chrome-192x192.png', 'android-chrome-512x512.png'):
        add('HRM_web_root_image', HRM_ORIGIN, '/suxinweb/' + relative + '?v=puty3', HRM_WEB / relative)
        add('IM_web_root_image', IM_ORIGIN, '/' + relative + '?v=puty3', IM_WEB / relative)
    for origin, label in ((HRM_ORIGIN, 'HRM'), (IM_ORIGIN, 'IM')):
        add(label + '_default_avatar', origin, '/resource/static/brand/default-avatar.png?v=puty3', brand_local / 'default-avatar.png')
    communication_chunks = []
    for path in (HRM_WEB / 'assets').glob('*.js'):
        text = path.read_text(encoding='utf-8', errors='replace')
        if 'brand/im/logo.png?v=puty3' in text:
            communication_chunks.append(path)
            add('HRM_communication_PNG_chunk', HRM_ORIGIN, '/suxinweb/assets/' + path.name, path)
    check('communication_compiled_PNG_logo_puty3', bool(communication_chunks))
    communication_source = (HRM / 'web/src/views/communication/index.vue').read_text(encoding='utf-8')
    check('communication_source_PNG_logo_and_favicon', 'brand/im/logo.png?v=puty3' in communication_source
          and 'brand/im/favicon-32x32.png?v=puty3' in communication_source
          and not re.search(r'imLogo\s*=.*\.ico', communication_source))
    def compare(item):
        (origin, path, auth), (label, local) = item
        status, raw, content = get(origin, path, auth)
        actual, expected = sha(raw), sha(local.read_bytes())
        value = {'label': label, 'origin': origin, 'path': path, 'status': status,
                 'authenticated': auth, 'localSHA256': expected, 'actualSHA256': actual,
                 'passed': status == 200 and actual == expected}
        if 'default_avatar' in label:
            value['passed'] = value['passed'] and actual == EXPECTED_AVATAR
        return value
    with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
        assets = list(pool.map(compare, requests.items()))
    for row in assets:
        check(row['label'] + ':' + row['path'], row['passed'], status=row['status'])
    image_proofs = []
    for name in ('HR.png', 'IM.png', 'Suxin.png', 'OA.png', '小工具.png'):
        path = pathlib.Path('E:/suxinlogo') / name
        with Image.open(path) as image:
            alpha = image.getchannel('A').getextrema() if 'A' in image.getbands() else None
            image_proofs.append({'name': name, 'format': image.format, 'mode': image.mode,
                                'alphaExtrema': alpha, 'sha256': sha(path.read_bytes())})
            check('source_PNG_transparency:' + name, image.format == 'PNG' and alpha is not None and alpha[0] == 0)
    logo2 = pathlib.Path('E:/suxinlogo/LOGO2.png')
    check('LOGO2_original_unchanged', sha(logo2.read_bytes()) == 'e213b10a08a1b2f331c7040790b4435812b05de1d0db15f80600c0efed9ba205')
    check('local_default_avatar_unchanged', sha((brand_local / 'default-avatar.png').read_bytes()) == EXPECTED_AVATAR)
    check('shared_admin_session_file_unchanged', sha(session.read_bytes()) == session_before)
    report = {'capturedAtUTC': datetime.datetime.now(datetime.timezone.utc).isoformat(),
              'readOnly': True, 'employeeDataRequested': False, 'runtimeModified': False,
              'passed': all(v['passed'] for v in checks), 'checksPassed': sum(v['passed'] for v in checks),
              'checksTotal': len(checks), 'checks': checks, 'assets': assets, 'sourceImageTransparency': image_proofs,
              'pluginPackageIdentityProvidedByRoot': EXPECTED_PLUGIN,
              'pluginPackageIdentityHTTPBoundary': 'Catalog exposes active/installed/version; exact active.json identity is verified by root separately.'}
    output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    protect(output)
    print(json.dumps({'report': str(output), 'passed': report['passed'], 'checksPassed': report['checksPassed'],
                      'checksTotal': report['checksTotal'], 'resourceCount': len(assets),
                      'failedCheckNames': [v['name'] for v in checks if not v['passed']]}, ensure_ascii=False))


if __name__ == '__main__':
    main()
