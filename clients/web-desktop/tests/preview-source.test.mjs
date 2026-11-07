import test from 'node:test';
import assert from 'node:assert/strict';
import { build } from 'esbuild';
const compiled=await build({entryPoints:['src/im/preview-source.ts'],write:false,bundle:true,format:'esm',platform:'node',plugins:[{name:'classification',setup(builder){
 builder.onResolve({filter:/^\.\/resources$/},()=>({path:'resource',namespace:'stub'}));
 builder.onLoad({filter:/.*/,namespace:'stub'},()=>({contents:'export const requiresResourceGrant=url=>url.startsWith("https://im.szputy.com:9885/attachments/");'}));
}}]});
const {previewImageSource}=await import('data:text/javascript;base64,'+Buffer.from(compiled.outputFiles[0].text).toString('base64'));
test('managed original and thumbnail reach the authorization component before any shared disk-cache conversion',()=>{
 const original='https://im.szputy.com:9885/attachments/publisher/openim38/full',thumb='https://im.szputy.com:9885/attachments/publisher/openim38/thumb';
 assert.equal(previewImageSource({url:original,thumbUrl:thumb},true,{[thumb]:'C:\\old-viewer.png'}),original);
 assert.equal(previewImageSource({url:'file:///C:/old-local.png',thumbUrl:thumb},true,{[thumb]:'C:\\old-viewer.png'}),thumb);
});
test('explicit local files without a protected source and browser/external images preserve their existing behavior',()=>{
 assert.equal(previewImageSource({url:'file:///C:/explicit.png'},true,{}),'file:///C:/explicit.png');
 assert.equal(previewImageSource({url:'https://external.invalid/full',thumbUrl:'https://external.invalid/thumb'},false,{}),'https://external.invalid/full');
});
