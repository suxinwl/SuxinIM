#!/usr/bin/env python3
"""Persist a one-time fresh-data cutoff before enabling any upstream jobs."""
import datetime
import fcntl
import json
import os
import pathlib
import subprocess

BASE=pathlib.Path('/srv/puty-suite')
marker=json.loads((BASE/'openim/instance.json').read_text())
assert marker['production'] is True and marker['testOnly'] is False and marker['root']==str(BASE/'openim')
assert (BASE/'gosuxin/devsource/developer/install/install.lock').is_file()
maintenance=BASE/'.backup-stage'
assert not maintenance.is_symlink()
maintenance.mkdir(mode=0o700,exist_ok=True)
lock=(maintenance/'backup.lock').open('a+b')
fcntl.flock(lock,fcntl.LOCK_EX|fcntl.LOCK_NB)
environment=BASE/'gosuxin/runtime-environment.json'
data=json.loads(environment.read_text())
activation=BASE/'activation.json'
if activation.exists():
    saved=json.loads(activation.read_text())
    assert saved['instanceID']==marker['instanceID']
    stamp=saved['startAt']
else:
    stamp=data.get('GOSUXIN_SYNC_START_AT') or datetime.datetime.now(datetime.timezone.utc).isoformat(timespec='seconds')
    assert datetime.datetime.fromisoformat(stamp.replace('Z','+00:00')).tzinfo is not None
    activation.write_text(json.dumps({'instanceID':marker['instanceID'],'startAt':stamp,'dataPolicy':'fresh-business-data'},indent=2)+'\n')
    activation.chmod(0o644)
assert not data.get('GOSUXIN_SYNC_START_AT') or data['GOSUXIN_SYNC_START_AT']==stamp
data.update(GOSUXIN_SYNC_START_AT=stamp,GOSUXIN_REHEARSAL='false')
temporary=environment.with_suffix('.new')
temporary.write_text(json.dumps(data,indent=2)+'\n')
temporary.chmod(0o600)
st=environment.stat()
os.chown(temporary,st.st_uid,st.st_gid)
temporary.replace(environment)
subprocess.run(['/www/server/panel/pyenv/bin/supervisorctl','restart','puty-hrm'],check=True)
print(json.dumps({'activated':True,'startAt':stamp,'historicalBackfill':False}))
