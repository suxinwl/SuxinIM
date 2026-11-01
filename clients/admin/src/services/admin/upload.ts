import { API_URL } from '@/config';
import { request } from '@umijs/max';
import { message } from 'antd';
import file2md5 from 'file2md5';
import md5 from 'md5';
import { UploadRequestOption } from 'rc-upload/lib/interface';

export enum minioUploadType {
  file = '1',
  video = '2',
  picture = '3',
}
export async function minioUpload(
  file: File,
  fileType: minioUploadType,
  onProgress?: (progress: number) => void,
) {
  if (!file.size) throw new Error('不能上传空文件');
  const limit = await request(API_URL + 'object/part_size', {method:'POST',data:{size:file.size}});
  const partSize = Number(limit.data.size);
  if (!Number.isSafeInteger(partSize) || partSize <= 0) throw new Error('无效的分片大小');
  const chunks:Blob[] = [], hashes:string[] = [];
  for (let start=0;start<file.size;start+=partSize) {
    const blob=file.slice(start,Math.min(start+partSize,file.size));
    chunks.push(blob);hashes.push(await file2md5(new File([blob],file.name)));
  }
  const name=`${localStorage.getItem('IMAdminUserID')}/${crypto.randomUUID()}/${file.name}`;
  const contentType=file.type || 'application/octet-stream';
  const initiated=await request(API_URL+'object/initiate_multipart_upload',{method:'POST',data:{hash:md5(hashes.join(',')),size:file.size,maxParts:chunks.length,cause:'admin-upload',name,contentType}});
  if (initiated.data.url) {onProgress?.(100);return {data:{url:initiated.data.url}};}
  const info=initiated.data.upload;let completed=0;
  for (let index=0;index<chunks.length;index++) {
    const initialPart=info.sign?.parts?.find((part:any)=>part.partNumber===index+1);
    const common=initialPart?info.sign:(await request(API_URL+'object/auth_sign',{method:'POST',data:{uploadID:info.uploadID,partNumbers:[index+1]}})).data;
    const part=initialPart||common.parts[0],url=new URL(part.url || common.url);
    for (const query of [common.query,part.query]) for (const [key,values] of Object.entries(query || {})) {
      url.searchParams.delete(key);for(const value of values as string[])url.searchParams.append(key,value);
    }
    await new Promise<void>((resolve,reject)=>{
      const xhr=new XMLHttpRequest();xhr.open('PUT',url.toString());
      for(const [key,values] of Object.entries({...common.header,...part.header}))xhr.setRequestHeader(key,(values as string[]).join(','));
      xhr.upload.onprogress=event=>onProgress?.((completed+event.loaded)*100/file.size);
      xhr.onerror=()=>reject(new Error('附件网络传输失败'));
      xhr.onload=()=>xhr.status>=200&&xhr.status<300?resolve():reject(new Error(`附件上传失败：HTTP ${xhr.status}`));
      xhr.send(chunks[index]);
    });
    completed+=chunks[index].size;
  }
  const result=await request(API_URL+'object/complete_multipart_upload',{method:'POST',data:{uploadID:info.uploadID,parts:hashes,name,contentType}});
  onProgress?.(100);return result;
}

export async function getCosAuthorization() {
  return request('/api/third/tencent_cloud_storage_credential', {
    method: 'POST',
    data: {},
    baseURL: API_URL,
  });
}

export async function upload(file: any, hash: any) {
  return request('/object/initiate_multipart_upload', {
    method: 'POST',
    data: {
      hash,
      size: file.size,
      partSize: file.size,
      maxParts: -1,
      cause: 'test-file',
      name: file.name,
      contentType: 'application/octet-stream',
    },
    baseURL: API_URL,
  });
}

export async function completeUpload(uploadID: string, hash: string, fileName: string) {
  return request('/object/complete_multipart_upload', {
    method: 'POST',
    data: {
      uploadID,
      parts: [hash],
      name: localStorage.getItem('IMAdminUserID') + '/' + fileName,
      contentType: 'application/octet-stream',
    },
    baseURL: API_URL,
  });
}

export async function handleFileUpload(data: UploadRequestOption) {
  try {
    const fileSize = (data.file as File).size;
    if (fileSize > 1024 * 1024 * 1) {
      message.error('图片尺寸过大');
      return;
    }

    const result=await minioUpload(data.file as File,minioUploadType.picture,percent=>data.onProgress?.({percent}));
    data.onSuccess?.(result);return result.data.url as string;
  } catch (error) {
    console.log(error);
    message.error('上传失败！');
    data.onError?.(error as Error);
  }
}
