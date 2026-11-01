export interface UserMessageItem {
  sendID: string;
  recvID: string;
  clientMsgID: string;
  serverMsgID: string;
  senderPlatformID: number;
  senderNickname: string;
  senderFaceURL: string;
  sessionType: number;
  msgFrom: number;
  contentType: number;
  content: string;
  sendTime: number;
  createTime: number;
  status: number;
  seq: number;
}

export interface GroupListItem {
  sendID: string;
  recvID: string;
  contentType: number;
  content: string;
  GroupOwnerID: string;
  GroupOwnerName: string;
  applyMemberFriend: number;
  createTime: number;
  creatorUserID: string;
  ex: string;
  faceURL: string;
  groupID: string;
  groupName: string;
  groupType: number;
  introduction: string;
  lookMemberInfo: number;
  memberCount: number;
  needVerification: number;
  notification: string;
  notificationUpdateTime: number;
  notificationUserID: string;
  ownerUserID: string;
  status: number;
  seq: number;
}

export interface GroupMemebrItem {
  appMangerLevel: number;
  ex: string;
  faceURL: string;
  groupID: string;
  inviterUserID: string;
  joinSource: number;
  joinTime: number;
  muteEndTime: number;
  nickname: string;
  operatorUserID: string;
  roleLevel: number;
  userID: string;
  onlineStr?: string;
  avatar?: string;
  index?: number;
}
