import { CustomEmojiItem } from "@/pages/chat/queryChat/MessageItem/FaceMessageRender";
import { LocaleString } from "@/store/type";
import * as localForage from "localforage";

localForage.config({
  name: "OpenIM38-Config",
});

type MomentsCache = {
  [key: string]: {
    [key: number]: string;
  };
};

export const setAreaCode = (areaCode: string) =>
  localStorage.setItem("IM_AREA_CODE", areaCode);
export const setPhoneNumber = (account: string) =>
  localStorage.setItem("IM_PHONE_NUM", account);
export const setEmail = (email: string) => localStorage.setItem("IM_EMAIL", email);
export const setLoginMethod = (method: string) =>
  localStorage.setItem("IM_LOGIN_METHOD", method);
export const setTMToken = (token: string) => localForage.setItem("IM_TOKEN", token);
export const setChatToken = (token: string) =>
  localForage.setItem("IM_CHAT_TOKEN", token);
export const setTMUserID = (userID: string) => localForage.setItem("IM_USERID", userID);
export const setIMProfile = async ({
  chatToken,
  imToken,
  userID,
}: {
  chatToken: string;
  imToken: string;
  userID: string;
}) => {
  await Promise.all([setTMToken(imToken),setChatToken(chatToken),setTMUserID(userID)]);
};

export const setAccessedFriendApplication = async (list: string[]) =>
  localForage.setItem(`${await getIMUserID()}_accessedFriendApplications`, list);
export const setAccessedGroupApplication = async (list: string[]) =>
  localForage.setItem(`${await getIMUserID()}_accessedGroupApplications`, list);
export const setUserCustomEmojis = async (list: CustomEmojiItem[]) =>
  localForage.setItem(`${await getIMUserID()}_customEmojis`, list);

export const setLocale = (locale: string) => localStorage.setItem("IM_LOCALE", locale);
export const setImageCache = async (caches: Record<string, string>) =>
  localForage.setItem(`imageCache`, caches);

export const clearIMProfile = async () => {
  await Promise.all([localForage.removeItem("IM_TOKEN"),localForage.removeItem("IM_CHAT_TOKEN"),localForage.removeItem("IM_USERID")]);
};

export const getAreaCode = () => localStorage.getItem("IM_AREA_CODE");
export const getPhoneNumber = () => localStorage.getItem("IM_PHONE_NUM");
export const getEmail = () => localStorage.getItem("IM_EMAIL");
export const getLoginMethod = () =>
  (localStorage.getItem("IM_LOGIN_METHOD") ?? "phone") as "phone" | "email";
export const getIMToken = async () => await localForage.getItem("IM_TOKEN");
export const getChatToken = async () => await localForage.getItem("IM_CHAT_TOKEN");
export const getIMUserID = async () => await localForage.getItem("IM_USERID");
export const getAccessedFriendApplication = async () =>
  (await localForage.getItem<string[]>(
    `${await getIMUserID()}_accessedFriendApplications`,
  )) ?? [];
export const getAccessedGroupApplication = async () =>
  (await localForage.getItem<string[]>(
    `${await getIMUserID()}_accessedGroupApplications`,
  )) ?? [];
export const getUserCustomEmojis = async (): Promise<CustomEmojiItem[]> =>
  (await localForage.getItem(`${await getIMUserID()}_customEmojis`)) ?? [];
export const getImageCache = async (): Promise<Record<string, string>> =>
  (await localForage.getItem(`imageCache`)) ?? {};

export const getLocale = (): LocaleString =>
  window.electronAPI?.ipcSendSync("getKeyStoreSync", { key: "language" }) ||
  (localStorage.getItem("IM_LOCALE") as LocaleString) ||
  "zh-CN";
