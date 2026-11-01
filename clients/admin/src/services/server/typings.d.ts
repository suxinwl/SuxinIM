declare namespace API {
  type CurrentUser = {
    [key: string]: any;
  };
  declare namespace UserManage {
    type GetUserParams = {
      keyword?: string;
      pagination: {
        pageNumber: number;
        showNumber: number;
      };
    };

    type GetFriendsParams = {
      userID?: string;
      pagination: {
        pageNumber: number;
        showNumber: number;
      };
    };

    type DeleteFriendParams = {
      ownerUserID: string;
      friendUserID: string;
    };

    type UserInfoItem = {
      birth?: number;
      email?: string;
      ex?: string;
      faceURL?: string;
      gender?: number;
      nickname?: string;
      phoneNumber?: string;
      userID: string;
      level?: number;
    };

    type RegisterUserParams = {
      deviceID?: string;
      user: {
        phoneNumber?: string;
        areaCode?: string;
        email?: string;
        account?: string;
        password: string;
        faceURL?: string;
        nickname: string;
        birth?: number;
        gender?: number;
      };
      platform?: number;
      invitationCode?: string;
    };

    type ResetUserPasswordParams = {
      userID: string;
      newPassword: string;
    };

    type BlockUserParams = {
      endDisableTime: string;
      userID: string;
    };
    type KickUserParams = {
      userID: string;
      platformID: number;
    };
    type UpdateAdminInfoParams = {
      nickname?: string;
      faceURL?: string;
      password?: string;
      userID: string;
    };
  }
  declare namespace GroupManage {
    type GetGroupParams = {
      groupName?: string;
      groupID?: string;
      pagination: {
        pageNumber: number;
        showNumber: number;
      };
    };
    type CreateGroupParams = {
      groupName: string;
      groupType: number;
      memberList: {
        roleLevel: number;
        userID: string;
      }[];
      ownerUserID: string;
      introduction?: string;
      notification?: string;
      faceURL?: string;
      ex?: string;
    };
    type UpdateGroupParams = {
      applyMemberFriend: number;
      ex?: string;
      faceURL: string;
      groupID: string;
      groupName: string;
      introduction: string;
      lookMemberInfo: number;
      needVerification: number;
      notification: string;
      groupInfoForSet: {
        groupID: string;
        groupName?: string;
        notification?: string;
        introduction?: string;
        faceURL?: string;
        ex?: string;
        needVerification?: 1;
        lookMemberInfo?: 1;
        applyMemberFriend?: 1;
      };
    };
    type GetGroupMembersParams = {
      userName?: string;
      groupID: string;
      pagination: {
        pageNumber: number;
        showNumber: number;
      };
    };
    type GetSomeGroupMembersParams = {
      groupID: string;
      userIDs: string[];
    };
    type MuteGroupMemebrParams = {
      mutedSeconds: number;
      userID: string;
      groupID: string;
    };
    type KickGroupMemebrParams = {
      groupID: string;
      kickedUserIDs: string[];
      reason: '';
    };
    type InviteGroupMemebrParams = {
      groupID: string;
      invitedUserIDs: string[];
      reason: '';
    };
    type UpdateGroupMemebrParams = {
      members: [
        {
          ex?: string;
          groupID: string;
          nickname?: string;
          roleLevel?: number;
          userGroupFaceUrl?: string;
          userID: string;
        },
      ];
    };
    type TransferGroupParams = {
      newOwnerUserID: string;
      oldOwnerUserID: string;
      groupID: string;
    };
  }
  declare namespace ChatLog {
    type GetChatLogParams = {
      sendID?: string;
      recvID?: string;
      msgType: number;
      sendTime?: string;
      sessionType: number;
      pagination: {
        pageNumber: number;
        showNumber: number;
      };
    };
    type GetSignalrecords = {
      operationID?: string;
      showNumber?: number;
      pageNumber?: number;
      mediaType?: string;
      senderID?: string;
      recvID?: string;
      createTime?: number;
    };
    type RevokeMessageParams = {
      conversationID: string;
      seq: number;
      userID: string;
    };
    type OfflinePushInfo = {
      desc: string;
      ex: string;
      iOSBadgeCount: boolean;
      iOSPushSound: string;
      title: string;
    };
    type BatchSendParams = {
      recvIDList: string[];
      sendID: string;
      text: string;
      pictureElem?: {
        sourcePicture: PicBaseInfo;
        bigPicture: PicBaseInfo;
        snapshotPicture: PicBaseInfo;
      };
      isSendAll: boolean;
    };
    type PicBaseInfo = {
      uuid: string;
      type: string;
      size: number;
      width: number;
      height: number;
      url: string;
    };
  }
  declare namespace AppManage {
    type ClintConfig = {
      adminURL?: string;
      discoverPageURL?: string;
      addFriendCtrl?: number;
      allowSendMsgNotFriend?: number;
      bossUserID?: string;
      needInvitationCodeRegister?: number;
      ordinaryUserAddFriend?: number;
    };
  }
  declare namespace RtcManage {
    type VideoParams = {
      pagination: {
        pageNumber: number;
        showNumber: number;
      };
      sendID: string;
      sesstionType: number;
      startTime: number;
      endTime: number;
    };
    type DeleteVideoParams = {
      sIDs: string[];
    };
    type MeetingParams = {
      pagination: {
        pageNumber: number;
        showNumber: number;
      };
      hostUserID: string;
      startTime: number;
      endTime: number;
    };
    type DeleteMeetingParams = {
      roomIDs: string[];
    };
  }
}
