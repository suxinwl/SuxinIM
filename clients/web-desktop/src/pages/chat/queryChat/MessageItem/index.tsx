import { useInViewport } from "ahooks";
import { Checkbox, Popover } from "antd";
import { CheckboxChangeEvent } from "antd/es/checkbox";
import clsx from "clsx";
import { MessageStatus, MessageType, SessionType } from "@openim/wasm-client-sdk";
import { GroupMemberItem, MergeElem } from "@/im/types";
import { FC, memo, useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";

import OIMAvatar from "@/components/OIMAvatar";
import { useConversationToggle } from "@/hooks/useConversationToggle";
import { isReadActivityActive, useReadActivity } from "@/hooks/useReadActivity";
import { IMSDK } from "@/layout/MainContentWrap";
import { ExMessageItem, useConversationStore, useMessageStore, useUserStore } from "@/store";
import emitter from "@/utils/events";
import { formatMessageTime } from "@/utils/imCommon";

import AnnouncementRenderer from "./AnnouncementRender";
import CardMessageRenderer from "./CardMessageRenderer";
import CatchMessageRender from "./CatchMsgRenderer";
import CustomMessageSwitcher from "./CustomMessageSwitcher";
import FaceMessageRender from "./FaceMessageRender";
import FileMessageRenderer from "./FileMessageRenderer";
import LocationMessageRenderer from "./LocationMessageRenderer";
import MediaMessageRender from "./MediaMessageRender";
import MergeMessageRenderer from "./MergeMessageRenderer";
import styles from "./message-item.module.scss";
import MessageItemErrorBoundary from "./MessageItemErrorBoundary";
import MessageMenuContent from "./MessageMenuContent";
import MessageReadState from "./MessageReadState";
import MessageSuffix from "./MessageSuffix";
import QuoteMessageRenderer from "./QuoteMessageRenderer";
import TextMessageRender from "./TextMessageRender";
import VoiceMessageRender from "./VoiceMessageRender";

export interface IMessageItemProps {
  message: ExMessageItem;
  isSender: boolean;
  disabled?: boolean;
  conversationID?: string;
  messageUpdateFlag?: string;
  showAlbum?: (clientMsgID: string) => void;
  showMergeModal?: (data: MergeElem) => void;
  updateAppendCount?: () => void;
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
  disabled,
  isSender,
  conversationID,
  showAlbum,
  showMergeModal,
  updateAppendCount,
}) => {
  const messageWrapRef = useRef<HTMLDivElement>(null);
  const [showMessageMenu, setShowMessageMenu] = useState(false);
  const [showAvatorMenu, setShowAvatorMenu] = useState(false);
  const isCheckMode = useMessageStore((state) => state.isCheckMode);
  const jumpClientMsgID = useMessageStore((state) => state.jumpClientMsgID);
  const updateMessage = useMessageStore((state) => state.updateOneMessage);
  const MessageRenderComponent = components[message.contentType] || CatchMessageRender;
  const readActive = useReadActivity();

  // locale re render
  useUserStore((state) => state.appSettings.locale);

  const isQuoteMessage =
    message.contentType === MessageType.QuoteMessage ||
    message.atTextElem?.quoteMessage;

  const [inViewport] = useInViewport(messageWrapRef, {
    root: document.getElementById("chat-main"),
  });

  useEffect(() => {
    if (
      disabled ||
      isSender ||
      !inViewport ||
      !readActive ||
      !isReadActivityActive() ||
      !conversationID ||
      useConversationStore.getState().currentConversation?.conversationID !== conversationID
    ) return;
    updateMessageAppendState();
  }, [
    readActive,
    conversationID,
    inViewport,
    isSender,
    disabled,
    message.isAppend,
  ]);

  useEffect(() => {
    let cancelled = false;
    const canReportRead = () =>
      !cancelled &&
      !disabled &&
      !isSender &&
      Boolean(inViewport && conversationID) &&
      isReadActivityActive() &&
      useConversationStore.getState().currentConversation?.conversationID === conversationID;
    if (
      !canReportRead() ||
      message.isRead ||
      !message.seq ||
      message.contentType === MessageType.GroupAnnouncementUpdated
    ) return;

    const reportRead = async () => {
      if (message.groupID) {
        try {
          await IMSDK.sendGroupMessageReadReceipt({
            conversationID,
            clientMsgIDList: [message.clientMsgID],
            seqs: { [message.clientMsgID]: message.seq },
            shouldSendReadReceipt: canReportRead,
          });
        } catch {
          return;
        }
      }
      if (!canReportRead()) return;
      updateMessage({
        clientMsgID: message.clientMsgID,
        isRead: true,
      } as ExMessageItem);
    };
    void reportRead();
    return () => {
      cancelled = true;
    };
  }, [
    readActive,
    conversationID,
    inViewport,
    isSender,
    disabled,
    message.clientMsgID,
    message.groupID,
    message.isRead,
    message.seq,
    message.contentType,
  ]);

  const onCheckChange = (e: CheckboxChangeEvent) => {
    updateMessage({ ...message, checked: e.target.checked });
  };

  const tryShowUserCard = useCallback(() => {
    if (disabled) return;
    window.userClick(message.sendID, message.groupID);
  }, []);

  const updateMessageAppendState = () => {
    if (!message.isAppend) return;
    const updateFields = {
      clientMsgID: message.clientMsgID,
      isAppend: false,
    } as ExMessageItem;
    if (message.sessionType === SessionType.Single) {
      updateFields.isRead = true;
    }
    updateMessage(updateFields);
    updateAppendCount?.();
  };

  const closeMessageMenu = useCallback(() => {
    setShowMessageMenu(false);
  }, []);

  const messageIsSuccess = message.status === MessageStatus.Succeed;
  const isAnnouncement = message.contentType === MessageType.GroupAnnouncementUpdated;
  const isCustomMessage = message.contentType === MessageType.CustomMessage;
  const showMessageReadState =
    isSender && messageIsSuccess && !isAnnouncement && !isCustomMessage;
  const canShowMessageMenu = !disabled && !isAnnouncement;

  return (
    <div
      id={`chat_${message.clientMsgID}`}
      className={clsx(
        "relative flex select-text px-5 py-3",
        message.gapTime && "!pt-9",
        message.errCode && "!pb-6",
        isCheckMode && "cursor-pointer",
        jumpClientMsgID === message.clientMsgID && styles["animate-container"],
      )}
      onClick={() =>
        isCheckMode &&
        onCheckChange({
          target: { checked: !message.checked },
        } as CheckboxChangeEvent)
      }
    >
      {isCheckMode && (
        <Checkbox
          checked={message.checked}
          disabled={isAnnouncement}
          onChange={onCheckChange}
          className="pointer-events-none mr-5 h-9"
        />
      )}
      <div
        className={clsx(
          styles["message-container"],
          isSender && styles["message-container-sender"],
          isCheckMode && "pointer-events-none",
        )}
      >
        <Popover
          className={styles["menu-wrap"]}
          content={
            <MessageAvatarMenuContent
              senderNickname={message.senderNickname}
              sourceID={message.sendID}
              closeContent={() => setShowAvatorMenu(false)}
            />
          }
          title={null}
          trigger="contextMenu"
          placement="bottomLeft"
          open={message.groupID ? showAvatorMenu : false}
          onOpenChange={(vis) => setShowAvatorMenu(vis)}
        >
          <OIMAvatar
            size={36}
            src={message.senderFaceUrl}
            text={message.senderNickname}
            onClick={tryShowUserCard}
          />
        </Popover>

        <div className={styles["message-wrap"]} ref={messageWrapRef}>
          <div className={styles["message-profile"]}>
            <div
              title={message.senderNickname}
              className={clsx(
                "max-w-[30%] truncate text-[var(--sub-text)]",
                isSender ? "ml-2" : "mr-2",
              )}
            >
              {message.senderNickname}
            </div>
            <div className="text-[var(--sub-text)]">
              {formatMessageTime(message.sendTime)}
            </div>
          </div>

          <Popover
            className={styles["menu-wrap"]}
            content={
              <MessageMenuContent
                message={message}
                conversationID={conversationID!}
                closeMenu={closeMessageMenu}
              />
            }
            title={null}
            trigger="contextMenu"
            open={canShowMessageMenu ? showMessageMenu : false}
            onOpenChange={(vis) => setShowMessageMenu(vis)}
          >
            <MessageItemErrorBoundary message={message}>
              <MessageRenderComponent
                message={message}
                isSender={isSender}
                disabled={disabled}
                showAlbum={showAlbum}
                showMergeModal={showMergeModal}
              />
            </MessageItemErrorBoundary>

            <MessageSuffix
              message={message}
              isSender={isSender}
              disabled={false}
              conversationID={conversationID}
            />
          </Popover>

          {isQuoteMessage && (
            <QuoteMessageRenderer
              message={message}
              isSender={isSender}
              disabled={false}
            />
          )}

          {showMessageReadState ? (
            <MessageReadState message={message} isSender={isSender} disabled={false} />
          ) : (
            <div className="h-5" />
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
          {/* <div className="text-xs text-[var(--sub-text)]">
            {t("toast.beingBlacklist")}
          </div>
          <div className="text-[var(--sub-text)]">
            {t("toast.usingFriendVerification")}
          </div>
          <div className="ml-2 cursor-pointer text-[var(--primary)]">
            {t("placeholder.verifyAdd")}
          </div> */}
        </div>
      )}
    </div>
  );
};

export default memo(MessageItem);

const MessageAvatarMenuContent = memo(
  ({
    sourceID,
    senderNickname,
    closeContent,
  }: {
    sourceID: string;
    senderNickname: string;
    closeContent: () => void;
  }) => {
    const { t } = useTranslation();

    const { toSpecifiedConversation } = useConversationToggle();

    const triggerAt = () => {
      emitter.emit("TRIGGER_GROUP_AT", {
        userID: sourceID,
        nickname: senderNickname,
      } as GroupMemberItem);
      closeContent();
    };

    return (
      <div className="p-1">
        <div
          className="max-w-[120px] cursor-pointer truncate rounded px-3 py-2 text-xs hover:bg-[var(--primary-active)]"
          onClick={triggerAt}
        >
          {`@${senderNickname}`}
        </div>
        <div
          className="cursor-pointer rounded px-3 py-2 text-xs hover:bg-[var(--primary-active)]"
          onClick={() =>
            toSpecifiedConversation({ sourceID, sessionType: SessionType.Single })
          }
        >
          {t("placeholder.sendMessage")}
        </div>
      </div>
    );
  },
);
