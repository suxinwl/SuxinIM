type CloseHost = {
 close: (...args: any[]) => any;
 closeDB?: (...args: any[]) => any;
};
const installed=Symbol.for("openim38.wasm.databaseCloseBridge");
export function isSDKDatabaseCloseStack(stack: string | undefined) {
 return Boolean(stack && /[/\\]openIM\.wasm(?::|\?|$)/.test(stack));
}
/** The pinned Go WASM IndexDB.Close calls global `close`; the JS RPC is `closeDB`. */
export function installDatabaseCloseBridge(host: CloseHost, isSDKCall=isSDKDatabaseCloseStack) {
 if((host as any)[installed])return;
 const nativeClose=host.close.bind(host);
 let closing: Promise<unknown> | undefined;
 host.close=(...args:any[])=>{
  // Preserve ordinary page/native close calls; only the fixed WASM driver is adapted.
  if(!isSDKCall(new Error().stack))return nativeClose(...args);
  if(typeof host.closeDB!=="function")throw new Error("OpenIM database close bridge is not initialized");
  return closing ??= Promise.resolve().then(()=>host.closeDB!(...args));
 };
 const reset=()=>{closing=undefined;};
 (host as any)[installed]={reset};
 return reset;
}
