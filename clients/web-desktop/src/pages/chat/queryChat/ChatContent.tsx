import { useLatest, useRequest } from "ahooks";
import { Layout, Spin } from "antd";
import clsx from "clsx";
import { SessionType } from "@openim/wasm-client-sdk";
import { MergeElem } from "@/im/types";
import { memo, useCallback, useEffect, useRef, useState } from "react";
import InfiniteScroll from "react-infinite-scroll-component-two-way";
import { useParams } from "react-router-dom";

import { SystemMessageTypes } from "@/constants/im";
import { OverlayVisibleHandle } from "@/hooks/useOverlayVisible";
import MergePreviewModal from "@/pages/common/MergePreviewModal";
import { useMessageStore, useUserStore } from "@/store";
import emitter from "@/utils/events";

import ForwardMediaPreview from "./MediaPreview";
import MessageItem from "./MessageItem";
import NotificationMessage from "./NotificationMessage";
import SystemNotification from "./SystemNotification";
import UnreadMessageSlider, { UnreadMessageSliderHandle } from "./UnreadMessageSlider";

const ChatContent = ({ isNotificationSession }: { isNotificationSession: boolean }) => {
  const { conversationID } = useParams();
  const [lockScroll, setLockScroll] = useState(true);
  const scrollDivRef = useRef<HTMLDivElement>(null);
  const [mergeData, setMergeData] = useState<MergeElem | null>(null);
  const mergePreviewRef = useRef<OverlayVisibleHandle>(null);
  const mediaPreviewRef = useRef<{ showAlbum: (clientMsgID: string) => void }>(null);
  const unreadSliderRef = useRef<UnreadMessageSliderHandle>(null);

  const selfUserID = useUserStore((state) => state.selfInfo.userID);
  const jumpClientMsgID = useMessageStore((state) => state.jumpClientMsgID);
  const messageList = useMessageStore((state) => state.historyMessageList);
  const latestMessageList = useLatest(messageList);
  const hasMoreMessage = useMessageStore((state) => state.hasMore);
  const laterHasMore = useMessageStore((state) => state.laterHasMore);
  const getHistoryMessageList = useMessageStore(
    (state) => state.getHistoryMessageListByReq,
  );
  const getHistoryMessageListReverse = useMessageStore(
    (state) => state.getHistoryMessageListReverseByReq,
  );

  const {
    loading,
    run: getMoreMessages,
    cancel,
  } = useRequest(getHistoryMessageList, {
    manual: true,
  });

  const {
    loading: reverseLoading,
    run: getMoreMessagesReverse,
    cancel: cancelReverse,
  } = useRequest(getHistoryMessageListReverse, {
    manual: true,
  });

  useEffect(() => {
    if (jumpClientMsgID) {
      const jumpEl = document.getElementById(`chat_${jumpClientMsgID}`);
      jumpEl?.scrollIntoView({
        behavior: "instant",
      });
      setTimeout(() => {
        setLockScroll(false);
      }, 1000);
    } else {
      setLockScroll(false);
    }
    return () => {
      setLockScroll(true);
    };
  }, [jumpClientMsgID]);

  useEffect(() => {
    const observer = new MutationObserver((mutationsList) => {
      if (
        mutationsList.length > 1 ||
        !document.hidden ||
        scrollDivRef.current?.scrollTop !== 0
      )
        return;

      const mutation = mutationsList[0];
      if (mutation.type === "childList" && mutation.addedNodes.length > 0) {
        setTimeout(() => {
          const el = document.getElementById(
            `chat_${latestMessageList.current[1].clientMsgID}`,
          );
          el?.scrollIntoView({
            block: "end",
          });
        });
      }
    });
    observer.observe(scrollDivRef.current!, { childList: true, subtree: true });
    return () => {
      observer.disconnect();
    };
  }, []);

  useEffect(() => {
    const toBottomHandle = (forceScroll: boolean) => {
      if (isNotificationSession) return;

      if (!forceScroll && document.hidden) {
        unreadSliderRef.current?.updateAppendCount();
        return;
      }

      if ((scrollDivRef.current?.scrollTop ?? 0) > -1000 || forceScroll) {
        scrollToBottom("smooth");
      } else {
        unreadSliderRef.current?.updateAppendCount();
      }
    };
    emitter.on("CHAT_LIST_SCROLL_TO_BOTTOM", toBottomHandle);
    return () => {
      cancel();
      cancelReverse();
      emitter.off("CHAT_LIST_SCROLL_TO_BOTTOM", toBottomHandle);
    };
  }, [isNotificationSession]);

  const scrollToBottom = (behavior?: ScrollBehavior) => {
    setTimeout(() => {
      scrollDivRef.current?.scrollTo({
        top: scrollDivRef.current.scrollHeight,
        behavior,
      });
    });
  };

  const updateAppendCount = useCallback(
    () => unreadSliderRef.current?.updateAppendCount(),
    [unreadSliderRef],
  );

  const loadMoreMessage = () => {
    if ((lockScroll && jumpClientMsgID) || loading || !hasMoreMessage) return;
    getMoreMessages(true);
  };

  const prevLoad = () => {
    if (lockScroll || reverseLoading || !laterHasMore) return;
    getMoreMessagesReverse();
  };

  const showMergeModal = useCallback((data: MergeElem) => {
    setMergeData(data);
    mergePreviewRef.current?.openOverlay();
  }, []);

  const showAlbum = useCallback(
    (clientMsgID: string) => mediaPreviewRef.current?.showAlbum(clientMsgID),
    [],
  );

  return (
    <Layout.Content className="relative flex overflow-hidden !bg-white" id="chat-main">
      <div
        id="scrollableDiv"
        ref={scrollDivRef}
        className={clsx("flex h-full w-full flex-col-reverse overflow-x-hidden", {
          "!flex-col": isNotificationSession,
          growable: !isNotificationSession,
        })}
      >
        <InfiniteScroll
          dataLength={messageList.length}
          lockTrigger={lockScroll}
          prev={prevLoad}
          next={loadMoreMessage}
          className={clsx("flex w-full flex-col-reverse", {
            "!flex-col": isNotificationSession,
          })}
          inverse={true}
          hasMore={hasMoreMessage}
          prevHasMore={laterHasMore}
          prevLoader={
            <div className="flex justify-center py-2">
              <Spin />
            </div>
          }
          loader={
            <div className="flex justify-center py-2">
              <Spin />
            </div>
          }
          scrollableTarget="scrollableDiv"
        >
          {messageList.map((message) => {
            if (message.sessionType === SessionType.Notification) {
              return <SystemNotification key={message.clientMsgID} message={message} />;
            }

            if (SystemMessageTypes.includes(message.contentType)) {
              return (
                <NotificationMessage key={message.clientMsgID} message={message} />
              );
            }
            const isSender = selfUserID === message.sendID;
            return (
              <MessageItem
                key={message.clientMsgID}
                conversationID={conversationID}
                message={message}
                messageUpdateFlag={message.senderNickname + message.senderFaceUrl}
                isSender={isSender}
                showAlbum={showAlbum}
                showMergeModal={showMergeModal}
                updateAppendCount={updateAppendCount}
              />
            );
          })}
        </InfiniteScroll>
      </div>
      <UnreadMessageSlider ref={unreadSliderRef} scrollToBottom={scrollToBottom} />
      <ForwardMediaPreview
        ref={mediaPreviewRef}
        conversationID={conversationID ?? ""}
      />
      <MergePreviewModal ref={mergePreviewRef} mergeData={mergeData!} />
    </Layout.Content>
  );
};

export default memo(ChatContent);
