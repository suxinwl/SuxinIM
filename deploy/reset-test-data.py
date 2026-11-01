"""Reset ONLY this isolated synthetic stack; never touches another Compose project.

Not run during load/acceptance. Requires the explicit stack-name guard on CLI.
"""
import argparse,datetime,pathlib,secrets,subprocess
from stack_guard import ROOT,PROJECT,guard,require_owned_resources
parser=argparse.ArgumentParser()
parser.add_argument('--confirm-stack',required=True)
parser.add_argument('--dry-run',action='store_true')
args=parser.parse_args()
if args.confirm_stack!=PROJECT or pathlib.Path.cwd().resolve()!=ROOT:
 raise SystemExit('Run from the isolated root with its exact stack confirmation')
spec,cli=guard()
require_owned_resources(spec)
if args.dry_run:
 print('Validated dry run: would remove only openim38-upgrade containers and owned volumes. Nothing changed.')
 raise SystemExit(0)
subprocess.run(cli+['down','--volumes','--remove-orphans'],check=True)
suffix=datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ')+'-'+secrets.token_hex(4)
for name in ('test-sessions.json','test-accounts.json'):
 path=ROOT/'.private'/name
 if path.exists():path.rename(path.with_name(name+'.before-reset-'+suffix))
print('Synthetic databases and attachments reset. Start this stack and rerun tests/seed_accounts.py; clients also need a fresh test profile.')
