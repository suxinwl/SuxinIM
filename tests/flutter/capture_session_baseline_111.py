"""Capture the owned retained111 session, save it privately, and check fixed APIs."""
from pathlib import Path
import base64
import datetime
import importlib.util
import json
import ssl
import subprocess
import time
import urllib.request
import xml.etree.ElementTree as ET

ROOT = Path('O:/')
PWSH = r'C:\Users\Administrator\.cache\codex-runtimes\codex-primary-runtime\dependencies\native\powershell\pwsh.exe'
source = ROOT / 'tests/flutter/revoke_stopped_ui_session.py'
spec = importlib.util.spec_from_file_location('revoke_guard', source)
guard = importlib.util.module_from_spec(spec)
spec.loader.exec_module(guard)
assert guard.sha(source.read_bytes()) == '20f7ca1c6ec2a2429e2848bbbfe202df690c1701f95e526d3e2ccf4f62edb82d'


def protect(path, directory=False):
    script = "$p='" + str(path).replace("'", "''") + "';"
    script += '$sid=[System.Security.Principal.WindowsIdentity]::GetCurrent().User;'
    script += "$sys=[System.Security.Principal.SecurityIdentifier]::new('S-1-5-18');"
    kind = 'DirectorySecurity' if directory else 'FileSecurity'
    script += '$acl=[System.Security.AccessControl.' + kind + ']::new();'
    script += '$acl.SetAccessRuleProtection($true,$false);'
    for principal in ('$sid', '$sys'):
        script += '$acl.AddAccessRule([System.Security.AccessControl.FileSystemAccessRule]::new('
        script += principal + ',[System.Security.AccessControl.FileSystemRights]::FullControl,'
        if directory:
            script += "[System.Security.AccessControl.InheritanceFlags]'ContainerInherit,ObjectInherit',"
            script += '[System.Security.AccessControl.PropagationFlags]::None,'
        script += '[System.Security.AccessControl.AccessControlType]::Allow));'
    script += 'Set-Acl -LiteralPath $p -AclObject $acl;'
    script += '$actual=Get-Acl -LiteralPath $p;'
    script += "if(-not $actual.AreAccessRulesProtected -or $actual.Access.Count -ne 2){exit 12};"
    script += "foreach($entry in $actual.Access){if($entry.IdentityReference.Translate([System.Security.Principal.SecurityIdentifier]).Value -notin @($sid.Value,'S-1-5-18')){exit 13}}"
    result = subprocess.run([PWSH, '-NoProfile', '-NonInteractive', '-Command', script],
                            capture_output=True, timeout=25)
    if result.returncode:
        raise RuntimeError('Private ACL check failed; details suppressed')


def exp(token):
    part = token.split('.')[1]
    return json.loads(base64.urlsafe_b64decode(part + '=' * ((-len(part)) % 4)))['exp']


output = ROOT / 'tests/flutter/android-111-current-session-valid-baseline.json'
assert not output.exists()
xml = guard.adb('shell', 'cat', f'/data/user/0/{guard.PACKAGE}/shared_prefs/FlutterSharedPreferences.xml')
values = [node.text for node in ET.fromstring(xml) if node.get('name') == 'flutter.loginCertificate']
assert len(values) == 1
pair = guard.checked_pair(json.loads(values[0]))
fingerprint = guard.pair_fingerprint(pair)
assert fingerprint == '6e50641ab3843e0e26f9ffcf16a4c294934ea4074ac33e5ae60200a73231e460'
blob = (json.dumps(pair, ensure_ascii=False, indent=2) + '\n').encode()
private = ROOT / '.private/android-ui-session-111-current.json'
assert private.read_bytes() == blob
protect(private)
canonical = ROOT / '.private/android-ui-session.json'
previous = canonical.read_bytes()
previous_pair = guard.checked_pair(json.loads(previous))
archive = ROOT / '.private/session-history/111-before-exact-revoke'
assert not archive.exists()
archive.mkdir(parents=True)
protect(archive, True)
backup = archive / 'android-ui-session-before-111.json'
backup.write_bytes(previous)
protect(backup)
manifest = {'capturedAt': datetime.datetime.now(datetime.timezone.utc).isoformat(),
            'oldFileSHA256': guard.sha(previous),
            'oldPairSHA256': guard.pair_fingerprint(previous_pair),
            'backupBytesEqual': backup.read_bytes() == previous,
            'replacementCapturedOnlyFromOwnUserdata': True}
manifest_path = archive / 'manifest.json'
manifest_path.write_text(json.dumps(manifest, indent=2) + '\n', encoding='utf-8')
protect(manifest_path)
canonical.write_bytes(blob)
protect(canonical)
assert canonical.read_bytes() == private.read_bytes()
context = ssl.create_default_context(cafile=str(guard.CA))
opener = urllib.request.build_opener(guard.NoRedirects(), urllib.request.HTTPSHandler(context=context),
                                   urllib.request.ProxyHandler({}))
checks = {
    'enterprise': guard.request(opener, pair, '/enterprise/v1/rtc/events?after=-1', None,
                                'android111-current-before-enterprise'),
    'im': guard.request(opener, pair, '/api/user/get_users_info', {'userIDs': [guard.USER]},
                        'android111-current-before-im'),
    'chat': guard.request(opener, pair, '/chat/user/find/full', {'userIDs': [guard.USER]},
                          'android111-current-before-chat'),
}
assert all(value.get('status') == 200 and value.get('errCode') == 0 for value in checks.values())
now = int(time.time())
result = {'at': datetime.datetime.now(datetime.timezone.utc).isoformat(),
          'PID': int(guard.adb('shell', 'pidof', guard.PACKAGE)),
          'source': 'Actual111 own retained SharedPreferences; not token injection',
          'userID': guard.USER, 'platformID': 2, 'pairSHA256': fingerprint,
          'matches110RecoveryPair': True,
          'privateFixture': '.private/android-ui-session-111-current.json',
          'privateFixtureSHA256': guard.sha(blob), 'canonicalSynchronizedToActualPair': True,
          'canonicalPreviousPairSHA256': manifest['oldPairSHA256'],
          'previousCanonicalPreservedPrivately': True,
          'privateACLVerified': 'Protected; exactly current SID and SYSTEM full-control ACEs',
          'expIM': exp(pair['imToken']), 'expChat': exp(pair['chatToken']),
          'remainingIMSeconds': exp(pair['imToken']) - now,
          'remainingChatSeconds': exp(pair['chatToken']) - now,
          'api': checks, 'normalTLS': True, 'CA_SHA256': guard.sha(guard.CA.read_bytes()),
          'noLocalStorageWritesOrCredentialInput': True,
          'preparationHarness': 'Two inline ACL attempts failed before canonical backup/write or HTTP because Windows PowerShell security module did not load under inherited PSModulePath. New snapshot immediately protected and verified; final tool uses configured pwsh. Not product or API failure.'}
with output.open('x', encoding='utf-8') as stream:
    json.dump(result, stream, indent=2)
    stream.write('\n')
print(json.dumps(result))
