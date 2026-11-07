import test from 'node:test';
import assert from 'node:assert/strict';
import {promises as fs} from 'node:fs';
import path from 'node:path';
import {tmpdir} from 'node:os';
import {execFileSync} from 'node:child_process';

test('real NSIS path function adds one exact puty_im leaf and preserves registered upgrade locations without installing anything',{skip:process.platform!=='win32'},async t=>{
  const temporary=await fs.realpath(await fs.mkdtemp(path.join(tmpdir(),'puty-nsis-path-')));
  t.after(async()=>{assert.equal(path.dirname(temporary),await fs.realpath(tmpdir()));assert.ok(path.basename(temporary).startsWith('puty-nsis-path-'));await fs.rm(temporary,{recursive:true,force:true});});
  const include=path.resolve('scripts/nsis-enterprise-trust.nsh'),source=await fs.readFile(include,'utf8'),builder=JSON.parse(await fs.readFile('electron-builder.json5','utf8'));
  assert.equal(builder.appId,'io.openim.enterprise.upgrade38');assert.equal(builder.executableName,'PUTY_IM');assert.equal(builder.nsis.deleteAppDataOnUninstall,false);
  assert.match(source,/certutil\.exe.*-addstore -f Root/);assert.doesNotMatch(source,/RMDir|Delete\s/);
  const cases=[
    ['C:\\','','','C:\\puty_im'],['D:\\','','','D:\\puty_im'],['D:\\Apps','','','D:\\Apps\\puty_im'],
    ['D:\\Apps\\','','','D:\\Apps\\puty_im'],['D:\\puty_im-parent','','','D:\\puty_im-parent\\puty_im'],
    ['D:\\puty_im\\child','','','D:\\puty_im\\child\\puty_im'],['D:\\puty_im','','','D:\\puty_im'],
    ['D:\\PUTY_IM\\','','','D:\\puty_im'],['D:\\Space Parent\\Subfolder','','','D:\\Space Parent\\Subfolder\\puty_im'],
    ['C:\\Old Enterprise','C:\\Old Enterprise','','C:\\Old Enterprise'],
    ['C:\\OLD Enterprise\\','c:\\old enterprise','','C:\\OLD Enterprise'],
    ['D:\\PerUser Old','','D:\\PerUser Old','D:\\PerUser Old'],
    ['D:\\Explicit New','C:\\Old Enterprise','','D:\\Explicit New\\puty_im']
  ];
  const exe=path.join(temporary,'path-check.exe'),output=path.join(temporary,'results.txt'),script=path.join(temporary,'test.nsi');
  const literal=value=>value.replaceAll('$','$$');
  const commands=cases.map(([input,machine,user])=>`StrCpy $INSTDIR "${literal(input)}"\nStrCpy $PutyMachineInstallLocation "${literal(machine)}"\nStrCpy $PutyUserInstallLocation "${literal(user)}"\nStrCpy $0 "REGISTER-PRESERVED"\nCall PutyInvokeOfficialDirectoryPre\nCall PutyNormalizeInstallDirectory\nFileWrite $8 "$INSTDIR|$0$\\r$\\n"`).join('\n');
  await fs.writeFile(script,`Unicode true\nRequestExecutionLevel user\nSilentInstall silent\nOutFile "${exe}"\nInstallDir "$TEMP\\synthetic-only"\n!define APP_FILENAME "PUTY_IM"\n!define PRODUCT_FILENAME "PUTY_IM"\n!define APP_GUID "PUTY-SYNTHETIC-NEVER-WRITTEN"\n!include "${include}"\n!define MUI_PAGE_CUSTOMFUNCTION_PRE OriginalUnusedPre\n!insertmacro customPageAfterChangeDir\n!if "\${MUI_PAGE_CUSTOMFUNCTION_PRE}" != "PutyInstallFilesPre"\n!error "PRE was not replaced"\n!endif\n!if "\${APP_FILENAME}" != "puty_im"\n!error "default installation leaf changed"\n!endif\n!if "\${PRODUCT_FILENAME}" != "PUTY_IM"\n!error "executable identity changed"\n!endif\n!if "\${INSTALL_REGISTRY_KEY}" != "Software\\PUTY-SYNTHETIC-NEVER-WRITTEN"\n!error "default registration identity changed"\n!endif\nFunction .onInit\n!insertmacro customInit\nFunctionEnd\nSection\nFileOpen $8 "${output}" w\nFileWrite $8 "$INSTDIR$\\r$\\n"\n${commands}\nFileClose $8\nSectionEnd\n`);
  const scriptText=await fs.readFile(script,'utf8');
  await fs.writeFile(script,scriptText.replace('Function .onInit',`Var OfficialPreCalls\nFunction instFilesPre\nIntOp $OfficialPreCalls $OfficialPreCalls + 1\nStrCpy $INSTDIR "$INSTDIR\\SYNTHETIC-OFFICIAL-APPEND"\nStrCpy $0 "OFFICIAL-REGISTER-CHANGED"\nFunctionEnd\nFunction .onInit`).replace('FileClose $8','FileWrite $8 "$OfficialPreCalls$\\r$\\n"\nFileClose $8'));
  const makensis=path.join(process.env.LOCALAPPDATA,'electron-builder','Cache','nsis','nsis-3.0.4.1','Bin','makensis.exe');
  execFileSync(makensis,['/WX','/V2',script],{windowsHide:true,stdio:'pipe',timeout:30000});
  execFileSync(exe,['/S','/D=D:\\Explicit-Silent-Parent'],{windowsHide:true,stdio:'pipe',timeout:30000});
  const actual=(await fs.readFile(output,'utf8')).trimEnd().split(/\r?\n/);assert.equal(actual.shift(),'D:\\Explicit-Silent-Parent\\puty_im');assert.equal(actual.pop(),'14');assert.deepEqual(actual,cases.map(row=>row[3]+'|REGISTER-PRESERVED'));
});
