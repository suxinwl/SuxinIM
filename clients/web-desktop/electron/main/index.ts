import { registerLocalProtocol } from "./localProtocol";
import { app } from "electron";
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
import { getLogger } from "../utils/log";
import { initI18n } from "../i18n";

export const logger = getLogger(join(app.getPath("userData"), `/OpenIMData/logs`));

app.setName("OpenIM38 Enterprise");
if(process.env.OPENIM38_TEST_DATA_DIR){mkdirSync(process.env.OPENIM38_TEST_DATA_DIR,{recursive:true});app.setPath("userData",process.env.OPENIM38_TEST_DATA_DIR);}

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
  screenshotsManage();
  if(process.env.OPENIM38_SMOKE_TEST)console.log("stage:ready");
};

setAppGlobalData();
performAppStartup();
setIpcMainListener();
setSingleInstance();
setAppListener(init);

app.whenReady().then(() => {
  isLinux ? setTimeout(init, 300) : init();
  checkPreferences();
});
