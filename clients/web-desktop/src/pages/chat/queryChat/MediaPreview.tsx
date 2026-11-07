import {isLocalFileURL,localFilePath} from '@/im/local-files';
import ProtectedPreviewGroup from '@/components/ProtectedPreviewGroup';
import PreviewToolsBar from '@/components/PreviewToolsBar';
import VideoPlayer from '@/components/VideoPlayer';
import {previewImageSource} from '@/im/preview-source';
import {forwardRef,memo,useImperativeHandle,useState,type ForwardRefRenderFunction} from 'react';
import {useMessageStore,useUserStore} from '@/store';
import {downloadFile,getDownloadTask} from '@/utils/common';
const MediaPreview:ForwardRefRenderFunction<{showAlbum:(clientMsgID:string)=>void},{conversationID:string}>=({conversationID},ref)=>{
  const [visible,setVisible]=useState(false),[current,setCurrent]=useState(0);
  const images=useMessageStore(state=>state.previewImgList),cache=useUserStore(state=>state.imageCache);
  async function showAlbum(clientMsgID:string){
    let index=images.findIndex(image=>image.clientMsgID===clientMsgID);
    if(index<0){await useMessageStore.getState().getConversationPreviewImgList();index=useMessageStore.getState().previewImgList.findIndex(image=>image.clientMsgID===clientMsgID);}
    if(index<0)return;setCurrent(index);setVisible(true);
  }
  function save(index:number){
    const image=images[index];if(!image)return;
    const source=image.videoUrl || image.url,task=getDownloadTask({downloadMap:useMessageStore.getState().downloadMap,compareKey:'clientMsgID',compareValue:image.clientMsgID});
    if(task?.downloadState==='pause'){void window.electronAPI?.ipcInvoke('resumeDownload',task.downloadUrl);return;}
    if(isLocalFileURL(source)){window.electronAPI?.showInFinder(localFilePath(source));return;}
    void downloadFile(source,{clientMsgID:image.clientMsgID,conversationID,isMediaMessage:true,showError:true,saveType:image.videoUrl?'video':'image'});
  }
  useImperativeHandle(ref,()=>({showAlbum}),[images]);
  return <div style={{display:'none'}}><ProtectedPreviewGroup items={images.map(image=>previewImageSource(image,Boolean(window.electronAPI),cache))} preview={{
    current,visible,onVisibleChange:setVisible,onChange:setCurrent,
    toolbarRender:(node,{current:index})=>{const image=images[index];return image?<PreviewToolsBar clientMsgID={image.clientMsgID} isFileDownloaded={isLocalFileURL(image.videoUrl || image.url)} dom={node as JSX.Element} hiddenImageAction={Boolean(image.videoUrl)} onDownloadOrShowFinder={()=>save(index)}/>:node;},
    imageRender:(node,{current:index})=>images[index]?.videoUrl?<div className="relative"><VideoPlayer url={images[index].videoUrl!} autoplay/></div>:node,
  }}/></div>;
};
export default memo(forwardRef(MediaPreview));
