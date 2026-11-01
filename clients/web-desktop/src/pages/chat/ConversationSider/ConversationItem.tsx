import { Badge, Popover } from "antd";
import clsx from "clsx";
import { t } from "i18next";
import { GroupAtType, MessageReceiveOptType, SessionType } from "@openim/wasm-client-sdk";
import type {
  ConversationItem,
  ConversationItem as ConversationItemType,
  MessageItem,
} from "@/im/types";
import { useRef, useState } from "react";
import { useNavigate } from "react-router-dom";

import disturb from "@/assets/images/disturb.png";
import OIMAvatar from "@/components/OIMAvatar";
import { parseTwemoji } from "@/components/Twemoji";
import { useConversationStore } from "@/store";
import { formatConversionTime, getConversationContent } from "@/utils/imCommon";

import styles from "./conversation-item.module.scss";
import ConversationMenuContent from "./ConversationMenuContent";
import { useDropFileAndDom } from "./useDropFileAndDom";

interface IConversationProps {
  conversation: ConversationItemType;
}

const ConversationItem = ({ conversation }: IConversationProps) => {
  const navigate = useNavigate();
  const conversationItemRef = useRef<HTMLDivElement>(null);
  const [showConversationMenu, setShowConversationMenu] = useState(false);
  const conversationID = useConversationStore(
    (state) => state.currentConversation?.conversationID,
  );
  const updateCurrentConversation = useConversationStore(
    (state) => state.updateCurrentConversation,
  );

  const { droping } = useDropFileAndDom({
    domRef: conversationItemRef,
    currentConversation: conversation,
  });

  const toSpecifiedConversation = () => {
    updateCurrentConversation({ ...conversation });
    navigate(`/chat/${conversation.conversationID}`);
  };

  const closeConversationMenu = () => {
    setShowConversationMenu(false);
  };

  const getMessagePrefix = () => {
    if (conversation.draftText && conversationID !== conversation.conversationID) {
      return t("messageDescription.drftPrefix");
    }
    let prefix = "";

    if (notNomalReceive && conversation.unreadCount > 0) {
      prefix = t("messageDescription.unreadCount", { count: conversation.unreadCount });
    }

    if (atReminder) {
      switch (conversation.groupAtType) {
        case GroupAtType.AtAll:
          prefix = t("messageDescription.atAllPrefix");
          break;
        case GroupAtType.AtMe:
          prefix = t("messageDescription.atYouPrefix");
          break;
        case GroupAtType.AtAllAtMe:
          prefix = t("messageDescription.atYouPrefix");
          break;
        case GroupAtType.AtGroupNotice:
          prefix = t("messageDescription.groupAnnouncementPrefix");
          break;
      }
    }

    return prefix;
  };

  const atReminder = conversation.groupAtType !== GroupAtType.AtNormal;
  const isNotification = conversation.conversationType === SessionType.Notification;
  const getLatestMessageContent = () => {
    if (conversation.draftText && conversationID !== conversation.conversationID) {
      const parser = new DOMParser();
      const doc = parser.parseFromString(conversation.draftText, "text/html");
      const atEls = doc.querySelectorAll("b.at-el");
      const imageEls = doc.querySelectorAll(".image-el");
      atEls.forEach((el) => {
        const text = el.textContent;
        const parent = el.parentNode;
        parent?.replaceChild(document.createTextNode(text!), el);
      });
      imageEls.forEach((el) => {
        const parent = el.parentNode;
        parent?.replaceChild(
          document.createTextNode(t("messageDescription.imageMessage")),
          el,
        );
      });
      return doc.body.innerHTML;
    }
    if (conversation.latestMsg) {
      try {
        return getConversationContent(
          JSON.parse(conversation.latestMsg) as MessageItem,
        );
      } catch (error) {
        return t("messageDescription.catchMessage");
      }
    }
    return "";
  };

  const latestMessageTime = formatConversionTime(conversation.latestMsgSendTime);

  const isActive =
    conversationID === conversation.conversationID || conversation.isPinned;
  const notNomalReceive = conversation.recvMsgOpt !== MessageReceiveOptType.Normal;

  return (
    <Popover
      overlayClassName="conversation-popover"
      placement="bottomRight"
      title={null}
      arrow={false}
      open={showConversationMenu}
      onOpenChange={(vis) => setShowConversationMenu(vis)}
      content={
        <ConversationMenuContent
          conversation={conversation}
          closeConversationMenu={closeConversationMenu}
        />
      }
      trigger="contextMenu"
    >
      <div
        ref={conversationItemRef}
        className={clsx(
          styles["conversation-item"],
          "border border-transparent",
          isActive && `bg-[var(--primary-active)]`,
          conversation.isPinned && styles["conversation-item-pinned"],
          droping && "!border-[var(--primary)]",
        )}
        onClick={toSpecifiedConversation}
      >
        <Badge size="small" count={notNomalReceive ? 0 : conversation.unreadCount}>
          <OIMAvatar
            src={conversation.faceURL}
            isgroup={Boolean(conversation.groupID)}
            isnotification={isNotification}
            text={conversation.showName}
          />
        </Badge>

        <div className="ml-3 flex h-11 flex-1 flex-col justify-between overflow-hidden">
          <div className="flex items-center justify-between">
            <div className="flex-1 truncate font-medium">{conversation.showName}</div>
            <div className="ml-2 text-xs text-[var(--sub-text)]">
              {latestMessageTime}
            </div>
          </div>

          <div className="flex items-center">
            <div className="flex min-h-[16px] flex-1 overflow-hidden text-xs">
              <div
                className={clsx("mr-px whitespace-nowrap text-[var(--primary)]", {
                  "!text-[var(--sub-text)]": notNomalReceive && !atReminder,
                })}
              >
                {getMessagePrefix()}
              </div>
              <div
                className="truncate text-[rgba(81,94,112,0.5)]"
                dangerouslySetInnerHTML={{
                  __html: parseTwemoji(getLatestMessageContent()),
                }}
              ></div>
            </div>

            <img
              className={notNomalReceive ? "visible" : "invisible"}
              src={disturb}
              width={14}
              alt="disturb"
            />
          </div>
        </div>
      </div>
    </Popover>
  );
};

export default ConversationItem;
