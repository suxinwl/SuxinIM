import fs from "fs";
import {
  BrowserWindow,
  Menu,
  app,
  desktopCapturer,
  dialog,
  ipcMain,
  shell,
} from "electron";
import { join } from "path";
import zipUtil from "adm-zip";
import {
  clearCache,
  closeWindow,
  createChildWindow,
  getWebContents,
  hotReload,
  minimize,
  showWindow,
  splashEnd,
  taskFlicker,
  updateMaximize,
} from "./windowManage";
import { t } from "i18next";
import { IpcMainToRender, IpcRenderToMain } from "../constants";
import { getStore,flushPreferences } from "./storeManage";
import { flicker, setTrayTitle } from "./trayManage";
import { setUserCachePath } from "./appManage";
import { downloadAndInstallUpdate } from "./verifiedUpdate";
import { changeLanguage } from "../i18n";

const childWindowMap: { [key: string]: number } = {};

const store = getStore();

export const clearChildWindows = () => {
  for (const key in childWindowMap) {
    const childWindow = BrowserWindow.getAllWindows().find(
      (win) => win.id === childWindowMap[key],
    );
    if (childWindow && !childWindow.isDestroyed()) {
      childWindow.close();
    }
    delete childWindowMap[key];
  }
};

export const openChildWindowHandle = (props) => {
  const { arg, search, key, options } = props;
  if (!childWindowMap[key]) {
    const childWindow = createChildWindow(arg, options, search);
    childWindowMap[key] = childWindow.id;
    return;
  }

  const childWindow = BrowserWindow.getAllWindows().find(
    (win) => win.id === childWindowMap[key],
  );
  if (childWindow) {
    if (childWindow.isMinimized()) {
      childWindow.restore();
    }
    if (childWindow.isVisible()) {
      childWindow.focus();
    } else {
      childWindow.show();
    }
  }
};

export const setIpcMainListener = () => {
  ipcMain.handle("getRuntimeVersions",()=>({app:app.getVersion(),electron:process.versions.electron,userData:app.getPath("userData")}));
  ipcMain.handle(IpcRenderToMain.clearSession, () => {
    clearCache();
  });

  // window manage
  ipcMain.handle("changeLanguage", async (_, locale) => {
    store.set("language", locale);
    await flushPreferences();await changeLanguage(locale);
      app.relaunch();
      app.exit(0);
  });
  ipcMain.handle("main-win-ready", () => {
    splashEnd();
  });
  ipcMain.handle(IpcRenderToMain.showMainWindow, () => {
    showWindow();
  });
  ipcMain.handle(IpcRenderToMain.openChildWindow, (_, props) => {
    openChildWindowHandle(props);
  });
  ipcMain.handle(IpcRenderToMain.minimizeWindow, (_, key) => {
    if (!key) {
      minimize();
      return;
    }
    const childWindow = BrowserWindow.getAllWindows().find(
      (win) => win.id === childWindowMap[key],
    );
    if (childWindow) {
      childWindow.minimize();
    }
  });
  ipcMain.handle(IpcRenderToMain.maxmizeWindow, (_, key) => {
    if (!key) {
      updateMaximize();
      return;
    }
    const childWindow = BrowserWindow.getAllWindows().find(
      (win) => win.id === childWindowMap[key],
    );
    if (childWindow) {
      if (childWindow.isMaximized()) {
        childWindow.unmaximize();
      } else {
        childWindow.maximize();
      }
    }
  });
  ipcMain.handle(IpcRenderToMain.closeWindow, (_, key) => {
    if (!key) {
      closeWindow();
      return;
    }
    const childWindow = BrowserWindow.getAllWindows().find(
      (win) => win.id === childWindowMap[key],
    );
    if (childWindow?.isDestroyed()) {
      delete childWindowMap[key];
    }
    if (childWindow && !childWindow.isDestroyed()) {
      childWindow.close();
      delete childWindowMap[key];
    }
  });
  ipcMain.handle(IpcRenderToMain.showMessageBox, (_, options) => {
    return dialog
      .showMessageBox(BrowserWindow.getFocusedWindow(), options)
      .then((res) => res.response);
  });

  // data transfer
  ipcMain.handle(IpcRenderToMain.transferChooseModalData, (_, { key, data }) => {
    let targetWebContents: Electron.WebContents;
    if (!key) {
      targetWebContents = getWebContents();
    } else {
      targetWebContents = BrowserWindow.getAllWindows().find(
        (win) => win.id === childWindowMap[key],
      ).webContents;
    }
    if (targetWebContents) {
      targetWebContents.send(IpcMainToRender.transferChooseModalData, data);
    }
  });
  ipcMain.handle(IpcRenderToMain.getContactStoreData, (_, { key }) => {
    const targetWebContents = getWebContents();
    targetWebContents.send(IpcMainToRender.getContactStoreData, key);
  });
  ipcMain.handle(IpcRenderToMain.transferContactStoreData, (_, { key, data }) => {
    const targetWebContents = BrowserWindow.getAllWindows().find(
      (win) => win.id === childWindowMap[key],
    )?.webContents;
    if (targetWebContents) {
      targetWebContents.send(IpcMainToRender.transferContactStoreData, data);
    }
  });
  ipcMain.handle(IpcRenderToMain.setKeyStore, async (_, { key, data }) => {
    store.set(key, data);
    await flushPreferences();
  });
  ipcMain.handle(IpcRenderToMain.getKeyStore, (_, { key }) => {
    return store.get(key);
  });
  ipcMain.on(IpcRenderToMain.getKeyStoreSync, (e, { key }) => {
    try{e.returnValue = store.get(key);}catch{e.returnValue=undefined;}
  });
  ipcMain.handle(IpcRenderToMain.showInputContextMenu, () => {
    const menu = Menu.buildFromTemplate([
      {
        label: t("system.copy"),
        type: "normal",
        role: "copy",
        accelerator: "CommandOrControl+c",
      },
      {
        label: t("system.paste"),
        type: "normal",
        role: "paste",
        accelerator: "CommandOrControl+v",
      },
      {
        label: t("system.selectAll"),
        type: "normal",
        role: "selectAll",
        accelerator: "CommandOrControl+a",
      },
    ]);
    menu.popup({
      window: BrowserWindow.getFocusedWindow()!,
    });
  });
  ipcMain.handle(IpcRenderToMain.updateUnreadCount, (_, count) => {
    app.setBadgeCount(count);
    setTrayTitle(count);
    flicker(count > 0);
    if (count > 0) {
      taskFlicker();
    }
  });
  ipcMain.handle("downloadAndInstallUpdate", async (_, metadata) => downloadAndInstallUpdate(metadata));
  ipcMain.handle(IpcRenderToMain.appUpdate, () => { throw new Error("Unverified installer and ASAR hot updates are disabled"); });
  ipcMain.handle(IpcRenderToMain.setUserCachePath, (_, userID) => {
    setUserCachePath(userID);
  });
  ipcMain.on(IpcRenderToMain.getDataPath, (e, key: string) => {
    switch (key) {
      case "public":
        e.returnValue = global.pathConfig.publicPath;
        break;
      case "fileCache":
        e.returnValue = global.pathConfig.fileCachePath;
        break;
      case "sentFileCache":
        e.returnValue = global.pathConfig.sentFileCachePath;
        break;
      case "extraResources":
        e.returnValue = global.pathConfig.extraResourcesPath;
      default:
        e.returnValue = global.pathConfig.publicPath;
        break;
    }
  });
  ipcMain.handle(IpcRenderToMain.dragFile, (e, filePath) => {
    e.sender.startDrag({
      file: filePath,
      icon: global.pathConfig.trayIcon,
    });
  });
  ipcMain.handle(IpcRenderToMain.showLogsInFinder, () => {
    shell.openPath(global.pathConfig.logsPath);
  });
  ipcMain.handle(IpcRenderToMain.prepareUploadLogs, async () => {
    const logsPath = global.pathConfig.logsPath;
    const zip = new zipUtil();
    zip.addLocalFolder(logsPath);
    let date = new Date();
    let dateStr =
      date.getFullYear() + "-" + (date.getMonth() + 1) + "-" + date.getDate();
    const zipPath = join(global.pathConfig.logsPath, `${dateStr}electronlog.zip`);
    await zip.writeZipPromise(zipPath);
    return zipPath;
  });
  ipcMain.handle(IpcRenderToMain.hotRelaunch, hotReload);

  // screen share
  ipcMain.handle(IpcRenderToMain.getScreenSource, async () => {
    const sources = await desktopCapturer.getSources({ types: ["screen"] });
    return sources[0]?.id;
  });
};
