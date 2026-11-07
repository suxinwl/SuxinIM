import { useUserStore } from '@/store'
import { useClipboard } from '@vueuse/core';
import { isAllUrl } from '@/utils/is';
import { Message } from '@arco-design/web-vue';
//获取随机数
export function randomRange(min:number, max:number){
    var returnStr = "",
    range = (max ? Math.round(Math.random() * (max-min)) + min : min),
    arr = ['0', '1', '2', '3', '4', '5', '6', '7', '8', '9', 'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i', 'j', 'k', 'l', 'm', 'n', 'o', 'p', 'q', 'r', 's', 't', 'u', 'v', 'w', 'x', 'y', 'z', 'A', 'B', 'C', 'D', 'E', 'F', 'G', 'H', 'I', 'J', 'K', 'L', 'M', 'N', 'O', 'P', 'Q', 'R', 'S', 'T', 'U', 'V', 'W', 'X', 'Y', 'Z'];
    for(var i=0; i<range; i++){
    var index = Math.round(Math.random() * (arr.length-1));
    returnStr += arr[index];
    }
    return returnStr;
}
//获取附件完整访问路径
export function GetFullPath(url:any):string{
    if(!url){
        return ""
    }else if(isAllUrl(url)){
        return url
    }else{
        var imgname = url.split('/');//分割url
        imgname = imgname[imgname.length-1];//通过最后一个数组下标获取图片名
        const userStore = useUserStore()
        if(!url.startsWith("/")) url="/"+url;
        if(url.startsWith('/common/storage/pan123/') || url.startsWith('/resource/static/brand/')) {
           url = (userStore.rooturls?.local || '') + url
        }else if(imgname.startsWith("local")){
           url = userStore.rooturls?.local+url
        }else if(imgname.startsWith("alioss")){
           url = userStore.rooturls?.alioss+url
        }else if(imgname.startsWith("tencentcos")){
           url = userStore.rooturls?.tencentcos+url
        }else if(imgname.startsWith("qiniuoss")){
           url = userStore.rooturls?.qiniuoss+url
        }else{//如果地址前缀无法判断文件存储位置，则使用设置上传方式的访问地址
           url = userStore.defrooturl+url
        }
        return url
    }
}
//获取本地附件完整访问路径
export function GetLocalFullUrl(url:any):string{
    if(!url){
        return ""
    }else if(isAllUrl(url)){
        return url
    }else{
        const userStore = useUserStore()
        return userStore.rooturls?.local+url
    }
}
//复制
var copyObj :any
export function CopyText(text:any,msg:string){
    if(!copyObj){
        const { copy } = useClipboard();
        copyObj=copy
    }
    copyObj(text);
    Message.success({content:msg,id:"copy"});
}
