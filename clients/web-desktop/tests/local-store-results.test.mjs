import test from 'node:test';
import assert from 'node:assert/strict';
import {build} from 'esbuild';
const built=await build({entryPoints:['src/im/local-store-results.ts'],bundle:true,write:false,format:'esm',platform:'node'});
const {requireStoreAck,requireCleanupPreview}=await import('data:text/javascript;base64,'+Buffer.from(built.outputFiles[0].text).toString('base64'));
test('a timed-out or ambiguous worker response cannot confirm a checkpoint or destructive clear',()=>{
  for(const value of [undefined,null,'{"errCode":10001,"errMsg":"timeout"}',{errCode:10001},{checkpointed:false},{}]){assert.throws(()=>requireStoreAck(value,'checkpointed'));assert.throws(()=>requireStoreAck(value,'cleaned'));}
  requireStoreAck({checkpointed:true},'checkpointed');requireStoreAck({cleaned:true},'cleaned');
});
test('cleanup confirmation requires a complete finite scope, including explicit all-conversation null',()=>{
  const scope={messages:2,attachments:1,bytes:1234,beforeMS:Date.parse('2026-11-04T00:00:00+08:00'),conversationIDs:['si_fixture']};assert.deepEqual(requireCleanupPreview(scope),scope);
  for(const value of ['{"errCode":10001}',{}, {...scope,beforeMS:undefined},{...scope,conversationIDs:undefined},{...scope,bytes:-1},{...scope,conversationIDs:['../outside']}])assert.throws(()=>requireCleanupPreview(value));
  assert.equal(requireCleanupPreview({...scope,conversationIDs:null}).conversationIDs,null);
});
