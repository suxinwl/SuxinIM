import { MessageType } from '@openim/wasm-client-sdk';

export const nomalMessageTypes = [
  MessageType.TextMessage,
  MessageType.AtTextMessage,
  MessageType.PictureMessage,
  MessageType.VideoMessage,
  MessageType.VoiceMessage,
  MessageType.FileMessage,
  MessageType.MergeMessage,
  MessageType.CardMessage,
  MessageType.LocationMessage,
  MessageType.CustomMessage,
  MessageType.RevokeMessage,
  MessageType.QuoteMessage,
  MessageType.FaceMessage,
];

export const tipTypes = [
  MessageType.FriendAdded,
  MessageType.GroupCreated,
  MessageType.GroupInfoUpdated,
  MessageType.GroupOwnerTransferred,
  MessageType.MemberQuit,
  MessageType.MemberInvited,
  MessageType.MemberKicked,
  MessageType.MemberEnter,
  MessageType.GroupDismissed,
  MessageType.GroupMuted,
  MessageType.GroupCancelMuted,
  MessageType.GroupMemberMuted,
  MessageType.GroupMemberCancelMuted,
  MessageType.GroupNameUpdated,
  MessageType.BurnMessageChange,
  MessageType.OANotification,
];

export const canSearchMessageTypes = [...nomalMessageTypes, ...tipTypes];
