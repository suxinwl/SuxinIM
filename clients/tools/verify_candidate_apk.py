"""Check explicit candidate APK public assets/signature-independent SDK identity."""
import argparse
import hashlib
import io
import json
import pathlib
import ssl
import zipfile
from PIL import Image

ROOT = pathlib.Path(__file__).resolve().parents[2]
ROOT_DER_SHA = '836c3d6c28d5078cc8da6ebeff8e5f9a7fb7f140554e6a7834011d72299c3d69'
RELEASED = {
    'arm64-v8a': ('IM-3.8.15-android-arm64.apk', '2907650b937c5b5149815a72bff7416da9b3a321ff4bc1ab0785acafab97e570'),
    'x86_64': ('IM-3.8.15-android-x64-validation.apk', 'c17dacac8a21e228ebcaaac988a36bc22fe3acbebc164bff689f8b486ed1ce15'),
}
def sha(data): return hashlib.sha256(data).hexdigest()

def verify(apk):
    with zipfile.ZipFile(apk) as package, zipfile.ZipFile(ROOT / 'sdks/flutter/android/libs/puty-core-sdk-3.8.3-patch15-enterprise.aar') as core:
        names = package.namelist()
        abis = sorted({n.split('/')[1] for n in names if n.startswith('lib/') and n.endswith('.so')})
        if len(abis) != 1 or abis[0] not in ('arm64-v8a', 'x86_64'): raise ValueError('Unexpected candidate ABI')
        abi = abis[0]
        go = package.read(f'lib/{abi}/libgojni.so')
        # AGP strips/alignment-normalizes release JNI libraries. Compare with
        # the exact previously accepted APK, rather than unstripped AAR bytes.
        baseline_name, baseline_sha = RELEASED[abi]
        baseline = ROOT / 'clients/flutter/build/delivery' / baseline_name
        if sha(baseline.read_bytes()) != baseline_sha: raise ValueError('Unexpected accepted baseline')
        with zipfile.ZipFile(baseline) as old:
            if go != old.read(f'lib/{abi}/libgojni.so'): raise ValueError('Unexpected release Go core change')
        source_aar_sha = sha((ROOT / 'sdks/flutter/android/libs/puty-core-sdk-3.8.3-patch15-enterprise.aar').read_bytes())
        if source_aar_sha != '0adc8b600e27aa596cf608b7b5260d9005892f6677c54fe92a0ec6a9f47838cb': raise ValueError('Unexpected source AAR')
        logo = package.read('assets/flutter_assets/packages/openim_common/assets/images/ic_login_logo.png')
        if logo != (ROOT / 'clients/flutter/openim_common/assets/images/ic_login_logo.png').read_bytes(): raise ValueError('Unexpected brand PNG')
        image = Image.open(io.BytesIO(logo))
        if image.mode != 'RGBA' or image.getchannel('A').getextrema() != (0, 255): raise ValueError('Missing transparent brand source')
        pem = package.read('assets/flutter_assets/packages/openim_common/assets/certificates/puty_enterprise_root_ca.crt')
        if sha(ssl.PEM_cert_to_DER_cert(pem.decode())) != ROOT_DER_SHA: raise ValueError('Unexpected Dart root')
        native_roots = []
        for name in names:
            if name.startswith('res/') and package.getinfo(name).file_size < 10000:
                value = package.read(name)
                if value.startswith(b'-----BEGIN CERTIFICATE-----'):
                    try:
                        if sha(ssl.PEM_cert_to_DER_cert(value.decode())) == ROOT_DER_SHA: native_roots.append(name)
                    except (UnicodeDecodeError, ValueError): pass
        if not native_roots: raise ValueError('Missing native pinned root')
        dex = [package.read(name) for name in names if name.startswith('classes') and name.endswith('.dex')]
        if not any(b'puty/enterprise_lan_tls' in value for value in dex): raise ValueError('Missing native PKIX method channel')
        app = package.read(f'lib/{abi}/libapp.so')
        if b'puty/enterprise_lan_tls' not in app: raise ValueError('Missing Dart strict bridge')
        return {'sha256': sha(pathlib.Path(apk).read_bytes()), 'bytes': pathlib.Path(apk).stat().st_size, 'abi': abi, 'approvedSourceSHA256': sha((ROOT / 'clients/branding/IM.png').read_bytes()), 'transparentLoginPNG': True, 'logoSHA256':sha(logo), 'rootDER_SHA256': ROOT_DER_SHA, 'nativeRootCopies': len(native_roots), 'fixedGoCoreMatchesPreviousRelease': True, 'fixedGoCoreSHA256':sha(go), 'sourceAAR_SHA256':source_aar_sha, 'strictNativeChannelPresent':True, 'strictDartChannelPresent':True, 'libappSHA256':sha(app), 'scope':'Explicit ZIP assets and previously accepted release SDK bytes; signing is independently checked with apksigner; device/UI acceptance separate'}

if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('--apk', required=True)
    parser.add_argument('--output', required=True)
    args = parser.parse_args()
    report = verify(pathlib.Path(args.apk).resolve())
    pathlib.Path(args.output).write_text(json.dumps(report, indent=2)+'\n', encoding='utf-8')
    print(json.dumps(report))
