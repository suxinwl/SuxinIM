import { Alert,Button,DatePicker,Divider,Input,Modal,Select,Space,Spin,Typography } from "antd";
import {useEffect,useState,useRef} from "react";
import {useConversationStore,useMessageStore} from "@/store";
import {feedbackToast} from "@/utils/common";
import {recoverLocalArchive} from '@/im/local-store-recovery';
import {getSSOSession} from '@/utils/storage';
import {getServerOrigin} from '@/config';
import {requireStoreAck,requireCleanupPreview} from '@/im/local-store-results';
import {subscribeResourceSession,getResourceSessionVersion,isResourceSessionReady} from '@/im/resource-session';
type Scope={conversationIDs?:string[];beforeDate?:string;beforeMS?:number};
type Preview={messages:number;attachments:number;bytes:number;beforeMS:number;conversationIDs:string[]|null};
type Stats={directory:string;messages:number;attachments:number;bytes:number;payloadCleanupPending?:boolean;restoreCleanupPending?:boolean};
const size=(bytes:number)=>`${(bytes/1024/1024).toFixed(1)} MiB`;
const invoke=<T,>(operation:string,data?:unknown)=>window.electronAPI!.ipcInvoke<T>('puty-store',operation,data);
export default function LocalStorageSettings({open,onClose}:{open:boolean;onClose:()=>void}) {
  const conversations=useConversationStore(state=>state.conversationList);
  const [stats,setStats]=useState<Stats>(),[directory,setDirectory]=useState(''),[busy,setBusy]=useState(false),[selected,setSelected]=useState<string[]>([]),[date,setDate]=useState<string>(),[preview,setPreview]=useState<Preview>(),[recovery,setRecovery]=useState('');
  const scope:Scope={conversationIDs:selected.length?selected:undefined,beforeDate:date};
  const scopeRevision=useRef(0);
  const confirmation=useRef<ReturnType<typeof Modal.confirm>>();
  function invalidate(){scopeRevision.current++;confirmation.current?.destroy();confirmation.current=undefined;setPreview(undefined);}
  useEffect(()=>{if(!open)return;const stop=subscribeResourceSession(invalidate);return()=>{stop();scopeRevision.current++;confirmation.current?.destroy();confirmation.current=undefined;};},[open]);
  function confirmCleanup(value:Preview){
    const revision=scopeRevision.current,session=getResourceSessionVersion();
    confirmation.current=Modal.confirm({title:'确认清理当前客户端？',content:'删除对应消息、月档及无其他引用的图片、视频和文件。此操作不会删除服务器资料。',okText:'确认清理',okButtonProps:{danger:true},onOk:()=>action(async()=>{
      if(revision!==scopeRevision.current || session!==getResourceSessionVersion() || !isResourceSessionReady())throw new Error('清理范围或账号会话已改变，请重新计算范围');
      const clean=(window as any).putyLocalCleanup;if(typeof clean!=='function')throw new Error('本地存储服务尚未就绪');const result=await clean({conversationIDs:value.conversationIDs || undefined,beforeMS:value.beforeMS});requireStoreAck(result,'cleaned');window.dispatchEvent(new Event('puty-local-cleanup'));useMessageStore.getState().clearHistoryMessage();await useConversationStore.getState().getConversationListByReq();setPreview(undefined);await refresh();feedbackToast({msg:'本机聊天记录已清理'});
    })});
  }
  async function checkpoint(){const fn=(window as any).putyLocalCheckpoint;if(typeof fn!=='function')throw new Error('加密工作库尚未就绪');requireStoreAck(await fn(),'checkpointed');}
  async function refresh(){const [config,current]=await Promise.all([invoke<{directory:string}>('config'),invoke<Stats>('stats')]);setDirectory(config.directory);setStats(current);}
  useEffect(()=>{if(open && window.electronAPI){setPreview(undefined);void refresh().catch(error=>feedbackToast({error}));}},[open]);
  async function action(fn:()=>Promise<unknown>){setBusy(true);try{await fn();}catch(error){feedbackToast({error});}finally{setBusy(false);}}
  return <Modal title="本地存储与聊天记录清理" open={open} onCancel={onClose} footer={<Button onClick={onClose}>关闭</Button>} width={650} destroyOnClose>
    {!window.electronAPI?<Alert type="info" message="本地加密归档和存储目录管理请在 PC、安卓或 iOS 客户端中使用。"/>:<Spin spinning={busy}>
      <Space direction="vertical" className="w-full" size="middle">
        <Alert type="info" message="消息和已主动下载的附件在本机加密保存，需在线验证账号后查看。预览不会自动存入长期档案。"/>
        <div><Typography.Text strong>数据目录</Typography.Text><Typography.Paragraph copyable className="mt-1 break-all">{directory || '尚未选择'}</Typography.Paragraph>
          <Button onClick={()=>void action(async()=>{await checkpoint();await invoke('choose-directory');})}>更改目录并迁移数据</Button></div>
        {stats && <Typography.Text>已存档 {stats.messages} 条消息、{stats.attachments} 个附件，附件占用 {size(stats.bytes)}</Typography.Text>}
        {stats?.payloadCleanupPending && <Alert type="warning" message="部分文件因占用等原因尚未释放，记录已隐藏；请解除文件占用后重试清理。"/>}
        {stats?.restoreCleanupPending && <Alert type="warning" message="恢复前的本机加密副本尚未释放；在线同步成功且文件可校验后会自动清理。"/>}
        <Divider className="my-1"/>
        <Typography.Text strong>清理当前客户端</Typography.Text>
        <Select className="w-full" mode="multiple" allowClear showSearch optionFilterProp="label" placeholder="选择人员或群组；留空表示全部会话" value={selected} onChange={value=>{invalidate();setSelected(value);}} options={conversations.map(value=>({value:value.conversationID,label:`${value.groupID?'群组':'人员'} · ${value.showName || value.userID || value.groupID || value.conversationID}`}))}/>
        <Space><Typography.Text>清理某日期之前</Typography.Text><DatePicker onChange={value=>{invalidate();setDate(value?.format('YYYY-MM-DD'));}} placeholder="留空表示全部日期"/></Space>
        <Typography.Text type="secondary">日期以北京时间当日 00:00 为界。仅清理本机，服务器和其他设备保留。另存到应用目录外的普通文件副本不随此删除。</Typography.Text>
        <Button onClick={()=>void action(async()=>{const calculate=(window as any).putyLocalCleanupPreview;if(typeof calculate!=='function')throw new Error('加密工作库尚未就绪');const revision=scopeRevision.current,result=requireCleanupPreview(await calculate(scope));if(revision===scopeRevision.current)setPreview(result);})}>计算清理范围</Button>
        {preview && <Alert type="warning" message={`将清理 ${preview.messages} 条消息、${preview.attachments} 个附件，预计释放 ${size(preview.bytes)}`} description={<Button danger className="mt-2" onClick={()=>confirmCleanup(preview)}>确认清理</Button>}/>}
        <Divider className="my-1"/>
        <Typography.Text strong>完整加密备份与恢复</Typography.Text><Typography.Text type="secondary">备份包括当前账号的聊天库、月档、已下载附件及口令保护的密钥。换机或系统重装后，在线登录同一账号并选择备份目录即可恢复。口令独立于账号密码。</Typography.Text>
        <Space.Compact className="w-full"><Input.Password value={recovery} onChange={e=>setRecovery(e.target.value)} placeholder="至少 12 个字符的独立恢复口令" autoComplete="new-password"/><Button disabled={recovery.length<12} onClick={()=>void action(async()=>{await checkpoint();const result=await invoke<{directory?:string}>('export-backup',{passphrase:recovery});if(result?.directory){setRecovery('');feedbackToast({msg:'完整加密备份已保存：'+result.directory});}})}>完整备份</Button></Space.Compact>
        <Button onClick={()=>void action(async()=>{const session=await getSSOSession();if(!session?.imToken || !session.chatToken || !session.userID)throw new Error('请在线重新登录后恢复');await checkpoint();await recoverLocalArchive({origin:getServerOrigin(),userID:session.userID,imToken:session.imToken,chatToken:session.chatToken});window.location.reload();})}>恢复完整加密备份</Button>
      </Space>
    </Spin>}
  </Modal>;
}
