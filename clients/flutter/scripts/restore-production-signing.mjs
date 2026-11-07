import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
const [backupPath,passwordPath,destination]=process.argv.slice(2);
if(!backupPath || !passwordPath) throw new Error('Usage: node restore-production-signing.mjs <encrypted-backup> <recovery-passphrase-file> [empty-private-destination]');
const envelope=JSON.parse(fs.readFileSync(backupPath,'utf8'));
if(envelope.format!=='PUTY-AES256GCM-SCRYPT-1' || envelope.kdf?.N!==32768 || envelope.kdf?.r!==8 || envelope.kdf?.p!==1) throw new Error('Unsupported signing backup format');
const key=crypto.scryptSync(fs.readFileSync(passwordPath,'utf8').trim(),Buffer.from(envelope.salt,'base64'),32,{N:32768,r:8,p:1,maxmem:128*1024*1024});
const decipher=crypto.createDecipheriv('aes-256-gcm',key,Buffer.from(envelope.nonce,'base64'));decipher.setAuthTag(Buffer.from(envelope.tag,'base64'));
const payload=JSON.parse(Buffer.concat([decipher.update(Buffer.from(envelope.ciphertext,'base64')),decipher.final()]).toString('utf8'));
const allowed=['puty-im-production.p12','signing.properties','puty-im-production-public.crt'];
if(payload.format!=='PUTY-ANDROID-RELEASE-KEY-1' || Object.keys(payload.files).length!==3 || !allowed.every(n=>typeof payload.files[n]==='string'))throw new Error('Invalid signing backup payload');
const certificate=new crypto.X509Certificate(Buffer.from(payload.files['puty-im-production-public.crt'],'base64'));
if(destination){
 fs.mkdirSync(destination,{recursive:true});if(fs.readdirSync(destination).length)throw new Error('Recovery destination must be an empty private directory');
 for(const name of allowed){let data=Buffer.from(payload.files[name],'base64');if(name==='signing.properties')data=Buffer.from(data.toString('utf8').replace(/^storeFile=.*$/m,'storeFile='+path.resolve(destination,'puty-im-production.p12').replaceAll('\\','/')));fs.writeFileSync(path.join(destination,name),data,{flag:'wx',mode:0o600});}
}
console.log(JSON.stringify({verified:true,restored:Boolean(destination),signingCertificateSHA256:certificate.fingerprint256.replaceAll(':','').toLowerCase(),keystoreSHA256:crypto.createHash('sha256').update(Buffer.from(payload.files['puty-im-production.p12'],'base64')).digest('hex')}));
