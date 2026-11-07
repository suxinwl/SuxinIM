// Inspect final bytes in place. No archive extraction, credential output or writes
// to the public manifest. The admin password is compared only in RAM.
import fs from 'node:fs/promises';
import {createReadStream} from 'node:fs';
import path from 'node:path';
import {createHash} from 'node:crypto';
import {createRequire} from 'node:module';
import AdmZip from 'adm-zip';
const require=createRequire(import.meta.url),asar=require('@electron/asar');
const client=path.resolve(import.meta.dirname,'..'),suite=path.resolve(client,'../..');
const metadata=JSON.parse(await fs.readFile(path.join(client,'build-version.json'),'utf8'));
const brand=JSON.parse(await fs.readFile(path.join(suite,'clients/brand.json'),'utf8'));
const archive=path.join(client,'release/win-unpacked/resources/app.asar');
const privateNames=/(^|\/)(?:\.private|secrets\.json|test-accounts\.json|test-sessions\.json|key\.properties)(?:\/|$)|\.(?:key|jks|keystore|p12|pfx)$/i;
const privatePEM=/-----BEGIN ((?:[A-Z0-9]+ )*PRIVATE KEY(?: BLOCK)?)-----(?:[ \t\r\n]|\\[rn])+[A-Za-z0-9+/=][A-Za-z0-9+/= \t\r\n\\rn]{30,}?-----END \1-----/;
let knownSecret;
try{const value=JSON.parse(await fs.readFile(path.join(suite,'.private/production/admin-login.json'),'utf8'));if(typeof value.password==='string'&&value.password.length>=8)knownSecret=Buffer.from(value.password);}catch(error){if(error.code!=='ENOENT')throw error;}
const flags=[],checks=[];
function inspect(name,bytes){if(privateNames.test(name))flags.push({path:name,rule:'private-file-name'});if(privatePEM.test(bytes.toString('latin1')))flags.push({path:name,rule:'armored-private-key-body'});if(knownSecret&&bytes.includes(knownSecret))flags.push({path:name,rule:'current-admin-credential'});}
async function hash(file){const digest=createHash('sha256');for await(const chunk of createReadStream(file))digest.update(chunk);return digest.digest('hex');}
let members=0;
const member=name=>name.replace(/\//g,path.sep),extract=name=>asar.extractFile(archive,member(name));
for(const entry of asar.listPackage(archive)){const name=entry.replace(/\\/g,'/').replace(/^\//,'');const info=asar.statFile(archive,member(name),false);if(info.files)continue;if(info.link)throw new Error('Candidate package contains a link');inspect('app.asar/'+name,extract(name));members++;}
for(const name of ['dist-electron/main/index.js','dist/index.html','dist/openIM.wasm','dist/sql-wasm.wasm','dist/wasm_exec.js'])if(!extract(name).equals(await fs.readFile(path.join(client,name))))throw new Error('Packaged runtime differs from tested runtime: '+name);
if(JSON.parse(asar.extractFile(archive,'package.json').toString()).version!==metadata.version)throw new Error('Packaged version mismatch');
if(!extract('node_modules/@jlongster/sql.js/dist/sql-wasm.wasm').subarray(0,4).equals(Buffer.from([0,97,115,109])))throw new Error('Packaged legacy SQL dependency is missing');
checks.push({kind:'electron-asar',members,sha256:await hash(archive),runtimeMatchesTestedBuild:true,legacySQLDependencyPresent:true});
const web=path.join(client,`release/${brand.technicalName}-${metadata.version}-web.zip`),zip=new AdmZip(web);
for(const item of zip.getEntries())if(!item.isDirectory)inspect('web.zip/'+item.entryName,zip.readFile(item));
checks.push({kind:'web-zip',members:zip.getEntries().length,sha256:await hash(web)});
const installer=path.join(client,`release/${brand.technicalName}-${metadata.version}-win-x64.exe`);
checks.push({kind:'windows-installer',bytes:(await fs.stat(installer)).size,sha256:await hash(installer)});
knownSecret?.fill(0);
const report={passed:flags.length===0,version:metadata.version,build:metadata.build,privateArchivesExtracted:false,credentialValuesPrinted:false,knownAdminCredentialComparedInRAM:!!knownSecret,checks,flags};
await fs.writeFile(path.join(suite,'.private/production/acceptance/security-storage-20261104/pc-artifact-audit.json'),JSON.stringify(report,null,2));
console.log(JSON.stringify({passed:report.passed,version:report.version,checks:checks.length,flags:flags.length}));
if(!report.passed)process.exitCode=1;
