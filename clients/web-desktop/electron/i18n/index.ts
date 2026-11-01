import i18n from "i18next";
import enUS from "./resources/en-US";
import zhCN from "./resources/zh-CN";
import { join } from "node:path";

import { getStore } from "../main/storeManage";
import { app } from "electron";

const store = getStore();

export const initI18n = async () => {
  const systemLanguage = app.getLocale();
  const language = store.get("language", systemLanguage) as string;

  console.log(language);

  await i18n.init({
    lng: language,
    fallbackLng: "en-US",
    resources:{"en-US":{translation:enUS},"zh-CN":{translation:zhCN}},
  });
};

export const changeLanguage = i18n.changeLanguage;
