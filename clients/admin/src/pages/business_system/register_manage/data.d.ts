import type { InvitationCodeStatus } from '@/constants/enum';

export interface InvitationCodeItem {
  createTime: string;
  invitationCode: string;
  lastTime: string;
  status: InvitationCodeStatus;
  userID: string;
}

export interface DefualtFriendItem {
  user: {
    nickname: string;
  };
  userID: string;
}
export interface DefualtGroupItem {
  groupID: string;
  groupName: any;
  createTime?: number;
}

export interface AssignIPItem {
  userID: string;
  ip: string;
}

export interface BrowseIPItem {
  createTime: string;
  ip: string;
  limitLogin: number;
  limitRegister: number;
}
