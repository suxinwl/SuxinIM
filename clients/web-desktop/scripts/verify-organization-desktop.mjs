// Static proof for the exact final desktop package; no network or accounts.
import asar from '@electron/asar';
import { readFile, writeFile } from 'node:fs/promises';
import { createHash, X509Certificate } from 'node:crypto';
import path from 'node:path';
const client = path.resolve(import.meta.dirname, '..');
const packagePath = path.join(client, 'release/win-unpacked/resources/app.asar');
const hash = bytes => createHash('sha256').update(bytes).digest('hex');
const extract = name => asar.extractFile(packagePath, path.normalize(name));
const pkg = JSON.parse(extract('package.json'));
if (pkg.version !== '3.8.9' || pkg.name !== 'openim-enterprise-upgrade') throw new Error('Desktop package identity changed');
const index = extract('dist/index.html').toString();
if (!index.includes('<title>PUTY内部通讯</title>') || !index.includes('type="image/png"') || !index.includes('favicon-32x32.png?v=647fa79d564a21d1')) throw new Error('Unexpected packaged entry brand');
for (const file of ['favicon-32x32.png', 'icons/icon.png', 'icons/icon.ico', 'icons/tray.png', 'icons/tray@2x.png']) {
  if (hash(extract('dist/' + file)) !== hash(await readFile(path.join(client, 'public', file)))) throw new Error('Unexpected packaged brand asset');
}
const root = new X509Certificate(await readFile(path.join(client, 'release/win-unpacked/resources/extraResources/certificates/Puty-Enterprise-Root-CA.crt')));
const rootSHA = hash(root.raw);
if (rootSHA !== '836c3d6c28d5078cc8da6ebeff8e5f9a7fb7f140554e6a7834011d72299c3d69') throw new Error('Desktop root pin mismatch');
const sdk = JSON.parse(extract('dist/sdk-assets.json'));
for (const item of sdk.assets) if (hash(extract('dist/' + item.file)) !== item.sha256) throw new Error('Packaged SDK bytes mismatch');
const report = { version: pkg.version, build: 380901, displayName:'PUTY内部通讯', stablePackageName:pkg.name, applicationID:'io.openim.enterprise.upgrade38', approvedSourceSHA256:hash(await readFile(path.join(client,'../branding/IM.png'))), packagedPNGIconMatched:true, packagedTrayICOAssetsMatched:true, embeddedRootDER_SHA256:rootSHA, packagedSDKAssetsMatched:sdk.assets.length, installerSHA256:hash(await readFile(path.join(client,'release/PUTY_IM-3.8.9-win-x64.exe'))), authenticodeSigned:false, scope:'Static ASAR/public CA and binary identity; actual installer/UI evidence separate' };
await writeFile(path.join(client,'build/acceptance/organization-desktop-static.json'),JSON.stringify(report,null,2));
console.log(JSON.stringify(report));
