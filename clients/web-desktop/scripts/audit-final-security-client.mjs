// Read final bytes in place, only after the operator explicitly supplies --run.

// No extraction, execution, network, install, build or public-manifest writes.

import fs from 'node:fs/promises';

import {createReadStream} from 'node:fs';

import path from 'node:path';

import {createHash,X509Certificate} from 'node:crypto';

import {createRequire} from 'node:module';

import {fileURLToPath} from 'node:url';

import {spawnSync} from 'node:child_process';

const require=createRequire(import.meta.url),asar=require('@electron/asar'),AdmZip=require('adm-zip');

const CLIENT=path.resolve(import.meta.dirname,'..'),SUITE=path.resolve(CLIENT,'../..');

const PRIVATE=path.join(SUITE,'.private/production');

const digest=bytes=>createHash('sha256').update(bytes).digest('hex');

const reject=code=>{throw Object.assign(new Error(code),{auditCode:code});};

const ensure=(value,code)=>{if(!value)reject(code);};

export function memberName(value){ensure(typeof value==='string'&&value.length>0&&!value.includes('\\')&&!value.startsWith('/')&&!value.includes(':')&&!/[\x00-\x1f\x7f]/.test(value)&&value.split('/').every(v=>v&&v!=='.'&&v!=='..'),'UNSAFE_ARCHIVE_MEMBER');return value;}

const privateNames=/(^|\/)(?:\.private|\.git|\.ssh|secrets\.json|suite-secrets\.json|test-accounts\.json|test-sessions\.json|key\.properties|master\.wrap|recovery\.putykey|\.env(?:\.[^/]*)?)(?:\/|$)|\.(?:key|jks|keystore|p12|pfx)$/i;

const privatePEM=/-----BEGIN ((?:[A-Z0-9]+ )*PRIVATE KEY(?: BLOCK)?)-----(?:[ \t\r\n]|\\[rn])+[A-Za-z0-9+/=][A-Za-z0-9+/= \t\r\n\\rn]{30,}?-----END \1-----/;

export function inspect(name,bytes,patterns=[]){const flags=[];if(privateNames.test(name))flags.push('private-file-name');if(privatePEM.test(bytes.toString('latin1')))flags.push('private-key-body');if(patterns.some(value=>bytes.includes(value)))flags.push('actual-production-secret');return flags;}

export function sameFiles(expected,actual){ensure(expected.size===actual.size,'MEMBER_SET_MISMATCH');for(const [name,value]of expected)ensure(actual.has(name)&&actual.get(name).sha256===value.sha256&&actual.get(name).bytes===value.bytes,'MEMBER_BYTES_MISMATCH');}

async function hashFile(file){const before=await fs.lstat(file);ensure(before.isFile()&&!before.isSymbolicLink(),'REGULAR_FILE_REQUIRED');const hash=createHash('sha256');for await(const bytes of createReadStream(file))hash.update(bytes);const after=await fs.lstat(file);ensure(before.ino===after.ino&&before.size===after.size&&before.mtimeMs===after.mtimeMs,'FILE_CHANGED_DURING_AUDIT');return {sha256:hash.digest('hex'),bytes:after.size};}

function peIdentity(file,description){const command="$ErrorActionPreference='Stop';[Console]::OutputEncoding=New-Object Text.UTF8Encoding($false);$p=[Console]::In.ReadToEnd()|ConvertFrom-Json;$v=[Diagnostics.FileVersionInfo]::GetVersionInfo($p);[Console]::Out.Write((@{productName=$v.ProductName;description=$v.FileDescription;productVersion=$v.ProductVersion;fileVersion=$v.FileVersion}|ConvertTo-Json -Compress))";const value=spawnSync('C:\\Windows\\System32\\WindowsPowerShell\\v1.0\\powershell.exe',['-NoLogo','-NoProfile','-NonInteractive','-Command',command],{input:JSON.stringify(file),encoding:'utf8',windowsHide:true,timeout:30000,maxBuffer:8192});ensure(value.status===0,'PE_IDENTITY_READ_FAILED');const result=JSON.parse(value.stdout);ensure(result.productName==='PUTY内部通讯'&&result.description===description&&/^3\.8\.19(?:\.0)?$/.test(result.productVersion),'PE_PRODUCT_IDENTITY_MISMATCH');return result;}

async function tree(directory){const result=new Map();async function walk(base,prefix=''){for(const name of await fs.readdir(base)){const file=path.join(base,name),s=await fs.lstat(file),relative=prefix+name;ensure(!s.isSymbolicLink(),'LOCAL_TREE_LINK_REJECTED');if(s.isDirectory())await walk(file,relative+'/');else{memberName(relative);result.set(relative,await hashFile(file));}}}await walk(directory);return result;}

const json=async file=>JSON.parse((await fs.readFile(file,'utf8')).replace(/^\uFEFF/,''));

async function secretPatterns(){const values=[];function add(value,minimum=16){if(typeof value!=='string'||value.length<minimum)return;values.push(Buffer.from(value));if(/^(?:[0-9a-fA-F]{2})+$/.test(value))values.push(Buffer.from(Buffer.from(value,'hex').toString('base64')));}

  for(const name of ['suite-secrets.json','openim/secrets.json'])for(const [key,value]of Object.entries(await json(path.join(PRIVATE,name))))if(!['instanceID','startAt','adminUsername'].includes(key)){add(value);if(key==='bootstrap'&&typeof value==='string')values.push(Buffer.from(createHash('md5').update(value).digest('hex')));}

  const runtime=await json(path.join(PRIVATE,'gosuxin/runtime-environment.json'));add(runtime.WECOM_CREDENTIAL_KEY);

  const admin=await json(path.join(PRIVATE,'admin-login.json'));if(typeof admin.password==='string'&&admin.password.length>=8){add(admin.password,8);values.push(Buffer.from(createHash('md5').update(admin.password).digest('hex')));}

  const base=path.join(PRIVATE,'key-rotation');for(const name of await fs.readdir(base)){if(!/^rotate-/.test(name))continue;const file=path.join(base,name,'bundle.json');const bundle=await json(file);ensure(bundle.format==='puty-runtime-key-rotation-v1','UNEXPECTED_PRIVATE_BUNDLE_FORMAT');for(const side of ['old','new'])for(const value of Object.values(bundle[side]))add(value);}

  return values;

}

export async function auditWeb({compareSecrets=false}={}){

  const patterns=compareSecrets?await secretPatterns():[],flags=[],web=path.join(CLIENT,'release/PUTY_IM-3.8.13-web.zip'),before=await hashFile(web),zip=new AdmZip(web),actual=new Map();

  ensure(before.sha256==='21d2d2d92ec8b9bdd3a6ba969a549df71f66665db6a9012aac6fd04e0cd4d18f','FINAL_WEB_SHA_MISMATCH');

  for(const item of zip.getEntries()){const name=memberName(item.entryName.replace(/\/$/,''));if(item.isDirectory){const directory=await fs.lstat(path.join(CLIENT,'dist-web',name));ensure(directory.isDirectory()&&!directory.isSymbolicLink(),'UNEXPECTED_ZIP_DIRECTORY');continue;}ensure(!actual.has(name)&&item.header.size<=256*1024*1024,'ZIP_ENTRY_REJECTED');const bytes=zip.readFile(item);for(const rule of inspect(name,bytes,patterns))flags.push({scope:'web',fileSHA256:digest(Buffer.from(name)),rule});actual.set(name,{sha256:digest(bytes),bytes:bytes.length});}

  sameFiles(await tree(path.join(CLIENT,'dist-web')),actual);

  const workers=[];for(const name of ['worker.js','worker-legacy.js']){const built=await fs.readFile(path.join(SUITE,'sdks/wasm/lib',name)),installed=await fs.readFile(path.join(CLIENT,'node_modules/@openim/wasm-client-sdk/lib',name));ensure(built.equals(installed)&&built.includes(Buffer.from('puty_local_deleted'))&&built.includes(Buffer.from('puty_archive_outbox')),'INSTALLED_SDK_WORKER_STALE');const sha=digest(built);ensure([...actual].filter(([n,v])=>n.startsWith('assets/')&&v.sha256===sha).length===1,'WEB_NEW_SDK_WORKER_MISSING');workers.push({name,sha256:sha,matched:true});}

  const sdk=JSON.parse(zip.readFile('sdk-assets.json'));for(const item of sdk.assets)ensure(digest(await fs.readFile(path.join(SUITE,'sdks/wasm/assets',memberName(item.file))))===item.sha256&&actual.get(item.file)?.sha256===item.sha256,'WEB_WASM_MISMATCH');

  const index=zip.readFile('index.html').toString();ensure(index.includes('<title>PUTY内部通讯</title>')&&index.includes('image/png'),'WEB_BRAND_MISMATCH');for(const name of ['favicon-32x32.png','icons/icon.png'])ensure(actual.get(name)?.sha256===digest(await fs.readFile(path.join(CLIENT,'public',name))),'WEB_ICON_MISMATCH');

  const after=await hashFile(web);ensure(before.sha256===after.sha256,'FINAL_WEB_CHANGED');const report={format:'puty-independent-final-web-audit-v1',createdAt:new Date().toISOString(),passed:flags.length===0,version:'3.8.13',build:381301,privateArchivesExtracted:false,productionRequests:0,credentialValuesPrintedOrPersisted:false,knownSecretPatternsComparedInRAM:patterns.length,flags,artifacts:{web:{...after,members:actual.size,indexSHA256:actual.get('index.html').sha256}},exactDistWebMatched:true,exactSuppressionWorkers:workers,goWasmAssetFilesMatched:sdk.assets.length};patterns.forEach(v=>v.fill(0));return report;

}

export async function audit({compareSecrets=false}={}){

  const metadata=await json(path.join(CLIENT,'build-version.json')),pkg=await json(path.join(CLIENT,'package.json')),builder=await json(path.join(CLIENT,'electron-builder.json5'));

  ensure(metadata.version==='3.8.19'&&metadata.build===381901&&pkg.version===metadata.version&&pkg.name==='openim-enterprise-upgrade'&&pkg.main==='dist-electron/main/index.js','APPLICATION_IDENTITY_CHANGED');

  ensure(builder.appId==='io.openim.enterprise.upgrade38'&&builder.productName==='PUTY内部通讯'&&builder.executableName==='PUTY_IM'&&builder.nsis.deleteAppDataOnUninstall===false,'INSTALLER_IDENTITY_CHANGED');

  const patterns=compareSecrets?await secretPatterns():[],flags=[],archive=path.join(CLIENT,'release/win-unpacked/resources/app.asar'),before=await hashFile(archive),packaged=new Map(),packagedHashes=new Map();let members=0;

  const extract=name=>asar.extractFile(archive,path.normalize(name));

  function scan(scope,name,bytes){for(const rule of inspect(name,bytes,patterns))flags.push({scope,fileSHA256:digest(Buffer.from(name)),rule});}

  for(const rawName of asar.listPackage(archive)){const name=rawName.replace(/\\/g,'/').replace(/^\//,'');memberName(name);const info=asar.statFile(archive,path.normalize(name),false);ensure(!info.link,'ASAR_LINK_REJECTED');if(info.files)continue;ensure(!packagedHashes.has(name)&&info.size<=256*1024*1024,'ASAR_FILE_REJECTED');const bytes=extract(name);ensure(bytes.length===info.size,'ASAR_SIZE_MISMATCH');scan('asar',name,bytes);packagedHashes.set(name,{sha256:digest(bytes),bytes:bytes.length});members++;}

  for(const directory of ['dist-electron','dist']){const expected=await tree(path.join(CLIENT,directory)),actual=new Map([...packagedHashes].filter(([name])=>name.startsWith(directory+'/')).map(([name,value])=>[name.slice(directory.length+1),value]));sameFiles(expected,actual);packaged.set(directory,{files:expected.size,exact:true});}

  const embedded=JSON.parse(extract('package.json'));ensure(embedded.version===metadata.version&&embedded.name===pkg.name&&embedded.main===pkg.main,'ASAR_PACKAGE_IDENTITY_MISMATCH');

  const main=extract('dist-electron/main/index.js').toString();for(const marker of ['PUTY-RESTORE-TRANSACTION-1','recoverRestoreTransaction','collectRestorePrevious','restorationPolicy','puty_local_deleted'])ensure(main.includes(marker),'NEW_MAIN_RUNTIME_MISSING');

  const requiredSources=['electron/main/index.ts','electron/main/storeManage.ts','electron/main/shortcutManage.ts','electron/main/screenshotShortcut.ts','electron/main/screenshotsManage.ts','electron/main/screenshotSession.ts','src/utils/screenshot-shortcut.ts','electron/main/localStore/bootstrap.ts','electron/main/localStore/runtime.ts','electron/main/localStore/legacyCoordinator.ts','electron/main/localStore/legacyFailure.ts','electron/main/localStore/legacyQuarantine.ts','electron/main/localStore/legacy.ts','electron/main/localStore/vault.ts','electron/main/localStore/backup.ts','electron/main/localStore/restorePolicy.ts','electron/main/localStore/restoreTransaction.ts','electron/preload/index.ts'];let matchedSources=0;

  for(const target of ['main','preload']){const name='dist-electron/'+target+'/index.js.map',map=JSON.parse(extract(name));for(const source of requiredSources.filter(v=>v.startsWith('src/')?target==='main':v.includes('/'+target+'/'))){let matched=false;for(let i=0;i<map.sources.length;i++){const resolved=path.resolve(CLIENT,path.dirname(name),map.sources[i]);if(resolved===path.join(CLIENT,source)){ensure(typeof map.sourcesContent[i]==='string'&&digest(Buffer.from(map.sourcesContent[i]))===digest(await fs.readFile(resolved)),'MAIN_SOURCE_MAP_MISMATCH');matched=true;}}ensure(matched,'MAIN_SOURCE_MAP_INPUT_MISSING');matchedSources++;}}

  ensure(extract('node_modules/@jlongster/sql.js/dist/sql-wasm.wasm').subarray(0,4).equals(Buffer.from([0,97,115,109])),'LEGACY_SQL_DEPENDENCY_MISSING');

  const SDK=path.join(SUITE,'sdks/wasm'),installed=path.join(CLIENT,'node_modules/@openim/wasm-client-sdk');const sdkWorkers=[];

  for(const file of ['worker.js','worker-legacy.js']){const built=await fs.readFile(path.join(SDK,'lib',file)),used=await fs.readFile(path.join(installed,'lib',file));ensure(built.equals(used)&&used.includes(Buffer.from('puty_local_deleted')),'INSTALLED_SDK_WORKER_STALE');const sha=digest(used),matches=[...packagedHashes].filter(([name,v])=>name.startsWith('dist/assets/')&&v.sha256===sha);ensure(matches.length===1,'PACKAGED_SDK_WORKER_MISSING_OR_AMBIGUOUS');sdkWorkers.push({file,sha256:sha,packaged:true});}

  const sdkAssets=JSON.parse(extract('dist/sdk-assets.json'));for(const item of sdkAssets.assets){const source=await fs.readFile(path.join(SDK,'assets',memberName(item.file)));ensure(digest(source)===item.sha256&&extract('dist/'+item.file).equals(source),'PACKAGED_GO_WASM_ASSET_CHANGED');}

  const index=extract('dist/index.html').toString();ensure(index.includes('<title>PUTY内部通讯</title>')&&index.includes('image/png'),'PACKAGED_BRAND_MISMATCH');for(const file of ['favicon-32x32.png','icons/icon.png','icons/icon.ico','icons/tray.png','icons/tray@2x.png'])ensure(extract('dist/'+file).equals(await fs.readFile(path.join(CLIENT,'public',file))),'PACKAGED_ICON_MISMATCH');

  const rootCA=new X509Certificate(await fs.readFile(path.join(CLIENT,'release/win-unpacked/resources/extraResources/certificates/Puty-Enterprise-Root-CA.crt')));ensure(digest(rootCA.raw)==='836c3d6c28d5078cc8da6ebeff8e5f9a7fb7f140554e6a7834011d72299c3d69','PUBLIC_ROOT_PIN_MISMATCH');

  const web=path.join(CLIENT,'release/PUTY_IM-3.8.13-web.zip'),webBefore=await hashFile(web),zip=new AdmZip(web),actualWeb=new Map();for(const item of zip.getEntries()){const name=memberName(item.entryName.replace(/\/$/,''));if(item.isDirectory){const directory=await fs.lstat(path.join(CLIENT,'dist-web',name));ensure(directory.isDirectory()&&!directory.isSymbolicLink(),'UNEXPECTED_ZIP_DIRECTORY');continue;}ensure(!actualWeb.has(name)&&item.header.size<=256*1024*1024,'ZIP_ENTRY_REJECTED');const bytes=zip.readFile(item);scan('web',name,bytes);actualWeb.set(name,{sha256:digest(bytes),bytes:bytes.length});}sameFiles(await tree(path.join(CLIENT,'dist-web')),actualWeb);

  const asarAfter=await hashFile(archive),webAfter=await hashFile(web);ensure(before.sha256===asarAfter.sha256&&webBefore.sha256===webAfter.sha256,'FINAL_ARTIFACT_CHANGED_DURING_AUDIT');const installer=await hashFile(path.join(CLIENT,'release/PUTY_IM-3.8.19-win-x64.exe'));

  const nativePE=peIdentity(path.join(CLIENT,'release/win-unpacked/PUTY_IM.exe'),builder.productName),installerPE=peIdentity(path.join(CLIENT,'release/PUTY_IM-3.8.19-win-x64.exe'),pkg.description);

  const report={format:'puty-independent-final-client-audit-v1',createdAt:new Date().toISOString(),passed:flags.length===0,version:metadata.version,build:metadata.build,applicationID:builder.appId,productName:builder.productName,stableIdentityVerified:true,installerEmbeddedAppIDParsed:false,installerIdentityFromReviewedBuilderAndEmbeddedPackage:true,PEIdentity:{native:nativePE,installer:installerPE},privateArchivesExtracted:false,actualMainExecutedByThisAudit:false,productionRequests:0,credentialValuesPrintedOrPersisted:false,knownSecretPatternsComparedInRAM:patterns.length,flags,artifacts:{asar:{...asarAfter,members,matchedTrees:Object.fromEntries(packaged)},web:{...webAfter,members:actualWeb.size,indexSHA256:actualWeb.get('index.html').sha256},installer},currentSourceMapInputsMatched:matchedSources,legacySQLDependencyPresent:true,exactSuppressionWorkers:sdkWorkers,goWasmAssetFilesMatched:sdkAssets.assets.length};

  patterns.forEach(value=>value.fill(0));return report;

}

export async function main(argv=process.argv.slice(2)){

  ensure(argv.every(v=>['--run','--web-only','--compare-production-secrets'].includes(v))&&new Set(argv).size===argv.length,'UNSUPPORTED_AUDIT_OPTION');

  if(!argv.includes('--run')){console.log(JSON.stringify({dryRun:true,privateInputsRead:0,finalArtifactsRead:0,productionRequests:0}));return 0;}

  const report=await (argv.includes('--web-only')?auditWeb:audit)({compareSecrets:argv.includes('--compare-production-secrets')}),file=path.join(PRIVATE,'acceptance/security-storage-20261104','pc-web-final-independent-'+Date.now()+'.json');await fs.writeFile(file,JSON.stringify(report,null,2)+'\n',{flag:'wx',mode:0o600});console.log(JSON.stringify({passed:report.passed,report:file,reportSHA256:digest(await fs.readFile(file)),secretPatterns:report.knownSecretPatternsComparedInRAM,flags:report.flags.length,asar:report.artifacts.asar?.sha256,web:report.artifacts.web.sha256,installer:report.artifacts.installer?.sha256}));return report.passed?0:1;

}

if(process.argv[1]&&path.resolve(process.argv[1])===fileURLToPath(import.meta.url)){try{process.exitCode=await main();}catch(error){console.error(JSON.stringify({passed:false,code:error.auditCode||'FINAL_CLIENT_AUDIT_FAILED',valuesPrinted:false}));process.exitCode=1;}}

