import importlib.util
import io
import pathlib
import tarfile
import tempfile
import unittest
import zipfile

ROOT = pathlib.Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location("archive_audit", ROOT / "tools/audit_public_artifacts.py")
AUDIT = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(AUDIT)


class ArchivePrivacyTests(unittest.TestCase):
    def test_public_certificate_and_header_literal_are_allowed(self):
        with tempfile.TemporaryDirectory() as directory:
            path = pathlib.Path(directory) / "OpenIM38-Development-CA.zip"
            with zipfile.ZipFile(path, "w") as archive:
                archive.writestr("CA.crt", "-----BEGIN CERTIFICATE-----\n" + "A" * 80 + "\n-----END CERTIFICATE-----")
                archive.writestr("parser.js", "const header = '-----BEGIN PRIVATE KEY-----';")
            result = AUDIT.scan_archive(path)
            self.assertTrue(result["passed"])
            self.assertEqual(len(result["sha256"]), 64)
            self.assertEqual(result["memberFilesChecked"], 2)

    def test_private_filename_is_flagged_without_reading_contents(self):
        with tempfile.TemporaryDirectory() as directory:
            path = pathlib.Path(directory) / "server-binaries.tar.gz"
            with tarfile.open(path, "w:gz") as archive:
                info = tarfile.TarInfo("nested/.private/secrets.json")
                info.size = 2
                archive.addfile(info, io.BytesIO(b"{}"))
            result = AUDIT.scan_archive(path)
            self.assertFalse(result["passed"])
            self.assertEqual(result["memberFilesChecked"], 0)

    def test_private_key_body_and_escaped_key_are_detected(self):
        for separator in (b"\n", b"\\n"):
            body = b"-----BEGIN RSA PRIVATE KEY-----" + separator + b"A" * 90 + separator + b"-----END RSA PRIVATE KEY-----"
            self.assertTrue(AUDIT.contains_private_key(io.BytesIO(body)))

    def test_private_key_across_stream_chunk_boundary(self):
        body = b"x" * (1024 * 1024 - 10) + b"-----BEGIN OPENSSH PRIVATE KEY-----\n" + b"A" * 90 + b"\n-----END OPENSSH PRIVATE KEY-----"
        self.assertTrue(AUDIT.contains_private_key(io.BytesIO(body)))

    def test_unapproved_archive_name_is_rejected(self):
        with self.assertRaises(ValueError):
            AUDIT.scan_archive(pathlib.Path("unapproved.zip"))


if __name__ == "__main__":
    unittest.main()
