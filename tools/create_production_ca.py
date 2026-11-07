"""Create the PUTY private-IP CA once; publish only its public certificate."""
import datetime as dt
import hashlib
import ipaddress
import json
import pathlib
import secrets

from cryptography import x509
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import rsa
from cryptography.x509.oid import ExtendedKeyUsageOID, NameOID

ROOT = pathlib.Path(__file__).resolve().parents[1]
PRIVATE = ROOT / ".private/production-ca"
PUBLIC = ROOT / "deploy/certificates"


def main():
    PRIVATE.mkdir(parents=True, exist_ok=True)
    PUBLIC.mkdir(parents=True, exist_ok=True)
    root_file = PUBLIC / "Puty-Enterprise-Root-CA.crt"
    if root_file.exists():
        cert = x509.load_pem_x509_certificate(root_file.read_bytes())
        print(json.dumps({"existing": True, "publicCertificate": str(root_file),
                          "sha256": cert.fingerprint(hashes.SHA256()).hex()}))
        return
    if any(PRIVATE.iterdir()):
        raise RuntimeError("Private CA directory already contains material; refusing replacement")
    now = dt.datetime.now(dt.timezone.utc)
    root_key = rsa.generate_private_key(public_exponent=65537, key_size=3072)
    root_name = x509.Name([x509.NameAttribute(NameOID.ORGANIZATION_NAME, "PUTY"),
                          x509.NameAttribute(NameOID.COMMON_NAME, "PUTY Enterprise Internal Root CA 2026")])
    root_cert = (x509.CertificateBuilder().subject_name(root_name).issuer_name(root_name)
                 .public_key(root_key.public_key()).serial_number(x509.random_serial_number())
                 .not_valid_before(now - dt.timedelta(minutes=10)).not_valid_after(now + dt.timedelta(days=3650))
                 .add_extension(x509.BasicConstraints(ca=True, path_length=0), critical=True)
                 .add_extension(x509.KeyUsage(digital_signature=False, content_commitment=False,
                                             key_encipherment=False, data_encipherment=False, key_agreement=False,
                                             key_cert_sign=True, crl_sign=True, encipher_only=None, decipher_only=None), critical=True)
                 .add_extension(x509.NameConstraints(permitted_subtrees=[x509.IPAddress(ipaddress.ip_network("192.168.10.11/32"))],
                                                     excluded_subtrees=None), critical=True)
                 .add_extension(x509.SubjectKeyIdentifier.from_public_key(root_key.public_key()), critical=False)
                 .sign(root_key, hashes.SHA256()))
    unlock = secrets.token_urlsafe(48).encode()
    (PRIVATE / "ca-unlock.json").write_text(json.dumps({"password": unlock.decode()}), encoding="utf-8")
    (PRIVATE / "root-ca-key.pem").write_bytes(root_key.private_bytes(serialization.Encoding.PEM,
          serialization.PrivateFormat.PKCS8, serialization.BestAvailableEncryption(unlock)))
    root_file.write_bytes(root_cert.public_bytes(serialization.Encoding.PEM))
    leaf_key = rsa.generate_private_key(public_exponent=65537, key_size=2048)
    leaf_name = x509.Name([x509.NameAttribute(NameOID.ORGANIZATION_NAME, "PUTY"),
                          x509.NameAttribute(NameOID.COMMON_NAME, "192.168.10.11")])
    leaf = (x509.CertificateBuilder().subject_name(leaf_name).issuer_name(root_name)
            .public_key(leaf_key.public_key()).serial_number(x509.random_serial_number())
            .not_valid_before(now - dt.timedelta(minutes=10)).not_valid_after(now + dt.timedelta(days=365))
            .add_extension(x509.BasicConstraints(ca=False, path_length=None), critical=True)
            .add_extension(x509.SubjectAlternativeName([x509.IPAddress(ipaddress.ip_address("192.168.10.11"))]), critical=False)
            .add_extension(x509.ExtendedKeyUsage([ExtendedKeyUsageOID.SERVER_AUTH]), critical=False)
            .add_extension(x509.KeyUsage(digital_signature=True, content_commitment=False, key_encipherment=True,
                                       data_encipherment=False, key_agreement=False, key_cert_sign=False,
                                       crl_sign=False, encipher_only=None, decipher_only=None), critical=True)
            .add_extension(x509.AuthorityKeyIdentifier.from_issuer_public_key(root_key.public_key()), critical=False)
            .sign(root_key, hashes.SHA256()))
    (PRIVATE / "lan-server.crt").write_bytes(leaf.public_bytes(serialization.Encoding.PEM) + root_file.read_bytes())
    (PRIVATE / "lan-server.key").write_bytes(leaf_key.private_bytes(serialization.Encoding.PEM,
                    serialization.PrivateFormat.PKCS8, serialization.NoEncryption()))
    manifest = {"name": "PUTY Enterprise Internal Root CA 2026", "version": 1,
                "sha256": root_cert.fingerprint(hashes.SHA256()).hex(),
                "fileSHA256": hashlib.sha256(root_file.read_bytes()).hexdigest(),
                "allowedPrivateIP": "192.168.10.11", "notAfter": root_cert.not_valid_after_utc.isoformat()}
    (PUBLIC / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({"created": True, "publicCertificate": str(root_file), "sha256": manifest["sha256"], "secretsPrinted": False}))


if __name__ == "__main__":
    main()
