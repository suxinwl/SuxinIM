/* eslint-disable no-case-declarations */
import { OBJECT_STORAGE } from '@/config';
import { getCosAuthorization, minioUpload, minioUploadType } from '@/services/admin/upload';
import { message } from 'antd';
import { isThisYear } from 'date-fns';
import dayjs from 'dayjs';
import calendar from 'dayjs/plugin/calendar';
import updateLocale from 'dayjs/plugin/updateLocale';
import moment from 'moment';
import { MessageItem } from '@openim/wasm-client-sdk';
import { MessageType } from '@openim/wasm-client-sdk';
import type { UploadRequestOption } from 'rc-upload/lib/interface';
import { cosUpload } from './cos';

dayjs.extend(calendar);
dayjs.extend(updateLocale);

dayjs.updateLocale('en', {
  calendar: {
    sameDay: 'HH:mm',
    nextDay: '[tomorrow]',
    nextWeek: 'dddd',
    lastDay: '[yesterday] HH:mm',
    lastWeek: 'dddd HH:mm',
    sameElse: 'YYYY/M/D HH:mm',
  },
});
dayjs.updateLocale('zh-cn', {
  calendar: {
    sameDay: 'HH:mm',
    nextDay: '[明天]',
    nextWeek: 'dddd',
    lastDay: '[昨天] HH:mm',
    lastWeek: 'dddd HH:mm',
    sameElse: 'YYYY年M月D日 HH:mm',
  },
});

export type TimeRange = 'today' | 'isoWeek' | 'month' | 'year' | 'custom';

export const uuid = (): string => {
  return (Math.random() * 36).toString(36).slice(2) + new Date().getTime().toString();
};

export const switchDateStr = (value: string, range: TimeRange) => {
  if (range !== 'isoWeek') {
    if (range === 'custom') {
      return moment.utc(value).format('YYYY-MM-DD');
    }
    return moment.utc(value).format('MM-DD');
  }
  const num = moment.utc(value).isoWeekday();
  switch (num) {
    case 1:
      return '星期一';
    case 2:
      return '星期二';
    case 3:
      return '星期三';
    case 4:
      return '星期四';
    case 5:
      return '星期五';
    case 6:
      return '星期六';
    case 7:
      return '星期天';
    default:
      return '';
  }
};

export const clearObjArr = (arr: any[], field: string) => {
  const res = new Map();
  return arr.filter((item) => !res.has(item[field]) && res.set(item[field], 1));
};

export const copy2Text = (text: string) => {
  if (navigator.clipboard) {
    navigator.clipboard.writeText(text);
  } else {
    const textarea = document.createElement('textarea');
    document.body.appendChild(textarea);
    textarea.style.position = 'fixed';
    textarea.style.clip = 'rect(0 0 0 0)';
    textarea.style.top = '10px';
    textarea.value = text;
    textarea.select();
    document.execCommand('copy', true);
    document.body.removeChild(textarea);
  }
  message.success('复制成功！');
};

export const switchUpload = async (uploadData: UploadRequestOption) => {
  if (OBJECT_STORAGE === 'minio') {
    return minioUpload(uploadData.file as File, minioUploadType.file);
  } else {
    await getCosAuthorization();
    return cosUpload(uploadData);
  }
};

export const formatDate = (timestamp: number): string => {
  const now = new Date(timestamp);
  const year = now.getFullYear();
  const month = now.getMonth() + 1;
  const date = now.getDate();
  const hour = now.getHours();
  let minute: any = now.getMinutes();
  if (minute.toString().length === 1) minute = '0' + minute;
  let second: any = now.getSeconds();
  if (second.toString().length === 1) second = '0' + second;
  const str1 = year + '年' + month + '月' + date + '日';
  const str2 = hour + ':' + minute;
  return `${str1} ${str2}`;
};

interface GetConversationIDParams {
  groupID?: string;
  sendID?: string;
  recvID?: string;
  isNotification?: boolean;
}
export const getConversationID = (params: GetConversationIDParams) => {
  if (params.isNotification) {
    return `sn_${params.sendID}_${params.recvID}`;
  }
  if (params.groupID) {
    return `sg_${params.groupID}`;
  }
  const array = [params.sendID, params.recvID].sort();
  return `si_${array[0]}_${array[1]}`;
};


export const bytesToSize = (bytes: number) => {
  if (bytes === 0) return '0 B';
  const k = 1024,
    sizes = ['B', 'KB', 'MB', 'GB', 'TB', 'PB', 'EB', 'ZB', 'YB'],
    i = Math.floor(Math.log(bytes) / Math.log(k));

  return `${(bytes / Math.pow(k, i)).toPrecision(3)} ${sizes[i]}`;
};

export const formatMessageTime = (timestemp: number, keepSameYear = false): string => {
  if (!timestemp) return '';

  const isRecent = dayjs().diff(timestemp, 'day') < 7;
  const keepYear = keepSameYear || !isThisYear(timestemp);

  if (!isRecent && !keepYear) {
    return dayjs(timestemp).format('M月D日 HH:mm');
  }

  return dayjs(timestemp).calendar();
};

export const formatBr = (str: string) => str.replace(/\n/g, '<br>');

const regex = /\b(http[s]?:\/\/[^\s]+)\b/g;
export const formatLink = (content: string) => {
  console.log(content);
  return content.replace(
    regex,
    (match) => `<a href="${match}" target="_blank" class="link-el">${match}</a>`,
  );
};

export const sec2Time = (seconds: number) => {
  let theTime1 = 0; // min
  let theTime2 = 0; // hour
  let theTime3 = 0; // day
  let mySeconds = seconds;
  if (mySeconds > 60) {
    theTime1 = parseInt(String(mySeconds / 60));
    mySeconds = parseInt(String(mySeconds % 60));
    if (theTime1 > 60) {
      theTime2 = parseInt(String(theTime1 / 60));
      theTime1 = parseInt(String(theTime1 % 60));
      if (theTime2 > 24) {
        theTime3 = parseInt(String(theTime2 / 24));
        theTime2 = parseInt(String(theTime2 % 24));
      }
    }
  }
  let result = '';
  if (mySeconds > 0) {
    result = '' + parseInt(String(mySeconds)) + '秒';
  }
  if (theTime1 > 0) {
    result = '' + parseInt(String(theTime1)) + '分钟' + result;
  }
  if (theTime2 > 0) {
    result = '' + parseInt(String(theTime2)) + '小时' + result;
  }
  if (theTime3 > 0) {
    result = '' + parseInt(String(theTime3)) + '天' + result;
  }
  return result;
};

export const parseMessageByType = (pmsg: MessageItem) => {
  const getName = (user: any) => {
    return user.nickname;
  };
  switch (pmsg.contentType) {
    case MessageType.TextMessage:
      return (pmsg.textElem?.content ?? '');
    case MessageType.AtTextMessage:
      return pmsg.atTextElem;
    case MessageType.PictureMessage:
      return "[图片]";
    case MessageType.VideoMessage:
      return "[视频]";
    case MessageType.VoiceMessage:
      return "[语音]";
    case MessageType.LocationMessage:
      return "[位置]";
    case MessageType.CardMessage:
      return "[名片]";
    case MessageType.MergeMessage:
      return "[聊天记录]";
    case MessageType.FileMessage:
      return `${(pmsg.fileElem?.fileName ?? '[文件]')}`;
    case MessageType.RevokeMessage:
      const data = JSON.parse((pmsg.notificationElem?.detail ?? '{}'));
      const revoker = data.revokerNickname;
      const sourcer = data.sourceMessageSenderNickname;
      const isAdminRevoke = data.revokerID !== data.sourceMessageSendID;
      if (isAdminRevoke) {
        return `${revoker}撤回了一条${sourcer}的消息`;
      }
      return `${revoker}撤回了一条消息`;
    case MessageType.CustomMessage:
      // const customEl = pmsg.customElem;
      // const customData = JSON.parse(customEl.data);
      // if (customData.customType) {
      //   return switchCustomMsg(customData);
      // }
      return "[自定义消息]";
    case MessageType.QuoteMessage:
      return "[引用消息]";
    case MessageType.FaceMessage:
      return "[表情]";
    case MessageType.FriendAdded:
      return "你们已经是好友了，开始聊天吧~";
    case MessageType.MemberEnter:
      const enterDetails = JSON.parse((pmsg.notificationElem?.detail ?? '{}'));
      const enterUser = enterDetails.entrantUser;
      return `${getName(enterUser)}进入了群聊`;
    case MessageType.GroupCreated:
      const groupCreatedDetail = JSON.parse((pmsg.notificationElem?.detail ?? '{}'));
      const groupCreatedUser = groupCreatedDetail.opUser;
      return `${getName(groupCreatedUser)}创建了群聊`;
    case MessageType.MemberInvited:
      const inviteDetails = JSON.parse((pmsg.notificationElem?.detail ?? '{}'));
      const inviteOpUser = inviteDetails.opUser;
      const invitedUserList = inviteDetails.invitedUserList ?? [];
      let inviteStr = "";
      invitedUserList.find(
        (user: any, idx: number) => (inviteStr += getName(user) + "、") && idx > 3,
      );
      inviteStr = inviteStr.slice(0, -1);
      return `${getName(inviteOpUser)}邀请了${inviteStr}${
        invitedUserList.length > 3 ? "..." : ""
      }进入群聊`;

    case MessageType.MemberKicked:
      const kickDetails = JSON.parse((pmsg.notificationElem?.detail ?? '{}'));
      const kickOpUser = kickDetails.opUser;
      const kickdUserList = kickDetails.kickedUserList ?? [];
      let kickStr = "";
      kickdUserList.find(
        (user: any, idx: number) => (kickStr += getName(user) + "、") && idx > 3,
      );
      kickStr = kickStr.slice(0, -1);
      return `${getName(kickOpUser)}踢出了${kickStr}${
        kickdUserList.length > 3 ? "..." : ""
      }`;
    case MessageType.MemberQuit:
      const quitDetails = JSON.parse((pmsg.notificationElem?.detail ?? '{}'));
      const quitUser = quitDetails.quitUser;
      return `${getName(quitUser)}退出了群聊`;
    case MessageType.GroupInfoUpdated:
      const groupUpdateDetail = JSON.parse((pmsg.notificationElem?.detail ?? '{}'));
      const groupUpdateUser = groupUpdateDetail.opUser;
      let updateFiled = "群设置";
      if (groupUpdateDetail.group.notification) {
        updateFiled = "群公告";
      }
      if (groupUpdateDetail.group.groupName) {
        updateFiled = `群名称为 ${groupUpdateDetail.group.groupName}`;
      }
      if (groupUpdateDetail.group.faceURL) {
        updateFiled = "群头像";
      }
      if (groupUpdateDetail.group.introduction) {
        updateFiled = "群介绍";
      }
      return `${getName(groupUpdateUser)}修改了${updateFiled}`;
    case MessageType.GroupOwnerTransferred:
      const transferDetails = JSON.parse((pmsg.notificationElem?.detail ?? '{}'));
      const transferOpUser = transferDetails.opUser;
      const newOwner = transferDetails.newGroupOwner;
      return `${getName(transferOpUser)}将群主转让给${getName(newOwner)}`;
    case MessageType.GroupDismissed:
      const dismissDetails = JSON.parse((pmsg.notificationElem?.detail ?? '{}'));
      const dismissUser = dismissDetails.opUser;
      return `${getName(dismissUser)}解散了群聊`;
    case MessageType.GroupMuted:
      const GROUPMUTEDDetails = JSON.parse((pmsg.notificationElem?.detail ?? '{}'));
      const groupMuteOpUser = GROUPMUTEDDetails.opUser;
      return `${getName(groupMuteOpUser)}开启了全体禁言`;
    case MessageType.GroupCancelMuted:
      const GROUPCANCELMUTEDDetails = JSON.parse((pmsg.notificationElem?.detail ?? '{}'));
      const groupCancelMuteOpUser = GROUPCANCELMUTEDDetails.opUser;
      return `${getName(groupCancelMuteOpUser)}取消了全体禁言`;
    case MessageType.GroupMemberMuted:
      const gmMutedDetails = JSON.parse((pmsg.notificationElem?.detail ?? '{}'));
      const muteTime = sec2Time(gmMutedDetails.mutedSeconds);
      return `${getName(gmMutedDetails.opUser)}禁言了${getName(
        gmMutedDetails.mutedUser,
      )} ${muteTime}`;
    case MessageType.GroupMemberCancelMuted:
      const gmcMutedDetails = JSON.parse((pmsg.notificationElem?.detail ?? '{}'));
      return `${getName(gmcMutedDetails.opUser)}取消了禁言${getName(
        gmcMutedDetails.mutedUser,
      )}`;
    case MessageType.GroupAnnouncementUpdated:
      const groupAnnouncementUpdateDetail = JSON.parse(
        (pmsg.notificationElem?.detail ?? '{}'),
      );
      const groupAnnouncementUpdateUser = groupAnnouncementUpdateDetail.opUser;
      return `${getName(groupAnnouncementUpdateUser)}修改了群公告`;
    case MessageType.GroupNameUpdated:
      const groupNameUpdateDetail = JSON.parse((pmsg.notificationElem?.detail ?? '{}'));
      const groupNameUpdateUser = groupNameUpdateDetail.opUser;
      return `${getName(groupNameUpdateUser)}修改了群名称为${
        groupNameUpdateDetail.group.groupName
      }`;
    case MessageType.OANotification:
      const customNoti = JSON.parse((pmsg.notificationElem?.detail ?? '{}'));
      return customNoti.text;
    case MessageType.BurnMessageChange:
      const burnDetails = JSON.parse((pmsg.notificationElem?.detail ?? '{}'));
      return `阅后即焚已${burnDetails.isPrivate ? "开启" : "关闭"}`;
    default:
      return "";
  }
};

export const tipMessaggeFormat = (msg: MessageItem) => {
  if (msg.contentType === MessageType.RevokeMessage) {
    const data = JSON.parse((msg.notificationElem?.detail ?? '{}'));
    const isAdminRevoke = data.revokerID !== data.sourceMessageSendID;
    if (isAdminRevoke) {
      return `${data.revokerNickname}撤回了一条${data.sourceMessageSenderNickname}的消息`;
    }
    return `${data.revokerNickname}撤回了一条消息`;
  }

  switch (msg.contentType) {
    case MessageType.FriendAdded:
      return `你们已经是好友了~`;
    case MessageType.GroupCreated:
      console.log(JSON.parse((msg.notificationElem?.detail ?? '{}')));
      const groupCreatedDetail = JSON.parse((msg.notificationElem?.detail ?? '{}'));
      const groupCreatedUser = groupCreatedDetail.opUser;
      return `${groupCreatedUser.nickname}创建了群聊`;
    case MessageType.GroupInfoUpdated:
      const groupUpdateDetail = JSON.parse((msg.notificationElem?.detail ?? '{}'));
      const groupUpdateUser = groupUpdateDetail.opUser;
      let updateFiled = '群设置';
      if (groupUpdateDetail.group.notification) {
        updateFiled = '群公告';
      }
      if (groupUpdateDetail.group.groupName) {
        updateFiled = `群名称为 ${groupUpdateDetail.group.groupName}`;
      }
      if (groupUpdateDetail.group.faceURL) {
        updateFiled = '群头像';
      }
      if (groupUpdateDetail.group.introduction) {
        updateFiled = '群介绍';
      }
      return `${groupUpdateUser.nickname}修改了${updateFiled}`;
    case MessageType.GroupOwnerTransferred:
      const transferDetails = JSON.parse((msg.notificationElem?.detail ?? '{}'));
      const transferOpUser = transferDetails.opUser;
      const newOwner = transferDetails.newGroupOwner;
      return `${transferOpUser.nickname}转让群主给${newOwner.nickname}`;
    case MessageType.MemberQuit:
      const quitDetails = JSON.parse((msg.notificationElem?.detail ?? '{}'));
      const quitUser = quitDetails.quitUser;
      return `${quitUser.nickname}退出了群组`;
    case MessageType.MemberInvited:
      const inviteDetails = JSON.parse((msg.notificationElem?.detail ?? '{}'));
      const inviteOpUser = inviteDetails.opUser;
      const invitedUserList = inviteDetails.invitedUserList ?? [];
      let inviteStr = '';
      invitedUserList.find(
        (user: any, idx: number) => (inviteStr += user.nickname + '、') && idx > 3,
      );
      inviteStr = inviteStr.slice(0, -1);
      return `${inviteOpUser.nickname} 邀请了${inviteStr}${
        invitedUserList.length > 3 ? '...' : ''
      }加入群聊`;
    case MessageType.MemberKicked:
      const kickDetails = JSON.parse((msg.notificationElem?.detail ?? '{}'));
      const kickOpUser = kickDetails.opUser;
      const kickdUserList = kickDetails.kickedUserList ?? [];
      let kickStr = '';
      kickdUserList.find((user: any, idx: number) => (kickStr += user.nickname + '、') && idx > 3);
      kickStr = kickStr.slice(0, -1);
      return `${kickOpUser.nickname} 踢出了${kickStr}${kickdUserList.length > 3 ? '...' : ''}`;
    case MessageType.MemberEnter:
      const enterDetails = JSON.parse((msg.notificationElem?.detail ?? '{}'));
      const enterUser = enterDetails.entrantUser;
      return `${enterUser.nickname}加入了群聊`;
    case MessageType.GroupDismissed:
      const dismissDetails = JSON.parse((msg.notificationElem?.detail ?? '{}'));
      const dismissUser = dismissDetails.opUser;
      return `${dismissUser.nickname}解散了群聊`;
    case MessageType.GroupMuted:
      const groupMutedDetails = JSON.parse((msg.notificationElem?.detail ?? '{}'));
      const groupMuteOpUser = groupMutedDetails.opUser;
      return `${groupMuteOpUser.nickname}开启了全体禁言`;
    case MessageType.GroupCancelMuted:
      const groupCancelMutedDetails = JSON.parse((msg.notificationElem?.detail ?? '{}'));
      const groupCancelMuteOpUser = groupCancelMutedDetails.opUser;
      return `${groupCancelMuteOpUser.nickname}关闭了全体禁言`;
    case MessageType.GroupMemberMuted:
      const gmMutedDetails = JSON.parse((msg.notificationElem?.detail ?? '{}'));
      const muteTime = sec2Time(gmMutedDetails.mutedSeconds);
      return `${gmMutedDetails.opUser.nickname}禁言了${gmMutedDetails.mutedUser.nickname} ${muteTime}`;
    case MessageType.GroupMemberCancelMuted:
      const gmcMutedDetails = JSON.parse((msg.notificationElem?.detail ?? '{}'));
      return `${gmcMutedDetails.opUser.nickname}取消了禁言${gmcMutedDetails.mutedUser.nickname}`;
    case MessageType.GroupNameUpdated:
      const groupNameUpdateDetail = JSON.parse((msg.notificationElem?.detail ?? '{}'));
      const groupNameUpdateUser = groupNameUpdateDetail.opUser;
      return `${groupNameUpdateUser.nickname}修改了群名称为${groupNameUpdateDetail.group.groupName}`;
    case MessageType.BurnMessageChange:
      const burnDetails = JSON.parse((msg.notificationElem?.detail ?? '{}'));
      return `阅后即焚已${burnDetails.isPrivate ? '开启' : '关闭'}`;
    case MessageType.OANotification:
      const customNoti = JSON.parse((msg.notificationElem?.detail ?? '{}'));
      return customNoti.text;
    default:
      return '';
  }
};
