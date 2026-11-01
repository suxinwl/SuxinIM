import { API_URL } from '@/config';
import { request } from '@umijs/max';

// new
export async function getGroupList(params: API.GroupManage.GetGroupParams) {
  return request('/group/get_groups', {
    method: 'POST',
    data: {
      ...params,
    },
    baseURL: API_URL,
  });
}

// new
export async function getGroupMemberList(params: API.GroupManage.GetGroupMembersParams) {
  return request<{
    data: any;
  }>('/group/get_group_member_list', {
    method: 'POST',
    data: {
      ...params,
    },
    baseURL: API_URL,
  });
}

// new
export async function getSomeGroupMemberList(params: API.GroupManage.GetSomeGroupMembersParams) {
  return request<{
    data: any;
  }>('/group/get_group_members_info', {
    method: 'POST',
    data: {
      ...params,
    },
    baseURL: API_URL,
  });
}

// new
export async function createGroup(params: API.GroupManage.CreateGroupParams) {
  return request<{
    data: any;
  }>('/group/create_group', {
    method: 'POST',
    data: {
      ...params,
    },
    baseURL: API_URL,
  });
}

// new
export async function updateGroup(params: API.GroupManage.UpdateGroupParams) {
  return request<{
    data: any;
  }>('/group/set_group_info', {
    method: 'POST',
    data: {
      ...params,
    },
    baseURL: API_URL,
  });
}

// new
export async function muteGroup(groupID: string) {
  return request<{
    data: any;
  }>('/group/mute_group', {
    method: 'POST',
    data: {
      groupID,
    },
    baseURL: API_URL,
  });
}

// new
export async function cancelMuteGroup(groupID: string) {
  return request<{
    data: any;
  }>('/group/cancel_mute_group', {
    method: 'POST',
    data: {
      groupID,
    },
    baseURL: API_URL,
  });
}

// new
export async function dismissGroup(groupID: string) {
  return request<{
    data: any;
  }>('/group/dismiss_group', {
    method: 'POST',
    data: {
      groupID,
    },
    baseURL: API_URL,
  });
}

//new
export async function kickGroupMemebr(params: API.GroupManage.KickGroupMemebrParams) {
  return request<{
    data: any;
  }>('/group/kick_group', {
    method: 'POST',
    data: {
      ...params,
    },
    baseURL: API_URL,
  });
}

// new
export async function invite2Group(params: API.GroupManage.InviteGroupMemebrParams) {
  return request<{
    data: any;
  }>('/group/invite_user_to_group', {
    method: 'POST',
    data: {
      ...params,
    },
    baseURL: API_URL,
  });
}

// new
export async function muteGroupMember(params: API.GroupManage.MuteGroupMemebrParams) {
  return request<{
    data: any;
  }>('/group/mute_group_member', {
    method: 'POST',
    data: {
      ...params,
    },
    baseURL: API_URL,
  });
}

// new
export async function cancelMuteGroupMember(
  params: Omit<API.GroupManage.MuteGroupMemebrParams, 'mutedSeconds'>,
) {
  return request<{
    data: any;
  }>('/group/cancel_mute_group_member', {
    method: 'POST',
    data: {
      ...params,
    },
    baseURL: API_URL,
  });
}

// new
export async function updateGroupMemebr(params: API.GroupManage.UpdateGroupMemebrParams) {
  return request<{
    data: any;
  }>('/group/set_group_member_info', {
    method: 'POST',
    data: {
      ...params,
    },
    baseURL: API_URL,
  });
}

// new
export async function transferGroup(params: API.GroupManage.TransferGroupParams) {
  return request<{
    data: any;
  }>('/group/transfer_group', {
    method: 'POST',
    data: {
      ...params,
    },
    baseURL: API_URL,
  });
}
