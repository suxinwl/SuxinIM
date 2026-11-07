import { getServerOrigin } from "@/config";
export interface PublicBrand { appName?: string; webTitle?: string }
export async function getPublicBrand(): Promise<PublicBrand> {
  const origin = getServerOrigin();
  if (new URL(origin).protocol !== "https:") throw new Error("品牌配置需要 HTTPS");
  const response = await fetch(`${origin}/chat/client_config/get`, {
    method: "POST", credentials: "omit", cache: "no-store", redirect: "error",
    headers: { "Content-Type": "application/json", operationID: crypto.randomUUID() },
    body: "{}", signal: AbortSignal.timeout(10000),
  });
  const value = await response.json();
  if (!response.ok || value.errCode !== 0) throw new Error("品牌配置暂不可用");
  const result: PublicBrand = {};
  for (const key of ["appName", "webTitle"] as const) {
    const text = value.data?.config?.[key];
    if (typeof text === "string" && text.trim() && text.length <= 128 && !/[\x00-\x1f\x7f]/.test(text)) result[key] = text.trim();
  }
  if (origin !== getServerOrigin()) throw new Error("通讯连接已改变");
  return result;
}
