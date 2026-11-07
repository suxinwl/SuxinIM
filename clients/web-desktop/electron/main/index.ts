import "./brandRuntime";
import { registerLocalProtocol } from "./localProtocol";
import { app,dialog } from "electron";
import { mkdirSync } from "node:fs";
import { join } from "node:path";
import { createMainWindow } from "./windowManage";
import { createTray } from "./trayManage";
import { setIpcMainListener } from "./ipcHandlerManage";
import {
  checkPreferences,
  performAppStartup,
  setAppGlobalData,
  setAppListener,
  setSingleInstance,
} from "./appManage";
import createAppMenu from "./menuManage";
import { isLinux } from "../utils";
import screenshotsManage from "./screenshotsManage";
import { installScreenshotShortcut } from "./shortcutManage";
import { getLogger } from "../utils/log";
import { initI18n } from "../i18n";
import { registerLocalStore } from "./localStore/runtime";
import {startupStorageError,configuredRoot} from './localStore/bootstrap';
import {initProtectedPreferences} from './storeManage';
import {runLegacyReaderMode} from './localStore/legacyReaderProcess';

export const logger = getLogger(join(app.getPath("userData"), `/OpenIMData/logs`));

const legacyReaderMode=process.argv.includes('--puty-legacy-reader');
if(process.env.OPENIM38_TEST_DATA_DIR && !legacyReaderMode){mkdirSync(process.env.OPENIM38_TEST_DATA_DIR,{recursive:true});app.setPath("userData",process.env.OPENIM38_TEST_DATA_DIR);}

let protocolRegistered=false;
const init = async () => {
  if(!process.env.VITE_DEV_SERVER_URL && !protocolRegistered){ registerLocalProtocol(); protocolRegistered=true; }
  await initI18n();
  if(process.env.OPENIM38_SMOKE_TEST)console.log("stage:create-main");
  createMainWindow();
  if(process.env.OPENIM38_SMOKE_TEST)console.log("stage:main-created");
  createAppMenu();
  if(!process.env.OPENIM38_SMOKE_TEST)createTray();
  if(process.env.OPENIM38_SMOKE_TEST)console.log("stage:screenshots");
  const screenshots = screenshotsManage();
  installScreenshotShortcut(() => screenshots.start(true));
  if(process.env.OPENIM38_SMOKE_TEST)console.log("stage:ready");
};

if(legacyReaderMode){
  void runLegacyReaderMode();
}else{
setAppGlobalData();
registerLocalStore();
performAppStartup();
setIpcMainListener();
setSingleInstance();
setAppListener(init);

app.whenReady().then(async () => {
  const preferences = await initProtectedPreferences(configuredRoot());
  if (startupStorageError || preferences.warning) await dialog.showMessageBox({
    type: 'warning', title: '本次界面偏好未保存',
    message: '界面偏好暂不可用，本次使用内存中的默认设置，界面设置更改不会保存。',
    detail: `原有加密设置、系统密钥和聊天档案均未更改。${startupStorageError ? '当前本地存储配置需要检查，请使用本地存储位置修复入口；修复前不会打开档案。' : '聊天档案仍需独立验证或恢复后才能打开。'}诊断阶段：${startupStorageError ? 'storage-location' : preferences.warning}`,
    buttons: ['继续'], defaultId: 0, noLink: true,
  });
  isLinux ? setTimeout(init, 300) : init();
  checkPreferences();
}).catch(()=>{dialog.showErrorBox('本地偏好存储不可用','无法读取系统保护的界面设置，请检查本地数据目录与系统密钥保护后重新启动。');app.quit();});
}
