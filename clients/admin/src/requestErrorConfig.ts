import type {RequestConfig} from '@umijs/max';
import {message} from 'antd';
function check(payload:any){
 if(typeof payload?.errCode==='number'&&payload.errCode!==0){
  const error=new Error(payload.errMsg||payload.errDlt||'请求失败');
  Object.assign(error,{code:payload.errCode});throw error;
 }
}
export const errorConfig:RequestConfig={
 errorConfig:{errorThrower:check,errorHandler:(error:any,options:any)=>{if(options?.skipErrorHandler)throw error;message.error(error.message||'网络请求失败');}},
 responseInterceptors:[response=>{check(response.data);return response;}],
};
