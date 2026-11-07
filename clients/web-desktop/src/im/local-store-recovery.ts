import {Input,Modal} from 'antd';
import {createElement} from 'react';
type Credentials={origin:string;userID:string;imToken:string;chatToken:string};
export function recoverLocalArchive(credentials:Credentials):Promise<void>{
  return new Promise((resolve,reject)=>{
    let passphrase='';
    Modal.confirm({title:'恢复当前账号的本地加密档案',content:createElement('div',null,
      createElement('p',null,'请选择完整备份目录并输入独立恢复口令。当前本机档案会移至加密回滚目录保留，服务器资料不受影响。'),
      createElement(Input.Password,{autoComplete:'new-password',placeholder:'至少 12 个字符的备份恢复口令',onChange:event=>{passphrase=event.target.value;}})),okText:'选择备份并恢复',cancelText:'取消',
      onOk:async()=>{if(passphrase.length<12)throw new Error('请输入至少 12 个字符的恢复口令');await window.electronAPI!.ipcInvoke('puty-store','restore-backup',{credentials,passphrase});passphrase='';resolve();},
      onCancel:()=>{passphrase='';reject(new Error('本地档案尚未恢复'));},
    });
  });
}
