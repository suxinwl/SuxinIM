"""Publish built frontend files while retaining assets used by open browser tabs."""
import gzip
import os
from pathlib import Path
import tempfile

ROOT = Path(__file__).resolve().parents[1]

def main():
    source = ROOT / 'web/dist'
    target = ROOT / 'resource/suxinweb'
    if not (source / 'index.html').is_file():
        raise SystemExit('Run npm run build in web first')
    files = [p for p in source.rglob('*') if p.is_file()]
    files.sort(key=lambda p: (p.name in ('index.html', 'index.html.gz'), p.as_posix()))
    for p in files:
        dest = target / p.relative_to(source)
        dest.parent.mkdir(parents=True, exist_ok=True)
        fd, tmp = tempfile.mkstemp(prefix='.frontend-', dir=dest.parent)
        try:
            with os.fdopen(fd, 'wb') as f:
                body = p.read_bytes()
                if p.suffix == '.html':
                    body = ('\n'.join(line.rstrip() for line in body.decode('utf-8').splitlines()) + '\n').encode('utf-8')
                f.write(body)
            os.replace(tmp, dest)
        finally:
            if os.path.exists(tmp):
                os.unlink(tmp)
    for name in ('index.html', 'config.js', 'devtools.html', 'exceltemplate.html'):
        p = target / name
        if p.exists() and (target / (name + '.gz')).exists():
            (target / (name + '.gz')).write_bytes(gzip.compress(p.read_bytes(), mtime=0))
    print(f'Frontend synchronized: {len(files)} files; prior assets retained')

if __name__ == '__main__':
    main()
