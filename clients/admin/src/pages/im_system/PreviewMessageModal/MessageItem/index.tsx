import clsx from 'clsx';
import { FC, memo, useRef, useState } from 'react';

import OIMAvatar from '@/components/OIMAvatar';
import { formatMessageTime } from '@/utils/common';

import AnnouncementRenderer from './AnnouncementRender';
import CardMessageRenderer from './CardMessageRenderer';
import CatchMessageRender from './CatchMsgRenderer';
import CustomMessageSwitcher from './CustomMessageSwitcher';
import FaceMessageRender from './FaceMessageRender';
import FileMessageRenderer from './FileMessageRenderer';
import LocationMessageRenderer from './LocationMessageRenderer';
import MediaMessageRender from './MediaMessageRender';
import MergeMessageRenderer from './MergeMessageRenderer';
import styles from './message-item.module.less';
import MessageItemErrorBoundary from './MessageItemErrorBoundary';
import QuoteMessageRenderer from './QuoteMessageRenderer';
import TextMessageRender from './TextMessageRender';
import VoiceMessageRender from './VoiceMessageRender';
import { ExMessageItem } from '@/types';
import { GroupMemberItem, MergeElem } from '@openim/wasm-client-sdk';
import { MessageType } from '@openim/wasm-client-sdk';

export interface IMessageItemProps {
  message: ExMessageItem;
  isSender: boolean;
  disabled?: boolean;
  conversationID?: string;
  messageUpdateFlag?: string;
  showAlbum?: (clientMsgID: string) => void;
  showMergeModal?: (data: MergeElem) => void;
  getAnnouncementPusher?: (userID: string) => void;
}

const components: Record<number, FC<IMessageItemProps>> = {
  [MessageType.TextMessage]: TextMessageRender,
  [MessageType.AtTextMessage]: TextMessageRender,
  [MessageType.QuoteMessage]: TextMessageRender,
  [MessageType.VoiceMessage]: VoiceMessageRender,
  [MessageType.PictureMessage]: MediaMessageRender,
  [MessageType.VideoMessage]: MediaMessageRender,
  [MessageType.FaceMessage]: FaceMessageRender,
  [MessageType.CardMessage]: CardMessageRenderer,
  [MessageType.FileMessage]: FileMessageRenderer,
  [MessageType.CustomMessage]: CustomMessageSwitcher,
  [MessageType.LocationMessage]: LocationMessageRenderer,
  [MessageType.MergeMessage]: MergeMessageRenderer,
  [MessageType.GroupAnnouncementUpdated]: AnnouncementRenderer,
};

const MessageItem: FC<IMessageItemProps> = ({
  message,
  isSender,
  showAlbum,
  showMergeModal,
}) => {
  const messageWrapRef = useRef<HTMLDivElement>(null);
  const [announcementPusher] = useState<GroupMemberItem>();
  const MessageRenderComponent = components[message.contentType] || CatchMessageRender;

  const isQuoteMessage = message.contentType === MessageType.QuoteMessage;

  return (
    <div
      className={clsx(
        'relative flex select-text px-5 py-3',
        message.gapTime && '!pt-9',
        message.errCode && '!pb-6',
      )}
    >
      <div
        className={clsx(
          styles['message-container'],
          isSender && styles['message-container-sender'],
        )}
      >
        <OIMAvatar
          size={36}
          src={announcementPusher?.faceURL ?? message.senderFaceUrl}
          text={announcementPusher?.nickname ?? message.senderNickname}
        />
        <div className={styles['message-wrap']} ref={messageWrapRef}>
          <div className={styles['message-profile']}>
            <div
              title={message.senderNickname}
              className={clsx(
                'max-w-[30%] truncate text-[var(--sub-text)]',
                isSender ? 'ml-2' : 'mr-2',
              )}
            >
              {announcementPusher?.nickname ?? message.senderNickname}
            </div>
            {/* <div className="text-[var(--sub-text)]">{formatMessageTime(message.sendTime)}</div> */}
          </div>
          <MessageItemErrorBoundary message={message}>
            <MessageRenderComponent
              message={message}
              isSender={isSender}
              disabled={false}
              showAlbum={showAlbum}
              showMergeModal={showMergeModal}
            />
          </MessageItemErrorBoundary>

          {/* <MessageSuffix
            message={message}
            isSender={isSender}
            disabled={false}
            conversationID={conversationID}
          /> */}

          {isQuoteMessage && (
            <QuoteMessageRenderer message={message} isSender={isSender} disabled={false} />
          )}
        </div>
      </div>
      {message.gapTime && (
        <div className="absolute left-1/2 top-0 -translate-x-1/2 text-xs text-[var(--sub-text)]">
          {formatMessageTime(message.sendTime, true)}
        </div>
      )}
      {message.errCode && (
        <div className="absolute bottom-1 left-1/2 flex -translate-x-1/2 items-center text-xs">
        </div>
      )}
    </div>
  );
};

export default memo(MessageItem);
