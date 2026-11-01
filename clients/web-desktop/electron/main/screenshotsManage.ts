import { clipboard, ipcMain, nativeImage } from "electron";
import { t } from "i18next";
import Screenshots from "electron-screenshots";
import { hideWindow, sendEvent, showWindow, toggleWindowVisible } from "./windowManage";
import { IpcMainToRender, IpcRenderToMain } from "../constants";
import { isWin } from "../utils";
import { createScreenshotSession } from "./screenshotSession";

let manager: ReturnType<typeof createScreenshotSession> | undefined;

export default () => {
  // A recreated main window must reuse the backend and its own IPC listeners.
  if (manager) return manager;
  const screenshots = new Screenshots({
    singleWindow: true,
    lang: {
      magnifier_position_label: t("system.magnifier_position_label"),
    },
  });
  const originalEndCapture = screenshots.endCapture.bind(screenshots);
  let endCapture: Promise<void> | undefined;
  // The dependency also closes after Save; both paths share one actual reset.
  screenshots.endCapture = () =>
    (endCapture ??= Promise.resolve().then(originalEndCapture));

  const session = createScreenshotSession({
    beforeCapture: (hidden) => {
      if (hidden) isWin ? toggleWindowVisible(false) : hideWindow();
    },
    startCapture: () => {
      endCapture = undefined;
      return screenshots.startCapture();
    },
    endCapture: async () => {
      try {
        await screenshots.endCapture();
      } finally {
        // An interrupted reset must not leave the input-blocking overlay open.
        const win = screenshots.$win;
        if (win && !win.isDestroyed() && win.isVisible()) {
          win.setKiosk(false);
          win.hide();
        }
      }
    },
    restoreWindow: () => {
      if (isWin) toggleWindowVisible(true);
      showWindow();
    },
    writeClipboard: (bytes) => {
      clipboard.writeImage(nativeImage.createFromBuffer(Buffer.from(bytes)));
    },
    respond: (bytes) => {
      // Return only once the overlay has closed, without a timer that can race
      // with a later screenshot or focus the editor below a live capture view.
      const result = bytes
        ? `data:image/png;base64,${Buffer.from(bytes).toString("base64")}`
        : undefined;
      sendEvent(IpcMainToRender.captureResponse, result);
    },
    reportError: (error) => console.error("Screenshot capture failed", error),
  });

  const cancelOnEscape = (event: Electron.Event, input: Electron.Input) => {
    if (input.type === "keyDown" && input.key === "Escape") {
      event.preventDefault();
      void session.cancel();
    }
  };
  screenshots.$view.webContents.on("before-input-event", cancelOnEscape);
  screenshots.on("windowCreated", (win) => {
    win.webContents.on("before-input-event", cancelOnEscape);
    win.on("closed", () => void session.cancel());
  });
  screenshots.on("cancel", (event) => {
    event.preventDefault();
    void session.cancel();
  });
  screenshots.on("ok", (event, bytes) => {
    // The library emits this before its own endCapture. Own the complete async
    // lifecycle so clipboard errors and duplicate clicks cannot strand it.
    event.preventDefault();
    void session.confirm(bytes);
  });
  screenshots.on("afterSave", (_event, _bytes, _data, saved) => {
    if (saved) void session.cancel();
  });

  ipcMain.handle(IpcRenderToMain.capture, (_event, hidden = false) =>
    session.start(Boolean(hidden)),
  );
  manager = session;
  return manager;
};
