"""Post-recovery RPC warmup with only unusable, unregistered probe JWTs.

No registered service or employee token is read. Nothing is registered or saved.
The enterprise private IM connection is deliberately outside this probe's reach.
"""
import argparse
import base64
import hashlib
import hmac
import json
import pathlib
import re
import secrets
import ssl
import subprocess
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

BASE = pathlib.Path('/srv/puty-suite')
PROJECT = 'puty-suite-prod'
CONTAINER = PROJECT+'-enterprise-service-1'
PROBE_USER = 'puty-runtime-key-proof-unregistered'


class Failure(Exception):
    pass


def require(ok, name):
    if not ok:
        raise Failure(name)


def encode(raw):
    return base64.urlsafe_b64encode(raw).rstrip(b'=')


def probe_token(secret, chat=False):
    now = int(time.time())
    claims = {'UserID': PROBE_USER, 'PlatformID': 0 if chat else 2, 'exp': now+60, 'iat': now-5, 'nbf': now-5, 'jti': secrets.token_hex(16)}
    if chat:
        claims['UserType'] = 1
    body = encode(b'{"alg":"HS256","typ":"JWT"}')+b'.'+encode(json.dumps(claims, separators=(',', ':')).encode())
    return (body+b'.'+encode(hmac.new(secret.encode(), body, hashlib.sha256).digest())).decode()


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *args, **kwargs):
        return None


def owned_environment(container, marker, activation):
    owner = marker.get('instanceID')
    require(isinstance(owner, str) and re.fullmatch(r'[0-9a-f]{32}', owner)
            and marker.get('production') is True and marker.get('testOnly') is False
            and marker.get('stackName') == PROJECT and activation.get('instanceID') == owner,
            'production_instance_markers_do_not_match')
    labels = container.get('Config', {}).get('Labels') or {}
    require(container.get('Name', '').lstrip('/') == CONTAINER
            and labels.get('com.puty.suite') == owner
            and labels.get('com.docker.compose.project') == PROJECT
            and labels.get('com.docker.compose.service') == 'enterprise-service'
            and container.get('State', {}).get('Running') is True,
            'foreign_or_stopped_enterprise_container')
    # Environment is inspected only after the ownership and runtime assertions.
    return dict(value.split('=', 1) for value in container['Config']['Env'] if '=' in value)


def request(opener, path, value, headers=None):
    require(path in ('/api/auth/parse_token', '/enterprise/v1/signaling/get_signal_invitation_info_start_app'), 'probe_route_not_read_only_allowlisted')
    req = urllib.request.Request('https://192.168.10.11:9885'+path, json.dumps(value).encode(), {'Content-Type': 'application/json', 'operationID': secrets.token_hex(16), **(headers or {})}, method='POST')
    try:
        response = opener.open(req, timeout=8)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        return response.status, json.loads(response.read(65536))


def warm_once(opener, chat_probe, im_probe):
    path = '/enterprise/v1/signaling/get_signal_invitation_info_start_app'
    status, body = request(opener, path, {}, {'token': im_probe, 'chatToken': chat_probe, 'platformID': '2'})
    require(status == 401 and body.get('errCode') == 1501 and body.get('errMsg') == 'Chat session expired or revoked', 'enterprise_Chat_RPC_registry_path_not_ready')
    status, body = request(opener, '/api/auth/parse_token', {'token': im_probe})
    require(status == 200 and body.get('errCode') == 1507 and body.get('errMsg') == 'TokenNotExistError', 'OpenIM_native_RPC_registry_path_not_ready')


def main():
    import yaml
    parser = argparse.ArgumentParser()
    parser.add_argument('--attempts', type=int, default=6)
    args = parser.parse_args()
    require(1 <= args.attempts <= 12, 'warmup_attempt_bound_invalid')
    paths = [BASE/'openim/instance.json', BASE/'activation.json', BASE/'openim/config/chat/chat-rpc-admin.yml', BASE/'openim/certs/puty-root.crt']
    require(BASE.resolve() == BASE and all(path.is_file() and not path.is_symlink() and path.resolve().is_relative_to(BASE) for path in paths), 'owned_runtime_files_missing_or_redirected')
    marker, activation = [json.loads(path.read_text()) for path in paths[:2]]
    raw = subprocess.check_output(['docker', 'inspect', CONTAINER], stderr=subprocess.DEVNULL, timeout=10)
    container = json.loads(raw)[0]
    env = owned_environment(container, marker, activation)
    config = BASE/'openim/config/chat'
    chat = yaml.safe_load((config/'chat-rpc-admin.yml').read_text())
    require(isinstance(chat['secret'], str) and len(chat['secret']) >= 32, 'active_Chat_configuration_invalid')
    context = ssl.create_default_context(cafile=str(BASE/'openim/certs/puty-root.crt'))
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), NoRedirect(), urllib.request.HTTPSHandler(context=context))
    started = time.monotonic()
    last = None
    for attempt in range(1, args.attempts+1):
        try:
            warm_once(opener, probe_token(chat['secret'], chat=True), probe_token(env['OPENIM_SECRET']))
            print(json.dumps({'authenticationWarmupPassed': True, 'unregisteredUnusableProbesOnly': True, 'registeredCredentialsRead': 0, 'newTokenRegistrations': 0, 'employeeCredentialsRead': 0, 'enterpriseChatRedisPath': True, 'ChatLoginForbiddenMongoPathWarmed': False, 'enterprisePrivateIMRPCPathWarmed': False, 'nativeIMRedisRegistryPath': True, 'attempts': attempt, 'seconds': round(time.monotonic()-started, 2)}), flush=True)
            return
        except (Failure, OSError, ValueError) as error:
            last = str(error) if isinstance(error, Failure) else 'RPC_or_TLS_transport_unavailable'
            if attempt < args.attempts:
                time.sleep(2)
    raise Failure(last or 'authentication_warmup_failed')


if __name__ == '__main__':
    try:
        main()
    except Exception as error:
        print(json.dumps({'authenticationWarmupPassed': False, 'errorType': type(error).__name__, 'category': str(error) if isinstance(error, Failure) else 'private_details_suppressed'}), flush=True)
        sys.exit(1)
