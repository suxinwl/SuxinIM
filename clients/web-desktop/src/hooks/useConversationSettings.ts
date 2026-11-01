import { App } from "antd";
import { t } from "i18next";
import { MessageReceiveOptType } from "@openim/wasm-client-sdk";
import { useCallback } from "react";

import { IMSDK } from "@/layout/MainContentWrap";
import { useConversationStore, useMessageStore } from "@/store";
import { feedbackToast } from "@/utils/common";
import { isGroupSession } from "@/utils/imCommon";

export function useConversationSettings() {
  const { modal } = App.useApp();

  const currentConversation = useConversationStore(
    (state) => state.currentConversation,
  );
  const clearHistoryMessage = useMessageStore((state) => state.clearHistoryMessage);

  const updateConversationPin = useCallback(
    async (isPinned: boolean) => {
      if (!currentConversation) return;

      try {
        await IMSDK.pinConversation({
          conversationID: currentConversation.conversationID,
          isPinned,
        });
      } catch (error) {
        feedbackToast({ error, msg: t("toast.pinConversationFailed") });
      }
    },
    [currentConversation?.conversationID],
  );

  const updateConversationMessageRemind = useCallback(
    async (checked: boolean, option: MessageReceiveOptType) => {
      if (!currentConversation) return;

      try {
        await IMSDK.setConversationRecvMessageOpt({
          conversationID: currentConversation.conversationID,
          opt: checked ? option : MessageReceiveOptType.Normal,
        });
      } catch (error) {
        feedbackToast({ error, msg: t("toast.setConversationRecvMessageOptFailed") });
      }
    },
    [currentConversation?.conversationID],
  );

  const updateConversationPrivateState = useCallback(async () => {
    if (!currentConversation) return;

    try {
      await IMSDK.setConversationPrivateChat({
        conversationID: currentConversation.conversationID,
        isPrivate: !currentConversation.isPrivateChat,
      });
    } catch (error) {
      feedbackToast({ error, msg: t("toast.updateConversationPrivateStateFailed") });
    }
  }, [currentConversation?.conversationID, currentConversation?.isPrivateChat]);

  const updateBurnDuration = useCallback(
    async (seconds: number) => {
      if (!currentConversation) return;

      try {
        await IMSDK.setConversationBurnDuration({
          conversationID: currentConversation.conversationID,
          burnDuration: seconds,
        });
      } catch (error) {
        feedbackToast({ error, msg: t("toast.updateConversationPrivateStateFailed") });
      }
    },
    [currentConversation?.conversationID],
  );

  const updateConversationMsgDestructState = useCallback(async () => {
    if (!currentConversation) return;

    try {
      await IMSDK.setConversationIsMsgDestruct({
        conversationID: currentConversation.conversationID,
        isMsgDestruct: !currentConversation.isMsgDestruct,
      });
    } catch (error) {
      feedbackToast({ error });
    }
  }, [currentConversation?.conversationID, currentConversation?.isMsgDestruct]);

  const updateDestructDuration = useCallback(
    async (seconds: number) => {
      if (!currentConversation) return;

      try {
        await IMSDK.setConversationMsgDestructTime({
          conversationID: currentConversation.conversationID,
          msgDestructTime: seconds,
        });
      } catch (error) {
        feedbackToast({ error });
      }
    },
    [currentConversation?.conversationID],
  );

  const clearConversationMessages = useCallback(() => {
    if (!currentConversation) return;
    modal.confirm({
      title: t("toast.clearChatHistory"),
      content: t("toast.confirmClearChatHistory"),
      onOk: async () => {
        try {
          await IMSDK.clearConversationAndDeleteAllMsg(
            currentConversation.conversationID,
          );
          clearHistoryMessage();
        } catch (error) {
          feedbackToast({ error, msg: t("toast.clearConversationMessagesFailed") });
        }
      },
    });
  }, [currentConversation?.conversationID]);

  return {
    currentConversation,
    updateBurnDuration,
    updateDestructDuration,
    updateConversationPin,
    updateConversationMessageRemind,
    updateConversationPrivateState,
    updateConversationMsgDestructState,
    clearConversationMessages,
  };
}
