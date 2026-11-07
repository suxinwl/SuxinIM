import { app, BrowserWindow, globalShortcut, ipcMain, IpcMainInvokeEvent } from "electron";
import { randomUUID } from "node:crypto";
import { toggleDevTools } from "./windowManage";
import { createScreenshotShortcutManager } from "./screenshotShortcut";
import { getStore, hasPersistentPreferences, saveScreenshotShortcutPreference } from "./storeManage";

let screenshotShortcut: ReturnType<typeof createScreenshotShortcutManager> | undefined;

export function installScreenshotShortcut(capture: () => Promise<unknown>) {
  if (screenshotShortcut) return screenshotShortcut;
  const manager = createScreenshotShortcutManager({
    register: (key, callback) => globalShortcut.register(key, callback),
    unregister: key => globalShortcut.unregister(key),
    load: () => getStore().get("screenshotShortcut"), save: saveScreenshotShortcutPreference,
    persistent: hasPersistentPreferences, capture,
  });
  screenshotShortcut = manager;
  const recordings = new Map<number, { token: string; clear: () => void }>();
  const trusted = (event: IpcMainInvokeEvent) => {
    const url = event.senderFrame?.url ?? "";
    if (!url.startsWith("openim38://app/") && !(process.env.VITE_DEV_SERVER_URL && url.startsWith("http://127.0.0.1:7777/"))) throw new Error("Untrusted screenshot shortcut settings window");
  };
  ipcMain.handle("puty-screenshot-shortcut", async (event, operation: unknown, input?: any) => {
    trusted(event);
    if (operation === "get") return manager.state();
    if (operation === "set") return manager.set(input);
    if (operation === "end-record") {
      const recording = recordings.get(event.sender.id);
      if (recording && input?.token === recording.token) recording.clear();
      // Unknown/old tokens are harmless, and cannot release a newer recorder.
      return { ok: true, state: await manager.suspend(recordings.size > 0) };
    }
    if (operation === "begin-record") {
      recordings.get(event.sender.id)?.clear();
      const sender = event.sender, token = randomUUID(), window = BrowserWindow.fromWebContents(sender);
      const clear = () => {
        if (recordings.get(sender.id)?.token !== token) return;
        recordings.delete(sender.id); clearTimeout(timeout);
        sender.off("destroyed", clear); sender.off("render-process-gone", clear); window?.off("blur", clear);
        void manager.suspend(recordings.size > 0);
        if (!sender.isDestroyed()) sender.send("puty-screenshot-shortcut-recording-ended", { token });
      };
      const timeout = setTimeout(clear, 120000); timeout.unref();
      recordings.set(sender.id, { token, clear });
      sender.once("destroyed", clear); sender.once("render-process-gone", clear); window?.once("blur", clear);
      return { ok: true, token, state: await manager.suspend(true) };
    }
    return { ok: false, code: "invalid", state: manager.state() };
  });
  app.once("will-quit", () => {
    manager.dispose();
    for (const recording of [...recordings.values()]) recording.clear();
    unregisterShortcuts();
  });
  return manager;
}

export const registerShortcuts = () => {
  globalShortcut.register("CmdOrCtrl+F12", toggleDevTools);
};

export const unregisterShortcuts = () => {
  // Window blur must leave the independently managed global screenshot key.
  globalShortcut.unregister("CmdOrCtrl+F12");
};
