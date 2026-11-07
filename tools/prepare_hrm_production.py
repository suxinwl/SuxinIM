"""Render private HRM runtime config, never printing credentials."""
import json
import base64
import pathlib
import secrets
import yaml

ROOT = pathlib.Path(__file__).resolve().parents[1]
HRM = pathlib.Path('E:/PUTY_AI/kaoqinstyem/gosuxinwl')
DEST = ROOT / '.private/production'


def main():
    suite = json.loads((DEST / 'suite-secrets.json').read_text())
    if 'jwt' not in suite:
        suite['jwt'] = secrets.token_hex(32)
        (DEST / 'suite-secrets.json').write_text(json.dumps(suite, indent=2) + '\n', encoding='utf-8')
    im = json.loads((DEST / 'openim/secrets.json').read_text())
    config_dir = DEST / 'gosuxin/manifest/config'
    config_dir.mkdir(parents=True, exist_ok=True)
    config = yaml.safe_load((HRM / 'manifest/config/config.example.yaml').read_text(encoding='utf-8'))
    config['server']['address'] = '127.0.0.1:18000'
    config['logger'].update(level='info', stdout=True)
    config['database']['default'].update(host='127.0.0.1', port='13306', user='gosuxin',
        **{'pass': suite['mysql_app']}, name='gosuxin_prod', type='mysql', prefix='gf_', debug=False)
    config['redis']['default'].update(address='127.0.0.1:6379', db=6)
    (config_dir / 'config.yaml').write_text(yaml.safe_dump(config, allow_unicode=True, sort_keys=False), encoding='utf-8')
    source_app = yaml.safe_load((HRM / 'manifest/config/app.yaml').read_text(encoding='utf-8'))
    source_app['app'].update(runEnv='release', loginCaptcha=True, SecretKey=suite['jwt'],
        allowurl='hr.szputy.com:9880,192.168.10.11:9880,im.szputy.com:9885,192.168.10.11:9885', superRoleAuth=False)
    (config_dir / 'app.yaml').write_text(yaml.safe_dump(source_app, allow_unicode=True, sort_keys=False), encoding='utf-8')
    upload = yaml.safe_load((HRM / 'manifest/config/upload.yaml').read_text(encoding='utf-8'))
    (config_dir / 'upload.yaml').write_text(yaml.safe_dump(upload, allow_unicode=True, sort_keys=False), encoding='utf-8')
    environment = {
        'TZ': 'Asia/Shanghai', 'GOSUXIN_REHEARSAL': 'false' if suite['startAt'] else 'true', 'GOSUXIN_IM_ENABLED': 'true',
        'GOSUXIN_HRM_PUBLIC_ADMINS_ONLY': 'false', 'GOSUXIN_IM_AUTO_DEPARTMENT_GROUPS': 'false',
        'GOSUXIN_HRM_PUBLIC_URL': 'https://hr.szputy.com:9880', 'GOSUXIN_HRM_LAN_URL': 'https://192.168.10.11:9880',
        'GOSUXIN_IM_PUBLIC_URL': 'https://im.szputy.com:9885', 'GOSUXIN_IM_LAN_URL': 'https://192.168.10.11:9885',
        'GOSUXIN_IM_BRIDGE_URL': 'http://127.0.0.1:10010', 'GOSUXIN_IM_HMAC_KEY': suite['bridge'],
        'GOSUXIN_IM_REDIS_URL': f"redis://:{im['redis']}@127.0.0.1:16379/6",
        'SUXIN_PLUGIN_DATA': '/srv/puty-suite/gosuxin/storage/plugins/biz-suite/data',
        'WECOM_CREDENTIAL_KEY': base64.b64encode(bytes.fromhex(suite['wecom'])).decode(), 'GOSUXIN_SYNC_START_AT': suite['startAt'],
    }
    # JSON is loaded by the supervisor launcher, never put in argv or logs.
    (DEST / 'gosuxin/runtime-environment.json').write_text(json.dumps(environment, indent=2) + '\n', encoding='utf-8')
    (DEST / 'gosuxin/initial-password.secret').write_text(suite['bootstrap'], encoding='utf-8')
    print(json.dumps({'hrmConfigReady': True, 'secretsPrinted': False}))


if __name__ == '__main__':
    main()
