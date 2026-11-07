import { Alert, Button, Form, Input, Radio, Space } from "antd";
import md5 from "md5";
import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { createPortal } from "react-dom";
import { exchangeSSOTicket, loginSSO, ssoRequest, SSOError } from "@/api/sso";
import { setIMProfile } from "@/utils/storage";
import { appDisplayName } from "@/config/brand";
import { useUserStore } from "@/store";
import { getPublicBrand } from "@/api/public-brand";
import { webDisplayTitle } from "@/config/brand";
import { getNetwork, isNativeClient, Network, setNetwork } from "@/config";
import ConfigModal from "./ConfigModal";
import {localStorageIPCMessage} from './local-storage-error';
type Captcha = { show?: boolean; enabled?: boolean; img?: string; image?: string; id?: string; captchaId?: string };
export default function LoginForm({repairActionsContainer}:{repairActionsContainer?:HTMLElement|null}) {
  const displayName = useUserStore(state => appDisplayName(state.appConfig));
  const navigate = useNavigate();
  const [busy, setBusy] = useState(false), [config, showConfig] = useState(false);
  const [captcha, setCaptcha] = useState<Captcha>({});
  const [failure, setFailure] = useState("");
  const [migration,setMigration]=useState('');
  const [changeTicket, setChangeTicket] = useState("");
  const [network, selectNetwork] = useState<Network>(getNetwork);
  useEffect(()=>window.electronAPI?.subscribe('puty-store-migration',(message:string)=>setMigration(message)),[]);
  useEffect(() => {
    let active = true;
    void getPublicBrand().then(brand => {
      if (!active) return;
      useUserStore.setState(state => ({ appConfig: { ...state.appConfig, ...brand } }));
      document.title = webDisplayTitle(brand);
    }).catch(() => { /* Offline sign-in retains the configured build-time brand. */ });
    return () => { active = false; };
  }, []);
  async function loadCaptcha() { try { setCaptcha(await ssoRequest<Captcha>("captcha")); } catch { setFailure("验证码暂不可用，请重试"); } }
  async function repairStorage(){
    if(busy || !window.electronAPI)return;
    setBusy(true);setFailure('');setMigration('');
    try{const result=await window.electronAPI.ipcInvoke('puty-store','repair-directory') as {changed?:boolean}|undefined;if(result?.changed===true)setFailure('本地存储位置已更新，原目录与文件保持不变。请重新登录；旧档案无法打开时需使用完整加密备份恢复。');}
    catch{setFailure('本地目录未更改。请选择有效的原存储目录或新空目录；请保留原文件和密钥。');}
    finally{setBusy(false);}
  }
  function changeNetwork(next:unknown){
    if(!isNativeClient() || busy || next!=="lan" && next!=="wan" || next===network)return;
    setBusy(true);setFailure('');
    try{
      // Keep the existing synchronous endpoint reset before this same-window reload.
      setNetwork(next);selectNetwork(next);window.location.reload();
    }catch{
      setFailure('通讯方式切换未完成，请重试。');setBusy(false);
    }
  }
  useEffect(() => {
    const url = new URL(window.location.href), ticket = url.searchParams.get("ticket");
    if (ticket) {
      url.searchParams.delete("ticket"); window.history.replaceState(null, "", url);
      setBusy(true);setMigration('');
      void exchangeSSOTicket(ticket).then(setIMProfile).then(() => navigate("/", {replace:true}))
        .catch(error => { setFailure(!(error instanceof SSOError) && localStorageIPCMessage(error,Boolean(window.electronAPI)) || "登录链接已失效，请重新从人事系统打开或使用统一账号登录"); void loadCaptcha(); }).finally(() => {setMigration('');setBusy(false);});
    } else void loadCaptcha();
  }, []);
  return <>{changeTicket ? <Form layout="vertical" onFinish={async ({newPassword,confirmPassword}) => {
    if (newPassword !== confirmPassword) { setFailure("两次输入的新密码不一致"); return; }
    setBusy(true); setFailure("");setMigration('');
    try { await ssoRequest("password/change", { changeTicket, newPassword: md5(newPassword) }); setChangeTicket(""); setFailure("密码已更新，请使用新密码重新登录"); void loadCaptcha(); }
    catch(error) { setFailure(error instanceof Error ? error.message : "密码修改失败"); }
    finally { setBusy(false); }
  }}><h2 className="mb-4 text-xl">设置本人新密码</h2><Alert type="info" message="管理员已重置密码，请先设置本人新密码。"/>{failure && <Alert className="my-3" type="warning" message={failure} style={{overflowWrap:"anywhere",wordBreak:"break-word"}}/>}
    <Form.Item name="newPassword" label="新密码" rules={[{required:true,pattern:/^(?=.*[0-9])(?=.*[a-zA-Z]).{6,20}$/,message:"请输入6至20位包含字母和数字的新密码"}]}><Input.Password autoComplete="new-password"/></Form.Item>
    <Form.Item name="confirmPassword" label="确认新密码" rules={[{required:true}]}><Input.Password autoComplete="new-password"/></Form.Item>
    <Button block type="primary" htmlType="submit" loading={busy}>修改并重新登录</Button><Button block type="link" onClick={() => { void ssoRequest("password/cancel",{changeTicket}).catch(()=>{}); setChangeTicket(""); setFailure(""); void loadCaptcha(); }}>返回登录</Button>
  </Form> : <Form layout="vertical" onFinish={async ({account, password, captcha: answer}) => {
    setBusy(true); setFailure("");setMigration('');
    try { await setIMProfile(await loginSSO(account.trim(), md5(password), answer, captcha.captchaId || captcha.id)); navigate("/", {replace:true}); }
    catch (error) { if(error instanceof SSOError && error.data?.reasonCode === "PASSWORD_CHANGE_REQUIRED" && error.data.changeTicket) { setChangeTicket(error.data.changeTicket); setFailure(""); } else { setFailure(!(error instanceof SSOError) && localStorageIPCMessage(error,Boolean(window.electronAPI)) || (error instanceof Error ? error.message : "登录未完成")); void loadCaptcha(); } }
    finally { setMigration('');setBusy(false); }
  }}>
    <h2 className="mb-5 text-xl">{displayName}</h2>
    {failure && <Alert type="error" message={failure} className="mb-3" style={{overflowWrap:"anywhere",wordBreak:"break-word"}}/>}
    {migration && <Alert type="info" message={migration} className="mb-3"/>}
    <Form.Item name="account" label="统一账号 / 工号" rules={[{required:true,message:"请输入统一账号"}]}><Input autoComplete="username"/></Form.Item>
    <Form.Item name="password" label="密码" rules={[{required:true,message:"请输入密码"}]}><Input.Password autoComplete="current-password"/></Form.Item>
    {(captcha.show || captcha.enabled) && <Space align="start"><Form.Item name="captcha" label="验证码" rules={[{required:true,message:"请输入验证码"}]}><Input autoComplete="off"/></Form.Item><img className="mt-7 cursor-pointer" width={100} src={captcha.image || captcha.img} onClick={() => void loadCaptcha()} alt="点击更换验证码"/></Space>}
    <Button type="primary" htmlType="submit" loading={busy} block>登录</Button>
    <p className="mt-4 text-xs text-gray-500">使用统一账号登录，可在个人设置中修改本人密码。</p>
    {isNativeClient() ? <Radio.Group className="mt-1" value={network} disabled={busy} onChange={event=>changeNetwork(event.target.value)} aria-label="通讯方式">
      <Radio value="lan">内部通讯</Radio><Radio value="wan">公网通讯</Radio>
    </Radio.Group> : <Button type="link" onClick={() => showConfig(true)}>内网 / 公网连接</Button>}
  </Form>}{!isNativeClient() && <ConfigModal visible={config} close={() => showConfig(false)}/>}
    {window.electronAPI && !changeTicket && repairActionsContainer && createPortal(<Button size="small" className="app-no-drag" disabled={busy} onClick={()=>void repairStorage()}>修复存储</Button>,repairActionsContainer)}
  </>;
}
