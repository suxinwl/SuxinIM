"""Match real trigger-side SDK actions to sanitized receiver database evidence."""
import json,pathlib
ROOT=pathlib.Path(__file__).resolve().parents[1];folder=ROOT/'tests/webhooks'
triggers=json.loads((folder/'triggers.json').read_text());receiver=json.loads((folder/'receiver-events.json').read_text())
steps=triggers['results'];events=receiver['events']
if not all(step.get('passed') for step in steps) or triggers['pageErrors']:raise SystemExit('Trigger-side test failed')
user=steps[0]['userID'];read=steps[1];offline=steps[3];cases=[]
checks=[('callbackAfterUserOnlineCommand',lambda row:row.get('userID')==user and row.get('platformID')==5),('callbackAfterUserOfflineCommand',lambda row:row.get('userID')==user and row.get('platformID')==5),('callbackAfterSingleMsgReadCommand',lambda row:row.get('userID')==user and row.get('conversationID')==read['conversationID'] and read['seq'] in row.get('seqs',[])),('callbackBeforeOfflinePushCommand',lambda row:row.get('clientMsgID')==offline['clientMsgID'] and user in row.get('userIDList',[]))]
for command,match in checks:
 matched=[row for row in events if row['command']==command and match(row)]
 cases.append({'command':command,'passed':bool(matched),'matchedEvents':matched})
report={'passed':all(row['passed'] for row in cases),'since':receiver['since'],'runID':triggers['runID'],'source':triggers['source'],'cases':cases,'limits':['No third-party push provider is configured; the before-offline-push callback is verified, external notification delivery is not.','Callbacks are diagnostics, not a durable business queue. Exact retries are idempotent; canonical payload hashing preserves different native read sequences.']}
(folder/'summary.json').write_text(json.dumps(report,indent=2)+'\n');print(json.dumps({'passed':report['passed'],'nativeCallbackCases':len(cases)}))
if not report['passed']:raise SystemExit(1)
