// Only a local generation is exposed to React/query keys, never a bearer token.
let generation = 0;
let ready = true;
const listeners = new Set<() => void>();
export const getResourceSessionVersion = () => generation;
export const isResourceSessionReady = () => ready;
export function subscribeResourceSession(listener: () => void) {
  listeners.add(listener);
  return () => { listeners.delete(listener); };
}
function changed(available = true) { ready = available; generation++; for (const listener of listeners) listener(); }
if (typeof window !== "undefined") {
  window.addEventListener("enterprise-disconnect", () => changed());
  window.addEventListener("enterprise-session-revoked", () => changed(false));
  window.addEventListener("puty-local-cleanup",()=>changed());
  window.addEventListener("puty-archive-locked",()=>changed(false));
  window.addEventListener("puty-archive-ready",()=>changed());
  window.addEventListener("offline", () => changed(false));
  window.addEventListener("im-profile-changing", () => changed(false));
  window.addEventListener("im-profile-changed", () => changed());
}
