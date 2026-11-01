import {spawn} from 'node:child_process';
import {resolve} from 'node:path';
const child=spawn(resolve('node_modules/electron/dist/electron.exe'),['.','--remote-debugging-port=17779'],{windowsHide:true,env:{...process.env,OPENIM38_SMOKE_TEST:'1',OPENIM38_TEST_DATA_DIR:resolve('../../.cache/electron-smoke')}});
try {
 let ep;for(let i=0;i<80;i++){try{ep=await(await fetch('http://127.0.0.1:17779/json/version')).json();break;}catch{await new Promise(r=>setTimeout(r,250));}}
 console.log(ep);const ws=new WebSocket(ep.webSocketDebuggerUrl);await new Promise(r=>ws.onopen=r);ws.onmessage=e=>console.log(e.data);
 const cmds=[['Browser.getVersion',{}],['Target.getBrowserContexts',{}],['Target.getTargets',{}],['Target.setDiscoverTargets',{discover:true}],['Target.setAutoAttach',{autoAttach:true,waitForDebuggerOnStart:false,flatten:true}]];
 cmds.forEach(([method,params],i)=>ws.send(JSON.stringify({id:i+1,method,params})));await new Promise(r=>setTimeout(r,10000));ws.close();
}finally{child.kill();}
