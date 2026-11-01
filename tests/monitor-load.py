"""Current acceptance run resource sampler, not an installed recurring job."""
import datetime,json,pathlib,subprocess,time
ROOT=pathlib.Path('/srv/openim38-upgrade')
assert pathlib.Path.cwd().resolve()==ROOT
start=json.loads((ROOT/'.private/load-start-at.json').read_text())['startAt']/1000
output=ROOT/'logs/load-resources.jsonl'
with output.open('w') as stream:
 while time.time()<start+1830:
  raw=subprocess.check_output(['docker','stats','--no-stream','--format','{{json .}}'],text=True)
  containers=[json.loads(line) for line in raw.splitlines() if line.strip()]
  tests=[x for x in containers if x.get('Name','').startswith('openim38-upgrade')]
  info=dict((line.split(':',1)[0],line.split(':',1)[1].strip()) for line in pathlib.Path('/proc/meminfo').read_text().splitlines())
  sample={'utc':datetime.datetime.now(datetime.timezone.utc).isoformat(),'secondsFromStart':round(time.time()-start,2),'hostMemoryKB':{key:info[key] for key in ['MemTotal','MemAvailable','SwapTotal','SwapFree']},'loadAverage':pathlib.Path('/proc/loadavg').read_text().split()[:3],'loadGenerator':[x for x in tests if x['Name']=='openim38-upgrade-load'],'businessContainers':[x for x in tests if x['Name']!='openim38-upgrade-load']}
  stream.write(json.dumps(sample)+'\n');stream.flush();time.sleep(15)
