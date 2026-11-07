"""Validate private captured Android UI queries and actual employee result rows."""
import json
import pathlib
import xml.etree.ElementTree as ET

ROOT = pathlib.Path(__file__).resolve().parents[2]
EVIDENCE = ROOT / '.private/production/acceptance/organization-android-final'
fixture = json.loads((ROOT / '.private/production/acceptance/organization-test-accounts.json').read_text(encoding='utf-8-sig'))
if fixture['namespace'] != 'organization-20261103': raise ValueError('Fixture rejected')
actor, colleague = fixture['accounts']
checks = []
for entry, prefix, suffix in [('organization','organization','-row-proof'),('add_friend','add-friend','-valid'),('global','global','-valid')]:
    for mode, value in [('phone',colleague['mobile']),('name',colleague['name']),('employee-no',colleague['username'])]:
        name = prefix+'-'+mode+suffix
        nodes = list(ET.parse(EVIDENCE/(name+'.xml')).getroot().iter('node'))
        edits = [node.get('text','') for node in nodes if node.get('class') == 'android.widget.EditText']
        labels = [node.get('text','')+' '+node.get('content-desc','') for node in nodes if node.get('class') != 'android.widget.EditText']
        exact_query = str(value) in edits
        row = any(colleague['name'] in label and colleague['username'] in label for label in labels)
        actor_row = any(actor['name'] in label and actor['username'] in label for label in labels)
        checks.append({'entry':entry,'mode':mode,'exactQueryMatched':exact_query,'counterpartResultRowMatched':row,'actorResultRowAbsent':not actor_row,'evidence':name+'.png','passed':exact_query and row and not actor_row})
report={'namespace':fixture['namespace'],'featureUIArtifactSHA256':'1ac0cc39c1e96ca0f62f34065e3bc3029e793500cf32fa27893942647ff4349c','checks':checks,'passed':all(check['passed'] for check in checks),'scope':'Exact query text plus separate result row matching both synthetic employee name and number; query input/header alone cannot satisfy result-row check; final home-label APK upgrade validation recorded separately'}
(EVIDENCE/'search-result-row-proof.json').write_text(json.dumps(report,indent=2)+'\n',encoding='utf-8')
print(json.dumps({'passed':report['passed'],'checks':len(checks),'failedCases':[check['entry']+':'+check['mode'] for check in checks if not check['passed']]}))
if not report['passed']: raise SystemExit(1)
