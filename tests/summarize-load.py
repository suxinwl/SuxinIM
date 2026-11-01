"""Aggregate three real-SDK partitions; never average percentile values."""
import datetime, json, pathlib, sys

ROOT = pathlib.Path(__file__).resolve().parents[1]
paths = [pathlib.Path(x) for x in sys.argv[1:]]
if len(paths) != 3:
    raise SystemExit('Pass Windows30, Linux35a, Linux35b summary JSON files')
parts = [json.loads(p.read_text(encoding='utf-8')) for p in paths]
checks = {
    'sameRun': len({p['sharedRun'] for p in parts}) == 1,
    'exact100DistinctSlots': sorted((p['offset'],p['count']) for p in parts)==[(0,30),(30,35),(65,35)],
    'allComplete': all(p['phase']=='complete' for p in parts),
    'atLeast30Minutes': all(p['elapsedSeconds']>=1800 for p in parts),
    'allConnectedAtCompletion': sum(p['connected'] for p in parts)==100,
    'noFailedSends': sum(p['failed'] for p in parts)==0,
    'noDisconnections': sum(p['disconnections'] for p in parts)==0,
    'noRuntimeErrors': not any(p['pageErrors'] for p in parts),
    'allSentMessagesReceived': sum(p['sent'] for p in parts)==sum(p['received'] for p in parts),
}
start=max(datetime.datetime.fromisoformat(p['startedAt']) for p in parts)
end=min(datetime.datetime.fromisoformat(p['finishedAt']) for p in parts)
checks['commonWindowAtLeast30Minutes']=(end-start).total_seconds()>=1800
report={
    'run':parts[0]['sharedRun'],'passed':all(checks.values()),'checks':checks,
    'clients':sum(p['clients'] for p in parts),'sent':sum(p['sent'] for p in parts),
    'received':sum(p['received'] for p in parts),'failed':sum(p['failed'] for p in parts),
    'disconnections':sum(p['disconnections'] for p in parts),
    'commonWindowSeconds':(end-start).total_seconds(),
    'latencyNote':'Partition percentiles retained separately; they are not averaged into a fake pooled percentile.',
    'partitions':[{k:p[k] for k in ['run','sharedRun','offset','count','elapsedSeconds','startedAt','finishedAt','connected','sent','received','failed','disconnections','latencyMs']} for p in parts],
    'evidence':[str(p.relative_to(ROOT)) if p.is_relative_to(ROOT) else str(p) for p in paths],
}
output=ROOT/'tests/load/summary.json';output.parent.mkdir(parents=True,exist_ok=True)
output.write_text(json.dumps(report,indent=2)+'\n',encoding='utf-8')
print(json.dumps(report,indent=2))
raise SystemExit(not report['passed'])
