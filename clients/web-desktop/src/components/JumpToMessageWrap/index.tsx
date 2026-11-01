import { Popover } from "antd";
import { t } from "i18next";
import { MessageItem } from "@/im/types";
import {
  forwardRef,
  ForwardRefRenderFunction,
  memo,
  useImperativeHandle,
  useState,
} from "react";

import { useConversationToggle } from "@/hooks/useConversationToggle";
import { useMessageStore, useUserStore } from "@/store";

interface IJumpToMessageWrap {
  message: MessageItem;
  conversationID: string;
  disabled?: boolean;
  children: React.ReactNode;
  afterJump?: () => void;
}

const JumpToMessageWrap: ForwardRefRenderFunction<
  { jumpToHistory: () => Promise<void> },
  IJumpToMessageWrap
> = ({ message, conversationID, children, disabled, afterJump }, ref) => {
  const [showAvatorMenu, setShowAvatorMenu] = useState(false);
  const getTwoWayHistoryMessage = useMessageStore(
    (state) => state.getTwoWayHistoryMessage,
  );

  const { toSpecifiedConversation } = useConversationToggle();

  const jumpToHistory = async () => {
    if (disabled || !conversationID || !message.clientMsgID) return;
    const loaded = await getTwoWayHistoryMessage({
      conversationID,
      message,
    });
    if (!loaded) return;
    const sourceID =
      message.groupID ||
      (message.sendID === useUserStore.getState().selfInfo.userID
        ? message.recvID
        : message.sendID);
    await toSpecifiedConversation(
      {
        sourceID,
        sessionType: message.sessionType,
      },
      true,
    );
    setShowAvatorMenu(false);
    afterJump?.();
  };

  useImperativeHandle(ref, () => ({ jumpToHistory }));

  return (
    <Popover
      content={
        <div className="p-1">
          <div
            className="cursor-pointer rounded px-2 py-1 text-xs hover:bg-[var(--primary-active)]"
            onClick={jumpToHistory}
          >
            {t("jumpToMessage")}
          </div>
        </div>
      }
      title={null}
      trigger="contextMenu"
      placement="bottom"
      open={disabled ? false : showAvatorMenu}
      onOpenChange={(vis) => setShowAvatorMenu(vis)}
    >
      {children}
    </Popover>
  );
};

export default memo(forwardRef(JumpToMessageWrap));
