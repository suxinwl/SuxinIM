import { defHttp } from '@/utils/http';
import { UploadFileParams } from '/#/axios';
enum Api {
    downCode = '/developer/packinstall/downCode',
    installCode = '/developer/packinstall/installCode',
    uninstallCode = '/developer/packinstall/uninstallCode',
    packCode = '/developer/packinstall/packCode',
    getInstallPack = '/developer/packinstall/getInstallPack',
    getPackdirs = '/developer/packinstall/getPackdirs',
    getMenutreeData = '/developer/packinstall/getMenutreeData',
    menuTreeToJson = '/developer/packinstall/menuTreeToJson',
}
//上传服务器域名
const DOMAIN= import.meta.env.VITE_APP_ENV=="production"? window?.globalConfig.Root_url: window?.globalConfig.Root_url_dev;
//下载安装代码包
export function downCode(params: object,baseurl:string) {
    return defHttp.post({ url: Api.downCode+"?baseurl="+baseurl, params:params}, { errorMessageMode: 'message' });
}
//安装
export function installCode(params: object) {
    return defHttp.post({ url: Api.installCode, params:params, timeout: 180000}, { errorMessageMode: 'message' });
}
//卸载
export function uninstallCode(params: object) {
    return defHttp.post({ url: Api.uninstallCode, params:params}, { errorMessageMode: 'message' });
}
//打包
export function packCode(params: object) {
    return defHttp.post({ url: Api.packCode, params:params}, { errorMessageMode: 'message' });
}

//获取已经安装的包
export function getInstallPack(params: object) {
    return defHttp.get({ url: Api.getInstallPack, params:params}, { errorMessageMode: 'message' });
}


//获取菜单数据
export function getMenutreeData(params: object) {
    return defHttp.get({ url: Api.getMenutreeData, params:params}, { errorMessageMode: 'message' });
}
//菜单id转JSON数据
export function menuTreeToJson(params: object) {
    return defHttp.post({ url: Api.menuTreeToJson, params:params}, { errorMessageMode: 'message' });
}
//安装本地代码包
export function installLocalCode(  params: UploadFileParams, onUploadProgress?: (progressEvent: any) => void) {
    return defHttp.uploadFile({url:`${DOMAIN}/developer/packinstall/installLocalCode`,onUploadProgress,timeout:600000},params);
}
//上传文件到公共仓
export function userUploadFile(
  params: UploadFileParams,
  onUploadProgress?: (progressEvent: any) => void
) {
  return defHttp.uploadFile<any>({ url:`${DOMAIN}/developer/packinstall/upfile`,onUploadProgress},params);
}
