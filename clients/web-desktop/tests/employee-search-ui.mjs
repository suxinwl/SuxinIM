// Real search components; synthetic HRM data and empty local SDK results only.
import assert from 'node:assert/strict';
import {build} from 'esbuild';
import {chromium} from 'playwright';
import {mkdir,writeFile} from 'node:fs/promises';
import {resolve} from 'node:path';
const entry=`import React,{useRef}from'react';import{createRoot}from'react-dom/client';import i18n from'i18next';import AddSearch from'./src/layout/TopSearchBar/SearchUserOrGroup';import GlobalSearch from'./src/pages/common/GlobalSearchModal';
function Harness(){const add=useRef(null),global=useRef(null);return <><button onClick={()=>add.current.openOverlay()}>添加好友入口</button><button onClick={()=>global.current.openOverlay()}>顶部搜索入口</button><AddSearch ref={add} isSearchGroup={false} openUserCardWithData={p=>{window.fixtureCards.push(p.userID);add.current.closeOverlay();}} openGroupCardWithData={()=>{}}/><GlobalSearch ref={global}/></>;}i18n.init({lng:'zh',resources:{zh:{translation:{confirm:'搜索',cancel:'取消',placeholder:{addFriends:'添加好友',overview:'全部',contacts:'好友',myGroup:'群聊',messageHistory:'聊天记录',document:'文件'}}}}}).then(()=>createRoot(document.getElementById('root')).render(<Harness/>));`;
const compiled=await build({stdin:{contents:entry,resolveDir:process.cwd(),loader:'tsx'},bundle:true,write:false,format:'iife',jsx:'automatic',define:{'process.env.NODE_ENV':'"production"'},plugins:[{name:'search-fixtures',setup(b){
 const fixtures=new Map([
  ['@openim/wasm-client-sdk','export const SessionType={WorkingGroup:3};export const MessageType={TextMessage:101,AtTextMessage:106,FileMessage:102,QuoteMessage:114,CardMessage:112,LocationMessage:105,MergeMessage:107};'],
  ['@/api/directory','export const getDirectory=query=>window.fixtureDirectory(query);'],
  ['@/layout/MainContentWrap','export const IMSDK={searchFriends:async()=>({data:[]}),searchGroups:async()=>({data:[]}),searchLocalMessages:async()=>({data:{searchResultItems:[]}}),getSpecifiedGroupsInfo:async()=>({data:[]})};'],
  ['@/components/OIMAvatar',`import React from'react';export default()=> <span aria-hidden="true">●</span>;`],
  ['@/components/DraggableModalWrap',`import{Modal}from'antd';export default Modal;`],
  ['@/AntdGlobalComp','export const message={warning:()=>{}};'],
  ['@/utils/common','export const feedbackToast=()=>window.fixtureFailures.push("request failed");'],
  ['@/utils/events','export default{emit:(name,data)=>{if(name==="OPEN_USER_CARD")window.fixtureCards.push(data.userID);}};'],
  ['@/utils/imCommon','export const getConversationContent=()=>"";'],
  ['@/store','export const useMessageStore=()=>({});useMessageStore.getState=()=>({downloadMap:{}});'],
 ]);
 b.onResolve({filter:/^@\//},args=>fixtures.has(args.path)?{path:args.path,namespace:'fixture'}:undefined);
 b.onResolve({filter:/^@openim\/wasm-client-sdk$/},()=>({path:'@openim/wasm-client-sdk',namespace:'fixture'}));
 b.onResolve({filter:/MainContentWrap$/},()=>({path:'@/layout/MainContentWrap',namespace:'fixture'}));
 b.onResolve({filter:/^\.\/(ChatLogsPanel|ContactPanel|DashboardPanel|FilePanel)$/},args=>args.importer.replace(/\\/g,'/').endsWith('GlobalSearchModal/index.tsx')?{path:'local-panel',namespace:'fixture'}:undefined);
 b.onResolve({filter:/\.scss$/},()=>({path:'style',namespace:'fixture'}));
 b.onLoad({filter:/.*/,namespace:'fixture'},args=>({resolveDir:process.cwd(),loader:'jsx',contents:args.path==='local-panel'?`import React from'react';export default React.forwardRef(()=> <div/>);`:args.path==='style'?`const style=document.createElement('style');style.textContent='.globalSearch{height:650px}.globalSearch .ant-modal-content,.globalSearch .ant-modal-body{height:100%}.globalSearch .ant-modal-body{display:flex;flex-direction:column}.searchTab{flex:1;min-height:0}.searchTab .ant-tabs-content,.searchTab .ant-tabs-tabpane{height:100%}';document.head.append(style);export default{'global-search-modal':'globalSearch','search-tab':'searchTab'};`:fixtures.get(args.path)}));
}}]});
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE});
const output='test-results/organization-candidate';await mkdir(output,{recursive:true});const checks=[];
try{
 const page=await browser.newPage({viewport:{width:1180,height:820}});await page.route('**/*',route=>route.abort());
 const errors=[];page.on('pageerror',error=>{errors.push(error.message);console.error('Offline search UI exception:',error.stack?.slice(0,3000)||error.message);});
 await page.setContent('<html><body><div id="root"></div></body></html>');
 await page.evaluate(()=>{
  window.fixtureCards=[];window.fixtureFailures=[];window.fixtureQueries=[];
  const people=[{userID:'person-a',nickname:'陈明',employeeNo:'1001',department:'研发部',phoneNumber:'13800000001'},{userID:'person-b',nickname:'陈明',employeeNo:'1002',department:'人事部',phoneNumber:'13800000002'}];
  window.fixtureDirectory=async query=>{window.fixtureQueries.push(query);if(query.keyword==='慢'){await new Promise(r=>setTimeout(r,250));return{items:people,total:2,departments:[]};}const items=people.filter(p=>[p.nickname,p.employeeNo,p.phoneNumber].some(v=>v.includes(query.keyword)));return{items,total:items.length,departments:[]};};
 });
 await page.addScriptTag({content:compiled.outputFiles[0].text});
 await page.getByRole('button',{name:'添加好友入口',exact:true}).click();
 const addInput=page.getByPlaceholder('输入员工手机号、姓名或工号');
 await addInput.fill('陈明');await addInput.press('Enter');
 await page.getByText('工号：1001 · 研发部',{exact:true}).filter({visible:true}).waitFor();await page.getByText('工号：1002 · 人事部',{exact:true}).filter({visible:true}).waitFor();
 checks.push('add-friend search retains both employees with the same name');
 await addInput.fill('13800000002');await addInput.press('Enter');
 await page.getByText('工号：1002 · 人事部',{exact:true}).filter({visible:true}).waitFor();
 await page.waitForFunction(()=>!document.body.innerText.includes('工号：1001'));
 await page.getByRole('button').filter({hasText:'工号：1002'}).click();
 assert.deepEqual(await page.evaluate(()=>window.fixtureCards),['person-b']);
 checks.push('phone search opens the exact employee profile from add-friend entry');
 await addInput.waitFor({state:'hidden'});
 await page.getByRole('button',{name:'顶部搜索入口',exact:true}).click();
 const globalInput=page.getByPlaceholder('搜索手机号、姓名、工号或聊天内容');
 await globalInput.fill('1001');await globalInput.press('Enter');
 await page.getByText('工号：1001 · 研发部',{exact:true}).filter({visible:true}).waitFor();
 checks.push('top global search finds HRM employees even when every local SDK search is empty');
 await page.getByRole('tab',{name:'企业同事',exact:true}).click();
 await globalInput.fill('陈明');await globalInput.press('Enter');
 await page.getByText('工号：1002 · 人事部',{exact:true}).filter({visible:true}).waitFor();
 checks.push('enterprise search tab supports same-name disambiguation');
 await globalInput.fill('慢');await globalInput.press('Enter');
 await globalInput.fill('1001');await globalInput.press('Enter');
 await page.waitForTimeout(350);
 assert.equal(await page.getByText('工号：1002 · 人事部',{exact:true}).filter({visible:true}).count(),0);
 checks.push('late prior-query response cannot replace the latest search');
 await page.getByRole('button').filter({hasText:'工号：1001'}).click();
 assert.deepEqual(await page.evaluate(()=>window.fixtureCards),['person-b','person-a']);
 checks.push('top search opens the existing profile flow with exact userID');
 assert.deepEqual(errors,[]);assert.deepEqual(await page.evaluate(()=>window.fixtureFailures),[]);
 await writeFile(`${output}/employee-search-offline-ui.json`,JSON.stringify({passed:true,scope:'real add/global-search components; synthetic HRM data, empty SDK searches, no production requests',checks,pageErrors:errors},null,2));
 console.log(`Employee search offline browser: ${checks.length} checks passed`);
}finally{await browser.close();}
