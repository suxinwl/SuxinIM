import {chromium} from '../clients/web-desktop/node_modules/playwright/index.mjs';
import {readFile,writeFile,unlink} from 'node:fs/promises';
import {createHash,randomBytes,randomUUID} from 'node:crypto';
const root=new URL('../',import.meta.url),state=JSON.parse(await readFile(new URL('.private/test-accounts.json',root),'utf8'));
const base='https://192.168.10.11:29880',results=[],temporary=randomBytes(18).toString('hex'),digest=s=>createHash('md5').update(s).digest('hex');
async function call(path,body,token){const response=await fetch(base+path,{method:'POST',headers:{'Content-Type':'application/json',operationID:randomUUID(),token:token||'',chatToken:token||''},body:JSON.stringify(body)});const value=await response.json();return {status:response.status,...value};}
const login=password=>call('/enterprise/v1/admin/login',{account:state.admin.account,password:digest(password)});
const auth=await login(state.admin.password);if(auth.errCode!==0)throw new Error('Synthetic admin login failed');
const adminToken=auth.data.adminToken,profile=await call('/enterprise/v1/admin/chat/account/info',{},adminToken);
if(profile.errCode!==0||'password' in profile.data)throw new Error('Profile returned an error or exposed password hash');
results.push({case:'Real account/info response omits password credentials',passed:true});
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE});let changed=false,restored=false,page;const requests=[],pageErrors=[];
await writeFile(new URL('.private/admin-profile-pending.json',root),JSON.stringify({temporary,adminToken,userID:profile.data.userID}));
try{
 page=await browser.newPage();page.on('response',r=>{if(/enterprise|account|change_password/.test(new URL(r.url()).pathname))requests.push({path:new URL(r.url()).pathname,status:r.status()});});page.on('pageerror',e=>pageErrors.push(e.message));await page.goto(base+'/admin/login');await page.locator('#basic_account').fill(state.admin.account);await page.locator('#basic_secrect').fill(state.admin.password);await page.locator('button[type=submit]').click();await page.waitForURL('**/business_system/user_manage/user_list');
 await page.goto(base+'/admin/profile/admin_password');
 for(const current of ['wrong-current-'+randomUUID(),state.admin.password]){
  await page.getByLabel('当前密码',{exact:true}).fill(current);await page.getByLabel('新密码',{exact:true}).fill(temporary);await page.getByLabel('确认密码',{exact:true}).fill(temporary);
  const [response]=await Promise.all([page.waitForResponse(r=>new URL(r.url()).pathname.endsWith('/account/change_password')),page.locator('form button[type=submit]').click()]);const value=await response.json();
  if(current!==state.admin.password){if(value.errCode===0)throw new Error('Wrong current password was accepted');results.push({case:'Original admin form rejects wrong current password',passed:true});}
  else{if(value.errCode!==0)throw new Error('Admin form did not change password');changed=true;await page.waitForFunction(()=>document.querySelector('#password')?.value==='');results.push({case:'Original admin form invokes correct Chat password endpoint and clears transient values',passed:true});}
 }
 const old=await login(state.admin.password),fresh=await login(temporary);if(old.errCode===0||fresh.errCode!==0)throw new Error('Changed password login verification failed');
 results.push({case:'Old password rejected and new password authenticates',passed:true});
}catch(error){const diagnostics=await page?.evaluate(()=>({formErrors:[...document.querySelectorAll('.ant-form-item-explain-error')].map(e=>e.textContent),toasts:[...document.querySelectorAll('.ant-message-notice-content')].map(e=>e.textContent),fields:[...document.querySelectorAll('form input')].map(e=>({id:e.id,length:e.value.length,type:e.type}))})).catch(()=>null);await page?.screenshot({path:new URL('tests/admin/profile-form.png',root).pathname.replace(/^\/(.:)/,'$1')}).catch(()=>{});results.push({case:'Admin profile regression',passed:false,error:String(error),diagnostics});process.exitCode=1;}
finally{
 if(changed){const response=await call('/enterprise/v1/admin/chat/account/change_password',{userID:profile.data.userID,currentPassword:digest(temporary),newPassword:digest(state.admin.password)},adminToken);restored=response.errCode===0&&(await login(state.admin.password)).errCode===0;if(!restored)process.exitCode=1;}
 if(restored||!changed)await unlink(new URL('.private/admin-profile-pending.json',root)).catch(()=>{});
 await browser.close();await writeFile(new URL('tests/admin/profile-report.json',root),JSON.stringify({results,requests,pageErrors,syntheticPasswordRestored:restored},null,2));console.log(JSON.stringify({results,requests,pageErrors,syntheticPasswordRestored:restored}));
}
