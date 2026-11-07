"""Validate and repair only the R6 image-captcha proxy query."""
import argparse
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from pathlib import Path
import socket
import subprocess
import tempfile
import threading
import time
import urllib.error
import urllib.request

NGINX = '/www/server/nginx/sbin/nginx'
OLD = 'proxy_pass http://127.0.0.1:19880/common/basetool/getCaptcha?type=image;'
NEW = 'proxy_pass http://127.0.0.1:19880$uri?type=image;'
FILES = [Path('/www/server/panel/vhost/nginx/gosuxin-development.conf'), Path('/srv/gosuxin-suite/deploy/nginx.conf'), Path('/srv/gosuxin-suite/deploy/nginx-development.conf')]

def validate_proxy():
    class Echo(BaseHTTPRequestHandler):
        def do_GET(self):
            body = json.dumps({'path': self.path}).encode()
            self.send_response(200)
            self.send_header('Content-Length', str(len(body)))
            self.end_headers()
            self.wfile.write(body)
        def log_message(self, *_args):
            pass
    server = ThreadingHTTPServer(('127.0.0.1', 0), Echo)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    try:
        with tempfile.TemporaryDirectory(prefix='attendance-captcha-proxy-') as directory:
            folder = Path(directory)
            (folder / 'logs').mkdir()
            with socket.socket() as probe:
                probe.bind(('127.0.0.1', 0))
                port = probe.getsockname()[1]
            config = folder / 'nginx.conf'
            config.write_text(f'''pid {folder}/nginx.pid;
error_log {folder}/error.log;
events {{}}
http {{ lua_package_path "/www/server/nginx/lib/lua/?.lua;;";
access_log off; server {{ listen 127.0.0.1:{port};
location = /common/basetool/getCaptcha {{
 if ($arg_type != image) {{ return 418; }}
 proxy_pass http://127.0.0.1:{server.server_port}$uri?type=image;
}}
}} }}
''')
            command = [NGINX, '-p', str(folder), '-c', str(config)]
            subprocess.run([*command, '-t'], check=True, capture_output=True)
            with (folder / 'startup.log').open('wb') as startup:
                proxy = subprocess.Popen([*command, '-g', 'daemon off; master_process off;'], stdout=startup, stderr=startup)
                try:
                    ready = False
                    for _ in range(60):
                        if proxy.poll() is not None:
                            break
                        try:
                            with socket.create_connection(('127.0.0.1', port), timeout=.2):
                                ready = True
                                break
                        except OSError:
                            time.sleep(.05)
                    if not ready:
                        logs = [(folder / name).read_text(errors='replace')[-4000:] for name in ['startup.log', 'error.log'] if (folder / name).exists()]
                        raise RuntimeError('Isolated nginx did not start: ' + '\n'.join(logs))
                    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
                    checks = []
                    for query, allowed in [('type=image', True), ('type=image&_t=123', True), ('type=image&type=mobile', True), ('type=mobile&type=image', False), ('type=mobile', False), ('type=email', False), ('', False)]:
                        try:
                            with opener.open(f'http://127.0.0.1:{port}/common/basetool/getCaptcha?{query}', timeout=5) as response:
                                assert allowed and json.load(response)['path'] == '/common/basetool/getCaptcha?type=image'
                        except urllib.error.HTTPError as error:
                            assert not allowed and error.code == 418
                        checks.append({'query': query, 'passed': True, 'image_only_upstream': allowed})
                    return checks
                finally:
                    proxy.terminate()
                    proxy.wait(timeout=5)
    finally:
        server.shutdown()
        server.server_close()

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--apply', action='store_true')
    args = parser.parse_args()
    checks = validate_proxy()
    report = {'loopback_checks': checks, 'applied': False}
    if not args.apply:
        print(json.dumps(report))
        return
    if os.geteuid() != 0:
        raise RuntimeError('sudo is required')
    before = {}
    for path in FILES:
        if path.is_symlink():
            raise RuntimeError('Unexpected configuration symlink')
        raw = path.read_bytes()
        if raw.count(OLD.encode()) != 1 or b'if ($arg_type != image) { return 418; }' not in raw or b'location @gosuxin_hrm_lan_captcha' not in raw:
            raise RuntimeError('Existing image-only policy changed; inspect before editing')
        before[path] = raw
    backup = Path('/srv/gosuxin-suite/shared/gosuxin/storage/plugins/biz-suite/backups') / ('captcha-proxy-' + time.strftime('%Y%m%d-%H%M%S'))
    backup.mkdir(mode=0o700)
    for index, (path, raw) in enumerate(before.items()):
        destination = backup / f'{index}-{path.name}'
        destination.write_bytes(raw)
        destination.chmod(0o600)
    def replace(path, raw):
        info = path.stat()
        temporary = path.with_name(path.name + '.captcha-fix-next')
        with temporary.open('xb') as output:
            output.write(raw)
        temporary.chmod(info.st_mode & 0o777)
        os.chown(temporary, info.st_uid, info.st_gid)
        os.replace(temporary, path)
    try:
        for path, raw in before.items():
            replace(path, raw.replace(OLD.encode(), NEW.encode()))
        subprocess.run([NGINX, '-t'], check=True, capture_output=True)
        subprocess.run([NGINX, '-s', 'reload'], check=True, capture_output=True)
    except Exception:
        for path, raw in before.items():
            replace(path, raw)
        subprocess.run([NGINX, '-t'], check=True, capture_output=True)
        subprocess.run([NGINX, '-s', 'reload'], check=True, capture_output=True)
        raise
    report.update(applied=True, backup=str(backup), files=[str(path) for path in FILES], image_only_policy_preserved=True)
    (backup / 'repair-report.json').write_text(json.dumps(report, indent=2))
    print(json.dumps(report))

if __name__ == '__main__':
    main()
