import { app, shell, systemPreferences } from "electron";
import { isExistMainWindow, showWindow } from "./windowManage";
import { join } from "node:path";
import { release } from "node:os";
import { isMac, isProd, isWin } from "../utils";
import { getStore } from "./storeManage";
import { singleInstanceLock } from "../config";
import { checkClearLogs } from "../utils/log";
import { flicker } from "./trayManage";

const store = getStore();

export const setSingleInstance = () => {
  if (!singleInstanceLock) return;

  if (!app.requestSingleInstanceLock()) {
    app.quit();
    process.exit(0);
  }

  app.on("second-instance", () => {
    showWindow();
  });
};

export const setAppListener = (startApp: () => void) => {
  app.on("web-contents-created", (event, contents) => {
    contents.setWindowOpenHandler(({ url }) => {
      if (!/^devtools/.test(url) && /^https?:\/\//.test(url)) {
        shell.openExternal(url);
      }
      return { action: "deny" };
    });
  });

  app.on("activate", () => {
    if (isExistMainWindow()) {
      showWindow();
    } else {
      startApp();
    }
  });

  app.on("window-all-closed", () => {
    if (isMac && !getIsForceQuit()) return;

    app.quit();
  });

  app.on("quit", () => {
    checkClearLogs();
    flicker(false);
  });
};

export const performAppStartup = () => {
  app.setAppUserModelId(app.getName());

  app.commandLine.appendSwitch("--autoplay-policy", "no-user-gesture-required");
  app.commandLine.appendSwitch(
    "disable-features",
    "HardwareMediaKeyHandling,MediaSessionService",
  );

  // Disable GPU Acceleration for Windows 7
  if (release().startsWith("6.1")) app.disableHardwareAcceleration();
};

export const setAppGlobalData = () => {
  const electronDistPath = join(__dirname, "../");
  const distPath = join(electronDistPath, "../dist");
  const publicPath = isProd ? distPath : join(electronDistPath, "../public");
  const asarPath = join(distPath, "/../..");

  global.pathConfig = {
    electronDistPath,
    distPath,
    publicPath,
    asarPath,
    logsPath: join(app.getPath("userData"), `/OpenIMData/logs`),
    autoUpdateCachePath: join(app.getPath("userData"), `/OpenIMData/autoUpdateCache`),
    extraResourcesPath: join(asarPath, "/extraResources"),
    trayIcon: join(publicPath, `/icons/${isWin ? "icon.ico" : "tray.png"}`),
    emptyTrayIcon: join(publicPath, `/icons/${"empty_tray.png"}`),
    indexHtml: join(distPath, "index.html"),
    splashHtml: join(distPath, "splash.html"),
    preload: join(__dirname, "../preload/index.js"),
  };
};

export const setUserCachePath = (userID: string) => {
  global.pathConfig.imageCachePath = join(
    app.getPath("userData"),
    `/OpenIMData/${userID}/imageCache`,
  );
  global.pathConfig.videoCachePath = join(
    app.getPath("userData"),
    `/OpenIMData/${userID}/videoCache`,
  );
  global.pathConfig.fileCachePath = join(
    app.getPath("userData"),
    `/OpenIMData/${userID}/fileCache`,
  );
  global.pathConfig.avatarCachePath = join(
    app.getPath("userData"),
    `/OpenIMData/${userID}/avatarCache`,
  );
  global.pathConfig.sentFileCachePath = join(
    app.getPath("userData"),
    `/OpenIMData/${userID}/sentFileCache`,
  );
};

export const checkPreferences = () => {
  if (process.platform !== "darwin") {
    return;
  }
  const version = release().split(".")[0];
  if (Number(version) < 21) {
    return;
  }
  let privileges = [];
  privileges.push(systemPreferences.getMediaAccessStatus("camera"));
  privileges.push(systemPreferences.getMediaAccessStatus("microphone"));
  privileges.push(systemPreferences.getMediaAccessStatus("screen"));

  privileges.map(async (privilege, idx) => {
    if (privilege !== "granted") {
      if (idx === 0) {
        await systemPreferences.askForMediaAccess("camera");
      }

      if (idx === 1) {
        await systemPreferences.askForMediaAccess("microphone");
      }
    }
  });
};

export const getIsForceQuit = () =>
  store.get("closeAction") === "quit" || global.forceQuit;
