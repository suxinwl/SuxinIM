"""Verify an explicit encrypted-store Android candidate without signing secrets.

This is a package/source identity check. It does not claim device acceptance.
"""
from __future__ import annotations

import argparse
import hashlib
import json
import os
import pathlib
import re
import ssl
import subprocess
import tempfile
import zipfile

ROOT = pathlib.Path(__file__).resolve().parents[2]
ROOT_SHA = "836c3d6c28d5078cc8da6ebeff8e5f9a7fb7f140554e6a7834011d72299c3d69"
SIGNER_SHA = "521723f5ecfd7ab4a7cc94f2cad50509e0d3bbc1bd4a565ad44b58c1fc12272f"
SQLCIPHER_AAR_SHA = "43fc10cae8f1f1059f0933e8942a1eb328989d13426378597cd4add1b34a34fd"
PRIVATE_ENTRY = re.compile(r"(^|/)(?:\.private|key\.properties)(?:/|$)|\.(?:key|jks|keystore|p8|p12|pfx)$", re.I)
PRIVATE_PEM = re.compile(
    rb"-----BEGIN ((?:[A-Z0-9]+ )*PRIVATE KEY(?: BLOCK)?)-----"
    rb"(?:[ \t\r\n]|\\[rn])+[A-Za-z0-9+/=][A-Za-z0-9+/= \t\r\n\\rn]{30,}?-----END \1-----"
)


def sha(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def run(command: list[str], env: dict[str, str]) -> str:
    completed = subprocess.run(command, capture_output=True, text=True, encoding="utf-8", env=env)
    if completed.returncode:
        # Tool diagnostics can contain paths; do not persist raw output.
        raise ValueError(f"Package verifier tool failed: {pathlib.Path(command[0]).name}")
    return completed.stdout


def verify(args: argparse.Namespace) -> dict:
    apk = pathlib.Path(args.apk).resolve(strict=True)
    aar = ROOT / "sdks/flutter/android/libs/puty-core-sdk-3.8.3-patch15-enterprise.aar"
    source_aar = aar.read_bytes()
    if sha(source_aar) != args.aar_sha256.lower():
        raise ValueError("Source AAR does not match the explicit frozen SHA")
    sqlcipher_aar = ROOT / "sdks/native-storage/sqlcipher-android-4.19.1/sqlcipher-android-4.19.1.aar"
    if sha(sqlcipher_aar.read_bytes()) != SQLCIPHER_AAR_SHA:
        raise ValueError("Unapproved SQLCipher AAR")
    env = os.environ.copy()
    env["JAVA_HOME"] = args.jdk
    sdk = pathlib.Path(args.sdk)
    tools = sdk / "build-tools/34.0.0"
    badging = run([str(tools / "aapt.exe"), "dump", "badging", str(apk)], env)
    package = re.search(r"^package: name='([^']+)' versionCode='(\d+)' versionName='([^']+)'", badging, re.M)
    if not package or package.groups() != ("com.szputy.im", str(args.build), args.version):
        raise ValueError("Unexpected application ID or candidate version")
    if "sdkVersion:'24'" not in badging or "targetSdkVersion:'35'" not in badging:
        raise ValueError("Unexpected minimum or target API")
    signature = run([str(tools / "apksigner.bat"), "verify", "--verbose", "--print-certs", str(apk)], env)
    signers = re.findall(r"Signer #(\d+) certificate SHA-256 digest: ([0-9a-fA-F]+)", signature)
    if signers != [("1", SIGNER_SHA)]:
        raise ValueError("Unexpected production signing certificate")
    if "Verified using v2 scheme (APK Signature Scheme v2): true" not in signature:
        raise ValueError("APK v2 signature did not verify")
    with zipfile.ZipFile(apk) as package_zip, zipfile.ZipFile(aar) as core, zipfile.ZipFile(sqlcipher_aar) as approved_sqlcipher:
        names = package_zip.namelist()
        if len(names) != len(set(names)):
            raise ValueError("Duplicate APK ZIP entry")
        for name in names:
            if PRIVATE_ENTRY.search(name.replace("\\", "/")):
                raise ValueError("Forbidden private material ZIP entry")
            if not package_zip.getinfo(name).is_dir() and PRIVATE_PEM.search(package_zip.read(name)):
                raise ValueError("Armored private key found in package bytes")
        abis = sorted({name.split('/')[1] for name in names if name.startswith('lib/') and name.endswith('.so')})
        if abis != [args.abi]:
            raise ValueError("Unexpected candidate ABI set")
        go_bytes = package_zip.read(f"lib/{args.abi}/libgojni.so")
        sqlcipher = package_zip.read(f"lib/{args.abi}/libsqlcipher.so")
        original_go = core.read(f"jni/{args.abi}/libgojni.so")
        original_sqlcipher = approved_sqlcipher.read(f"jni/{args.abi}/libsqlcipher.so")
        strip = sdk / "ndk/26.3.11579264/toolchains/llvm/prebuilt/windows-x86_64/bin/llvm-strip.exe"
        with tempfile.TemporaryDirectory(prefix="puty-apk-verify-", dir=args.temp) as temporary:
            stripped = pathlib.Path(temporary) / "libgojni.so"
            stripped.write_bytes(original_go)
            run([str(strip), "--strip-unneeded", str(stripped)], env)
            if stripped.read_bytes() != go_bytes:
                raise ValueError("Packaged Go JNI differs from the frozen stripped AAR")
            stripped_sqlcipher = pathlib.Path(temporary) / "libsqlcipher.so"
            stripped_sqlcipher.write_bytes(original_sqlcipher)
            run([str(strip), "--strip-unneeded", str(stripped_sqlcipher)], env)
            if stripped_sqlcipher.read_bytes() != sqlcipher:
                raise ValueError("Packaged SQLCipher differs from the approved stripped AAR")
        logo_path = "assets/flutter_assets/packages/openim_common/assets/images/ic_login_logo.png"
        logo = package_zip.read(logo_path)
        if logo != (ROOT / "clients/flutter/openim_common/assets/images/ic_login_logo.png").read_bytes():
            raise ValueError("Unexpected approved logo asset")
        pem_path = "assets/flutter_assets/packages/openim_common/assets/certificates/puty_enterprise_root_ca.crt"
        if sha(ssl.PEM_cert_to_DER_cert(package_zip.read(pem_path).decode("ascii"))) != ROOT_SHA:
            raise ValueError("Unexpected Dart enterprise root")
        native_roots = []
        for name in names:
            if name.startswith("res/") and package_zip.getinfo(name).file_size < 10000:
                value = package_zip.read(name)
                if value.startswith(b"-----BEGIN CERTIFICATE-----"):
                    if sha(ssl.PEM_cert_to_DER_cert(value.decode("ascii"))) == ROOT_SHA:
                        native_roots.append(name)
        if not native_roots:
            raise ValueError("Native pinned enterprise root is absent")
        dex = b"".join(package_zip.read(name) for name in names if re.fullmatch(r"classes\d*\.dex", name))
        app = package_zip.read(f"lib/{args.abi}/libapp.so")
        required = [b"puty/local_store", b"archive/previewClearScopes", b"archive/clearScopes", b"puty/enterprise_lan_tls"]
        if args.build >= 118:
            required.append(b"attachment/previewUri")
            if b"PUTY_AUTH_EVENT" in dex or b"PUTY_AUTH_EVENT" in app:
                raise ValueError("Private diagnostic marker present in the release package")
        if not all(marker in dex and marker in app for marker in required):
            raise ValueError("Native/Dart encrypted-store or TLS route marker is absent")
        return {
            "format": "PUTY-ANDROID-CANDIDATE-PACKAGE-1",
            "fileName": apk.name, "bytes": apk.stat().st_size,
            "sha256": sha(apk.read_bytes()), "version": args.version, "build": args.build,
            "applicationID": "com.szputy.im", "abi": args.abi,
            "minimumAPI": 24, "targetAPI": 35,
            "signatureV2Verified": True, "signingCertificateSHA256": SIGNER_SHA,
            "sourceAAR_SHA256": sha(source_aar), "packagedGoMatchesFrozenStrippedAAR": True,
            "packagedGoSHA256": sha(go_bytes), "sqlcipherSHA256": sha(sqlcipher),
            "approvedSQLCipherAAR_SHA256": SQLCIPHER_AAR_SHA, "packagedSQLCipherMatchesApprovedAAR": True,
            "embeddedEnterpriseRootDER_SHA256": ROOT_SHA, "nativeRootCopies": len(native_roots),
            "logoSHA256": sha(logo), "libappSHA256": sha(app),
            "encryptedStoreAndStrictTLSRoutesPresent": True,
            "privateKeyEntryNamesAbsent": True, "armoredPrivateKeysAbsentInAllEntries": True,
            "productionMainAccepted": False,
            "scope": "Explicit APK identity and frozen-core bytes only; resource audit and device acceptance are separate",
        }


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--apk", required=True)
    parser.add_argument("--abi", choices=["x86_64", "arm64-v8a"], required=True)
    parser.add_argument("--aar-sha256", required=True)
    parser.add_argument("--version", default="3.8.17")
    parser.add_argument("--build", type=int, default=117)
    parser.add_argument("--output", required=True)
    parser.add_argument("--sdk", default="E:/AndroidDev/sdk")
    parser.add_argument("--jdk", default="E:/AndroidDev/jdk/jdk-17.0.20.1+1")
    parser.add_argument("--temp", default="D:/PUTY-local-build/mobile-temp")
    arguments = parser.parse_args()
    result = verify(arguments)
    pathlib.Path(arguments.output).write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps(result, ensure_ascii=False))
