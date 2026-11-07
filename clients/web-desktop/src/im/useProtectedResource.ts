import { useCallback, useEffect, useRef, useState, useSyncExternalStore } from "react";
import { openResourcePreview, requiresResourceGrant, resolvePublicBrandResource, type ResourcePreview } from "./resources";
import { getResourceSessionVersion, isResourceSessionReady, subscribeResourceSession } from "./resource-session";

export function useProtectedResource(source?: string, { forceOnLoad = false, enabled = true }: { forceOnLoad?: boolean; enabled?: boolean } = {}) {
  const version = useSyncExternalStore(subscribeResourceSession, getResourceSessionVersion);
  const value = source ? resolvePublicBrandResource(source) : source;
  const protectedResource = Boolean(value && requiresResourceGrant(value));
  const key = `${version}:${enabled}:${forceOnLoad}:${value ?? ""}`;
  const currentKey = useRef(key); currentKey.current = key;
  const mounted = useRef(true);
  const active = useRef<ResourcePreview>();
  const operation = useRef<AbortController>();
  const retry = useRef({ key, used: false });
  const [result, setResult] = useState<{ key: string; url?: string; failed?: boolean }>();
  useEffect(() => { mounted.current = true; return () => { mounted.current = false; operation.current?.abort(); active.current?.release(); }; }, []);
  useEffect(() => {
    let live = true;
    operation.current?.abort();active.current?.release();active.current=undefined;
    const controller=new AbortController();operation.current=controller;
    retry.current = { key, used: false };
    if (enabled && protectedResource && value && isResourceSessionReady()) {
      void openResourcePreview(value, { force: forceOnLoad,signal:controller.signal }).then(preview => {
        if (live && currentKey.current === key && operation.current===controller && !controller.signal.aborted) { active.current=preview;setResult({ key, url:preview.url }); } else preview.release();
      }).catch(() => { if (live && currentKey.current === key && operation.current===controller && !controller.signal.aborted) setResult({ key, failed: true }); });
    }
    return () => { live = false; controller.abort();active.current?.release();active.current=undefined; };
  }, [key, protectedResource, value, enabled, forceOnLoad]);
  const onError = useCallback(() => {
    if (!enabled || !protectedResource || !value || retry.current.key !== key || !isResourceSessionReady()) return false;
    if (retry.current.used) { setResult({ key, failed: true }); return false; }
    retry.current.used = true;
    operation.current?.abort();active.current?.release();active.current=undefined;
    const controller=new AbortController();operation.current=controller;
    setResult({ key });
    void openResourcePreview(value, { force: true,signal:controller.signal }).then(preview => {
      if (mounted.current && currentKey.current === key && operation.current===controller && !controller.signal.aborted) { active.current=preview;setResult({ key, url:preview.url }); } else preview.release();
    }).catch(() => { if (mounted.current && currentKey.current === key && operation.current===controller && !controller.signal.aborted) setResult({ key, failed: true }); });
    return true;
  }, [key, protectedResource, value, enabled]);
  const retryResource = useCallback(() => {
    retry.current = { key, used: false };
    return onError();
  }, [key, onError]);
  // Effects run after render. Never expose the previous src/session meanwhile.
  const bound = result?.key === key ? result : undefined;
  return { src: !enabled ? undefined : protectedResource ? isResourceSessionReady() ? bound?.url : undefined : value, protectedResource, onError, retry: retryResource, failed: enabled && protectedResource && Boolean(bound?.failed), pending: enabled && protectedResource && !bound?.url && !bound?.failed };
}
