import { defHttp } from '@/utils/http';
enum Api {
    getConfig = '/datacenter/uploadconfig/getConfig',
    saveConfig = '/datacenter/uploadconfig/saveConfig',
}

//列表数据
export function getConfig(params: object) {
  return defHttp.get({ url: Api.getConfig, params:params }, { errorMessageMode: 'message' });
}
//提交数据
export function saveConfig(params: any) {
    return defHttp.post({ url: Api.saveConfig, params:params}, { errorMessageMode: 'message' });
}

export interface Pan123TestResult {
 success: boolean;
 uid: number;
 steps: { name: string; ok: boolean; message: string }[];
}
export function testConnection(params: object) {
 return defHttp.post<Pan123TestResult>({ url: '/datacenter/uploadconfig/testConnection', params, timeout: 330000 }, { errorMessageMode: 'message' });
}
