import { ChildWindowOptions } from "@/types/common";

export const openPersonalSettings = () => {
  const options: ChildWindowOptions = {
    width: 600,
    height: 668,
    minWidth: 600,
    frame: false,
    skipTaskbar: true,
  };
  window.electronAPI?.ipcInvoke("openChildWindow", {
    arg: "third/personal-settings",
    key: "personal-settings",
    options,
  });
};

export const openAbout = () => {
  const options: ChildWindowOptions = {
    width: 360,
    height: 400,
    resizable: false,
    frame: false,
    skipTaskbar: true,
  };
  window.electronAPI?.ipcInvoke("openChildWindow", {
    arg: "third/about",
    key: "about",
    options,
  });
};

export const openBlackList = () => {
  const options: ChildWindowOptions = {
    width: 480,
    height: 668,
    resizable: false,
    frame: false,
    skipTaskbar: true,
  };
  window.electronAPI?.ipcInvoke("openChildWindow", {
    arg: "third/black-list",
    key: "black-list",
    options,
  });
};

export const openChangePassword = () => {
  const options: ChildWindowOptions = {
    width: 320,
    height: 380,
    resizable: false,
    frame: false,
    skipTaskbar: true,
  };
  window.electronAPI?.ipcInvoke("openChildWindow", {
    arg: "third/change-password",
    key: "change-password",
    options,
  });
};

export const openChooseContact = (precheck?: string) => {
  const options: ChildWindowOptions = {
    width: 680,
    height: 680,
    resizable: false,
    frame: false,
    skipTaskbar: true,
  };
  window.electronAPI?.ipcInvoke("openChildWindow", {
    arg: `third/choose-contact`,
    key: "choose-contact",
    search: precheck ? `precheck=${precheck}` : undefined,
    options,
  });
};
