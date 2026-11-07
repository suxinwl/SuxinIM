"""Preserve/verify the fixed native core modifications outside rebuildable cache."""
from pathlib import Path
import argparse, hashlib, json, subprocess

SUITE = Path(__file__).resolve().parents[3]
CORE = SUITE / '.cache/sdk-core-patch15-reference'
OVERLAY = SUITE / 'sdks/flutter/native-core-overlay'
BASE = 'd6e0b549db904d0327d76ef7c9c283203879a095'

def git(*args):
    return subprocess.check_output(['git','-C',str(CORE),*args], text=True, stderr=subprocess.DEVNULL).strip()

def main():
    parser=argparse.ArgumentParser();parser.add_argument('mode',choices=['capture','verify']);args=parser.parse_args()
    if git('rev-parse','HEAD') != BASE: raise SystemExit('Unexpected fixed core revision')
    manifest=OVERLAY/'overlay-manifest.json'
    if args.mode == 'capture':
        files=set(git('diff','--name-only').splitlines()+git('ls-files','--others','--exclude-standard').splitlines())
        items=[]
        for name in sorted(files):
            if not name or Path(name).is_absolute() or '..' in Path(name).parts:raise SystemExit('Invalid overlay path')
            source=CORE/name
            if not source.is_file() or source.is_symlink():raise SystemExit('Invalid overlay source')
            data=source.read_bytes();target=OVERLAY/name;target.parent.mkdir(parents=True,exist_ok=True);target.write_bytes(data)
            items.append({'path':name,'sha256':hashlib.sha256(data).hexdigest()})
        manifest.write_text(json.dumps({'baseCommit':BASE,'files':items},indent=2)+'\n',encoding='utf8')
    else:
        model=json.loads(manifest.read_text(encoding='utf8'))
        if model['baseCommit'] != BASE:raise SystemExit('Unexpected overlay base')
        for item in model['files']:
            name=item['path']
            if Path(name).is_absolute() or '..' in Path(name).parts:raise SystemExit('Invalid overlay path')
            for root in [CORE,OVERLAY]:
                if hashlib.sha256((root/name).read_bytes()).hexdigest()!=item['sha256']:raise SystemExit('Native core source differs from captured overlay')
    print('Fixed native core overlay '+args.mode+' passed')

if __name__=='__main__':main()
