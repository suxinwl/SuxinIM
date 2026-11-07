#!/usr/bin/env python3
"""Summarize known error categories without printing identities or request bodies."""
import collections,json,re,subprocess
p=subprocess.run(['docker','logs','--since','8m','puty-suite-prod-enterprise-service-1'],capture_output=True)
counts=collections.Counter()
for line in (p.stdout+p.stderr).decode(errors='replace').splitlines():
    try:d=json.loads(line)
    except ValueError:continue
    if d.get('msg')!='enterprise request failed':continue
    value=str(d.get('error',''))
    if value.startswith('rpc error: code = '):
        code=value.split(' desc = ')[0]
        for term in ['connection refused','context canceled','deadline exceeded','token','epoch','session','permission']:
            if term in value.lower():code+=' / '+term
        counts[code]+=1
    elif value.startswith('OpenIM /'):
        matched=re.match(r'(OpenIM /[a-zA-Z0-9_/]+ rejected \(\d+\): [a-zA-Z0-9_ ]+)',value)
        counts[matched[1].strip() if matched else 'OpenIM upstream error']+=1
    else:
        counts['other error (details withheld)']+=1
print(json.dumps({'enterpriseErrorCategories':counts}))
