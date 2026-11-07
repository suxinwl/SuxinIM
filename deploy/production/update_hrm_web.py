#!/usr/bin/env python3
"""Publish verified HRM assets first, then atomically switch the HTML entry."""
import hashlib,json,os,pathlib,pwd,zipfile
base=pathlib.Path('/srv/puty-suite')
marker=json.loads((base/'openim/instance.json').read_text())
assert marker['production'] and marker['testOnly'] is False and marker['root']==str(base/'openim')
archive=base/'staging/hrm-web.zip'
assert hashlib.sha256(archive.read_bytes()).hexdigest()=='382d191afdb9cdbaaed4e33be554cb254af10cc1b71f876ea89d516d809af1f2'
target=base/'gosuxin/resource/suxinweb'
owner=pwd.getpwnam('puty')
with zipfile.ZipFile(archive) as z:
    for member in z.infolist():
        p=pathlib.PurePosixPath(member.filename)
        assert not p.is_absolute() and '..' not in p.parts
    for member in z.infolist():
        if member.is_dir() or member.filename=='index.html':continue
        file=target/member.filename
        assert file.resolve().is_relative_to(target.resolve()) and not file.is_symlink()
        file.parent.mkdir(parents=True,exist_ok=True,mode=0o755)
        file.write_bytes(z.read(member));file.chmod(0o644);os.chown(file,owner.pw_uid,owner.pw_gid)
    content=z.read('index.html')
    assert hashlib.sha256(content).hexdigest()=='e5b9e6676a8e8e37bda306ada4d7f1155a6e8be2f7c31782e8927037cffd655f'
    temp=target/'index.html.next';temp.write_bytes(content);temp.chmod(0o644);os.chown(temp,owner.pw_uid,owner.pw_gid);temp.replace(target/'index.html')
print(json.dumps({'hrmWebUpdated':True,'serviceRestarted':False,'indexSHA256':hashlib.sha256(content).hexdigest()}))
