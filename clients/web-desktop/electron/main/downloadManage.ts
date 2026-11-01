import fs from "node:fs";
import path from "node:path";
import { ipcMain } from "electron";
import { IpcMainToRender, IpcRenderToMain } from "../constants";
import type { AttachmentDownloadRequest } from "../../src/im/attachment-download";
import {
  attachmentSavePath,
  downloadRequestKey,
  normalizeDownloadRequest,
} from "./downloadTarget";

export const initDownloadManage = (webContents: Electron.WebContents) => {
  const active = new Map<Electron.DownloadItem, AttachmentDownloadRequest>();
  const pending = new Map<
    string,
    { request: AttachmentDownloadRequest; timer: ReturnType<typeof setTimeout> }
  >();
  const send = (channel: string, ...args: unknown[]) => {
    if (!webContents.isDestroyed()) webContents.send(channel, ...args);
  };

  const onDownload = (event: Electron.Event, item: Electron.DownloadItem) => {
    const key = downloadRequestKey(item.getURLChain()[0] || item.getURL());
    const entry = pending.get(key);
    if (entry) {
      clearTimeout(entry.timer);
      pending.delete(key);
    }
    // URL queries are storage-server data. Only explicit IPC metadata can
    // select an image/video cache; do not interpret remote query parameters.
    const request =
      entry?.request ??
      normalizeDownloadRequest(item.getURLChain()[0] || item.getURL());
    try {
      const cacheDirectory =
        global.pathConfig[`${request.saveType ?? "file"}CachePath`];
      const savePath = attachmentSavePath(
        cacheDirectory,
        item.getFilename(),
        request.randomPrefix,
      );
      fs.mkdirSync(path.dirname(savePath), { recursive: true });
      item.setSavePath(savePath);
    } catch {
      event.preventDefault();
      item.cancel();
      send(IpcMainToRender.downloadFailed, request.url);
      return;
    }
    active.set(item, request);
    item.on("updated", (_, state) => {
      if (state === "interrupted") {
        send(IpcMainToRender.downloadPaused, request.url);
      } else if (state === "progressing" && !item.isPaused()) {
        const total = item.getTotalBytes();
        const progress =
          total > 0
            ? Math.min(100, Math.round((item.getReceivedBytes() / total) * 100))
            : 0;
        send(IpcMainToRender.downloadProgress, request.url, progress);
      }
    });
    item.once("done", (_, state) => {
      active.delete(item);
      if (state !== "completed") cleanupTemporaryFile(item.getSavePath());
      send(
        state === "completed"
          ? IpcMainToRender.downloadSuccess
          : IpcMainToRender.downloadFailed,
        request.url,
        item.getSavePath(),
      );
    });
  };
  webContents.session.on("will-download", onDownload);

  ipcMain.handle(IpcRenderToMain.startDownload, (event, input: unknown) => {
    if (event.sender !== webContents) throw new Error("Invalid attachment window");
    const request = normalizeDownloadRequest(input);
    const key = downloadRequestKey(request.url);
    if (
      pending.has(key) ||
      [...active.values()].some((value) => value.url === request.url)
    )
      return;
    const timer = setTimeout(() => {
      if (pending.delete(key)) send(IpcMainToRender.downloadFailed, request.url);
    }, 30_000);
    pending.set(key, { request, timer });
    try {
      // Preserve signed URL bytes; cache metadata travels separately in IPC.
      webContents.session.downloadURL(request.url);
    } catch (error) {
      clearTimeout(timer);
      pending.delete(key);
      throw error;
    }
  });

  const findItem = (url: string) =>
    [...active].find(([, request]) => request.url === url)?.[0];
  ipcMain.handle(IpcRenderToMain.pauseDownload, (event, url: string) => {
    if (event.sender !== webContents) return;
    const item = findItem(url);
    if (item && !item.isPaused()) item.pause();
  });
  ipcMain.handle(IpcRenderToMain.resumeDownload, (event, url: string) => {
    if (event.sender !== webContents) return;
    const item = findItem(url);
    if (item?.isPaused()) item.resume();
  });
  ipcMain.handle(IpcRenderToMain.cancelDownload, (event, url: string) => {
    if (event.sender !== webContents) return;
    const key = downloadRequestKey(url);
    const entry = pending.get(key);
    if (entry) {
      clearTimeout(entry.timer);
      pending.delete(key);
    }
    findItem(url)?.cancel();
    send(IpcMainToRender.downloadCancel, url);
  });

  return () => {
    webContents.session.removeListener("will-download", onDownload);
    for (const { timer } of pending.values()) clearTimeout(timer);
    pending.clear();
    for (const item of active.keys()) {
      item.cancel();
      cleanupTemporaryFile(item.getSavePath());
    }
    active.clear();
    for (const name of [
      "startDownload",
      "pauseDownload",
      "resumeDownload",
      "cancelDownload",
    ] as const)
      ipcMain.removeHandler(IpcRenderToMain[name]);
  };
};

const cleanupTemporaryFile = (filePath: string) => {
  if (!filePath) return;
  try {
    if (fs.existsSync(filePath) && fs.statSync(filePath).isFile())
      fs.unlinkSync(filePath);
  } catch {
    /* Do not interrupt download events when cleanup fails. */
  }
};
