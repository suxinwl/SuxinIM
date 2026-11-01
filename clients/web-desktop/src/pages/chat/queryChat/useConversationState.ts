import { GroupAtType, GroupStatus } from "@openim/wasm-client-sdk";
import { useEffect } from "react";
import { useParams } from "react-router-dom";

import { useCurrentMemberRole } from "@/hooks/useCurrentMemberRole";
import { isReadActivityActive, useReadActivity } from "@/hooks/useReadActivity";
import { scheduleConversationRead } from "@/im/read-receipt-gate";
import { IMSDK } from "@/layout/MainContentWrap";
import { useConversationStore } from "@/store";

export default function useConversationState(readReady: boolean) {
  const { conversationID } = useParams();
  const readActive = useReadActivity();
  const currentConversation = useConversationStore(
    (state) => state.currentConversation,
  );
  const isMutedGroup = useConversationStore(
    (state) => state.currentGroupInfo?.status === GroupStatus.Muted,
  );

  const { isJoinGroup, isNomal, currentIsMuted } = useCurrentMemberRole();

  useEffect(() => {
    if (!readReady || !readActive || !conversationID) return;
    return scheduleConversationRead({
      conversationID,
      canRead: isReadActivityActive,
      getConversation: () => {
        const conversation = useConversationStore.getState().currentConversation;
        if (!conversation) return;
        return {
          conversationID: conversation.conversationID,
          unreadCount: conversation.unreadCount,
          resetGroupAt:
            conversation.groupAtType !== GroupAtType.AtNormal &&
            conversation.groupAtType !== GroupAtType.AtGroupNotice,
        };
      },
      markRead: (id) => IMSDK.markConversationMessageAsRead(id),
      resetGroupAt: (id) => IMSDK.resetConversationGroupAtType(id),
    });
  }, [
    readReady,
    readActive,
    conversationID,
    currentConversation?.conversationID,
    currentConversation?.groupAtType,
    currentConversation?.unreadCount,
  ]);

  const getIsCanSendMessage = () => {
    if (currentConversation?.userID) {
      return true;
    }

    if (!isJoinGroup) {
      return false;
    }

    if (isMutedGroup && isNomal) {
      return false;
    }

    return !currentIsMuted;
  };

  return { getIsCanSendMessage, currentIsMuted, isMutedGroup, currentConversation };
}
