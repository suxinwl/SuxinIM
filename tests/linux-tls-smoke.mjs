import {chromium} from 'playwright';
const browser=await chromium.launch({headless:true});
try{const page=await browser.newPage();const response=await page.goto('https://192.168.10.11:29880/',{waitUntil:'domcontentloaded'});if(response.status()!==200)throw new Error('HTTPS rejected');console.log(JSON.stringify({normalTLS:true,status:response.status(),ignoreHTTPSErrors:false,node:process.version}));}finally{await browser.close();}
