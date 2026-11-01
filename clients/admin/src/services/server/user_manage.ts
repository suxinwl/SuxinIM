import { ACCOUNT_URL, API_URL, CHAT_URL } from '@/config';
import { request } from '@umijs/max';

// new
export async function getUserList(params: API.UserManage.GetUserParams) {
  return request('/user/search/full', {
    method: 'POST',
    data: {
      ...params,
      // 0全部 1未封 2已封
      normal: 1,
      genders: 0,
    },
    headers: {
      isAccount: true,
    },
    baseURL: CHAT_URL,
  });
}

// new
export async function updateUserInfo(params: API.UserManage.UserInfoItem) {
  return request<{
    data: any;
  }>('user/update', {
    method: 'POST',
    data: {
      ...params,
    },
    headers: {
      isAccount: true,
    },
    baseURL: CHAT_URL,
  });
}

// new
export async function registerUser(params: API.UserManage.RegisterUserParams) {
  return request<{
    data: any;
  }>('/account/add_user', {
    method: 'POST',
    data: {
      ...params,
      platform: 5,
    },
    headers: {
      isAccount: true,
    },
    baseURL: ACCOUNT_URL,
  });
}

// new
export async function resetUserPassword(params: API.UserManage.ResetUserPasswordParams) {
  return request<{
    data: any;
  }>('/user/password/reset', {
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

//new
export async function getBlockList(params: API.UserManage.GetUserParams) {
  return request<{
    data: any;
  }>('/block/search', {
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

// new
// op  1 block 2 unblock user
export async function updateBlock(userID: string, reason = '') {
  return request<{
    data: any;
  }>('/block/add', {
    method: 'POST',
    data: {
      userID,
      reason,
    },
    headers: {
      isAccount: true,
    },
    baseURL: ACCOUNT_URL,
  });
}

// 解封
export async function deleBlock(userID: string) {
  return request<{
    data: any;
  }>('/block/del', {
    method: 'POST',
    data: {
      userIDs: [userID],
    },
    headers: {
      isAccount: true,
    },
    baseURL: ACCOUNT_URL,
  });
}

// new
export async function getFriendList(params: API.UserManage.GetFriendsParams) {
  return request<{
    data: any;
  }>('/friend/get_friend_list', {
    method: 'POST',
    data: {
      ...params,
    },
    baseURL: API_URL,
  });
}

// new
export async function deleteFriend(params: API.UserManage.DeleteFriendParams) {
  return request<{
    data: any;
  }>('/friend/delete_friend', {
    method: 'POST',
    data: {
      ...params,
    },
    baseURL: API_URL,
  });
}

// new 下线
export async function forceLogout(params: API.UserManage.KickUserParams) {
  return request<{
    data: any;
  }>('/auth/force_logout', {
    method: 'POST',
    data: {
      ...params,
    },
    baseURL: API_URL,
  });
}

// new
export async function getUsersOnlineStatus(userIDs: string[]) {
  return request<{
    data: any;
  }>('/user/get_users_online_token_detail', {
    method: 'POST',
    data: {
      userIDs,
    },
    baseURL: API_URL,
  });
}

// new
export async function update_info(params: API.UserManage.UpdateAdminInfoParams) {
  return request<{
    data: any;
  }>('/account/update', {
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

export async function changeAdminPassword(params: {userID: string; currentPassword: string; newPassword: string}) {
  return request('/account/change_password', {method: 'POST', data: params, baseURL: ACCOUNT_URL});
}

// new
