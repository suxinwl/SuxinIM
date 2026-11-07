#!/usr/bin/env python3
"""Supervisor entry point; secrets remain in the child environment."""
import json
import os
import pathlib
import sys

BASE = pathlib.Path('/srv/puty-suite/gosuxin')
if BASE.resolve() != BASE or not (BASE / 'main').is_file():
    sys.exit('Unexpected HRM deployment root')
os.chdir(BASE)
environment = os.environ.copy()
environment.update(json.loads((BASE / 'runtime-environment.json').read_text()))
os.execve(str(BASE / 'main'), [str(BASE / 'main')], environment)
