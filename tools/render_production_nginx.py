"""Render IP/domain HTTPS and TURN entrypoints for the existing BaoTa Nginx."""
import json
import pathlib

ROOT = pathlib.Path(__file__).resolve().parents[1]
OUT = ROOT / '.private/production/nginx'
CERTS = '/srv/puty-suite/openim/certs'


def access_zone_maps():
    # Only the configured IP authority on its real TLS entry may be internal.
    # Domains remain internet even when reached by a private source address.
    return '''geo $puty_private_source {
 default 0;
 10.0.0.0/8 1;
 172.16.0.0/12 1;
 192.168.0.0/16 1;
 127.0.0.0/8 1;
 ::1 1;
 fc00::/7 1;
}
map "$http_host|$server_port|$ssl_server_name" $puty_ip_authority {
 default 0;
 "192.168.10.11:9880|9880|" 1;
 "192.168.10.11:9880|9880|192.168.10.11" 1;
 "192.168.10.11:9885|9885|" 1;
 "192.168.10.11:9885|9885|192.168.10.11" 1;
}
map "$puty_private_source:$puty_ip_authority" $puty_access_zone {
 default internet;
 "1:1" lan;
}
'''


def headers(entry):
    return f'''proxy_http_version 1.1;
 proxy_cache off;
 proxy_set_header Host $http_host;
 proxy_set_header X-Real-IP $remote_addr;
 proxy_set_header X-GoSuxin-Client-IP $remote_addr;
 proxy_set_header X-Forwarded-For $remote_addr;
 proxy_set_header X-Forwarded-Proto https;
 proxy_set_header X-Forwarded-Host $http_host;
 proxy_set_header X-Puty-Entry {entry};
 proxy_set_header X-Puty-Access-Zone $puty_access_zone;
 proxy_set_header Upgrade $http_upgrade;
 proxy_set_header Connection $puty_connection;
 proxy_read_timeout 3600s;
 proxy_send_timeout 3600s;'''


def hrm(entry, host, certificate):
    default = ' default_server' if entry == 'lan' else ''
    return f'''server {{
 listen 9880 ssl{default};
 server_name {host};
 ssl_certificate {CERTS}/{certificate}.crt;
 ssl_certificate_key {CERTS}/{certificate}.key;
 ssl_protocols TLSv1.2 TLSv1.3;
 access_log /srv/puty-suite/openim/logs/nginx-access.log puty_private;
 error_log /srv/puty-suite/openim/logs/nginx-error.log crit;
 client_max_body_size 1g;
 add_header X-Content-Type-Options nosniff always;
 {headers(entry)}
 location = / {{ return 302 /suxinweb/; }}
 location /suxinweb/ {{ root /srv/puty-suite/gosuxin/resource; try_files $uri $uri/ /suxinweb/index.html; }}
 location /resource/static/brand/ {{ alias /srv/puty-suite/gosuxin/resource/static/brand/; add_header Cross-Origin-Resource-Policy cross-origin always; add_header X-Content-Type-Options nosniff always; }}
 location = /_puty_hrm_asset_auth {{
  internal;
  proxy_pass http://127.0.0.1:18000/internal/hrm-assets/check;
  proxy_method GET;
  proxy_pass_request_body off;
  proxy_set_header Content-Length "";
  proxy_set_header Authorization "";
  proxy_set_header X-Original-URI $request_uri;
  proxy_set_header X-Original-Authorization $http_authorization;
  proxy_set_header X-GoSuxin-Client-IP $remote_addr;
  proxy_set_header Cookie $http_cookie;
  proxy_set_header Host $http_host;
  proxy_set_header X-Puty-Entry {entry};
  proxy_set_header X-Puty-Access-Zone $puty_access_zone;
 }}
 location ^~ /resource/uploads/ {{
  auth_request /_puty_hrm_asset_auth;
  alias /srv/puty-suite/gosuxin/resource/uploads/;
  autoindex off;
  add_header Cache-Control "private, no-store" always;
 }}
 location ^~ /internal/ {{ return 404; }}
 location / {{ proxy_pass http://127.0.0.1:18000; }}
}}
'''


def im(entry, host, certificate):
    default = ' default_server' if entry == 'lan' else ''
    hrm_url = 'https://192.168.10.11:9880' if entry == 'lan' else 'https://hr.szputy.com:9880'
    origin = 'https://192.168.10.11:9885' if entry == 'lan' else 'https://im.szputy.com:9885'
    return f'''server {{
 listen 9885 ssl{default};
 server_name {host};
 ssl_certificate {CERTS}/{certificate}.crt;
 ssl_certificate_key {CERTS}/{certificate}.key;
 ssl_protocols TLSv1.2 TLSv1.3;
 root /srv/puty-suite/openim/www;
 access_log /srv/puty-suite/openim/logs/nginx-access.log puty_private;
 error_log /srv/puty-suite/openim/logs/nginx-error.log crit;
 client_max_body_size 1g;
 proxy_hide_header Access-Control-Allow-Origin;
 proxy_hide_header Access-Control-Allow-Methods;
 proxy_hide_header Access-Control-Allow-Headers;
 proxy_hide_header Access-Control-Allow-Credentials;
 {headers(entry)}
 proxy_set_header X-Request-Api {origin}/api;
 add_header Cross-Origin-Opener-Policy same-origin always;
 add_header Cross-Origin-Embedder-Policy require-corp always;
 add_header Cross-Origin-Resource-Policy cross-origin always;
 add_header X-Content-Type-Options nosniff always;
 add_header Access-Control-Allow-Origin $puty_cors always;
 add_header Access-Control-Allow-Methods 'GET, HEAD, POST, PUT, DELETE, OPTIONS' always;
 add_header Access-Control-Allow-Headers 'Content-Type, Authorization, token, chatToken, platformID, operationID, X-Requested-With, Range, X-Amz-Date, X-Amz-Content-Sha256, X-Puty-Resource-Proof' always;
 add_header Access-Control-Expose-Headers 'ETag, Content-Disposition, Content-Length, Content-Range, Accept-Ranges' always;
 add_header Vary Origin always;
 add_header Content-Disposition $puty_apk_disposition;
 if ($request_method = OPTIONS) {{ return 204; }}
 location = /healthz {{ access_log off; return 200 'puty-suite-prod\\n'; }}
 location ^~ /internal/ {{ return 404; }}
 location ^~ /api/auth/get_admin_token {{ return 403; }}
 location ^~ /api/auth/get_user_token {{ return 403; }}
 location = /api/user/user_register {{ return 403; }}
 location ^~ /api/msg/delete_msg_phsical_by_seq {{ return 403; }}
 location ^~ /api/msg/delete_msg_physical {{ return 403; }}
 location ^~ /enterprise/v1/admin/ {{ return 403; }}
 location ^~ /openim38/ {{ return 403; }}
 location = /openim38 {{ return 403; }}
 location ^~ /admin/ {{ return 302 {hrm_url}/suxinweb/communication; }}
 location /sso/ {{ proxy_pass http://127.0.0.1:18000/common/im-sso/; }}
 location /attachments/ {{ proxy_request_buffering off; proxy_buffering off; proxy_pass http://127.0.0.1:18090; }}
 location /enterprise/v1/ {{ proxy_pass http://127.0.0.1:18090; }}
 location /office/ {{ proxy_pass http://127.0.0.1:18090; }}
 location /signaling/ {{ proxy_pass http://127.0.0.1:18090; }}
 location /rtc/ {{ proxy_pass http://127.0.0.1:18090; }}
 location /livekit/ {{ proxy_pass http://127.0.0.1:18090; }}
 location = /livekit {{ proxy_pass http://127.0.0.1:18090; }}
 location = /api/msg/mark_group_message_read {{ proxy_pass http://127.0.0.1:18090/msg/mark_group_message_read; }}
 location = /api/msg/get_group_message_read_num {{ proxy_pass http://127.0.0.1:18090/msg/get_group_message_read_num; }}
 location = /api/msg/get_group_message_has_read {{ proxy_pass http://127.0.0.1:18090/msg/get_group_message_has_read; }}
 location /api/ {{ proxy_pass http://127.0.0.1:10002/; }}
 location /chat/ {{ proxy_pass http://127.0.0.1:10008/; }}
 location = /ws {{ proxy_pass http://127.0.0.1:10001/; }}
 location /ws/ {{ proxy_pass http://127.0.0.1:10001/; }}
 location /resource/static/brand/ {{ alias /srv/puty-suite/gosuxin/resource/static/brand/; }}
 location /resource/uploads/ {{ proxy_pass http://127.0.0.1:18090; }}
 location = /downloads {{ return 301 /downloads/; }}
 location = /downloads/ {{ try_files /downloads/index.html =404; }}
 location ~* ^/downloads/[A-Za-z0-9_.-]+\\.apk$ {{
  types {{ application/vnd.android.package-archive apk; }}
  autoindex off;
  try_files $uri =404;
 }}
 location /downloads/ {{ autoindex off; try_files $uri =404; }}
 location / {{ try_files $uri $uri/ /index.html; }}
}}
'''


def main():
    OUT.mkdir(parents=True, exist_ok=True)
    common = '''map $http_upgrade $puty_connection { default upgrade; '' close; }
map $uri $puty_log_uri { default $uri; ~^/attachments/ /attachments/[redacted]; }
# APK downloads retain their original file bytes; no header is attached to errors.
map "$uri|$status" $puty_apk_disposition {
 default '';
 ~*^/downloads/(?<puty_apk_filename>[A-Za-z0-9_.-]+\\.apk)\\|(200|206|304)$ 'attachment; filename="$puty_apk_filename"';
}
log_format puty_private '$remote_addr $request_method $puty_log_uri $status $body_bytes_sent';
map $http_origin $puty_cors {
 default '';
 'https://192.168.10.11:9880' $http_origin;
 'https://192.168.10.11:9885' $http_origin;
 'https://hr.szputy.com:9880' $http_origin;
 'https://im.szputy.com:9885' $http_origin;
 'openim38://app' $http_origin;
 'http://127.0.0.1:7777' $http_origin;
 'http://127.0.0.1:7778' $http_origin;
 'null' 'null';
}
'''
    internal = '''server {
 listen 172.30.38.1:18001;
 server_name _;
 allow 172.30.38.10;
 deny all;
 access_log off;
 proxy_cache off;
 location = /internal/im-access/check {
  proxy_set_header Host 127.0.0.1:18000;
  proxy_pass http://127.0.0.1:18000;
 }
 location /resource/uploads/ {
  alias /srv/puty-suite/gosuxin/resource/uploads/;
  autoindex off;
 }
 location / { return 404; }
}
server {
 listen 172.30.38.1:17880;
 server_name _;
 allow 172.30.38.10;
 deny all;
 access_log off;
 proxy_cache off;
 location / { proxy_http_version 1.1; proxy_set_header Upgrade $http_upgrade; proxy_set_header Connection $puty_connection; proxy_pass http://127.0.0.1:7880; proxy_read_timeout 3600s; }
}
'''
    http = common + access_zone_maps() + hrm('lan', '192.168.10.11', 'lan') + hrm('internet', 'hr.szputy.com', 'hr') + im('lan', '192.168.10.11', 'lan') + im('internet', 'im.szputy.com', 'im') + internal
    (OUT / 'puty-suite.conf').write_text(http, encoding='utf-8')
    stream = f'''map $ssl_server_name $puty_turn_cert {{
 default {CERTS}/lan.crt;
 im.szputy.com {CERTS}/im.crt;
}}
map $ssl_server_name $puty_turn_key {{
 default {CERTS}/lan.key;
 im.szputy.com {CERTS}/im.key;
}}
server {{
 listen 25349 ssl;
 ssl_certificate $puty_turn_cert;
 ssl_certificate_key $puty_turn_key;
 ssl_protocols TLSv1.2 TLSv1.3;
 proxy_pass 127.0.0.1:15349;
 proxy_ssl on;
 proxy_ssl_verify on;
 proxy_ssl_verify_depth 4;
 proxy_ssl_server_name on;
 proxy_ssl_name im.szputy.com;
 proxy_protocol on;
 proxy_ssl_trusted_certificate /etc/ssl/certs/ca-certificates.crt;
 proxy_connect_timeout 10s;
 proxy_timeout 3600s;
}}
server {{
 listen 23478;
 proxy_pass 127.0.0.1:15349;
 proxy_ssl on;
 proxy_ssl_verify on;
 proxy_ssl_verify_depth 4;
 proxy_ssl_server_name on;
 proxy_ssl_name im.szputy.com;
 proxy_protocol on;
 proxy_ssl_trusted_certificate /etc/ssl/certs/ca-certificates.crt;
 proxy_connect_timeout 10s;
 proxy_timeout 3600s;
}}
'''
    (OUT / 'puty-turn.conf').write_text(stream, encoding='utf-8')
    print(json.dumps({'nginxConfigReady': True, 'directory': str(OUT)}))


if __name__ == '__main__':
    main()
