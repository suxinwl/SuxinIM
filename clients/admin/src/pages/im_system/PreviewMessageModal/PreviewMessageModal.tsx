import { API_URL } from '@/config';
import { request } from '@umijs/max';
import { Alert, Button, DatePicker, Empty, Input, List, Modal, Space, Spin } from 'antd';
import type { ConversationItem, MessageItem } from '@openim/wasm-client-sdk';
import { useEffect, useRef, useState } from 'react';
import HistoryMessageItem from './HistoryMessageItem';

// Read history with the administrator's own server session. Never log in as an employee.
async function post(path: string, data: unknown) {
  const result = await request(API_URL + path, { method: 'POST', data });
  return result.data;
}
function normalise(raw: any): MessageItem {
  let content: any = raw.content ?? {};
  if (typeof content === 'string') {
    try { content = JSON.parse(content); } catch {
      try { content = JSON.parse(new TextDecoder().decode(Uint8Array.from(atob(content), c => c.charCodeAt(0)))); }
      catch { content = { content }; }
    }
  }
  const elements: Record<number, string> = {101:'textElem',102:'pictureElem',103:'soundElem',104:'videoElem',105:'fileElem',106:'atTextElem',107:'mergeElem',108:'cardElem',109:'locationElem',110:'customElem',114:'quoteElem',1400:'notificationElem'};
  const key = elements[raw.contentType] ?? 'notificationElem';
  return {...raw, content, [key]: content[key] ?? content, attachedInfoElem: {}, attachedInfo: raw.attachedInfo ?? '', status: raw.status ?? 2} as MessageItem;
}
export default function PreviewMessageModal({closeModal,userID}:{closeModal:()=>void;userID:string}) {
  const [conversations,setConversations]=useState<ConversationItem[]>([]);
  const [selected,setSelected]=useState<ConversationItem>();
  const [messages,setMessages]=useState<MessageItem[]>([]);
  const [loading,setLoading]=useState(false);
  const [error,setError]=useState('');
  const [keyword,setKeyword]=useState('');
  const [day,setDay]=useState('');
  const [filter,setFilter]=useState('');
  const [cursor,setCursor]=useState(0);
  const [more,setMore]=useState(false);
  const generation=useRef(0);
  useEffect(()=>{
    const current=++generation.current;setLoading(true);
    post('conversation/get_all_conversations',{ownerUserID:userID}).then(data=>{
      if(current===generation.current)setConversations(data.conversations??[]);
    }).catch(reason=>setError(reason.message??'加载会话失败')).finally(()=>setLoading(false));
    return ()=>{generation.current++;};
  },[userID]);
  async function load(item:ConversationItem,reset:boolean) {
    const current=++generation.current;setLoading(true);setError('');if(reset)setMessages([]);
    try {
      let next=cursor;
      if(reset){const data=await post('msg/get_conversations_has_read_and_max_seq',{userID,conversationIDs:[item.conversationID]});next=Number(data.seqs?.[item.conversationID]?.maxSeq??0);}
      const found:MessageItem[]=[];
      // Bounded batches, continuing across all history when a filter is set.
      do {
        if(next<=0)break;
        const begin=Math.max(1,next-199);
        const data=await post('msg/pull_msg_by_seq',{userID,seqRanges:[{conversationID:item.conversationID,begin,end:next,num:200}],order:0});
        const bucket=data.msgs?.[item.conversationID];
        const rows:any[]=Array.isArray(bucket)?bucket:bucket?.msgs??[];
        for(const raw of rows){const message=normalise(raw);if(Number(message.status)===4)continue;
          if(day&&new Date(message.sendTime).toLocaleDateString('sv-SE')!==day)continue;
          if(keyword&&!JSON.stringify(message.content).toLowerCase().includes(keyword.toLowerCase()))continue;
          found.push(message);
        }
        next=begin-1;if(current!==generation.current)return;
      }while(found.length<20&&next>0);
      if(current!==generation.current)return;
      setCursor(next);setMore(next>0);
      setMessages(previous=>[...new Map((reset?found:[...found,...previous]).map(message=>[message.clientMsgID,message])).values()].sort((a,b)=>a.seq-b.seq));
    }catch(reason:any){if(current===generation.current)setError(reason.message??'加载消息失败');}
    finally{if(current===generation.current)setLoading(false);}
  }
  return <Modal open width={1080} title={'聊天记录 · '+userID} onCancel={closeModal} footer={null} destroyOnClose>
    <div style={{display:'flex',height:650,gap:16}}>
      <div style={{width:240,overflow:'auto',borderRight:'1px solid #eee',paddingRight:12}}>
        <Input.Search placeholder="搜索会话" value={filter} onChange={e=>setFilter(e.target.value)}/>
        <List dataSource={conversations.filter(item=>[item.showName,item.userID,item.groupID].join(' ').includes(filter))} renderItem={item=><List.Item style={{cursor:'pointer',background:selected?.conversationID===item.conversationID?'#e6f4ff':undefined}} onClick={()=>{setSelected(item);void load(item,true);}}>{item.showName||item.groupID||item.userID||item.conversationID}</List.Item>}/>
      </div>
      <div style={{flex:1,minWidth:0,overflow:'auto'}}>
        <Space wrap style={{marginBottom:12}}>
          <Input.Search placeholder="搜索消息内容" value={keyword} onChange={e=>setKeyword(e.target.value)} onSearch={()=>selected&&void load(selected,true)}/>
          <DatePicker onChange={(_,value)=>setDay(typeof value==='string'?value:'')}/>
          <Button disabled={!selected||loading} onClick={()=>selected&&void load(selected,true)}>查询</Button>
          <Button disabled={!selected||!more||loading} onClick={()=>selected&&void load(selected,false)}>更早消息</Button>
        </Space>
        {error&&<Alert type="error" message={error} showIcon/>}
        <Spin spinning={loading}>{!selected?<Empty description="选择左侧会话查看记录"/>:messages.length===0&&!loading?<Empty description="没有匹配消息"/>:messages.map(message=><HistoryMessageItem key={message.clientMsgID} message={message} isSelf={message.sendID===userID}/>)}</Spin>
      </div>
    </div>
  </Modal>;
}
