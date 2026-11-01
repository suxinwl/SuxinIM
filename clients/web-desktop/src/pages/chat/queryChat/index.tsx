import { InfoCircleOutlined } from "@ant-design/icons";
import { useRequest, useUnmount } from "ahooks";
import { Layout, Spin } from "antd";
import { t } from "i18next";
import { SessionType } from "@openim/wasm-client-sdk";
import { useEffect } from "react";
import { useParams } from "react-router-dom";

import { IMSDK } from "@/layout/MainContentWrap";
import { useConversationStore, useMessageStore } from "@/store";
import emitter from "@/utils/events";

import ChatContent from "./ChatContent";
import ChatFooter from "./ChatFooter";
import MultipleActionBar from "./ChatFooter/MultipleActionBar";
import ChatHeader from "./ChatHeader";
import useConversationState from "./useConversationState";
import { useDropAndPaste } from "./useDropAndPaste";
import { useMessageReceipt } from "./useMessageReceipt";

export const QueryChat = () => {
  const { conversationID } = useParams();
  const isCheckMode = useMessageStore((state) => state.isCheckMode);
  const jumpLoading = useMessageStore((state) => state.jumpLoading);
  const updateCheckMode = useMessageStore((state) => state.updateCheckMode);
  const updateCurrentConversation = useConversationStore(
    (state) => state.updateCurrentConversation,
  );
  const updateQuoteMessage = useConversationStore((state) => state.updateQuoteMessage);
  const getHistoryMessageList = useMessageStore(
    (state) => state.getHistoryMessageListByReq,
  );
  const getConversationPreviewImgList = useMessageStore(
    (state) => state.getConversationPreviewImgList,
  );
  const clearHistoryMessage = useMessageStore((state) => state.clearHistoryMessage);

  const {
    loading,
    run: initMessages,
    cancel,
  } = useRequest(getHistoryMessageList, {
    manual: true,
  });

  const { getIsCanSendMessage, isMutedGroup, currentIsMuted, currentConversation } =
    useConversationState(!loading && !jumpLoading);
  useMessageReceipt();

  const isNotificationSession =
    currentConversation?.conversationType === SessionType.Notification;

  const { droping } = useDropAndPaste({
    currentConversation,
    getIsCanSendMessage,
  });

  useEffect(() => {
    const refresh = () => {
      initMessages();
      getConversationPreviewImgList();
    };
    emitter.on("REFRESH_CHAT_LIST", refresh);
    if (!useMessageStore.getState().jumpClientMsgID) refresh();
    return () => {
      emitter.off("REFRESH_CHAT_LIST", refresh);
      cancel();
      updateCheckMode(false);
      updateQuoteMessage();
      if (!useMessageStore.getState().jumpClientMsgID) {
        clearHistoryMessage();
      }
    };
  }, [conversationID]);

  useUnmount(() => {
    updateCurrentConversation();
  });

  const switchFooter = () => {
    if (isNotificationSession) {
      return null;
    }
    if (isCheckMode) {
      return <MultipleActionBar />;
    }
    if (!getIsCanSendMessage()) {
      let tip = t("toast.notCanSendMessage");
      if (isMutedGroup) tip = t("toast.groupMuted");
      if (currentIsMuted) tip = t("toast.currentMuted");

      if (currentConversation?.draftText) {
        IMSDK.setConversationDraft({
          conversationID: currentConversation.conversationID,
          draftText: "",
        });
      }

      return (
        <div className="flex justify-center py-4.5 text-xs text-[var(--sub-text)]">
          <InfoCircleOutlined rev={undefined} />
          <span className="ml-1">{tip}</span>
        </div>
      );
    }
    return <ChatFooter />;
  };

  return (
    <Layout id="chat-container" className="relative overflow-hidden">
      <ChatHeader />
      {loading || jumpLoading ? (
        <div className="flex h-full items-center justify-center bg-white pt-1">
          <Spin spinning />
        </div>
      ) : (
        <ChatContent isNotificationSession={isNotificationSession} />
      )}
      {switchFooter()}
      {droping && (
        <div className="absolute left-0 top-0 flex h-full w-full items-center justify-center bg-[rgba(248,229,229,0.4)]">
          <div className="max-w-[200px] truncate text-[var(--sub-text)]">{`${t(
            "placeholder.loosenToSend",
          )} ${currentConversation?.showName}`}</div>
        </div>
      )}
    </Layout>
  );
};
