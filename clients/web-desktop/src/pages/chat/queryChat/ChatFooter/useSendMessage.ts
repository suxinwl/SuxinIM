import { MessageStatus } from "@openim/wasm-client-sdk";
import { SendMsgParams } from "@openim/wasm-client-sdk/lib/types/params";
import { useCallback } from "react";

import { IMSDK } from "@/layout/MainContentWrap";
import { ExMessageItem, useConversationStore, useMessageStore } from "@/store";
import emitter from "@/utils/events";

export type SendMessageParams = Partial<Omit<SendMsgParams, "message">> & {
  message: ExMessageItem;
  needPush?: boolean;
  isResend?: boolean;
  throwOnError?: boolean;
};

export function useSendMessage() {
  const pushNewMessage = useMessageStore((state) => state.pushNewMessage);
  const tryAddPreviewImg = useMessageStore((state) => state.tryAddPreviewImg);
  const updateOneMessage = useMessageStore((state) => state.updateOneMessage);
  const deleteAndPushOneMessage = useMessageStore(
    (state) => state.deleteAndPushOneMessage,
  );

  const sendMessage = useCallback(
    async ({ recvID, groupID, message, needPush, isResend, throwOnError }: SendMessageParams) => {
      const currentConversation = useConversationStore.getState().currentConversation;
      const sourceID = recvID || groupID;
      const inCurrentConversation =
        currentConversation?.userID === sourceID ||
        currentConversation?.groupID === sourceID ||
        !sourceID;
      needPush = needPush ?? inCurrentConversation;

      if (needPush) {
        pushNewMessage(message);
        emitter.emit("CHAT_LIST_SCROLL_TO_BOTTOM", true);
      }

      const options = {
        recvID: recvID ?? currentConversation?.userID ?? "",
        groupID: groupID ?? currentConversation?.groupID ?? "",
        message,
      };

      try {
        const { data: successMessage } = await IMSDK.sendMessage(options);
        if (isResend) {
          deleteAndPushOneMessage(successMessage as ExMessageItem);
          return;
        }
        updateOneMessage(successMessage as ExMessageItem);
        tryAddPreviewImg([successMessage as ExMessageItem]);
      } catch (error) {
        updateOneMessage({
          ...message,
          status: MessageStatus.Failed,
        });
        if (throwOnError) throw error;
      }
    },
    [],
  );

  return {
    sendMessage,
    updateOneMessage,
  };
}
