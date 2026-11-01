import test from "node:test";
import assert from "node:assert/strict";
import {build} from "esbuild";
const result=await build({entryPoints:["src/im/local-files.ts"],write:false,bundle:true,format:"esm",platform:"node"});
const {toLocalFileURL,isLocalFileURL,localFilePath}=await import(`data:text/javascript;base64,${Buffer.from(result.outputFiles[0].text).toString("base64")}`);
test("Windows Unicode cache paths preserve special characters",()=>{const path=String.raw`C:\cache\图片 #1&2.png`;const url=toLocalFileURL(path);assert.equal(isLocalFileURL(url),true);assert.equal(localFilePath(url),path);assert.equal(toLocalFileURL(undefined),"");});
test("remote attachment URLs are never treated as local cache",()=>{const url="https://files.example/a.mp4";assert.equal(isLocalFileURL(url),false);assert.equal(localFilePath(url),url);assert.equal(isLocalFileURL(undefined),false);});
