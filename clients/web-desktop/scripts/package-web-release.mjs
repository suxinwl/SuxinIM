import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import AdmZip from 'adm-zip';
const client=path.resolve(import.meta.dirname,'..'),dist=path.join(client,'dist-web');
const version=JSON.parse(fs.readFileSync(path.join(client,'build-version.json'),'utf8')).version;
const brand=JSON.parse(fs.readFileSync(path.join(client,'../brand.json'),'utf8'));
for(const name of ['index.html','openIM.wasm','sql-wasm.wasm'])if(!fs.statSync(path.join(dist,name)).isFile())throw new Error('Missing Web distribution member '+name);
const digest=data=>crypto.createHash('sha256').update(data).digest('hex');
const zip=new AdmZip(),files={};
function add(directory,prefix=''){for(const name of fs.readdirSync(directory).sort()){const source=path.join(directory,name),member=prefix+name,info=fs.lstatSync(source);if(info.isSymbolicLink())throw new Error('No symlinks in public Web delivery');if(info.isDirectory())add(source,member+'/');else if(info.isFile()){const data=fs.readFileSync(source);zip.addFile(member,data);files[member]={sha256:digest(data),size:data.length};}}}
add(dist);const archive=brand.technicalName+'-'+version+'-web.zip',target=path.join(client,'release',archive);zip.writeZip(target);
const bytes=fs.readFileSync(target),manifest={version,createdAt:new Date().toISOString(),format:'dist contents at archive root',archive,sha256:digest(bytes),size:bytes.length,files};
fs.writeFileSync(target.replace(/\.zip$/,'.sha256.json'),JSON.stringify(manifest,null,2)+'\n');fs.writeFileSync(target+'.sha256',manifest.sha256+'  '+archive+'\n');console.log(JSON.stringify({archive,size:manifest.size,sha256:manifest.sha256,files:Object.keys(files).length}));

