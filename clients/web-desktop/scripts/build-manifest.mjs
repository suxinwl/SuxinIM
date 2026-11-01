import { readFile,readdir,writeFile } from "node:fs/promises";
import { createHash } from "node:crypto";
const artifacts=[],history=[];
const current=JSON.parse(await readFile("build-version.json","utf8"));
const knownBuilds={"3.8.3":380301,"3.8.4":380401,[current.version]:current.build};
for(const manifest of ["release-manifest.json","release-history-manifest.json"]){
 const prior=await readFile(manifest,"utf8").then(JSON.parse).catch(()=>null);
 for(const item of prior?.artifacts || [])knownBuilds[item.version] ??= item.build;
}
for(const file of (await readdir("release")).sort()){
 const version=/^OpenIM38-Enterprise-([0-9.]+)-win-x64\.exe$/.exec(file)?.[1];
 if(!version)continue;
 if(!knownBuilds[version])throw new Error(`Unknown build for ${file}`);
 const path=`release/${file}`,data=await readFile(path);
 const item={file:path,size:data.length,sha256:createHash("sha256").update(data).digest("hex"),version,build:knownBuilds[version],platform:"windows",arch:"x64",applicationID:"io.openim.enterprise.upgrade38",codeSigned:false};
 if(version===current.version)artifacts.push({...item,status:"current"});
 else history.push({...item,status:"withdrawn",purpose:"Historical validation fixture; not part of the current delivery or recommended for installation"});
}
if(artifacts.length!==1)throw new Error(`Expected exactly one installer for current application ${current.version}`);
const metadata={application:"OpenIM38 Enterprise",node:process.version,sdk:"3.8.3-patch.15.1"};
await writeFile("release-manifest.json",JSON.stringify({...metadata,version:current.version,build:current.build,status:"current",historicalManifest:"release-history-manifest.json",artifacts},null,2));
await writeFile("release-history-manifest.json",JSON.stringify({...metadata,status:"withdrawn",artifacts:history},null,2));
