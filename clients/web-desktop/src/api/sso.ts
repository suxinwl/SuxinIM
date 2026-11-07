import { getServerOrigin } from "@/config";
import { getSSOSession, setIMProfile } from "@/utils/storage";
export interface SSOSession {
  userID: string; imToken: string; chatToken: string; sessionToken: string;
  expiresAt: number; profile: Record<string, unknown>; attachmentToken?: string;
  policyClient?: "web_desktop" | "web_mobile" | "pc" | "android" | "ios";
  effectivePolicy?: { login?: boolean; save?: boolean; preview?: boolean; zone?: string; policyRevision?: number };
}
export class SSOError extends Error { constructor(message: string, public readonly status: number, public readonly data?: {reasonCode?:string;changeTicket?:string;expiresAt?:number}) { super(message); } }
export async function ssoRequest<T>(operation: string, body?: unknown): Promise<T> {
  if (!["login", "refresh", "logout", "exchange", "captcha", "directory", "person", "password/change", "password/status", "password/cancel", "policy"].includes(operation)) throw new Error("Unsupported account operation");
  if(new URL(getServerOrigin()).protocol!=="https:") throw new SSOError("统一账号登录需要 HTTPS 连接",403);
  const response = await fetch(`${getServerOrigin()}/sso/${operation}`, {
    method: body === undefined ? "GET" : "POST", cache: "no-store", credentials: "omit",
    headers: { "Content-Type": "application/json" }, body: body === undefined ? undefined : JSON.stringify(body),
    signal: AbortSignal.timeout(25000),
  });
  const result = await response.json();
  if (!response.ok || result.code !== 0) throw new SSOError(result.msg || result.message || "统一账号服务暂不可用", response.status, result.data);
  return result.data as T;
}
export function validateSession(value: SSOSession): SSOSession {
  if (![value.userID, value.imToken, value.chatToken, value.sessionToken].every(v => typeof v === "string" && v.length > 0)
      || !Number.isFinite(value.expiresAt) || value.expiresAt <= Date.now() / 1000) throw new Error("统一账号服务返回无效会话");
  return value;
}
export async function loginSSO(account: string, password: string, captcha?: string, captchaId?: string) {
  return validateSession(await ssoRequest<SSOSession>("login", { account, password, captcha, captchaId, platform: window.electronAPI ? "windows" : "web" }));
}
export async function exchangeSSOTicket(ticket: string) {
  return validateSession(await ssoRequest<SSOSession>("exchange", { ticket, platform: window.electronAPI ? "windows" : "web" }));
}
let sessionGeneration=0;
window.addEventListener?.("enterprise-disconnect",()=>{sessionGeneration++;});
let refreshFlight: Promise<SSOSession> | undefined;
export async function refreshSSO(force = false): Promise<SSOSession> {
  const current = await getSSOSession();
  if (!current?.sessionToken) throw new SSOError("请使用统一账号重新登录", 401);
  if (!force && current.expiresAt > Date.now() / 1000 + 600) return current;
  const generation=sessionGeneration;
  return refreshFlight ??= ssoRequest<SSOSession>("refresh", {sessionToken: current.sessionToken})
    .then(validateSession).then(async session => { if ((await getSSOSession())?.sessionToken !== current.sessionToken || generation!==sessionGeneration) throw new SSOError("账号会话已改变",409); await setIMProfile(session); return session; })
    .finally(() => { refreshFlight = undefined; });
}
export async function logoutSSO() {
  sessionGeneration++;
  const current = await getSSOSession();
  if (current?.sessionToken) await ssoRequest("logout", { sessionToken: current.sessionToken });
}
export async function changeOwnPassword(oldPassword: string, newPassword: string) {
  const current = await getSSOSession(); if (!current?.sessionToken) throw new SSOError("请重新登录后修改密码", 401);
  return ssoRequest("password/change", { sessionToken: current.sessionToken, oldPassword, newPassword });
}
