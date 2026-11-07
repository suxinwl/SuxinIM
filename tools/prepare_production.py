"""Render the fresh PUTY production stack without touching any existing stack."""
import contextlib
import io
import json
import pathlib
import secrets
import shutil

import yaml
from generate_instance import generate

ROOT = pathlib.Path(__file__).resolve().parents[1]
DEST = ROOT / ".private/production"
MYSQL_IMAGE = "mysql:8.0.46@sha256:62fb722c78b24245ddff1796a0fcee4a49cc5b87e0aaaf20c92d1da9e0a2497b"


def write_yaml(path, data):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(yaml.safe_dump(data, allow_unicode=True, sort_keys=False), encoding="utf-8")


def main():
    DEST.mkdir(parents=True, exist_ok=True)
    lock = DEST / "suite-secrets.json"
    if lock.exists():
        suite = json.loads(lock.read_text(encoding="utf-8"))
    else:
        suite = {key: secrets.token_hex(32) for key in ["mysql_root", "mysql_app", "bridge", "wecom", "bootstrap", "backup"]}
        suite.update(instanceID=secrets.token_hex(16), adminUsername="admin", startAt="")
        lock.write_text(json.dumps(suite, indent=2) + "\n", encoding="utf-8")
    im = DEST / "openim"
    with contextlib.redirect_stdout(io.StringIO()):
        generate(im, "192.168.10.11")
    keys = json.loads((im / "secrets.json").read_text())
    # Production keeps server messages and objects until an explicit delete.
    # Both expiry paths must remain disabled across future regenerations.
    retention = yaml.safe_load((im / "config/openim-server/openim-crontask.yml").read_text())
    retention.update(retainChatRecords=0, fileExpireTime=0, deleteObjectType=[])
    write_yaml(im / "config/openim-server/openim-crontask.yml", retention)
    compose = yaml.safe_load((im / "compose.yaml").read_text())
    compose["name"] = "puty-suite-prod"
    compose["networks"] = {"openim38": {"name": "puty-suite-prod-net", "driver": "bridge", "ipam": {"config": [{"subnet": "172.30.38.0/24", "gateway": "172.30.38.1", "ip_range": "172.30.38.128/25"}]}}}
    services = compose["services"]
    services.pop("nginx")
    redis_url = f"redis://:{keys['redis']}@redis:6379/2"
    for service in services.values():
        service.setdefault("labels", {}).update({"com.puty.suite": suite["instanceID"], "com.puty.environment": "production"})
    common = {"HRM_SSO_REQUIRED": "true", "HRM_REDIS_URL": redis_url, "HRM_BRIDGE_HMAC_KEY": suite["bridge"]}
    for name, service in services.items():
        if name not in ["mongo", "redis", "etcd", "kafka", "minio", "livekit"]:
            service.setdefault("environment", {}).update(common)
            service["environment"]["HRM_ACCESS_CHECK_URL"] = "http://enterprise-service:10010/internal/hrm/access/check"
    services["redis"]["ports"] = ["127.0.0.1:16379:6379"]
    services["openim-api"]["ports"] = ["127.0.0.1:10002:10002"]
    services["openim-msggateway"]["ports"] = ["127.0.0.1:10001:10001"]
    services["chat-api"]["ports"] = ["127.0.0.1:10008:10008"]
    for ingress in ['openim-api', 'openim-msggateway', 'chat-api']:
        services[ingress]['environment'].update(TRUSTED_ENTRY_PROXY_CIDRS='172.30.38.1/32',
            PUBLIC_LAN_ORIGIN='https://192.168.10.11:9885', PUBLIC_INTERNET_ORIGIN='https://im.szputy.com:9885')
    services["minio"]["ports"] = ["127.0.0.1:19000:9000"]
    services["minio"]["environment"]["MINIO_API_CORS_ALLOW_ORIGIN"] = "https://192.168.10.11:9885,https://im.szputy.com:9885"
    enterprise = services["enterprise-service"]
    enterprise["networks"] = {"openim38": {"ipv4_address": "172.30.38.10"}}
    enterprise["ports"] = ["127.0.0.1:18090:18090", "127.0.0.1:10010:10010"]
    # Measure the actual owned data volumes without reading their contents.
    # The operator verifies Docker Mounts/volume ownership before marking them
    # authoritative; fresh templates must never claim an unverified data disk.
    enterprise.setdefault("volumes", []).extend(["mongo-data:/capacity/mongo:ro", "minio-data:/capacity/minio:ro"])
    enterprise["environment"].update({"HRM_BRIDGE_LISTEN": "0.0.0.0:10010", "LIVEKIT_PUBLIC_URL": "wss://im.szputy.com:9885/livekit",
        "LIVEKIT_LAN_URL": "wss://192.168.10.11:9885/livekit", "IM_PUBLIC_URL": "https://im.szputy.com:9885",
        "IM_LAN_URL": "https://192.168.10.11:9885", "MINIO_INTERNAL_URL": "http://minio:9000",
        "PUBLIC_LAN_ORIGIN": "https://192.168.10.11:9885", "PUBLIC_INTERNET_ORIGIN": "https://im.szputy.com:9885",
        "OBJECT_STORAGE_UPSTREAM": "http://minio:9000", "TRUSTED_ENTRY_PROXY_CIDRS": "172.30.38.1/32",
        "HRM_ACCESS_CHECK_URL": "http://172.30.38.1:18001/internal/im-access/check",
        "OPENIM_MONGO_DATABASE": "openim38", "OBJECT_STORAGE_ACCESS_KEY_ID": "root",
        "OBJECT_STORAGE_SECRET_ACCESS_KEY": keys["minio"], "OBJECT_STORAGE_REGION": "us-east-1",
        "HRM_RESOURCE_UPSTREAM": "http://172.30.38.1:18001",
        "PUBLIC_HRM_LAN_ORIGIN": "https://192.168.10.11:9880", "PUBLIC_HRM_INTERNET_ORIGIN": "https://hr.szputy.com:9880",
        "ALLOWED_UPDATE_HOSTS": "192.168.10.11,im.szputy.com", "STORAGE_CAPACITY_DATA_VERIFIED": "false", "TZ": "Asia/Shanghai"})
    services["livekit"]["ports"] = ["192.168.10.11:27881:27881/tcp", "192.168.10.11:27882:27882/udp", "192.168.10.11:23478:23478/udp", "127.0.0.1:15349:25349/tcp"]
    # Host networking lets the SFU advertise the actual LAN interface alongside
    # its NAT public address. Its HTTP control plane stays loopback-only.
    services["livekit"].pop("ports")
    services["livekit"].pop("networks")
    services["livekit"]["network_mode"] = "host"
    enterprise["environment"]["LIVEKIT_HTTP_URL"] = "http://172.30.38.1:17880"
    services["mysql"] = {"image": MYSQL_IMAGE, "restart": "unless-stopped", "pull_policy": "never",
        "environment": {"MYSQL_ROOT_PASSWORD": suite["mysql_root"], "MYSQL_DATABASE": "gosuxin_prod", "MYSQL_USER": "gosuxin", "MYSQL_PASSWORD": suite["mysql_app"], "TZ": "Asia/Shanghai"},
        "command": ["--character-set-server=utf8mb4", "--collation-server=utf8mb4_unicode_ci"],
        "ports": ["127.0.0.1:13306:3306"], "volumes": ["mysql-data:/var/lib/mysql"], "networks": ["openim38"],
        "labels": {"com.puty.suite": suite["instanceID"], "com.puty.environment": "production"},
        "healthcheck": {"test": ["CMD-SHELL", 'MYSQL_PWD="$$MYSQL_ROOT_PASSWORD" mysql -h 127.0.0.1 -uroot --batch --skip-column-names -e "SELECT 1"'], "interval": "10s", "timeout": "5s", "retries": 30},
        "logging": {"driver": "json-file", "options": {"max-size": "20m", "max-file": "3"}}}
    compose["volumes"]["mysql-data"] = {}
    for definition in compose["volumes"].values():
        definition["labels"] = {"com.puty.suite": suite["instanceID"]}
    write_yaml(im / "compose.yaml", compose)
    minio = yaml.safe_load((im / "config/openim-server/minio.yml").read_text())
    minio["externalAddress"] = "https://im.szputy.com:9885"
    write_yaml(im / "config/openim-server/minio.yml", minio)
    livekit = yaml.safe_load((im / "config/livekit.yml").read_text())
    livekit["rtc"].update(use_external_ip=False, node_ip="61.144.183.217", advertise_internal_ip=True)
    livekit['bind_addresses'] = ['127.0.0.1']
    livekit['rtc']['ips'] = {'includes': ['192.168.10.11/32']}
    livekit['webhook']['urls'] = ['http://127.0.0.1:18090/internal/livekit/webhook']
    livekit["turn"].update(domain="im.szputy.com", cert_file="/certs/im.crt", key_file="/certs/im.key",
        tls_port=15349, advertised_tls_port=25349, bind_addresses=['0.0.0.0'], proxy_protocol=True,
        proxy_protocol_trusted_cidrs=['127.0.0.1/32'])
    write_yaml(im / "config/livekit.yml", livekit)
    (im / "instance.json").write_text(json.dumps({"host": "192.168.10.11", "production": True, "testOnly": False,
        "stackName": "puty-suite-prod", "instanceID": suite["instanceID"], "dataPolicy": "fresh-business-data",
        "root": "/srv/puty-suite/openim"}, indent=2) + "\n", encoding="utf-8")
    certs = im / "certs"
    for name in ["hr", "im"]:
        for suffix in ["crt", "key"]:
            shutil.copy2(pathlib.Path("E:/PUTY_AI/puty_ssl/state/production/certificates") / f"{name}.szputy.com.{suffix}", certs / f"{name}.{suffix}")
    for suffix in ["crt", "key"]:
        shutil.copy2(ROOT / ".private/production-ca" / f"lan-server.{suffix}", certs / f"lan.{suffix}")
    shutil.copy2(ROOT / "deploy/certificates/Puty-Enterprise-Root-CA.crt", certs / "puty-root.crt")
    (DEST / "operator-credentials.txt").write_text("HRM administrator: " + suite["adminUsername"] + "\nInitial password: " + suite["bootstrap"] + "\n", encoding="utf-8")
    print(json.dumps({"prepared": str(DEST), "stack": "puty-suite-prod", "secretsPrinted": False}))


if __name__ == "__main__":
    main()
