// Offline renderer checks only. Does not render files, load credentials, or use HTTP.
import assert from 'node:assert/strict';
import {execFileSync} from 'node:child_process';
import {readFileSync} from 'node:fs';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'..');
const renderer=path.join(root,'tools','render_production_nginx.py');
const python=process.env.PYTHON_BIN || 'python3';
const code="import importlib.util,json,sys;s=importlib.util.spec_from_file_location('renderer',sys.argv[1]);m=importlib.util.module_from_spec(s);s.loader.exec_module(m);print(json.dumps([m.im('lan','192.168.10.11','puty-lan'),m.im('internet','im.szputy.com','puty-wan')]))";
const rendered=JSON.parse(execFileSync(python,['-c',code,renderer],{encoding:'utf8'}));
const expected=['Content-Type','Authorization','token','chatToken','platformID','operationID','X-Requested-With','Range','X-Amz-Date','X-Amz-Content-Sha256','X-Puty-Resource-Proof'];
let checks=0;
for(const configuration of rendered){
 const match=/add_header Access-Control-Allow-Headers '([^']+)' always;/.exec(configuration);
 assert.ok(match);checks++;
 assert.deepEqual(match[1].split(',').map(s=>s.trim()),expected);checks++;
 assert.match(configuration,/add_header Access-Control-Allow-Origin \$puty_cors always;/);checks++;
 assert.doesNotMatch(configuration,/Access-Control-Allow-Origin\s+['"]?\*/);checks++;
 assert.match(configuration,/location \/attachments\/ \{ proxy_request_buffering off; proxy_buffering off; proxy_pass http:\/\/127\.0\.0\.1:18090; \}/);checks++;
 assert.match(configuration,/client_max_body_size 1g;/);checks++;
 assert.match(configuration,/proxy_cache off;/);checks++;
 assert.match(configuration,/proxy_set_header Host \$http_host;/);checks++;
 assert.match(configuration,/if \(\$request_method = OPTIONS\) \{ return 204; \}/);checks++;
}
const source=readFileSync(renderer,'utf8');
const originMap=/map \$http_origin \$puty_cors \{([\s\S]*?)\n\}/.exec(source)?.[1];
assert.ok(originMap);checks++;
assert.equal(originMap.trim(),`default '';
 'https://192.168.10.11:9880' $http_origin;
 'https://192.168.10.11:9885' $http_origin;
 'https://hr.szputy.com:9880' $http_origin;
 'https://im.szputy.com:9885' $http_origin;
 'openim38://app' $http_origin;
 'http://127.0.0.1:7777' $http_origin;
 'http://127.0.0.1:7778' $http_origin;
 'null' 'null';`);checks++;
console.log(JSON.stringify({passed:true,checks,renderedProfiles:rendered.length,networkUsed:false,filesRendered:false}));
