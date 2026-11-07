import brand from "../../../brand.json";
export const CLIENT_BRAND = brand;
export function appDisplayName(config?: { appName?: unknown }) {
  return typeof config?.appName === "string" && config.appName.trim() ? config.appName.trim() : brand.displayName;
}
export function webDisplayTitle(config?: { appName?: unknown; webTitle?: unknown }) {
  return typeof config?.webTitle === "string" && config.webTitle.trim() ? config.webTitle.trim() : appDisplayName(config);
}
