import { ContentType } from '@/constants/enum';
import { useModel } from '@umijs/max';
import { Image, Typography } from 'antd';

import call_video from '@/assets/images/call_video.png';

type ContentParseProps = {
  record: any;
};

const ContentParse = ({ record }: ContentParseProps) => {
  const { setInitialState } = useModel('@@initialState');

  const showVideo = (url: string) => {
    setInitialState((s) => ({
      ...s,
      videoUrl: url,
    }));
  };

  // 101
  if (record.contentType === ContentType.TextMessage) {
    return (
      <Typography.Text
        ellipsis={{ tooltip: JSON.parse(record.content).content }}
        style={{ width: '200px' }}
      >
        {JSON.parse(record.content).content}
      </Typography.Text>
    );
  }
  // 102
  if (record.contentType === ContentType.PicMessage) {
    return (
      <Image
        width={100}
        src={
          JSON.parse(record.content).snapshotPicture.url ??
          JSON.parse(record.content).sourcePicture.url
        }
      />
    );
  }
  // 103
  if (record.contentType === ContentType.VoiceMessage) {
    return (
      <Typography.Link
        href={JSON.parse(record.content).sourceUrl}
        target="_blank"
        style={{ width: '200px' }}
      >
        【音频下载】
      </Typography.Link>
    );
  }
  // 104
  if (record.contentType === ContentType.VideoMessage) {
    return (
      <div
        className="video-img cursor-pointer"
        onClick={() => showVideo(JSON.parse(record.content).videoUrl)}
      >
        <img src={JSON.parse(record.content).snapshotUrl} alt="video" width={100} />
        <div className="video-image-mask">
          <img src={call_video} alt="" width={40} />
        </div>
      </div>
    );
  }
  // 105
  if (record.contentType === ContentType.FileMessage) {
    return (
      <Typography.Link
        href={JSON.parse(record.content).sourceUrl}
        target="_blank"
        style={{ width: '200px' }}
      >
        【文件下载】
      </Typography.Link>
    );
  }
  // 106
  if (record.contentType === ContentType.TextAtMessage) {
    return (
      <Typography.Text
        ellipsis={{ tooltip: JSON.parse(record.content).text }}
        style={{ width: '200px' }}
      >
        {JSON.parse(record.content).text}
      </Typography.Text>
    );
  }
  // 107
  if (record.contentType === ContentType.MergeMessage) {
    return <span>合并消息</span>;
  }
  // 108
  if (record.contentType === ContentType.CardMessage) {
    return (
      <Typography.Text
        ellipsis={{
          tooltip: `${JSON.parse(record.content).nickname}的名片`,
        }}
        style={{ width: '200px' }}
      >
        {JSON.parse(record.content).nickname}的名片
      </Typography.Text>
    );
  }
  // 109
  if (record.contentType === ContentType.LocationMessage) {
    return (
      <Typography.Text
        ellipsis={{ tooltip: JSON.parse(JSON.parse(record.content).description).addr }}
        style={{ width: '200px' }}
      >
        {JSON.parse(JSON.parse(record.content).description).addr}
      </Typography.Text>
    );
  }
  // 110
  if (record.contentType === ContentType.CustomMessage) {
    const customType = JSON.parse(JSON.parse(record.content).data).customType;
    if (customType === 905) {
      return <span>【视频会议】</span>;
    }
    return <span>【自定义消息】</span>;
  }
  // 114
  if (record.contentType === ContentType.QuoteMessage) {
    return (
      <Typography.Text
        ellipsis={{ tooltip: JSON.parse(record.content).text }}
        style={{ width: '200px' }}
      >
        {JSON.parse(record.content).text}{' '}
      </Typography.Text>
    );
  } // 115
  if (record.contentType === ContentType.FaceMessage) {
    return <Image width={100} src={JSON.parse(JSON.parse(record.content).data).url} />;
  }
  // 1201
  if (record.contentType === ContentType.FriendAppApproved) {
    return <span>同意</span>;
  }
  // 1501
  if (record.contentType === ContentType.GroupCreated) {
    return (
      <Typography.Text
        ellipsis={{
          tooltip: `${
            JSON.parse(JSON.parse(record.content).detail).groupOwnerUser.userID + ' '
          }创建了群`,
        }}
        style={{ width: '200px' }}
      >
        {JSON.parse(JSON.parse(record.content).detail).groupOwnerUser.userID + ' '}创建了群
      </Typography.Text>
    );
  }
  // 1504
  if (record.contentType === ContentType.MemberQuit) {
    return (
      <Typography.Text
        ellipsis={{
          tooltip: `${
            JSON.parse(JSON.parse(record.content).detail).quitUser.nickname + ' '
          }退出群聊通知`,
        }}
        style={{ width: '200px' }}
      >
        {JSON.parse(JSON.parse(record.content).detail).quitUser.nickname + ' '}退出群聊通知
      </Typography.Text>
    );
  }
  // 1507
  if (record.contentType === ContentType.GroupOwnerTransferred) {
    const opUser = JSON.parse(JSON.parse(record.content).detail).opUser.nickname;
    const newOwner = JSON.parse(JSON.parse(record.content).detail).newGroupOwner.nickname;
    return (
      <Typography.Text
        ellipsis={{ tooltip: `${opUser + ' '}将群转让给了${' ' + newOwner}` }}
        style={{ width: '200px' }}
      >
        {opUser + ' '}将群转让给了{' ' + newOwner}
      </Typography.Text>
    );
  }
  // 1508
  if (record.contentType === ContentType.MemberKicked) {
    const opUser = JSON.parse(JSON.parse(record.content).detail).opUser.nickname;
    const listStr = JSON.parse(JSON.parse(record.content).detail)
      .kickedUserList.map((e: any) => e.nickname)
      .join(',');
    return (
      <Typography.Text
        ellipsis={{ tooltip: `${listStr + ' '}被${' ' + opUser}踢出群聊` }}
        style={{ width: '200px' }}
      >
        {listStr + ' '}被{' ' + opUser}踢出群聊
      </Typography.Text>
    );
  }
  // 1509
  if (record.contentType === ContentType.MemberInvited) {
    const opUser = JSON.parse(JSON.parse(record.content).detail).opUser.nickname;
    const listStr = JSON.parse(JSON.parse(record.content).detail)
      .invitedUserList.map((e: any) => e.nickname)
      .join(',');
    return (
      <Typography.Text
        ellipsis={{ tooltip: `${opUser + ' '}邀请${'' + listStr}加入群聊` }}
        style={{ width: '200px' }}
      >
        {opUser + ' '}邀请{'' + listStr}加入群聊
      </Typography.Text>
    );
  }
  // 1510
  if (record.contentType === ContentType.MemberEnter) {
    return (
      <Typography.Text
        ellipsis={{
          tooltip: `${
            JSON.parse(JSON.parse(record.content).detail).entrantUser.nickname + ' '
          }加入了群`,
        }}
        style={{ width: '200px' }}
      >
        {JSON.parse(JSON.parse(record.content).detail).entrantUser.nickname + ' '}加入了群
      </Typography.Text>
    );
  }
  // 1511
  if (record.contentType === ContentType.DismissGroup) {
    const opUser = JSON.parse(JSON.parse(record.content).detail).opUser.nickname;
    return (
      <Typography.Text
        ellipsis={{ tooltip: `${opUser + ' '}解散了群聊` }}
        style={{ width: '200px' }}
      >
        {opUser + ' '}解散了群聊
      </Typography.Text>
    );
  }
  // 1512
  if (record.contentType === ContentType.MemberMutedNotification) {
    const opUser = JSON.parse(JSON.parse(record.content).detail).opUser.nickname;
    const mutedUser = JSON.parse(JSON.parse(record.content).detail).mutedUser.nickname;
    const mutedSeconds = JSON.parse(JSON.parse(record.content).detail).mutedSeconds;
    return (
      <Typography.Text
        ellipsis={{ tooltip: `${mutedUser + ' '}被${' ' + opUser + ' '}禁言${mutedSeconds}秒` }}
        style={{ width: '200px' }}
      >
        {mutedUser + ' '}被{' ' + opUser + ' '}禁言{mutedSeconds}秒
      </Typography.Text>
    );
  }
  // 1513
  if (record.contentType === ContentType.MemberCancelMutedNotification) {
    const opUser = JSON.parse(JSON.parse(record.content).detail).opUser.nickname;
    const mutedUser = JSON.parse(JSON.parse(record.content).detail).mutedUser.nickname;
    return (
      <Typography.Text
        ellipsis={{ tooltip: `${opUser + ' '}取消了${' ' + mutedUser + ' '}的禁言` }}
        style={{ width: '200px' }}
      >
        {opUser + ' '}取消了{' ' + mutedUser + ' '}的禁言
      </Typography.Text>
    );
  }
  // 1514
  if (record.contentType === ContentType.MutedNotification) {
    return (
      <Typography.Text
        ellipsis={{
          tooltip: `${
            JSON.parse(JSON.parse(record.content).detail).opUser.nickname + ' '
          }开起了群禁言`,
        }}
        style={{ width: '200px' }}
      >
        {JSON.parse(JSON.parse(record.content).detail).opUser.nickname + ' '}开起了群禁言
      </Typography.Text>
    );
  }
  // 1515
  if (record.contentType === ContentType.CancelMutedNotification) {
    return (
      <Typography.Text
        ellipsis={{
          tooltip: `${
            JSON.parse(JSON.parse(record.content).detail).opUser.nickname + ' '
          }关闭了群禁言`,
        }}
        style={{ width: '200px' }}
      >
        {JSON.parse(JSON.parse(record.content).detail).opUser.nickname + ' '}关闭了群禁言
      </Typography.Text>
    );
  }
  // 1519
  if (record.contentType === ContentType.GroupAnnouncementUpdated) {
    return (
      <Typography.Text
        ellipsis={{
          tooltip: `${
            JSON.parse(JSON.parse(record.content).detail).opUser.nickname + ' '
          }修改了群公告`,
        }}
        style={{ width: '200px' }}
      >
        {JSON.parse(JSON.parse(record.content).detail).opUser.nickname + ' '}修改了群公告
      </Typography.Text>
    );
  }
  // 1520
  if (record.contentType === ContentType.GroupNameUpdated) {
    return (
      <Typography.Text
        ellipsis={{
          tooltip: `${
            JSON.parse(JSON.parse(record.content).detail).opUser.nickname + ' '
          }修改了群名称`,
        }}
        style={{ width: '200px' }}
      >
        {JSON.parse(JSON.parse(record.content).detail).opUser.nickname + ' '}修改了群名称
      </Typography.Text>
    );
  }
  // 1701
  if (record.contentType === ContentType.PrivateMessage) {
    return (
      <Typography.Text
        ellipsis={{
          tooltip: `${
            JSON.parse(JSON.parse(record.content).detail).isPrivate ? '私有消息' : '非私有消息'
          }`,
        }}
        style={{ width: '200px' }}
      >
        {JSON.parse(JSON.parse(record.content).detail).isPrivate ? '私有消息' : '非私有消息'}
      </Typography.Text>
    );
  }
  // 2101
  if (record.contentType === ContentType.MsgRevokeNotification) {
    return (
      <Typography.Text
        ellipsis={{
          tooltip: `${
            JSON.parse(JSON.parse(record.content).detail).revokerNickname + ' '
          }撤回了一条消息`,
        }}
        style={{ width: '200px' }}
      >
        {JSON.parse(JSON.parse(record.content).detail).revokerNickname + ' '}撤回了一条消息
      </Typography.Text>
    );
  }
  return <Typography.Text style={{ width: '200px' }}>{record.content}</Typography.Text>;
};

export default ContentParse;
