export function attachmentSourceName(explicit?:string,source?:string){
  if(explicit)return explicit;
  const name=source?.split(/[\\/]/).pop();if(!name)return undefined;
  if(/^(?:puty-local|https?|file):/i.test(source || ''))try{return decodeURIComponent(name);}catch{}
  return name;
}
function metadataName(message:any):string|undefined{
  try{const extra=typeof message?.ex==='string'?JSON.parse(message.ex):message?.ex,name=extra?.putySourceName;return typeof name==='string' && name.length<=1024 && name.trim() && !/[\\/\x00-\x1f]/.test(name) && !['.','..'].includes(name)?name:undefined;}catch{return;}
}
export function attachmentMessageSourceName(message:any){
  if(message?.fileElem?.fileName)return message.fileElem.fileName;
  return metadataName(message) || attachmentSourceName(undefined,message?.pictureElem?.sourcePath || message?.videoElem?.videoPath || message?.soundElem?.soundPath);
}
export function withAttachmentSourceName<T extends {ex?:string}>(message:T,name:string):T{
  if(!name || /[\\/\x00-\x1f]/.test(name) || name.length>1024)throw new Error('附件源文件名无效');
  let extra:Record<string,unknown>={};try{const prior=JSON.parse(message.ex || '{}');if(prior && typeof prior==='object' && !Array.isArray(prior))extra=prior;}catch{if(message.ex)extra.putyOriginalEx=message.ex;}
  return {...message,ex:JSON.stringify({...extra,putySourceName:name})};
}
