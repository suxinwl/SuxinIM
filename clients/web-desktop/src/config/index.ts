import buildVersion from "../../build-version.json";
export const NETWORKS = { lan: "https://192.168.10.11:9885", wan: "https://im.szputy.com:9885" } as const;
export type Network = keyof typeof NETWORKS;
export const isNativeClient = () => Boolean(window.electronAPI);
export const getNetwork = (): Network => typeof localStorage!=="undefined" && localStorage.getItem("puty:network") === "wan" ? "wan" : "lan";
export const getServerOrigin = () => isNativeClient() ? NETWORKS[getNetwork()] : (window.location.origin || new URL(window.location.href).origin);
export function setNetwork(network: Network) {
  localStorage.setItem("puty:network", network);
  for (const key of ["wsUrl", "apiUrl", "chatUrl", "enterpriseUrl"]) localStorage.removeItem(`upgrade38:${key}`);
}
export const getWsUrl = () => `${getServerOrigin().replace(/^https:/, "wss:")}/ws`;
export const getApiUrl = () => `${getServerOrigin()}/api`;
export const getChatUrl = () => `${getServerOrigin()}/chat`;
export const getEnterpriseUrl = () => `${getServerOrigin()}/enterprise/v1`;
export const WS_URL = getWsUrl();
export const API_URL = getApiUrl();
export const CHAT_URL = getChatUrl();
export const APP_VERSION = buildVersion.version;
export const APP_VERSION_CODE = buildVersion.build;
export const SDK_VERSION = "3.8.3-patch.15.1";
export const CHECK_UPDATE_PREFIX = "";
export const isSaveLog = true;
