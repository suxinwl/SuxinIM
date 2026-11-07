"""Small offline certificate and installer safety regression suite."""
import datetime as dt
import contextlib
import hashlib
import importlib.util
import io
import json
import pathlib
import tarfile
import tempfile
import types
import unittest
from unittest import mock

from cryptography import x509
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import rsa
from cryptography.x509.oid import NameOID, ExtendedKeyUsageOID

HERE = pathlib.Path(__file__).resolve().parent


def load(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


local = load('production_certificates', HERE.parents[1] / 'tools/production_certificates.py')
installer = load('certificates_install', HERE / 'certificates_install.py')
NOW = dt.datetime(2026, 11, 3, 0, 0, tzinfo=dt.timezone.utc)


def ca():
    key = rsa.generate_private_key(public_exponent=65537, key_size=2048)
    name = x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, 'offline fixture root')])
    root = (x509.CertificateBuilder().subject_name(name).issuer_name(name).public_key(key.public_key())
            .serial_number(x509.random_serial_number()).not_valid_before(NOW - dt.timedelta(days=1))
            .not_valid_after(NOW + dt.timedelta(days=3650))
            .add_extension(x509.BasicConstraints(ca=True, path_length=0), critical=True)
            .add_extension(x509.KeyUsage(digital_signature=False, content_commitment=False, key_encipherment=False,
                                       data_encipherment=False, key_agreement=False, key_cert_sign=True,
                                       crl_sign=True, encipher_only=None, decipher_only=None), critical=True)
            .add_extension(x509.SubjectKeyIdentifier.from_public_key(key.public_key()), critical=False)
            .sign(key, hashes.SHA256()))
    return root, key


def public_leaf(root, signer, hostname='hr.szputy.com', eku=ExtendedKeyUsageOID.SERVER_AUTH, expired=False):
    key = rsa.generate_private_key(public_exponent=65537, key_size=2048)
    name = x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, hostname)])
    cert = (x509.CertificateBuilder().subject_name(name).issuer_name(root.subject).public_key(key.public_key())
            .serial_number(x509.random_serial_number()).not_valid_before(NOW - dt.timedelta(days=2))
            .not_valid_after(NOW + dt.timedelta(days=-1 if expired else 90))
            .add_extension(x509.BasicConstraints(ca=False, path_length=None), critical=True)
            .add_extension(x509.SubjectAlternativeName([x509.DNSName(hostname)]), critical=False)
            .add_extension(x509.ExtendedKeyUsage([eku]), critical=False)
            .add_extension(x509.KeyUsage(digital_signature=True, content_commitment=False, key_encipherment=True,
                                       data_encipherment=False, key_agreement=False, key_cert_sign=False,
                                       crl_sign=False, encipher_only=None, decipher_only=None), critical=True)
            .add_extension(x509.AuthorityKeyIdentifier.from_issuer_public_key(signer.public_key()), critical=False)
            .sign(signer, hashes.SHA256()))
    return cert, key


class CertificateTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.root, cls.signer = ca()

    def test_root_pin_signature_and_validity(self):
        pem = self.root.public_bytes(serialization.Encoding.PEM)
        pin = self.root.fingerprint(hashes.SHA256()).hex()
        self.assertEqual(local.root_certificate(pem, NOW, pin), self.root)
        with self.assertRaisesRegex(RuntimeError, 'Pinned'):
            local.root_certificate(pem, NOW, '0' * 64)
        with self.assertRaisesRegex(RuntimeError, 'expired'):
            local.root_certificate(pem, NOW + dt.timedelta(days=4000), pin)

    def test_public_chain_san_server_auth_and_key_match(self):
        leaf, key = public_leaf(self.root, self.signer)
        actual = local.check_public([leaf], key, 'hr.szputy.com', NOW, [self.root])
        self.assertEqual(actual['verifiedChainLength'], 2)
        wrong_key = rsa.generate_private_key(public_exponent=65537, key_size=2048)
        with self.assertRaisesRegex(RuntimeError, 'key differ'):
            local.check_public([leaf], wrong_key, 'hr.szputy.com', NOW, [self.root])
        with self.assertRaisesRegex(RuntimeError, 'SAN'):
            local.check_public([leaf], key, 'im.szputy.com', NOW, [self.root])
        bad, bad_key = public_leaf(self.root, self.signer, eku=ExtendedKeyUsageOID.CLIENT_AUTH)
        with self.assertRaisesRegex(RuntimeError, 'ServerAuth'):
            local.check_public([bad], bad_key, 'hr.szputy.com', NOW, [self.root])
        expired, expired_key = public_leaf(self.root, self.signer, expired=True)
        with self.assertRaisesRegex(RuntimeError, 'expired'):
            local.check_public([expired], expired_key, 'hr.szputy.com', NOW, [self.root])

    def test_untrusted_public_issuer_fails(self):
        leaf, key = public_leaf(self.root, self.signer)
        other, _key = ca()
        with self.assertRaises(Exception):
            local.check_public([leaf], key, 'hr.szputy.com', NOW, [other])

    def test_one_year_lan_leaf_keeps_root_and_only_fixed_ip(self):
        # Real pinned CA has this exact IP name constraint; fixture adds it too.
        import ipaddress
        name = self.root.subject
        constrained = (x509.CertificateBuilder().subject_name(name).issuer_name(name)
                       .public_key(self.signer.public_key()).serial_number(x509.random_serial_number())
                       .not_valid_before(NOW - dt.timedelta(days=1)).not_valid_after(NOW + dt.timedelta(days=3650))
                       .add_extension(x509.BasicConstraints(ca=True, path_length=0), True)
                       .add_extension(x509.NameConstraints([x509.IPAddress(ipaddress.ip_network('192.168.10.11/32'))], None), True)
                       .sign(self.signer, hashes.SHA256()))
        before = constrained.fingerprint(hashes.SHA256())
        leaf, key = local.issue_lan(constrained, self.signer, NOW)
        self.assertEqual(leaf.not_valid_after_utc - NOW, dt.timedelta(days=365))
        self.assertEqual(local.check_lan([leaf, constrained], key, constrained, NOW)['verifiedChainLength'], 2)
        self.assertEqual(constrained.fingerprint(hashes.SHA256()), before)
        with self.assertRaisesRegex(RuntimeError, 'expired'):
            local.check_lan([leaf, constrained], key, constrained, NOW + dt.timedelta(days=366))
        local.check_lan([leaf, constrained], key, constrained, NOW + dt.timedelta(days=366), allow_expired=True)
        with self.assertRaisesRegex(RuntimeError, 'signing key'):
            local.issue_lan(constrained, rsa.generate_private_key(public_exponent=65537, key_size=2048), NOW)


class InstallerTests(unittest.TestCase):
    def test_staging_contains_only_leaf_keys_and_public_root(self):
        with tempfile.TemporaryDirectory() as temporary:
            base = pathlib.Path(temporary)
            files = {name: ('fixture-' + name).encode() for name in local.FILES}
            details = {'certificates': {}, 'pinnedRootSHA256': local.PINNED_ROOT_SHA256,
                       'lanRenewalNeeded': False, 'lanIssued': False, 'rootReplaced': False}
            output = io.StringIO()
            with mock.patch.object(local, 'ROOT', base), mock.patch.object(local, 'protect'), \
                 mock.patch.object(local, 'material', return_value=(files, details)), contextlib.redirect_stdout(output):
                local.prepare(types.SimpleNamespace(source=base, renew_lan_if_needed=True))
            result = json.loads(output.getvalue())
            with tarfile.open(result['bundle'], 'r:gz') as archive:
                self.assertEqual(set(archive.getnames()), set(local.FILES) | {'manifest.json'})
                manifest = json.load(archive.extractfile('manifest.json'))
                self.assertFalse(manifest['rootPrivateKeyIncluded'])
            self.assertFalse(result['serverModified'])
            requests = pathlib.Path(result['sshRequests']).read_text()
            self.assertNotIn('root-ca-key.pem', requests)
            self.assertNotIn('ca-unlock.json', requests)
            self.assertIn('certificates_install.py', requests)

    def test_bundle_path_escape_rejected_before_extraction(self):
        with tempfile.TemporaryDirectory() as temporary:
            base = pathlib.Path(temporary)
            folder = base / 'staging/certificates/fixture'
            folder.mkdir(parents=True)
            bundle = folder / 'certificates.tar.gz'
            with tarfile.open(bundle, 'w:gz') as archive:
                info = tarfile.TarInfo('../root-ca-key.pem')
                info.size = 6
                archive.addfile(info, io.BytesIO(b'secret'))
            target = base / 'unpacked'
            target.mkdir()
            with mock.patch.object(installer, 'BASE', base):
                with self.assertRaisesRegex(RuntimeError, 'Unexpected'):
                    installer.extract_bundle(bundle, hashlib.sha256(bundle.read_bytes()).hexdigest(), target)
            self.assertFalse((base / 'root-ca-key.pem').exists())

    def test_nginx_failure_restores_all_certificates_and_does_not_restart_livekit(self):
        with tempfile.TemporaryDirectory() as temporary:
            base = pathlib.Path(temporary)
            certs = base / 'openim/certs'
            certs.mkdir(parents=True)
            old = {name: ('old-' + name).encode() for name in installer.FILES}
            for name, value in old.items():
                (certs / name).write_bytes(value)
            commands, checks = [], [0]
            def command(args, **_kw):
                commands.append(args)
                if args == [installer.NGINX, '-t']:
                    checks[0] += 1
                    if checks[0] == 2:
                        raise RuntimeError('simulated invalid new certificate')
                return b''
            def extract(_bundle, _sha, stage):
                for name in installer.FILES:
                    (stage / name).write_bytes(('new-' + name).encode())
                return {'bundleID': 'fixture-bundle'}
            def copy(source, destination, _group):
                destination.write_bytes(source.read_bytes())
            with mock.patch.object(installer, 'BASE', base), mock.patch.object(installer, 'CERTS', certs), \
                 mock.patch.object(installer, 'guard', return_value=({'instanceID': 'fixture'}, 'fixture-livekit', True)), \
                 mock.patch.object(installer, 'grp', types.SimpleNamespace(getgrnam=lambda _: types.SimpleNamespace(gr_gid=1))), \
                 mock.patch.object(installer, 'fcntl', types.SimpleNamespace(flock=lambda *_: None, LOCK_EX=1, LOCK_NB=2)), \
                 mock.patch.object(installer, 'extract_bundle', side_effect=extract), mock.patch.object(installer, 'validate'), \
                 mock.patch.object(installer, 'run', side_effect=command), mock.patch.object(installer, 'install_file', side_effect=copy):
                with self.assertRaisesRegex(RuntimeError, 'previous certificates restored'):
                    installer.apply_bundle(base / 'fixture.tar.gz', 'a' * 64)
            self.assertEqual({name: (certs / name).read_bytes() for name in old}, old)
            self.assertFalse(any(args[:2] == ['docker', 'restart'] for args in commands))
            journal = json.loads((base / '.certificate-backups/fixture-bundle/journal.json').read_text())
            self.assertEqual(journal['state'], 'rolled-back')
            self.assertIn([installer.NGINX, '-s', 'reload'], commands)


if __name__ == '__main__':
    unittest.main()
