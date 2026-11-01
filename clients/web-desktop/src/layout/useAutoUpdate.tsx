import { useEffect, useState } from "react";
import { Button, Spin } from "antd";
import { APP_VERSION, APP_VERSION_CODE, getEnterpriseUrl } from "@/config";
import { enterpriseRequest } from "@/im/enterprise";
import { shouldUpgrade, validateRelease, type Release } from "@/im/update-policy";
export function useAutoUpdate() {
 const [checking,setChecking]=useState(Boolean(window.electronAPI));
 const [release,setRelease]=useState<Release>();
 const [busy,setBusy]=useState(false);
 const [error,setError]=useState("");
 const install=async(r:Release)=>{
  setBusy(true);setError("");
  try {await window.electronAPI!.ipcInvoke("downloadAndInstallUpdate",{...r,metadataOrigin:new URL(getEnterpriseUrl()).origin});}
  catch(e){setError(e instanceof Error?e.message:String(e));}
  finally{setBusy(false);}
 };
 useEffect(()=>{
  if(!window.electronAPI)return;
  let mounted=true;
  enterpriseRequest<Release|null>("/application/latest",{platform:"windows",version:APP_VERSION,build:APP_VERSION_CODE}).then(({data})=>{
   if(!mounted || !data || !shouldUpgrade(data,APP_VERSION,APP_VERSION_CODE))return;
   validateRelease(data);setRelease(data);
   if(!data.force)window.electronAPI!.ipcInvoke<number>("showMessageBox",{type:"info",title:"应用更新",message:`发现新版本 ${data.version}\n${data.text || ""}`,buttons:["下载并安装","稍后"]}).then(choice=>{if(choice===0)void install(data);});
  }).catch(e=>console.debug("Version check unavailable",e)).finally(()=>{if(mounted)setChecking(false);});
  return()=>{mounted=false;};
 },[]);
 if(checking)return <div className="flex h-screen items-center justify-center"><Spin tip="检查应用版本" /></div>;
 if(release?.force)return <div className="flex h-screen flex-col items-center justify-center gap-4"><h2>需要更新到 {release.version}</h2><p>{release.text}</p><Button type="primary" loading={busy} onClick={()=>void install(release)}>下载并安装</Button>{error && <p role="alert">{error}，请重试。</p>}</div>;
 return null;
}
