export interface UserListItem {
  appMangerLevel: number;
  createTime: number;
  createIp: string;
  email: string;
  faceURL: string;
  gender: number;
  birth: number;
  forbidden: number;
  LastLoginIp: string;
  LastLoginTime: string;
  // login_limit: number;
  // login_times: number;
  nickname: string;
  phoneNumber: string;
  userID: string;
  index?: number;
  onlineStr?: string;
  avatar?: string;
  inGroup?: boolean;
}

export interface BlockListItem {
  beginDisableTime: string;
  endDisableTime: string;
  disableTimeRange: string[];
  email: string;
  faceURL: string;
  gender: number;
  birth: number;
  isBlock: boolean;
  // last_login_ip: string;
  // last_login_time: string;
  // login_limit: number;
  // login_times: number;
  nickname: string;
  phoneNumber: string;
  userID: string;
  index?: number;
  avatar?: string;
}

export interface FriendListItem {
  addSource: number;
  createTime: number;
  operatorUserID: string;
  ownerUserID: string;
  remark: string;
  friendUser: {
    nickname: string;
    userID: string;
    index?: number;
    avatar?: string;
    faceURL: string;
    onlineStr?: string;
  };
}
