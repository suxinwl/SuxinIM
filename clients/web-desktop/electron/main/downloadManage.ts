import fs from "fs";
import path from "path";
import { IpcMainToRender, IpcRenderToMain } from "../constants";
import { ipcMain } from "electron";
import { setProgressBar } from "./windowManage";

interface SearchObjectType {
  "is-update"?: string;
  "save-type"?: string;
  "random-prefix"?: string;
}

const customTypes = ["image", "video", "avatar"];
const getFileType = (type?: string) => {
  if (customTypes.includes(type)) return type;
  return "file";
};

const getRealUrl = (item: Electron.DownloadItem) => item.getURLChain()[0];

export const initDownloadManage = (webContents: Electron.WebContents) => {
  const downloadItems = [] as Electron.DownloadItem[];

  const webContentsSend = (channel: string, ...args: any[]) => {
    if (webContents.isDestroyed()) return;
    webContents.send(channel, ...args);
  };

  webContents.session.on("will-download", (_, item) => {
    downloadItems.push(item);
    const realUrl = getRealUrl(item);
    const searchParams = new URL(realUrl).searchParams;
    const searchObject = {} as SearchObjectType;

    for (const [key, value] of searchParams.entries()) {
      searchObject[key] = value;
    }
    const isUpdate = !!searchObject["is-update"];

    if (isUpdate) {
      item.setSavePath(
        path.join(global.pathConfig.autoUpdateCachePath, item.getFilename()),
      );
    } else {
      const fileType = getFileType(searchObject["save-type"]);
      const fileNamePrefix = searchObject["random-prefix"] ?? "";
      let savePath = path.join(
        global.pathConfig[`${fileType}CachePath`],
        `${fileNamePrefix}${item.getFilename()}`,
      );
      item.setSavePath(savePath);
    }

    item.on("updated", (_, state) => {
      if (state === "interrupted") {
        webContentsSend(
          IpcMainToRender[isUpdate ? "updateDownloadPaused" : "downloadPaused"],
          realUrl,
        );
      } else if (state === "progressing") {
        if (!item.isPaused()) {
          const receivedBytes = item.getReceivedBytes();
          const totalBytes = item.getTotalBytes();
          const progress = Math.round((receivedBytes / totalBytes) * 100);
          webContentsSend(
            IpcMainToRender[isUpdate ? "uploadDownloadProgress" : "downloadProgress"],
            realUrl,
            progress,
          );
          if (isUpdate) setProgressBar(progress);
        }
      }
    });

    item.once("done", (_, state) => {
      const successEvent =
        IpcMainToRender[isUpdate ? "updateDownloadSuccess" : "downloadSuccess"];
      const failedEvent =
        IpcMainToRender[isUpdate ? "updateDownloadFailed" : "downloadFailed"];
      webContentsSend(
        state === "completed" ? successEvent : failedEvent,
        realUrl,
        item.getSavePath(),
      );
      if (isUpdate) setProgressBar(-1);

      const itemIndex = downloadItems.indexOf(item);
      if (itemIndex !== -1) {
        downloadItems.splice(itemIndex, 1);
      }
    });
  });

  // ipcMain
  ipcMain.handle(IpcRenderToMain.startDownload, (_, url: string) => {
    webContents.session.downloadURL(url);
  });

  ipcMain.handle(IpcRenderToMain.pauseDownload, (_, url: string) => {
    const item = downloadItems.find((item) => getRealUrl(item) === url);
    if (item && !item.isPaused()) {
      item.pause();
    }
  });

  ipcMain.handle(IpcRenderToMain.resumeDownload, (_, url: string) => {
    const item = downloadItems.find((item) => getRealUrl(item) === url);
    if (item && item.isPaused()) {
      item.resume();
    }
  });

  ipcMain.handle(IpcRenderToMain.cancelDownload, (_, url: string) => {
    const item = downloadItems.find((item) => getRealUrl(item) === url);
    if (item) {
      item.cancel();
      const itemIndex = downloadItems.indexOf(item);
      if (itemIndex !== -1) {
        downloadItems.splice(itemIndex, 1);
        cleanupTemporaryFile(item.getSavePath());
      }
    }
  });

  return () => {
    downloadItems.forEach((item) => {
      item.cancel();
      cleanupTemporaryFile(item.getSavePath());
    });
  };
};

const cleanupTemporaryFile = (filePath: string) => {
  if (fs.existsSync(filePath)) {
    const tempFileDir = path.dirname(filePath);
    fs.unlinkSync(filePath);
    if (fs.readdirSync(tempFileDir).length === 0) {
      fs.rmdirSync(tempFileDir);
    }
  }
};
