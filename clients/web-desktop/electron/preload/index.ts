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
  return Boolean(ipcRenderer.sendSync("puty-file-exists", path));
};

const openFile = (path: string) => {
  void ipcRenderer.invoke("puty-store", "export-file", {path}).catch(()=>{});
};

const showInFinder = (path: string) => {
  void ipcRenderer.invoke("puty-store", "show-folder", {path}).catch(()=>{});
};

const getTempFileURL = (path: string) => {
  return `openim38://app/__local?path=${encodeURIComponent(path)}`;
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
  const saved=await ipcRenderer.invoke("puty-store","stash-file",{name:file.name,type:file.type,bytes:arrayBuffer});
  return saved.path;
};

const getFileByPath = async (filePath: string) => {
  try {const file=await ipcRenderer.invoke("puty-store","file-info",{path:filePath});return new File([file.bytes],file.name,{type:file.type});}
  catch {return null;}
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
