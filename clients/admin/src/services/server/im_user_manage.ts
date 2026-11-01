import { API_URL } from '@/config';
import { request } from '@umijs/max';

export async function getUserList(params: API.UserManage.GetUserParams) {
  return request('/user/get_users', {
    method: 'POST',
    data: {
      ...params,
      // 0全部 1未封 2已封
      normal: 1,
      genders: [1],
    },
    baseURL: API_URL,
  });
}
