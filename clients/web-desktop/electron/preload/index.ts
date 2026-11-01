import fs from "fs";
import path from "path";
import os from "os";
import { DataPath, IElectronAPI } from "./../../src/types/globalExpose.d";
import { contextBridge, ipcRenderer, shell } from "electron";
import { isProd } from "../utils";

const getPlatform = () => {
  if (process.platform === "darwin") {
    return 4;
  }
  if (process.platform === "win32") {
    return 3;
  }
  return 7;
};

const getOsArch = () => os.arch();

const getDataPath = (key: DataPath) => {
  switch (key) {
    case "public":
      return isProd ? "openim38://app" : "";
    case "extraResources":
      return isProd ? "openim38://app/__extra" : "";
    default:
      return "";
  }
};

const subscribe = (channel: string, callback: (...args: any[]) => void) => {
  const subscription = (_, ...args) => callback(...args);
  ipcRenderer.on(channel, subscription);
  return () => ipcRenderer.removeListener(channel, subscription);
};

const subscribeOnce = (channel: string, callback: (...args: any[]) => void) => {
  ipcRenderer.once(channel, (_, ...args) => callback(...args));
};

const unsubscribeAll = (channel: string) => {
  ipcRenderer.removeAllListeners(channel);
};

const ipcInvoke = (channel: string, ...arg: any) => {
  return ipcRenderer.invoke(channel, ...arg);
};

const ipcSendSync = (channel: string, ...arg: any) => {
  return ipcRenderer.sendSync(channel, ...arg);
};

const fileExists = (path: string) => {
  return fs.existsSync(path);
};

const openFile = (path: string) => {
  shell.openPath(path);
};

const showInFinder = (path: string) => {
  shell.showItemInFolder(path);
};

const getTempFileURL = (path: string) => {
  const file = fs.readFileSync(path);
  const bolb = new Blob([file]);
  return URL.createObjectURL(bolb);
};

const saveFileToDisk = async ({
  file,
  type,
  sync,
}: {
  file: File;
  type: "fileCache" | "sentFileCache";
  sync?: boolean;
}): Promise<string> => {
  const arrayBuffer = await file.arrayBuffer();
  const saveDir = ipcRenderer.sendSync("getDataPath", type);
  const savePath = path.join(saveDir, file.name);
  if (!fs.existsSync(saveDir)) {
    fs.mkdirSync(saveDir, { recursive: true });
  }
  if (sync) {
    await fs.promises.writeFile(savePath, Buffer.from(arrayBuffer));
  } else {
    fs.promises.writeFile(savePath, Buffer.from(arrayBuffer));
  }
  return savePath;
};

const getFileByPath = async (filePath: string) => {
  try {
    const filename = path.basename(filePath);
    const data = await fs.promises.readFile(filePath);
    return new File([data], filename);
  } catch (error) {
    console.log(error);
    return null;
  }
};

const Api: IElectronAPI = {
  getDataPath,
  getVersion: () => process.version,
  getPlatform,
  getOsArch,
  getSystemVersion: process.getSystemVersion,
  subscribe,
  subscribeOnce,
  unsubscribeAll,
  ipcInvoke,
  ipcSendSync,
  fileExists,
  openFile,
  showInFinder,
  getTempFileURL,
  saveFileToDisk,
  getFileByPath,
};

contextBridge.exposeInMainWorld("electronAPI", Api);
