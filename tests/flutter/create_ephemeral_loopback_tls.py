"""Create disposable loopback-only TLS test material, never production identity.

Called by the independent Dart IO websocket tests. No private key is embedded in
source or printed. The caller owns a newly-created empty temporary directory.
"""
from __future__ import annotations

import argparse
from datetime import datetime, timedelta, timezone
import ipaddress
from pathlib import Path

from cryptography import x509
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import rsa
from cryptography.x509.oid import ExtendedKeyUsageOID, NameOID


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("directory", type=Path)
    args = parser.parse_args()
    target = args.directory.resolve(strict=True)
    if target.is_symlink() or not target.is_dir() or any(target.iterdir()):
        raise SystemExit("Requires a new, empty temporary directory")
    if not target.name.startswith("lk-io-106-"):
        raise SystemExit("Unexpected temporary-directory ownership prefix")
    if ".private" in target.parts:
        raise SystemExit("Private instance directories are not test inputs")
    key = rsa.generate_private_key(public_exponent=65537, key_size=2048)
    subject = x509.Name(
        [x509.NameAttribute(NameOID.COMMON_NAME, "Disposable loopback IO test")]
    )
    now = datetime.now(timezone.utc)
    certificate = (
        x509.CertificateBuilder()
        .subject_name(subject)
        .issuer_name(subject)
        .public_key(key.public_key())
        .serial_number(x509.random_serial_number())
        .not_valid_before(now - timedelta(minutes=1))
        .not_valid_after(now + timedelta(days=1))
        .add_extension(x509.BasicConstraints(ca=True, path_length=0), critical=True)
        .add_extension(
            x509.SubjectAlternativeName(
                [
                    x509.DNSName("localhost"),
                    x509.IPAddress(ipaddress.ip_address("127.0.0.1")),
                ]
            ),
            critical=False,
        )
        .add_extension(
            x509.ExtendedKeyUsage([ExtendedKeyUsageOID.SERVER_AUTH]), critical=False
        )
        .sign(key, hashes.SHA256())
    )
    (target / "loopback-certificate.pem").write_bytes(
        certificate.public_bytes(serialization.Encoding.PEM)
    )
    (target / "loopback-key.pem").write_bytes(
        key.private_bytes(
            serialization.Encoding.PEM,
            serialization.PrivateFormat.PKCS8,
            serialization.NoEncryption(),
        )
    )
    # The command deliberately has no stdout containing keys or certificates.


if __name__ == "__main__":
    main()
