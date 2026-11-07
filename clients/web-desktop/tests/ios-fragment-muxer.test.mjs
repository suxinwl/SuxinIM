import test from 'node:test';
import assert from 'node:assert/strict';
import {execFileSync} from 'node:child_process';
import {mkdtemp,readFile,rm} from 'node:fs/promises';
import path from 'node:path';
import {tmpdir} from 'node:os';
const directory=await mkdtemp(path.join(tmpdir(),'puty-iso-bmff-'));
const source=path.resolve('../../sdks/flutter/ios/Classes');
try{
 const toolchain='E:/AndroidDev/sdk/ndk/26.3.11579264/toolchains/llvm/prebuilt/windows-x86_64/bin/';
 for(const [file,output] of [[path.join(source,'PMFragmentMuxer.c'),'mux.o'],['tests/ios-fragment-muxer.c','test.o']])execFileSync(toolchain+'clang.exe',['--target=wasm32','-std=c11','-Wall','-Wextra','-Werror','-ffreestanding','-fno-builtin','-c','-I',source,file,'-o',path.join(directory,output)]);
 execFileSync(toolchain+'ld.lld.exe',['-flavor','wasm','--no-entry','--export-all','--initial-memory=16777216',path.join(directory,'mux.o'),path.join(directory,'test.o'),'-o',path.join(directory,'mux.wasm')]);
 const {instance}=await WebAssembly.instantiate(await readFile(path.join(directory,'mux.wasm')),{}),api=instance.exports;
 assert.equal(api.build_fixture(),0);
 const body=Buffer.from(new Uint8Array(api.memory.buffer,api.bytes(),api.size()));
 function boxes(bytes,start=0,end=bytes.length){const items=[];for(let at=start;at<end;){assert.ok(end-at>=8);const size=bytes.readUInt32BE(at);assert.ok(size>=8&&at+size<=end);items.push({name:bytes.toString('ascii',at+4,at+8),at,size,body:bytes.subarray(at+8,at+size)});at+=size;}return items;}
 test('actual C init contains two configured tracks, empty sample tables and mvex',()=>{
  const top=boxes(body);assert.deepEqual(top.map(v=>v.name),['ftyp','moov','moof','mdat','moof','mdat','moof','mdat']);
  const moov=boxes(top[1].body);assert.equal(moov.filter(v=>v.name==='trak').length,2);assert.ok(moov.some(v=>v.name==='mvex'));
  assert.ok(body.includes(Buffer.from('avcC')));assert.ok(body.includes(Buffer.from('esds')));assert.ok(body.includes(Buffer.from('iso6')));
 });
 test('fragment track/time/duration/sample-size and moof-relative data offsets match every mdat',()=>{
  const top=boxes(body),trackIDs=[],times=[];
  for(let index=2;index<top.length;index+=2){const moof=top[index],mdat=top[index+1],children=boxes(moof.body),traf=boxes(children.find(v=>v.name==='traf').body),tfhd=traf.find(v=>v.name==='tfhd').body,tfdt=traf.find(v=>v.name==='tfdt').body,trun=traf.find(v=>v.name==='trun').body;
   trackIDs.push(tfhd.readUInt32BE(4));times.push(tfdt.readBigUInt64BE(4));assert.equal(tfhd.readUInt32BE(0)&0xffffff,0x20000);assert.equal(trun.readUInt32BE(0)&0xffffff,0x701);assert.equal(trun.readUInt32BE(4),1);assert.equal(trun.readUInt32BE(8),moof.size+8);assert.equal(trun.readUInt32BE(16),mdat.body.length);assert.ok(trun.readUInt32BE(12)>0);
  }assert.deepEqual(trackIDs,[1,2,1]);assert.deepEqual(times,[0n,0n,3000n]);
 });
 test('bad NAL lengths, backwards decode time, post-start configuration and rejected output fail closed',()=>{assert.equal(api.reject_invalid(),0);assert.equal(api.reject_output(),0);});
 test('duplicate/overflow times and mismatched AAC config are rejected before any output',()=>{assert.equal(api.reject_bounds(),0);});
 test('silent capture produces one video track without an unpopulated audio track',()=>{assert.equal(api.video_only(),0);const bytes=Buffer.from(new Uint8Array(api.memory.buffer,api.bytes(),api.size())),top=boxes(bytes),moov=boxes(top.find(v=>v.name==='moov').body);assert.equal(moov.filter(v=>v.name==='trak').length,1);assert.ok(!bytes.includes(Buffer.from('mp4a')));assert.deepEqual(top.map(v=>v.name),['ftyp','moov','moof','mdat']);});
}finally{await rm(directory,{recursive:true,force:true});}
