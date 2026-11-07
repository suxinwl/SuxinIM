#!/usr/bin/env python3
"""Run one fixed deployment tool with the private HRM environment."""
import json
import os
import pathlib
import sys

BASE = pathlib.Path('/srv/puty-suite/gosuxin')
if len(sys.argv) < 2 or sys.argv[1] not in ['bootstrap-system', 'runtime-plugin-install', 'wecom-config']:
    sys.exit('Unsupported HRM tool')
os.chdir(BASE)
environment = os.environ.copy()
environment.update(json.loads((BASE / 'runtime-environment.json').read_text()))
binary = BASE / 'tools' / sys.argv[1]
os.execve(str(binary), [str(binary), *sys.argv[2:]], environment)
