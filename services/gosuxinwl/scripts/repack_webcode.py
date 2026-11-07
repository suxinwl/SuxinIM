"""Keep the local installer frontend identical to web source, excluding local artifacts."""
from pathlib import Path
import os
import tempfile
import zipfile

root = Path(__file__).resolve().parents[1]
web = root / "web"
target = root / "devsource/developer/install/webcode.zip"
skip = {".git", "node_modules", "dist", "__pycache__", ".cache"}


def frontend_files(source=web):
    for directory, dirs, files in os.walk(source):
        dirs[:] = sorted(d for d in dirs if d not in skip)
        # Exclude nested frontend copies created by the installer.
        if Path(directory) == source:
            dirs[:] = [d for d in dirs if d not in {"goframepro", "gosuxin-web"}]
        for name in sorted(files):
            if name.endswith((".local", ".log")) or ".timestamp-" in name:
                continue
            yield Path(directory) / name


def main():
    fd, name = tempfile.mkstemp(prefix=".webcode-", suffix=".zip", dir=target.parent)
    os.close(fd)
    temporary = Path(name)
    count = 0
    try:
        with zipfile.ZipFile(temporary, "w", zipfile.ZIP_DEFLATED) as archive:
            for path in frontend_files():
                archive.write(path, "webcode/" + path.relative_to(web).as_posix())
                count += 1
        os.replace(temporary, target)
    finally:
        temporary.unlink(missing_ok=True)
    print(f"Installer frontend synchronized: {count} files")


if __name__ == "__main__":
    main()
