import { ContentType } from './enum';

export const MessageTypeOtions = [
  {
    label: '文字消息',
    value: ContentType.TextMessage,
  },
  {
    label: '图片消息',
    value: ContentType.PicMessage,
  },
  {
    label: '语音消息',
    value: ContentType.VoiceMessage,
  },
  {
    label: '视频消息',
    value: ContentType.VideoMessage,
  },
  {
    label: '文件消息',
    value: ContentType.FileMessage,
  },
  {
    label: '@消息',
    value: ContentType.TextAtMessage,
  },
  {
    label: '合并消息',
    value: ContentType.MergeMessage,
  },
  {
    label: '名片消息',
    value: ContentType.CardMessage,
  },
  {
    label: '位置消息',
    value: ContentType.LocationMessage,
  },
  {
    label: '自定义消息',
    value: ContentType.CustomMessage,
  },
  {
    label: '引用消息',
    value: ContentType.QuoteMessage,
  },
  {
    label: '表情消息',
    value: ContentType.FaceMessage,
  },
  {
    label: '同意加好友申请通知',
    value: ContentType.FriendAppApproved,
  },
  {
    label: '群创建',
    value: ContentType.GroupCreated,
  },
  {
    label: '群成员退出通知',
    value: ContentType.MemberQuit,
  },
  {
    label: '群主更换通知',
    value: ContentType.GroupOwnerTransferred,
  },
  {
    label: '群成员被踢通知',
    value: ContentType.MemberKicked,
  },
  {
    label: '邀请好友',
    value: ContentType.MemberInvited,
  },
  {
    label: '群成员进群通知',
    value: ContentType.MemberEnter,
  },
  {
    label: '群解散',
    value: ContentType.DismissGroup,
  },
  {
    label: '群成员禁言通知',
    value: ContentType.MemberMutedNotification,
  },
  {
    label: '取消群成员禁言通知',
    value: ContentType.MemberCancelMutedNotification,
  },
  {
    label: '群禁言通知',
    value: ContentType.MutedNotification,
  },
  {
    label: '取消群禁言通知',
    value: ContentType.CancelMutedNotification,
  },
  {
    label: '群公告修改',
    value: ContentType.GroupAnnouncementUpdated,
  },
  {
    label: '群名称修改',
    value: ContentType.GroupNameUpdated,
  },
  {
    label: '私有会话',
    value: ContentType.PrivateMessage,
  },
  {
    label: '撤回消息',
    value: ContentType.MsgRevokeNotification,
  },
];
