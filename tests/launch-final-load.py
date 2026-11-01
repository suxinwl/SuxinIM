"""Detach the one reviewed Linux test coordinator; never start a recurring job."""
import json
import pathlib
import subprocess
import sys

root = pathlib.Path('/srv/openim38-upgrade')
run_id = 'final-100-20261101a'
if root.resolve() != root or pathlib.Path.cwd().resolve() != root:
    raise RuntimeError('Run from the real isolated test root')
source = root / 'tests/start-final-load.py'
log = root / 'logs/final-load-coordinator.log'
with log.open('x', encoding='utf-8') as output:
    child = subprocess.Popen(
        [sys.executable, '-u', str(source), '--run-id', run_id],
        cwd=root, stdin=subprocess.DEVNULL, stdout=output,
        stderr=subprocess.STDOUT, start_new_session=True,
    )
print(json.dumps({'startedCoordinatorPID': child.pid, 'runID': run_id,
                  'log': str(log.relative_to(root)), 'recurringJob': False}))
