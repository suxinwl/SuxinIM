import { CustomEmojiItem } from "@/pages/chat/queryChat/MessageItem/FaceMessageRender";
import type { SSOSession } from "@/api/sso";
import { LocaleString } from "@/store/type";
import browserStore from "localforage";
import { getServerOrigin } from "@/config";
import {recoverLocalArchive} from '@/im/local-store-recovery';

// Native Chromium sessions never persist business LocalStorage/IndexedDB. The
// encrypted main-process vault owns durable preferences; credentials stay in RAM.
const nativeMemory=new Map<string,unknown>();
const localForage: Pick<typeof browserStore,"config"|"getItem"|"setItem"|"removeItem"> = window.electronAPI ? {
  config:()=>true,
  getItem:async<T>(key:string)=>nativeMemory.get(key) as T ?? null,
  setItem:async<T>(key:string,value:T)=>{nativeMemory.set(key,value);if(!["IM_TOKEN","IM_CHAT_TOKEN","IM_USERID","PUTY_SSO"].includes(key))await window.electronAPI!.ipcInvoke("puty-store","preferences",{[key]:value});return value;},
  removeItem:async(key:string)=>{nativeMemory.delete(key);},
} as any : browserStore;

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
  ...session
}: {
  chatToken: string;
  imToken: string;
  userID: string;
} & Partial<SSOSession>) => {
  if(window.electronAPI) {
    const credentials={origin:getServerOrigin(),userID,imToken,chatToken};
    try{await window.electronAPI.ipcInvoke("puty-store","unlock",credentials);}catch(error){if(!/恢复包|恢复密钥|档案校验失败/.test(String(error)))throw error;await recoverLocalArchive(credentials);}
    const preferences=await window.electronAPI.ipcInvoke<Record<string,unknown>>("puty-store","preferences");
    for(const [key,value]of Object.entries(preferences))nativeMemory.set(key,value);
  }
  const previous = await getSSOSession();
  const changed = previous?.userID !== userID || previous?.sessionToken !== session.sessionToken;
  if (changed) window.dispatchEvent(new Event("im-profile-changing"));
  try {
    await Promise.all([setTMToken(imToken),setChatToken(chatToken),setTMUserID(userID),localForage.setItem("PUTY_SSO", {...session,imToken,chatToken,userID})]);
  } finally { if (changed) window.dispatchEvent(new Event("im-profile-changed")); }
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
  window.dispatchEvent(new Event("im-profile-changing"));
  try {
    await Promise.all([localForage.removeItem("IM_TOKEN"),localForage.removeItem("IM_CHAT_TOKEN"),localForage.removeItem("IM_USERID"),localForage.removeItem("PUTY_SSO")]);
  } finally { if(window.electronAPI)await window.electronAPI.ipcInvoke("puty-store","lock");window.dispatchEvent(new Event("im-profile-changed")); }
};

export const getSSOSession = () => localForage.getItem<SSOSession>("PUTY_SSO");
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
