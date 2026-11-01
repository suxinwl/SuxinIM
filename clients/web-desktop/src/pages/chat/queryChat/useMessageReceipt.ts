import { CbEvents } from "@openim/wasm-client-sdk";
import { SessionType } from "@openim/wasm-client-sdk";
import {
  GroupMessageReceiptInfo,
  ReceiptInfo,
  WSEvent,
} from "@/im/types";
import { useEffect } from "react";

import { IMSDK } from "@/layout/MainContentWrap";
import {
  ExMessageItem,
  useConversationStore,
  useMessageStore,
  useUserStore,
} from "@/store";

export function useMessageReceipt() {
  const selfUserID = useUserStore((state) => state.selfInfo.userID);
  const updateOneMessage = useMessageStore((state) => state.updateOneMessage);

  useEffect(() => {
    setIMListener();
    return () => {
      disposeIMListener();
    };
  }, [selfUserID]);

  const setIMListener = () => {
    IMSDK.on(CbEvents.OnRecvC2CReadReceipt, singleMessageHasReadedHander);
    IMSDK.on(CbEvents.OnRecvGroupReadReceipt, groupMessageHasReadedHander);
  };

  const disposeIMListener = () => {
    IMSDK.off(CbEvents.OnRecvC2CReadReceipt, singleMessageHasReadedHander);
    IMSDK.off(CbEvents.OnRecvGroupReadReceipt, groupMessageHasReadedHander);
  };

  const singleMessageHasReadedHander = ({ data }: WSEvent<ReceiptInfo[]>) => {
    if (
      useConversationStore.getState().currentConversation?.conversationType !==
      SessionType.Single
    )
      return;

    data.map((receipt) => {
      (receipt.msgIDList ?? []).map((clientMsgID: string) => {
        updateOneMessage({
          clientMsgID,
          isRead: true,
        } as ExMessageItem);
      });
    });
  };

  const groupMessageHasReadedHander = ({ data }: WSEvent<GroupMessageReceiptInfo>) => {
    if (
      useConversationStore.getState().currentConversation?.conversationID !==
      data.conversationID
    )
      return;

    data.groupMessageReadInfo.map((receipt) => {
      const hasSelfRead = receipt.readMembers?.some(
        (member) => member.userID === selfUserID,
      );
      const oldMessage = useMessageStore
        .getState()
        .historyMessageList.find(
          (message) => message.clientMsgID === receipt.clientMsgID,
        );
      updateOneMessage({
        ...oldMessage,
        isRead: hasSelfRead ? true : oldMessage?.isRead,
        attachedInfoElem: {
          ...oldMessage?.attachedInfoElem,
          groupHasReadInfo: {
            hasReadCount: receipt.hasReadCount,
            unreadCount: receipt.unreadCount,
          },
        },
      } as ExMessageItem);
    });
  };
}
