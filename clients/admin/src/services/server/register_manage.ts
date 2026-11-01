import { ACCOUNT_URL } from '@/config';
import type { InvitationCodeStatus } from '@/constants/enum';
import { request } from '@umijs/max';

export async function getAllForbiddenIP(pageNumber: number, showNumber: number) {
  return request<{
    data: any;
  }>('/admin/account/query_ip_forbidden_register', {
    method: 'POST',
    data: {
      pageNumber: pageNumber - 1,
      showNumber,
    },
    headers: {
      isAccount: true,
    },
    baseURL: ACCOUNT_URL,
  });
}

// new
export async function addDefaultGroup(groupIDs: string[]) {
  return request<{
    data: any;
  }>('/default/group/add', {
    method: 'POST',
    data: {
      groupIDs,
    },
    headers: {
      isAccount: true,
    },
    baseURL: ACCOUNT_URL,
  });
}

// new
export async function removeDefaultGroup(groupIDs: string[]) {
  return request('/default/group/del', {
    method: 'POST',
    data: {
      groupIDs,
    },
    headers: {
      isAccount: true,
    },
    baseURL: ACCOUNT_URL,
  });
}

// new
export async function getuserregistergroupidlist(
  pageNumber: number,
  showNumber: number,
  keyword = '',
) {
  return request<{
    data: any;
  }>('/default/group/search', {
    method: 'POST',
    data: {
      keyword,
      pagination: {
        pageNumber: pageNumber,
        showNumber: showNumber,
      },
    },
    headers: {
      isAccount: true,
    },
    baseURL: ACCOUNT_URL,
  });
}

// 查询指定IP注册用户
export async function getIPRegister(pageNumber: number, showNumber: number, ip: string) {
  return request<{
    data: any;
  }>('/admin/account/query_ip_user_register', {
    method: 'POST',
    data: {
      ip,
      pageNumber: pageNumber - 1,
      showNumber,
    },
    headers: {
      isAccount: true,
    },
    baseURL: ACCOUNT_URL,
  });
}

export type UpdateIPLimitParams = {
  ip: string;
  limitRegister: number;
  limitLogin: number;
};

// 是否允许指定IP注册登录
export async function updateIPLimit(params: UpdateIPLimitParams) {
  return request<{
    data: any;
  }>('/admin/account/add_ip_forbidden_register', {
    method: 'POST',
    data: {
      ...params,
    },
    headers: {
      isAccount: true,
    },
    baseURL: ACCOUNT_URL,
  });
}

// 取消禁止指定IP注册登录
export async function cancelBlockIP(ip: string) {
  return request<{
    data: any;
  }>('/admin/account/remove_ip_forbidden_register', {
    method: 'POST',
    data: {
      ip,
    },
    headers: {
      isAccount: true,
    },
    baseURL: ACCOUNT_URL,
  });
}

// 用户只能在指定ip登录注册
export async function getUserLimitIP(userID: string, pageNumber: number, showNumber: number) {
  return request<{
    data: any;
  }>('/admin/account/query_user_ip_limit_login', {
    method: 'POST',
    data: {
      userID,
      pageNumber: pageNumber - 1,
      showNumber,
    },
    headers: {
      isAccount: true,
    },
    baseURL: ACCOUNT_URL,
  });
}

export async function addUserLimitIP(userID: string, ip: string) {
  return request<{
    data: any;
  }>('/admin/account/add_user_ip_limit_login', {
    method: 'POST',
    data: {
      ip,
      userID,
    },
    headers: {
      isAccount: true,
    },
    baseURL: ACCOUNT_URL,
  });
}

// ip为空时删除全部
export async function cancelUserLimitIP(userID: string, ip?: string) {
  return request<{
    data: any;
  }>('/admin/account/remove_user_ip_limit_login', {
    method: 'POST',
    data: {
      ip,
      userID,
    },
    headers: {
      isAccount: true,
    },
    baseURL: ACCOUNT_URL,
  });
}

interface GetInvitationCodeParams {
  status?: InvitationCodeStatus;
  codeList: string[];
  pageNumber: number;
  showNumber: number;
}

// new
export async function getInvitationCode(data: GetInvitationCodeParams) {
  return request<{
    data: any;
  }>('/invitation_code/search', {
    method: 'POST',
    data: {
      ...data,
    },
    headers: {
      isAccount: true,
    },
    baseURL: ACCOUNT_URL,
  });
}

// new
export async function generateInvitationCode(num = 1, len = 8) {
  return request<{
    data: any;
  }>('/invitation_code/gen', {
    method: 'POST',
    data: {
      num,
      len,
      chars: 'ABCDEFGHIJKLM0123456789',
    },
    headers: {
      isAccount: true,
    },
    baseURL: ACCOUNT_URL,
  });
}

// new
export async function getDefaultFriends(pageNumber: number, showNumber: number, keyword: string) {
  return request<{
    data: any;
  }>('/default/user/search', {
    method: 'POST',
    data: {
      keyword,
      pagination: {
        pageNumber: pageNumber,
        showNumber: showNumber,
      },
    },
    headers: {
      isAccount: true,
    },
    baseURL: ACCOUNT_URL,
  });
}

// new
export async function addDefaultFriends(userIDs: string[]) {
  return request<{
    data: any;
  }>('/default/user/add', {
    method: 'POST',
    data: {
      userIDs,
    },
    headers: {
      isAccount: true,
    },
    baseURL: ACCOUNT_URL,
  });
}

// new
export async function removeDefaultFriends(userIDs: string[]) {
  return request<{
    data: any;
  }>('/default/user/del', {
    method: 'POST',
    data: {
      userIDs,
    },
    headers: {
      isAccount: true,
    },
    baseURL: ACCOUNT_URL,
  });
}
