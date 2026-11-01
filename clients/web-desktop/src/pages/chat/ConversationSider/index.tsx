import { ConversationItem } from "@/im/types";
import { useEffect, useRef } from "react";
import { useMatches } from "react-router-dom";
import { Virtuoso, VirtuosoHandle } from "react-virtuoso";

import FlexibleSider from "@/components/FlexibleSider";
import { useConversationStore } from "@/store";
import emitter from "@/utils/events";

import ConversationItemComp from "./ConversationItem";

function getNextUnreadIndex(
  conversationList: ConversationItem[],
  currentIndex: number,
) {
  let nextIndex = currentIndex + 1;
  let count = 0;
  while (count < conversationList.length) {
    if (nextIndex >= conversationList.length) {
      nextIndex = 0;
    }
    if (conversationList[nextIndex].unreadCount > 0) {
      return nextIndex;
    }
    nextIndex++;
    count++;
  }
  return -1;
}

const ConversationSider = () => {
  const matches = useMatches();
  const conversationList = useConversationStore((state) => state.conversationList);
  const getConversationListByReq = useConversationStore(
    (state) => state.getConversationListByReq,
  );
  const virtuoso = useRef<VirtuosoHandle>(null);
  const hasmore = useRef(true);
  const currentIndex = useRef(0);

  const inConversation = Boolean(matches[matches.length - 1].params.conversationID);

  useEffect(() => {
    const scrollToUnread = () => {
      currentIndex.current = getNextUnreadIndex(
        useConversationStore.getState().conversationList,
        currentIndex.current,
      );
      if (currentIndex.current > -1) {
        virtuoso.current?.scrollToIndex({
          index: currentIndex.current,
          behavior: "smooth",
        });
      }
    };
    emitter.on("TRY_JUMP_TO_UNREAD", scrollToUnread);
    return () => {
      emitter.off("TRY_JUMP_TO_UNREAD", scrollToUnread);
    };
  }, []);

  const endReached = async () => {
    if (!hasmore.current) return;
    hasmore.current = await getConversationListByReq(true);
  };

  return (
    <FlexibleSider needHidden={inConversation} wrapClassName="left-2 right-2 top-3">
      <Virtuoso
        data={conversationList}
        ref={virtuoso}
        endReached={endReached}
        computeItemKey={(_, item) => item.conversationID}
        itemContent={(_, conversation) => (
          <ConversationItemComp conversation={conversation} />
        )}
      />
    </FlexibleSider>
  );
};

export default ConversationSider;
