import test from 'node:test';
import assert from 'node:assert/strict';
import {build} from 'esbuild';
const built=await build({entryPoints:['src/im/attachment-name.ts'],bundle:true,write:false,format:'esm',platform:'node'});
const {attachmentSourceName,attachmentMessageSourceName,withAttachmentSourceName}=await import('data:text/javascript;base64,'+Buffer.from(built.outputFiles[0].text).toString('base64'));
test('media source names survive sender path redaction and preserve existing extension metadata',()=>{
  const source={videoElem:{videoPath:''},ex:JSON.stringify({meetingCode:'synthetic-only'})};
  const message=withAttachmentSourceName(source,'现场拍摄%20（原片）.mp4');
  assert.deepEqual(JSON.parse(message.ex),{meetingCode:'synthetic-only',putySourceName:'现场拍摄%20（原片）.mp4'});assert.equal(attachmentMessageSourceName(message),'现场拍摄%20（原片）.mp4');assert.equal(source.ex,JSON.stringify({meetingCode:'synthetic-only'}));
});
test('official file names take priority and metadata never carries a directory',()=>{
  assert.equal(attachmentMessageSourceName({fileElem:{fileName:'职员名单.xlsx'},ex:JSON.stringify({putySourceName:'other.xlsx'})}),'职员名单.xlsx');
  for(const name of ['../private.txt','C:\\private.txt','a\u0000b.txt']){assert.equal(attachmentMessageSourceName({ex:JSON.stringify({putySourceName:name})}),undefined);assert.throws(()=>withAttachmentSourceName({},name));}
});
test('legacy local basenames preserve percent text while managed URI basenames decode exactly once',()=>{
  assert.equal(attachmentSourceName(undefined,'E:\\data\\原名%20.txt'),'原名%20.txt');
  assert.equal(attachmentMessageSourceName({pictureElem:{sourcePath:'puty-local://pending/synthetic/%E5%8E%9F%E5%90%8D%2520.png'}}),'原名%20.png');
});
