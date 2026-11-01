import {build} from 'esbuild';import {mkdir,copyFile,readFile,writeFile,cp} from 'node:fs/promises';import {resolve} from 'node:path';
const dest=resolve('../../.cache/load-bundle');await mkdir(dest+'/static',{recursive:true});
await build({entryPoints:['tests/load-client.ts'],bundle:true,format:'esm',platform:'browser',outfile:dest+'/static/load-client.js'});
for(const name of ['openIM.wasm','sql-wasm.wasm','wasm_exec.js'])await copyFile('public/'+name,dest+'/static/'+name);
for(const name of ['worker.js','worker-legacy.js'])await copyFile('node_modules/@openim/wasm-client-sdk/lib/'+name,dest+'/static/'+name);
for(const name of ['load-host.html','load.html'])await writeFile(dest+'/static/'+name,(await readFile('tests/'+name,'utf8')).replace('/tests/load-client.ts','/load-client.js'));
for(const name of ['load-100.mjs','load-static-server.mjs'])await copyFile('tests/'+name,dest+'/'+name);
await mkdir(dest+'/node_modules',{recursive:true});for(const name of ['playwright','playwright-core'])await cp('node_modules/'+name,dest+'/node_modules/'+name,{recursive:true});
await writeFile(dest+'/package.json',JSON.stringify({type:'module',private:true,dependencies:{playwright:'1.64.1'}}));console.log(dest);
