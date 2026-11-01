"""Separate application resources from synthetic load generation."""
import json,pathlib,re,statistics
ROOT=pathlib.Path(__file__).resolve().parents[1]
samples=[json.loads(line) for line in (ROOT/'tests/load/resources.jsonl').read_text().splitlines() if line.strip()]
samples=[s for s in samples if 0<=s['secondsFromStart']<=1800]
def memory(text):
 value,unit=re.fullmatch(r'([\d.]+)([A-Za-z]+)',text.split(' / ')[0]).groups()
 return float(value)*{'B':1,'kB':1000,'KB':1000,'MB':1000**2,'GB':1000**3,'KiB':1024,'MiB':1024**2,'GiB':1024**3}[unit]
rows=[]
for sample in samples:
 business=[c for c in sample['businessContainers'] if c['Name']!='openim38-upgrade-mail-fixture']
 rows.append({'seconds':sample['secondsFromStart'],'businessMemoryGiB':sum(memory(c['MemUsage']) for c in business)/1024**3,'businessCPUCorePercent':sum(float(c['CPUPerc'].rstrip('%')) for c in business),'loadGeneratorMemoryGiB':sum(memory(c['MemUsage']) for c in sample['loadGenerator'])/1024**3,'hostAvailableGiB':int(sample['hostMemoryKB']['MemAvailable'].split()[0])/1024**2})
result={'samples':len(rows),'firstSecond':rows[0]['seconds'],'lastSecond':rows[-1]['seconds'],'notes':['Business totals exclude the Linux SDK generator and SMTP fixture.','CPU 100% means one fully occupied logical CPU; server has 24 vCPUs.','Docker sampled memory is not a capacity guarantee; other existing services remain on the host.'],'businessPeakMemoryGiB':max(x['businessMemoryGiB'] for x in rows),'businessPeakCPUCorePercent':max(x['businessCPUCorePercent'] for x in rows),'businessAverageCPUCorePercent':statistics.mean(x['businessCPUCorePercent'] for x in rows),'generatorPeakMemoryGiB':max(x['loadGeneratorMemoryGiB'] for x in rows),'hostMinimumAvailableGiB':min(x['hostAvailableGiB'] for x in rows)}
(ROOT/'tests/load/resource-summary.json').write_text(json.dumps(result,indent=2)+'\n')
print(json.dumps(result,indent=2))
