import { defHttp } from '@/utils/http';
import { FilterParam } from '/@/utils';
//类型
export interface LoginData {
  username: string;
  password: string;
}
enum Api {
    getList = '/system/dept/getList',
    getParent = '/system/dept/getParent',
    save = '/system/dept/save',
    upStatus = '/system/dept/upStatus',
    upIMGroup = '/system/dept/upIMGroup',
    getLeaderOptions = '/system/dept/getLeaderOptions',
    del = '/system/dept/del',
    dragDept = '/system/dept/dragDept',
}

//列表数据
export function getList(params: any) {
  params=FilterParam(params)
  return defHttp.get({ url: Api.getList, params:params }, { errorMessageMode: 'message' });
}
//选择数据
export function getParent(params: any) {
  return defHttp.get({ url: Api.getParent, params:params}, { errorMessageMode: 'message' });
}
//提交数据
export function save(params: object) {
    return defHttp.post({ url: Api.save, params:params}, { errorMessageMode: 'message' });
}
//更新状态
export function upStatus(params: object) {
    return defHttp.post({ url: Api.upStatus, params:params}, { errorMessageMode: 'message' });
}
export interface DepartmentLeader {
  id: number;
  name: string;
  employee_no: string;
  dept_id: string;
  department: string;
}
export interface DepartmentLeaderPage {
  items: DepartmentLeader[];
  total: number;
  page: number;
  pageSize: number;
}
export function getLeaderOptions(params: { keyword?: string; dept_id?: number; parent_id?: number; page?: number; pageSize?: number }, signal?: AbortSignal) {
  // An unavailable directory must settle instead of leaving the selector spinning.
  return defHttp.get<DepartmentLeaderPage>({ url: Api.getLeaderOptions, params, signal, timeout: 15000 }, {
    errorMessageMode: 'none', retryRequest: { isOpenRetry: false, count: 0, waitTime: 0 },
  });
}
export function upIMGroup(params: { id: number; im_group_enabled: number }) {
  return defHttp.post({ url: Api.upIMGroup, params }, { errorMessageMode: 'message' });
}
//删除数据
export function del(params: object) {
    return defHttp.delete({ url: Api.del, params:params}, { errorMessageMode: 'message' });
}
//移动数据
export function dragDept(params: object) {
    return defHttp.post({ url: Api.dragDept, params:params}, { errorMessageMode: 'message' });
}
