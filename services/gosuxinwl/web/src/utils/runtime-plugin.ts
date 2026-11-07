import { defHttp } from '@/utils/http';

const runtimePluginPath = /^\/plugins\/([a-z][a-z0-9-]{0,63})\/admin(?:\/|$)/;

export function safeRuntimePluginTarget(value: unknown): string | null {
  if (typeof value !== 'string' || value.trim() === '') return null;
  try {
    const parsed = new URL(value, window.location.origin);
    if (parsed.origin !== window.location.origin) return null;
    if (!runtimePluginPath.test(parsed.pathname)) return null;
    return `${parsed.pathname}${parsed.search}${parsed.hash}`;
  } catch {
    return null;
  }
}

export function isRuntimePluginTarget(value: unknown): value is string {
  return safeRuntimePluginTarget(value) !== null;
}

export async function bootstrapRuntimePluginAdmin(target: string): Promise<void> {
  const safeTarget = safeRuntimePluginTarget(target);
  if (!safeTarget) throw new Error('插件跳转地址无效');
  const match = safeTarget.match(runtimePluginPath);
  if (!match) throw new Error('插件管理地址无效');

  // The host HTTP client supplies the current biztoken, including the
  // configured short-lived dynamic-token wrapper, and keeps this exchange
  // same-origin. The response never contains a browser-readable token.
  await defHttp.post(
    { url: `/plugins/${match[1]}/admin/__host_auth/bootstrap`, withCredentials: true },
    { isRootUrl: false, joinPrefix: false },
  );
}

export async function redirectRuntimePluginAfterLogin(requested: unknown): Promise<boolean> {
  const target = safeRuntimePluginTarget(requested);
  if (!target) return false;
  await bootstrapRuntimePluginAdmin(target);
  window.location.assign(target);
  return true;
}
