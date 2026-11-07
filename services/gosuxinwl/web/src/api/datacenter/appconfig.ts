import { defHttp } from '@/utils/http';
enum Api {
    getConfig = '/datacenter/appconfig/getConfig',
    saveConfig = '/datacenter/appconfig/saveConfig',
}

//列表数据
export function getConfig(params: object) {
  return defHttp.get({ url: Api.getConfig, params:params }, { errorMessageMode: 'message' });
}
//提交数据
export function saveConfig(params: any) {
    if(params.fileSize){
        params.fileSize= parseFloat(params.fileSize)*pow1024(3)
    }
    return defHttp.post({ url: Api.saveConfig, params:params}, { errorMessageMode: 'message' });
}
// 求次幂
function pow1024(num:number) {
    return Math.pow(1024, num)
}
/**数据类型 */
export   interface menuItem {
    id:number,
    title: string;
}
