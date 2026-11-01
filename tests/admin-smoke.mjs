import {chromium} from '../clients/web-desktop/node_modules/playwright/index.mjs';
import {readFile,writeFile,mkdir} from 'node:fs/promises';
const state=JSON.parse(await readFile(new URL('../.private/test-accounts.json',import.meta.url),'utf8'));
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE || undefined});
const page=await browser.newPage();const errors=[],requests=[];
page.on('pageerror',e=>errors.push(e.message));
page.on('response',async r=>{if(r.url().includes('/enterprise/')||r.url().includes('/chat/')){let code;try{code=(await r.json()).errCode}catch{}requests.push({path:new URL(r.url()).pathname,status:r.status(),code});}});
const steps=[];
try{
 await page.goto('https://192.168.10.11:29880/admin/login');
 await page.locator('#basic_account').fill(state.admin.account);
 await page.locator('#basic_secrect').fill(state.admin.password);
 await page.locator('button[type=submit]').click();
 await page.waitForURL('**/business_system/user_manage/user_list',{timeout:30000});
 await page.waitForTimeout(1800);steps.push({route:'login',ok:true});
 for(const route of ['business_system/user_manage/user_list','business_system/user_manage/block_list','business_system/register_manage/invition_code','business_system/register_manage/defualt_friends','business_system/register_manage/defualt_group','business_system/app_manage/discover_manage','business_system/app_manage/versions','im_system/user_manage/user_list','im_system/group_manage/group_list','im_system/message_manage/user_message','im_system/message_manage/group_message','im_system/message_manage/send_message','rtc_system/video_call','rtc_system/meeting','profile/admin_info','profile/admin_password']){
  const before=requests.length;await page.goto('https://192.168.10.11:29880/admin/'+route);await page.waitForTimeout(2200);
  steps.push({route,title:await page.title(),network:requests.slice(before)});
 }
 await page.goto('https://192.168.10.11:29880/admin/im_system/user_manage/user_list');
 await page.getByText('关系链',{exact:true}).first().click();await page.waitForURL('**/relation_list?userID=*');await page.waitForTimeout(2200);steps.push({route:'relation_list via original user action',ok:true});
 await page.goto('https://192.168.10.11:29880/admin/im_system/group_manage/group_list');
 await page.getByText('群成员',{exact:true}).first().click();await page.waitForURL('**/group_member');await page.waitForTimeout(2200);steps.push({route:'group_member via original group action',ok:true});
 await page.screenshot({path:new URL('./admin-smoke.png',import.meta.url).pathname.replace(/^\/(.:)/,'$1'),fullPage:true});
 const leaked=await page.evaluate(()=>Object.keys(localStorage).some(k=>k==='IMAdminToken'&&localStorage[k]));
 if(leaked)errors.push('IM management token persisted in browser');
}catch(e){errors.push(e.message)}finally{
 await mkdir(new URL('./admin',import.meta.url),{recursive:true});
 await writeFile(new URL('./admin/smoke.json',import.meta.url),JSON.stringify({time:new Date().toISOString(),steps,errors,requests},null,2));await browser.close();
}
console.log(JSON.stringify({steps:steps.length,errors,failedRequests:requests.filter(r=>r.status>=400||(r.code!==undefined&&r.code!==0))}));
if(errors.length)process.exitCode=1;
