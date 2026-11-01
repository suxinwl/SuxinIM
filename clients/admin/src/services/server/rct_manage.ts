const ACCOUNT_URL = '/enterprise/v1/admin/';
import { request } from '@umijs/max';

export async function getVideoCallList(params: API.RtcManage.VideoParams) {
  return request('/rtc/get_signal_invitation_records', {
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

export async function deleteVideoCall(params: API.RtcManage.DeleteVideoParams) {
  return request('/rtc/delete_signal_records', {
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

export async function getMeetingList(params: API.RtcManage.MeetingParams) {
  return request('/rtc/get_meeting_records', {
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

export async function deleteMeeting(params: API.RtcManage.DeleteMeetingParams) {
  return request('/rtc/delete_meeting_records', {
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
