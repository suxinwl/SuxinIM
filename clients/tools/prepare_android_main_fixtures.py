"""Prepare nonce-isolated synthetic inputs. Does not contact device/server."""
import hashlib
import json
import pathlib
import secrets
import struct
import zipfile
import zlib

ROOT = pathlib.Path(__file__).resolve().parents[2]
BASE = ROOT / ".private/production/acceptance/android-local-store-117"


def png_chunk(kind, value):
    return struct.pack(">I", len(value)) + kind + value + struct.pack(">I", zlib.crc32(kind + value))


def prepare():
    directory = BASE / "fixtures"
    directory.mkdir(parents=True, exist_ok=True)
    manifest_path = BASE / "fixture-plan.json"
    if manifest_path.exists():
        # Never replace a fixture already bound to a specific upload/review.
        return json.loads(manifest_path.read_text(encoding="utf-8"))
    nonce = secrets.token_hex(12)
    marker = f"PUTY-local-store-117-synthetic-{nonce}"
    text = ((marker + "\n").encode() * 100)[:4096].ljust(4096, b" ")
    upload_name = f"puty-android-main-{nonce}.txt"
    (directory / upload_name).write_bytes(text)
    image = b"\x89PNG\r\n\x1a\n" + png_chunk(b"IHDR", struct.pack(">IIBBBBB", 64, 64, 8, 6, 0, 0, 0))
    image += png_chunk(b"tEXt", b"synthetic\0" + marker.encode())
    image += png_chunk(b"IDAT", zlib.compress((b"\0" + bytes([0, 160, 230, 160]) * 64) * 64)) + png_chunk(b"IEND", b"")
    (directory / "透明测试.png").write_bytes(image)
    xlsx = directory / "职员名单.xlsx"
    with zipfile.ZipFile(xlsx, "w", zipfile.ZIP_DEFLATED) as archive:
        archive.writestr("[Content_Types].xml", '<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/></Types>')
        archive.writestr("_rels/.rels", '<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>')
        archive.writestr("xl/workbook.xml", '<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Synthetic" sheetId="1" r:id="rId1"/></sheets></workbook>')
        archive.writestr("xl/_rels/workbook.xml.rels", '<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/></Relationships>')
        archive.writestr("xl/worksheets/sheet1.xml", f'<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>{marker}</t></is></c></row></sheetData></worksheet>')
    # Previously generated, independently verified synthetic H264 input; never
    # media from an employee or device camera. It is not uploaded in this plan.
    source_video = ROOT / ".private/production/acceptance/attachment-20261103/fixtures/test-video.mp4"
    source_manifest = json.loads((source_video.parent / "manifest.json").read_text(encoding="utf-8-sig"))
    records = source_manifest.get("inputs", source_manifest.get("files", []))
    expected = next(item for item in records if item.get("name", item.get("fileName")) == "test-video.mp4")
    video_bytes = source_video.read_bytes()
    if hashlib.sha256(video_bytes).hexdigest() != expected["sha256"]:
        raise ValueError("Synthetic source video does not match its frozen manifest")
    (directory / "本机预览.mp4").write_bytes(video_bytes)
    files = [{"name": path.name, "bytes": path.stat().st_size, "sha256": hashlib.sha256(path.read_bytes()).hexdigest(),
              "uploadAllowedOnlyAfterIUserIDBound": path.name == upload_name,
              "mustNotUpload": path.name != upload_name} for path in sorted(directory.iterdir()) if path.is_file()]
    result = {"format": "PUTY-ANDROID-MAIN-FIXTURES-1", "namespace": "local-store-117-main", "nonce": nonce,
              "syntheticMarker": marker, "senderUserID": None, "uploadName": upload_name, "files": files,
              "externalOriginalsRetained": True, "deviceOrServerRequestsMade": 0}
    manifest_path.write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    return result


if __name__ == "__main__":
    result = prepare()
    # No credentials, UID, marker or content enters stdout.
    print(json.dumps({"prepared": True, "files": len(result["files"]), "senderBound": result["senderUserID"] is not None}))
