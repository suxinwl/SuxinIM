#!/usr/bin/env python3
"""Validate production certificates and prepare private, pinned-root updates.

No SSH connection is made. The existing production_remote.py helper consumes
the generated requests when the operator chooses to install the bundle.
"""
import argparse
import csv
import datetime as dt
import hashlib
import io
import ipaddress
import json
import os
import pathlib
import secrets
import shlex
import shutil
import ssl
import subprocess
import tarfile
import warnings

from cryptography import x509
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import rsa, ec
from cryptography.x509.oid import ExtendedKeyUsageOID, NameOID
from cryptography.x509.verification import PolicyBuilder, Store

ROOT = pathlib.Path(__file__).resolve().parents[1]
SOURCE = pathlib.Path('E:/PUTY_AI/puty_ssl/state/production/certificates')
CA = ROOT / '.private/production-ca'
PUBLIC_CA = ROOT / 'deploy/certificates/Puty-Enterprise-Root-CA.crt'
PINNED_ROOT_SHA256 = '836c3d6c28d5078cc8da6ebeff8e5f9a7fb7f140554e6a7834011d72299c3d69'
IP = '192.168.10.11'
FORMAT = 'puty-production-certificates-v1'
FILES = ['hr.crt', 'hr.key', 'im.crt', 'im.key', 'lan.crt', 'lan.key', 'puty-root.crt']


def require(condition, message):
    if not condition:
        raise RuntimeError(message)


def canonical(value):
    return json.dumps(value, sort_keys=True, separators=(',', ':')).encode()


def sha(data):
    return hashlib.sha256(data).hexdigest()


def protect(path):
    if os.name == 'nt':
        result = subprocess.run(['whoami', '/user', '/fo', 'csv', '/nh'], capture_output=True, check=True)
        sid = next(csv.reader([result.stdout.decode().strip()]))[1]
        rights = '(OI)(CI)F' if path.is_dir() else 'F'
        subprocess.run(['icacls', str(path), '/inheritance:r', '/grant:r', '*' + sid + ':' + rights],
                       check=True, stdout=subprocess.DEVNULL, stderr=subprocess.PIPE)
    else:
        path.chmod(0o700 if path.is_dir() else 0o600)


def private_directory(path):
    path = path.absolute()
    require(path.resolve().is_relative_to((ROOT / '.private').resolve()), 'Certificate staging must stay inside this checkout .private')
    for ancestor in [path, *path.parents]:
        require(not ancestor.is_symlink(), 'Linked certificate staging path refused')
        if ancestor == ROOT:
            break
    path.mkdir(parents=True, exist_ok=False)
    protect(path)
    return path


def valid_now(cert, now):
    require(cert.not_valid_before_utc <= now < cert.not_valid_after_utc, 'Certificate not yet valid or expired')


def public_der(key):
    return key.public_bytes(serialization.Encoding.DER, serialization.PublicFormat.SubjectPublicKeyInfo)


def read_key(data, password=None):
    key = serialization.load_pem_private_key(data, password)
    require((isinstance(key, rsa.RSAPrivateKey) and key.key_size >= 2048)
            or (isinstance(key, ec.EllipticCurvePrivateKey) and key.key_size >= 256),
            'Unsupported or insufficient certificate private key')
    return key


def root_certificate(data, now, pin=PINNED_ROOT_SHA256):
    cert = x509.load_pem_x509_certificate(data)
    require(cert.fingerprint(hashes.SHA256()).hex() == pin, 'Pinned public root CA mismatch; root rotation is forbidden')
    valid_now(cert, now)
    cert.verify_directly_issued_by(cert)
    require(cert.extensions.get_extension_for_class(x509.BasicConstraints).value.ca,
            'Pinned issuer is not a CA')
    require(cert.extensions.get_extension_for_class(x509.KeyUsage).value.key_cert_sign,
            'Pinned issuer cannot sign certificates')
    return cert


def check_leaf(cert, key, now, allow_expired=False):
    if allow_expired:
        require(cert.not_valid_before_utc <= now, 'Existing leaf is not yet valid')
    else:
        valid_now(cert, now)
    require(public_der(cert.public_key()) == public_der(key.public_key()), 'Certificate and private key differ')
    require(not cert.extensions.get_extension_for_class(x509.BasicConstraints).value.ca, 'Server leaf must not be a CA')
    require(ExtendedKeyUsageOID.SERVER_AUTH in cert.extensions.get_extension_for_class(x509.ExtendedKeyUsage).value,
            'Leaf is missing ServerAuth EKU')
    require(cert.extensions.get_extension_for_class(x509.KeyUsage).value.digital_signature,
            'Leaf lacks digital-signature usage')


def system_roots():
    # Use the operator OS trust store, never treat downloaded intermediates as anchors.
    roots = []
    with warnings.catch_warnings():
        warnings.simplefilter('ignore')
        for data in ssl.create_default_context().get_ca_certs(binary_form=True):
            try:
                roots.append(x509.load_der_x509_certificate(data))
            except ValueError:
                continue
    require(roots, 'No operating-system public CA trust anchors are available')
    return roots


def check_public(chain, key, hostname, now, roots=None):
    require(chain, 'Missing public certificate chain')
    check_leaf(chain[0], key, now)
    for cert in chain:
        valid_now(cert, now)
    names = chain[0].extensions.get_extension_for_class(x509.SubjectAlternativeName).value.get_values_for_type(x509.DNSName)
    require(hostname in names, 'Required exact public DNS SAN missing')
    for index in range(len(chain) - 1):
        chain[index].verify_directly_issued_by(chain[index + 1])
    verified = PolicyBuilder().store(Store(roots or system_roots())).time(now).build_server_verifier(x509.DNSName(hostname)).verify(chain[0], chain[1:])
    for cert in verified:
        valid_now(cert, now)
    return summary(chain[0], len(verified))


def check_lan(chain, key, root, now, allow_expired=False):
    require(len(chain) == 2 and chain[1].fingerprint(hashes.SHA256()) == root.fingerprint(hashes.SHA256()),
            'LAN fullchain must contain its leaf and the unchanged pinned root')
    leaf = chain[0]
    check_leaf(leaf, key, now, allow_expired=allow_expired)
    require(public_der(key.public_key()) != public_der(root.public_key()), 'A root signing key cannot be used as a server leaf key')
    leaf.verify_directly_issued_by(root)
    san = leaf.extensions.get_extension_for_class(x509.SubjectAlternativeName).value
    require(list(san) == [x509.IPAddress(ipaddress.ip_address(IP))], 'LAN SAN must contain only the fixed server IP')
    constraints = root.extensions.get_extension_for_class(x509.NameConstraints).value
    allowed = constraints.permitted_subtrees or []
    require(any(isinstance(x, x509.IPAddress) and ipaddress.ip_address(IP) in x.value for x in allowed),
            'Pinned CA does not permit this LAN IP')
    require(leaf.not_valid_after_utc <= root.not_valid_after_utc, 'Leaf outlives its pinned issuer')
    return summary(leaf, 2)


def summary(cert, length):
    return {'certificateSHA256': cert.fingerprint(hashes.SHA256()).hex(),
            'notBefore': cert.not_valid_before_utc.isoformat(), 'notAfter': cert.not_valid_after_utc.isoformat(),
            'verifiedChainLength': length}


def issue_lan(root, root_key, now):
    require(public_der(root.public_key()) == public_der(root_key.public_key()), 'Local signing key does not match the pinned root')
    require(now + dt.timedelta(days=365) <= root.not_valid_after_utc, 'Pinned root cannot cover a new one-year leaf; root rotation is not permitted')
    key = rsa.generate_private_key(public_exponent=65537, key_size=3072)
    name = x509.Name([x509.NameAttribute(NameOID.ORGANIZATION_NAME, 'PUTY'), x509.NameAttribute(NameOID.COMMON_NAME, IP)])
    cert = (x509.CertificateBuilder().subject_name(name).issuer_name(root.subject).public_key(key.public_key())
            .serial_number(x509.random_serial_number()).not_valid_before(now - dt.timedelta(minutes=10))
            .not_valid_after(now + dt.timedelta(days=365))
            .add_extension(x509.BasicConstraints(ca=False, path_length=None), critical=True)
            .add_extension(x509.SubjectAlternativeName([x509.IPAddress(ipaddress.ip_address(IP))]), critical=False)
            .add_extension(x509.ExtendedKeyUsage([ExtendedKeyUsageOID.SERVER_AUTH]), critical=False)
            .add_extension(x509.KeyUsage(digital_signature=True, content_commitment=False, key_encipherment=True,
                                       data_encipherment=False, key_agreement=False, key_cert_sign=False,
                                       crl_sign=False, encipher_only=None, decipher_only=None), critical=True)
            .add_extension(x509.AuthorityKeyIdentifier.from_issuer_public_key(root_key.public_key()), critical=False)
            .sign(root_key, hashes.SHA256()))
    return cert, key


def material(source, now, renew=False):
    root_data = PUBLIC_CA.read_bytes()
    root = root_certificate(root_data, now)
    files, certificates = {'puty-root.crt': root_data}, {}
    for prefix in ('hr', 'im'):
        host = prefix + '.szputy.com'
        chain = x509.load_pem_x509_certificates((source / (host + '.crt')).read_bytes())
        issuer_path = source / (host + '.issuer.crt')
        if issuer_path.exists():
            known = {c.fingerprint(hashes.SHA256()) for c in chain}
            for cert in x509.load_pem_x509_certificates(issuer_path.read_bytes()):
                if cert.fingerprint(hashes.SHA256()) not in known:
                    chain.append(cert)
                    known.add(cert.fingerprint(hashes.SHA256()))
        key_data = (source / (host + '.key')).read_bytes()
        key = read_key(key_data)
        require(public_der(key.public_key()) != public_der(root.public_key()), 'Refusing to stage the pinned root signing key')
        certificates[prefix] = check_public(chain, key, host, now)
        files[prefix + '.crt'] = b''.join(c.public_bytes(serialization.Encoding.PEM) for c in chain)
        files[prefix + '.key'] = key_data
    lan_chain = x509.load_pem_x509_certificates((CA / 'lan-server.crt').read_bytes())
    lan_key_data = (CA / 'lan-server.key').read_bytes()
    check_lan(lan_chain, read_key(lan_key_data), root, now, allow_expired=renew)
    needed = lan_chain[0].not_valid_after_utc - now < dt.timedelta(days=30)
    issued = False
    if needed and renew:
        unlock = json.loads((CA / 'ca-unlock.json').read_text())['password'].encode()
        signer = read_key((CA / 'root-ca-key.pem').read_bytes(), unlock)
        leaf, key = issue_lan(root, signer, now)
        del signer, unlock
        lan_chain = [leaf, root]
        lan_key_data = key.private_bytes(serialization.Encoding.PEM, serialization.PrivateFormat.PKCS8, serialization.NoEncryption())
        issued = True
    files['lan.crt'] = b''.join(c.public_bytes(serialization.Encoding.PEM) for c in lan_chain)
    files['lan.key'] = lan_key_data
    certificates['lan'] = check_lan(lan_chain, read_key(lan_key_data), root, now)
    return files, {'certificates': certificates, 'pinnedRootSHA256': PINNED_ROOT_SHA256,
                   'lanRenewalNeeded': needed, 'lanIssued': issued, 'rootReplaced': False}


def prepare(args):
    now = dt.datetime.now(dt.timezone.utc)
    files, details = material(args.source, now, args.renew_lan_if_needed)
    identifier = now.strftime('%Y%m%dT%H%M%SZ') + '-' + secrets.token_hex(8)
    stage = private_directory(ROOT / '.private/certificates-staging' / identifier)
    for name, data in files.items():
        path = stage / name
        path.write_bytes(data)
        protect(path)
    manifest = {'format': FORMAT, 'bundleID': identifier, 'created': now.isoformat(), **details,
                'files': [{'name': n, 'bytes': len(files[n]), 'sha256': sha(files[n])} for n in FILES],
                'rootPrivateKeyIncluded': False}
    (stage / 'manifest.json').write_bytes(canonical(manifest) + b'\n')
    protect(stage / 'manifest.json')
    bundle = stage / 'certificates.tar.gz'
    with tarfile.open(bundle, 'w:gz') as archive:
        for name in FILES + ['manifest.json']:
            archive.add(stage / name, arcname=name, recursive=False)
    protect(bundle)
    bundle_sha = sha(bundle.read_bytes())
    installer = ROOT / 'deploy/production/certificates_install.py'
    remote = '/srv/puty-suite/staging/certificates/' + identifier
    guard = ('import json,pathlib,re\nb=pathlib.Path("/srv/puty-suite")\np=b/"openim"/"instance.json"\n'
             'if b.resolve()!=b or (b/"openim").resolve()!=b/"openim" or p.is_symlink(): raise RuntimeError("Wrong suite root")\n'
             'm=json.loads(p.read_text())\n'
             'if not (m.get("production") is True and m.get("testOnly") is False and m.get("root")==str(b/"openim") and m.get("stackName")=="puty-suite-prod" and re.fullmatch("[a-f0-9]{32}",m.get("instanceID",""))): raise RuntimeError("Wrong owner marker")\n'
             'd=pathlib.Path(' + repr(remote) + ')\n'
             'if any(x.is_symlink() for x in [d,*d.parents]): raise RuntimeError("Linked staging path")\n'
             'd.mkdir(parents=True,mode=0o700,exist_ok=False)\nd.chmod(0o700)')
    requests = [
        {'op': 'run', 'command': 'python3 -c ' + shlex.quote(guard), 'timeout': 30},
        {'op': 'put', 'local': str(bundle), 'remote': remote + '/certificates.tar.gz', 'mode': 0o600},
        {'op': 'put', 'local': str(installer), 'remote': remote + '/certificates_install.py', 'mode': 0o700},
        {'op': 'run', 'command': 'python3 ' + shlex.quote(remote + '/certificates_install.py') + ' --bundle ' + shlex.quote(remote + '/certificates.tar.gz') + ' --sha256 ' + bundle_sha, 'timeout': 600},
    ]
    request_file = stage / 'ssh-requests.jsonl'
    request_file.write_text(''.join(json.dumps(x) + '\n' for x in requests), encoding='utf-8')
    protect(request_file)
    (stage / 'bundle.sha256').write_text(bundle_sha + '  certificates.tar.gz\n')
    if details['lanIssued']:
        history = private_directory(CA / 'leaf-history' / identifier)
        for suffix, name in (('crt', 'lan.crt'), ('key', 'lan.key')):
            cached = CA / ('lan-server.' + suffix)
            shutil.copy2(cached, history / cached.name)
            protect(history / cached.name)
            temporary = CA / ('.lan-server.' + suffix + '.' + secrets.token_hex(4))
            temporary.write_bytes(files[name])
            protect(temporary)
            os.replace(temporary, cached)
    print(json.dumps({'staging': str(stage), 'bundle': str(bundle), 'bundleSHA256': bundle_sha,
                      'sshRequests': str(request_file), **details, 'serverModified': False}))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--source', type=pathlib.Path, default=SOURCE)
    commands = parser.add_subparsers(dest='command', required=True)
    commands.add_parser('validate')
    stage = commands.add_parser('prepare')
    stage.add_argument('--renew-lan-if-needed', action='store_true')
    args = parser.parse_args()
    if args.command == 'prepare':
        prepare(args)
    else:
        _files, details = material(args.source, dt.datetime.now(dt.timezone.utc))
        print(json.dumps({**details, 'validated': True, 'serverModified': False, 'privateMaterialWritten': False}))


if __name__ == '__main__':
    try:
        main()
    except Exception as error:
        raise SystemExit(type(error).__name__ + ': ' + str(error))
