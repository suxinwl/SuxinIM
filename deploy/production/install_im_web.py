#!/usr/bin/env python3
import fcntl,hashlib,json,pathlib,zipfile
base=pathlib.Path('/srv/puty-suite/openim')
marker=json.loads((base/'instance.json').read_text())
assert marker['production'] is True and marker['testOnly'] is False and marker['root']==str(base)
maintenance=base.parent/'.backup-stage';assert not maintenance.is_symlink();maintenance.mkdir(mode=0o700,exist_ok=True)
lock=(maintenance/'backup.lock').open('a+b');fcntl.flock(lock,fcntl.LOCK_EX|fcntl.LOCK_NB)
archive=base.parent/'staging/im-web.zip'
assert hashlib.sha256(archive.read_bytes()).hexdigest()=='7ec50d129542e3ed0a9a82ef5c371e176165a652220f4d10c9491aeb27cd0363'
target=base/'www'
target.mkdir(exist_ok=True)
with zipfile.ZipFile(archive) as z:
    for member in z.infolist():
        p=pathlib.PurePosixPath(member.filename)
        assert not p.is_absolute() and '..' not in p.parts
    for member in z.infolist():
        if member.is_dir() or member.filename=='index.html':continue
        file=target/member.filename
        assert file.resolve().is_relative_to(target.resolve()) and not file.is_symlink()
        file.parent.mkdir(parents=True,exist_ok=True,mode=0o755)
        file.write_bytes(z.read(member));file.chmod(0o644)
    temp=target/'index.html.next';temp.write_bytes(z.read('index.html'));temp.chmod(0o644);temp.replace(target/'index.html')
assert (target/'index.html').is_file()
print(json.dumps({'imWebInstalled':True,'indexSHA256':hashlib.sha256((target/'index.html').read_bytes()).hexdigest()}))
