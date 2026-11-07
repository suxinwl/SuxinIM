// Real synthetic H.264/AAC packets -> production C muxer -> Chromium demux/decode.
// This checks portable media bytes, not a Mac/iOS framework build.
import test from 'node:test';
import assert from 'node:assert/strict';
import {execFileSync} from 'node:child_process';
import {createHash} from 'node:crypto';
import {mkdtemp,readFile,rm,writeFile,mkdir} from 'node:fs/promises';
import path from 'node:path';
import {tmpdir} from 'node:os';
import {chromium} from 'playwright';
const toolchain='E:/AndroidDev/sdk/ndk/26.3.11579264/toolchains/llvm/prebuilt/windows-x86_64/bin/';
const source=path.resolve('../../sdks/flutter/ios/Classes');
test('production C muxer emits playable video and audio/video from real WebCodecs packets',{timeout:60000},async t=>{
  const directory=await mkdtemp(path.join(tmpdir(),'puty-mux-playback-'));
  let browser;
  try{
    for(const [file,output] of [[path.join(source,'PMFragmentMuxer.c'),'mux.o'],['tests/ios-fragment-muxer.c','test.o']])execFileSync(toolchain+'clang.exe',['--target=wasm32','-std=c11','-Wall','-Wextra','-Werror','-ffreestanding','-fno-builtin','-c','-I',source,file,'-o',path.join(directory,output)],{windowsHide:true});
    execFileSync(toolchain+'ld.lld.exe',['-flavor','wasm','--no-entry','--export-all','--initial-memory=16777216',path.join(directory,'mux.o'),path.join(directory,'test.o'),'-o',path.join(directory,'mux.wasm')],{windowsHide:true});
    const wasm=await readFile(path.join(directory,'mux.wasm'));
    const fallbackAudio=JSON.parse(await readFile('tests/fixtures/aac-lc-44100-mono.json','utf8'));
    assert.equal(fallbackAudio.syntheticOnly,true);assert.equal(fallbackAudio.generatorSHA256,createHash('sha256').update(await readFile('tests/fixtures/PutyAACFixture.java')).digest('hex'));
    browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE || path.join(process.env.LOCALAPPDATA,'ms-playwright/chromium-1208/chrome-win64/chrome.exe'),args:['--autoplay-policy=no-user-gesture-required']});
    const page=await browser.newPage();
    await page.route('https://puty-mux.invalid/**',route=>route.fulfill({status:200,contentType:'text/html',body:'<!doctype html><title>Synthetic mux acceptance</title>'}));
    await page.goto('https://puty-mux.invalid/');
    const result=await page.evaluate(async({wasmBytes,fallbackAudio})=>{
      const check=(ok,message)=>{if(!ok)throw new Error(message);};
      const videoConfig={codec:'avc1.42001f',width:64,height:64,bitrate:240000,framerate:30,latencyMode:'realtime',avc:{format:'avc'}};
      check((await VideoEncoder.isConfigSupported(videoConfig)).supported,'H264 encoder unavailable');
      const canvas=new OffscreenCanvas(64,64),context=canvas.getContext('2d'),videos=[];let avcc,videoError;
      const encoder=new VideoEncoder({output:(chunk,metadata)=>{const bytes=new Uint8Array(chunk.byteLength);chunk.copyTo(bytes);videos.push({bytes:Array.from(bytes),timestamp:chunk.timestamp,key:chunk.type==='key'});if(metadata.decoderConfig?.description)avcc=Array.from(new Uint8Array(metadata.decoderConfig.description));},error:error=>{videoError=error;}});
      encoder.configure(videoConfig);
      for(let i=0;i<12;i++){context.fillStyle=i<6?'rgb(240,20,20)':'rgb(20,20,240)';context.fillRect(0,0,64,64);const frame=new VideoFrame(canvas,{timestamp:Math.round(i*1e6/30),duration:33333});encoder.encode(frame,{keyFrame:i===0});frame.close();}
      await encoder.flush();encoder.close();if(videoError)throw videoError;check(videos.length===12&&avcc,'H264 packet/config missing');
      const description=new Uint8Array(avcc);let cursor=6;const spsCount=description[5]&31,sps=[],pps=[];
      for(let i=0;i<spsCount;i++){const count=(description[cursor]<<8)|description[cursor+1];cursor+=2;sps.push(description.slice(cursor,cursor+count));cursor+=count;}
      const ppsCount=description[cursor++];for(let i=0;i<ppsCount;i++){const count=(description[cursor]<<8)|description[cursor+1];cursor+=2;pps.push(description.slice(cursor,cursor+count));cursor+=count;}
      check(sps.length===1&&pps.length===1,'Unexpected SPS/PPS configuration');
      const audioConfig={codec:'mp4a.40.2',sampleRate:44100,numberOfChannels:1,bitrate:64000},nativeAudioEncoder=typeof AudioEncoder!=='undefined'&&(await AudioEncoder.isConfigSupported(audioConfig)).supported;
      let audioDescription,audioError;const audios=[];
      if(nativeAudioEncoder){const audioEncoder=new AudioEncoder({output:(chunk,metadata)=>{const bytes=new Uint8Array(chunk.byteLength);chunk.copyTo(bytes);audios.push(Array.from(bytes));if(metadata.decoderConfig?.description)audioDescription=Array.from(new Uint8Array(metadata.decoderConfig.description));},error:error=>{audioError=error;}});audioEncoder.configure(audioConfig);
        for(let i=0;i<18;i++){const samples=new Float32Array(1024);for(let n=0;n<1024;n++)samples[n]=Math.sin(2*Math.PI*440*(i*1024+n)/44100)*0.2;const data=new AudioData({format:'f32',sampleRate:44100,numberOfFrames:1024,numberOfChannels:1,timestamp:Math.round(i*1024*1e6/44100),data:samples});audioEncoder.encode(data);data.close();}await audioEncoder.flush();audioEncoder.close();if(audioError)throw audioError;check(audios.length>0&&audioDescription?.length===2,'AAC-LC configuration missing');}
      else{const decode=value=>Array.from(atob(value),char=>char.charCodeAt(0));check(fallbackAudio.rate===44100&&fallbackAudio.channels===1,'AAC fixture format differs');audioDescription=decode(fallbackAudio.audioSpecificConfig);audios.push(...fallbackAudio.packets.map(decode));check(audioDescription.length===2&&audios.length>=18,'AAC fixture is incomplete');}
      const {instance}=await WebAssembly.instantiate(new Uint8Array(wasmBytes),{}),api=instance.exports;
      const put=bytes=>{check(bytes.length<=api.input_capacity(),'Synthetic packet exceeds bound');new Uint8Array(api.memory.buffer,api.input_bytes(),bytes.length).set(bytes);};
      async function playback(withAudio){api.reset_mux();put([...sps[0],...pps[0]]);check(api.configure_video(64,64,sps[0].length,pps[0].length)===0,'Video configuration rejected');if(withAudio){put(audioDescription);check(api.configure_audio(44100,1,audioDescription.length)===0,'Audio configuration rejected');}check(api.start_mux()===0,'Mux start rejected');
        const packets=videos.map((packet,i)=>({kind:'v',time:i/30,index:i,packet}));if(withAudio)audios.forEach((packet,i)=>packets.push({kind:'a',time:i*1024/44100,index:i,packet}));packets.sort((a,b)=>a.time-b.time);
        for(const item of packets){if(item.kind==='v'){put(item.packet.bytes);check(api.video_sample(item.packet.bytes.length,BigInt(item.index*3000),3000,item.packet.key?1:0)===0,'Video packet rejected');}else{put(item.packet);check(api.audio_sample(item.packet.length,BigInt(item.index*1024),1024)===0,'Audio packet rejected');}}
        check(api.finish_mux()===0,'Mux finish rejected');const bytes=new Uint8Array(api.memory.buffer,api.bytes(),api.size()).slice();let audioDecoded=false;
        if(withAudio){const audioContext=new AudioContext({sampleRate:44100});try{const decoded=await audioContext.decodeAudioData(bytes.buffer.slice(0)),samples=decoded.getChannelData(0);let energy=0;for(const value of samples)energy+=value*value;check(decoded.numberOfChannels===1&&samples.length>4000&&Math.sqrt(energy/samples.length)>0.02,'Real AAC packets did not decode');audioDecoded=true;}finally{await audioContext.close();}}
        const video=document.createElement('video'),url=URL.createObjectURL(new Blob([bytes],{type:'video/mp4'}));video.muted=true;video.preload='auto';video.src=url;document.body.append(video);
        const wait=event=>new Promise((resolve,reject)=>{const timeout=setTimeout(()=>reject(new Error('Media timeout '+event)),12000);video.addEventListener(event,()=>{clearTimeout(timeout);resolve();},{once:true});video.addEventListener('error',()=>{clearTimeout(timeout);reject(new Error('Media decoder error '+video.error?.code));},{once:true});});
        try{await wait('loadeddata');check(video.videoWidth===64&&video.videoHeight===64,'Decoded dimensions differ');const output=new OffscreenCanvas(64,64),ctx=output.getContext('2d');ctx.drawImage(video,0,0);const first=Array.from(ctx.getImageData(32,32,1,1).data);check(first[0]>150&&first[2]<80,'First real H264 frame did not decode');const seeking=wait('seeked');video.currentTime=0.3;await seeking;ctx.drawImage(video,0,0);const later=Array.from(ctx.getImageData(32,32,1,1).data);check(later[2]>150&&later[0]<80,'Later real H264 frame did not decode');const duration=video.duration;check(Number.isFinite(duration)&&duration>=0.39&&duration<0.6,'Fragment duration is not playable');video.currentTime=0;await video.play();await wait('ended');return {withAudio,bytes:bytes.length,width:video.videoWidth,height:video.videoHeight,duration,decodedFirstAndLaterFrames:true,playbackEnded:true,audioDecoded,audioPackets:withAudio?audios.length:0};}finally{video.pause();video.removeAttribute('src');video.load();video.remove();URL.revokeObjectURL(url);}
      }
      const outputs=[await playback(false),await playback(true)];return {videoPackets:videos.length,audioSource:nativeAudioEncoder?'WebCodecs AAC encoder':'Android API30 MediaCodec synthetic fixture',outputs};
    },{wasmBytes:Array.from(wasm),fallbackAudio});
    assert.equal(result.videoPackets,12);assert.ok(result.outputs.every(value=>value.playbackEnded));
    const report={passed:true,macFrameworkBuildTested:false,realBrowserVersion:browser.version(),...result};
    const output=path.resolve('build/acceptance/ios-muxer-playback.json');await mkdir(path.dirname(output),{recursive:true});await writeFile(output,JSON.stringify(report,null,2));t.diagnostic(JSON.stringify(report));
  }finally{await browser?.close();await rm(directory,{recursive:true,force:true});}
});
