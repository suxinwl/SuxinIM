"""Read-only, allowlisted diagnostic of the isolated test stack's call window."""
import json, re, subprocess

root = '/srv/openim38-upgrade'
window = ['--since', '2026-11-01T03:30:40Z', '--until', '2026-11-01T03:32:00Z']
allowed = ('time', 'ts', 'level', 'msg', 'room', 'roomID', 'participant', 'pID', 'error', 'duration', 'elapsed')
jwt = re.compile(r'eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+')
report = {'readOnly': True, 'scope': root, 'window': window, 'services': {}, 'lineCounts': {}, 'parserVersion': 2}
for service in ('nginx', 'enterprise-service', 'livekit'):
    result = subprocess.run(['docker', 'compose', '-f', root + '/compose.yaml', 'logs', '--no-log-prefix', *window, service], capture_output=True, text=True, timeout=30, check=True)
    rows = []
    report['lineCounts'][service] = {'raw': len(result.stdout.splitlines()), 'unparsed': 0}
    for line in result.stdout.splitlines():
        if service == 'nginx':
            match = re.fullmatch(r'([\d.]+) (GET|POST) (/[^ ?]+) (\d{3}) (\d+)', line)
            if match and ('signaling' in match[3] or match[4] != '200'):
                rows.append(dict(zip(('address', 'method', 'path', 'status', 'bytes'), match.groups())))
            continue
        try:
            value = json.loads(line)
        except json.JSONDecodeError:
            # LiveKit's console logger prefixes an otherwise JSON field map.
            offset = line.find('{')
            if offset < 0:
                report['lineCounts'][service]['unparsed'] += 1
                continue
            try:
                value = json.loads(line[offset:])
            except json.JSONDecodeError:
                report['lineCounts'][service]['unparsed'] += 1
                continue
            value['msg'] = re.sub(r'([?&](?:token|access_token|jwt)=)[^ &\"]+', r'\1<redacted>', line[:offset])[:512]
        rows.append({key: jwt.sub('<JWT-redacted>', str(value[key]))[:1200] for key in allowed if key in value})
    report['services'][service] = rows
print(json.dumps(report, ensure_ascii=False))
