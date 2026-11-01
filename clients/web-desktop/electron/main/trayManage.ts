import { app, Menu, Tray } from "electron";
import { t } from "i18next";
import { hideWindow, showWindow } from "./windowManage";

let appTray: Tray;
let timer: NodeJS.Timeout | null = null;

export const createTray = () => {
  const trayMenu = Menu.buildFromTemplate([
    {
      label: t("system.showWindow"),
      click: showWindow,
    },
    {
      label: t("system.hideWindow"),
      click: hideWindow,
    },
    {
      label: t("system.quit"),
      click: () => {
        global.forceQuit = true;
        app.quit();
      },
    },
  ]);
  appTray = new Tray(global.pathConfig.trayIcon);
  appTray.setToolTip(app.getName());
  appTray.setIgnoreDoubleClickEvents(true);
  appTray.on("click", showWindow);

  appTray.setContextMenu(trayMenu);
};

export const destroyTray = () => {
  if (!appTray || appTray.isDestroyed()) return;
  appTray.destroy();
  appTray = null;
};

export const setTrayTitle = (num: number) => {
  if (!appTray || appTray.isDestroyed()) {
    return;
  }
  appTray.setTitle(num === 0 ? "" : num > 99 ? "99+" : num + "");
};

export const flicker = (isFlicker: boolean) => {
  if (!appTray || appTray.isDestroyed()) {
    return;
  }
  if (process.platform === "darwin") return;
  if (isFlicker) {
    if (timer !== null) return;
    let count = 0;
    timer = setInterval(() => {
      if (!appTray || appTray.isDestroyed()) {
        clearInterval(timer);
        return;
      }
      count++;
      try {
        if (count % 2 == 0) {
          appTray.setImage(global.pathConfig.emptyTrayIcon);
        } else {
          appTray.setImage(global.pathConfig.trayIcon);
        }
      } catch (error) {
        console.log(error);
      }
    }, 500);
  } else {
    clearInterval(timer);
    timer = null;
    try {
      appTray.setImage(global.pathConfig.trayIcon);
    } catch (error) {
      console.log(error);
    }
  }
};
