export enum SessionType {
  Single = 1,
  Group = 2,
  SuperGroup = 3,
  Notification = 4,
}

export enum ContentType {
  TextMessage = 101,
  PicMessage = 102,
  VoiceMessage = 103,
  VideoMessage = 104,
  FileMessage = 105,
  TextAtMessage = 106,
  MergeMessage = 107,
  CardMessage = 108,
  LocationMessage = 109,
  CustomMessage = 110,
  QuoteMessage = 114,
  FaceMessage = 115,
  FriendAppApproved = 1201,
  GroupCreated = 1501,
  MemberQuit = 1504,
  GroupOwnerTransferred = 1507,
  MemberKicked = 1508,
  MemberInvited = 1509,
  MemberEnter = 1510,
  DismissGroup = 1511,
  MemberMutedNotification = 1512,
  MemberCancelMutedNotification = 1513,
  MutedNotification = 1514,
  CancelMutedNotification = 1515,
  GroupAnnouncementUpdated = 1519,
  GroupNameUpdated = 1520,
  PrivateMessage = 1701,
  MsgRevokeNotification = 2101,
}

export enum InvitationCodeStatus {
  All = 1,
  UnUsed = 2,
  Used = 3,
}

export enum GroupVerificationType {
  ApplyNeedInviteNot,
  AllNeed,
  AllNot,
}

export enum AllowType {
  Allowed,
  NotAllowed,
}

export enum GroupStatus {
  Nomal = 0,
  Baned = 1,
  Dismissed = 2,
  Muted = 3,
}

// export enum GroupRole {
//     Nomal = 1,
//     Owner = 2,
//     Admin = 3,
// }

export enum GroupRole {
  Nomal = 20,
  Owner = 100,
  Admin = 60,
}

export enum GroupJoinSource {
  Invitation = 2,
  Search = 3,
  QrCode = 4,
}
