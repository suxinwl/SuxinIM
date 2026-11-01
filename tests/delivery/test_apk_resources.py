import hashlib
import importlib.util
import json
import pathlib
import stat
import tempfile
import unittest
import zipfile

ROOT = pathlib.Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location("apk_audit", ROOT / "tools/audit_apk_resources.py")
AUDIT = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(AUDIT)


class ApkResourceAuditTests(unittest.TestCase):
    def apk(self, directory, entries):
        path = pathlib.Path(directory) / "synthetic.apk"
        with zipfile.ZipFile(path, "w") as archive:
            for name, value in entries:
                archive.writestr(name, value)
        return path, hashlib.sha256(path.read_bytes()).hexdigest()

    def test_code_labels_public_cert_and_channel_name_are_not_credentials(self):
        with tempfile.TemporaryDirectory() as directory:
            path, digest = self.apk(directory, [
                ("assets/labels.json", json.dumps({"token": "Access token", "password": "Password", "type": "string"})),
                ("assets/sdk.js", "const token = 'token'; const header='-----BEGIN PRIVATE KEY-----'; const channel='openim38/native_probe';"),
                ("assets/CA.crt", "-----BEGIN CERTIFICATE-----\n" + "A" * 80 + "\n-----END CERTIFICATE-----"),
                ("classes.dex", b"fake binary token fixture excluded"),
                ("AndroidManifest.xml", b"\x03\x00\x08\x00binary"),
            ])
            report = AUDIT.scan_apk(path, digest)
            self.assertTrue(report["zipAndReadableResourcesPassed"])
            self.assertEqual(report["readableTextResourcesChecked"], 3)
            self.assertEqual(report["binaryXmlOrNonUtf8ResourcesSkipped"], 1)
            self.assertEqual(report["compiledOrNonTextEntriesSkipped"], 1)

    def test_private_paths_and_probe_fixtures_flagged_without_reading(self):
        with tempfile.TemporaryDirectory() as directory:
            path, digest = self.apk(directory, [("assets/.private/key.json", b"not text"),
                ("assets/test-accounts.json", b"not JSON"), ("assets/validation/native_probe.dart", b"not text")])
            report = AUDIT.scan_apk(path, digest)
            self.assertFalse(report["zipAndReadableResourcesPassed"])
            self.assertEqual(report["readableTextResourcesChecked"], 0)
            self.assertEqual(len(report["privateMaterialFlags"]), 2)
            self.assertEqual(report["validationOrProbeFixtureEntryNames"], ["assets/validation/native_probe.dart"])

    def test_credential_shaped_json_values_flagged_but_never_reported(self):
        with tempfile.TemporaryDirectory() as directory:
            password = "SyntheticTestOnly123"
            token = "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJ1MSJ9.c2lnbmF0dXJl"
            path, digest = self.apk(directory, [("assets/config.json", json.dumps({
                "accounts": [{"userID": "synthetic-u1", "password": password, "imToken": token}],
                "schema": {"token": {"type": "string"}},
            }))])
            report = AUDIT.scan_apk(path, digest)
            self.assertEqual(len(report["privateMaterialFlags"]), 2)
            self.assertNotIn(password, json.dumps(report))
            self.assertNotIn(token, json.dumps(report))

    def test_actual_pem_body_and_escaped_key_are_flagged(self):
        for separator in ("\n", "\\n"):
            with self.subTest(separator=separator), tempfile.TemporaryDirectory() as directory:
                body = "-----BEGIN PRIVATE KEY-----" + separator + "A" * 90 + separator + "-----END PRIVATE KEY-----"
                path, digest = self.apk(directory, [("assets/config.txt", body)])
                report = AUDIT.scan_apk(path, digest)
                self.assertEqual(report["privateMaterialFlags"][0]["rule"], "armored private key body")

    def test_hash_mismatch_does_not_read_resources(self):
        with tempfile.TemporaryDirectory() as directory:
            path, _ = self.apk(directory, [("assets/labels.json", "{}")])
            report = AUDIT.scan_apk(path, "0" * 64)
            self.assertFalse(report["sha256Matches"])
            self.assertEqual(report["zipEntries"], 0)
            self.assertEqual(report["readableTextResourcesChecked"], 0)

    def test_unsafe_entries_and_symlink_entries_are_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            link = zipfile.ZipInfo("assets/link.txt")
            link.create_system = 3
            link.external_attr = (stat.S_IFLNK | 0o777) << 16
            path, digest = self.apk(directory, [("../outside.json", "{}"), (link, "target")])
            report = AUDIT.scan_apk(path, digest)
            self.assertEqual(len(report["privateMaterialFlags"]), 2)
            self.assertEqual(report["readableTextResourcesChecked"], 0)

    def test_private_input_path_is_refused(self):
        with tempfile.TemporaryDirectory() as directory:
            private = pathlib.Path(directory) / ".private"
            private.mkdir()
            path, digest = self.apk(private, [("assets/config.json", "{}")])
            with self.assertRaisesRegex(ValueError, "Private directory"):
                AUDIT.scan_apk(path, digest)


if __name__ == "__main__":
    unittest.main()
