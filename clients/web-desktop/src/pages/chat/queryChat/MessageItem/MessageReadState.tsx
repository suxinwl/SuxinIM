import { CloseOutlined } from "@ant-design/icons";
import { useKeyPress, useRequest } from "ahooks";
import { Popover } from "antd";
import clsx from "clsx";
import { CbEvents } from "@openim/wasm-client-sdk";
import { SessionType } from "@openim/wasm-client-sdk";
import {
  GroupMemberItem,
  GroupMessageReceiptInfo,
  WSEvent,
} from "@/im/types";
import { FC, memo, useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Virtuoso } from "react-virtuoso";

import OIMAvatar from "@/components/OIMAvatar";
import { IMSDK } from "@/layout/MainContentWrap";
import { useConversationStore } from "@/store";

import { IMessageItemProps } from ".";

const MessageReadState: FC<IMessageItemProps> = ({ message }) => {
  const { t } = useTranslation();
  const [showReadList, setShowReadList] = useState(false);
  const isSingle = message.sessionType === SessionType.Single;
  const hasReadSummary = message.attachedInfoElem?.groupHasReadInfo?.unreadCount !== undefined;
  const unReadCount = message.attachedInfoElem?.groupHasReadInfo?.unreadCount ?? 0;

  const getReadStateStr = () => {
    if (isSingle) {
      return message.isRead ? t("placeholder.isRead") : t("placeholder.unread");
    }

    if (!hasReadSummary) return "--";
    return unReadCount < 1
      ? t("placeholder.allIsRead")
      : t("placeholder.unreadNum", { num: unReadCount });
  };

  const closeOverlay = useCallback(() => setShowReadList(false), []);

  return (
    <Popover
      content={
        <ReadedList
          clientMsgID={message.clientMsgID}
          readedCount={message.attachedInfoElem?.groupHasReadInfo.hasReadCount ?? 0}
          unReadCount={unReadCount}
          closeOverlay={closeOverlay}
        />
      }
      destroyTooltipOnHide
      trigger="click"
      placement="bottomLeft"
      overlayClassName="profile-popover"
      title={null}
      arrow={false}
      open={isSingle ? false : showReadList}
      onOpenChange={(vis) => setShowReadList(vis)}
    >
      <div
        className={clsx("mt-1 text-xs text-[#0289FA]", {
          "!text-[var(--sub-text)]": isSingle ? message.isRead : unReadCount < 1,
          "!cursor-pointer": !isSingle,
        })}
      >
        {getReadStateStr()}
      </div>
    </Popover>
  );
};

export default MessageReadState;

interface IReadedListProps {
  clientMsgID: string;
  unReadCount: number;
  readedCount: number;
  closeOverlay: () => void;
}

const ReadedList = memo(
  ({ clientMsgID, unReadCount, readedCount, closeOverlay }: IReadedListProps) => {
    const { t } = useTranslation();
    const [readedData, setReadedData] = useState<GroupMemberItem[]>([]);
    const [unReadData, setUnReadData] = useState<GroupMemberItem[]>([]);
    const hasMoreReaded = useRef(true);
    const hasMoreUnread = useRef(true);
    const conversationID =
      useConversationStore((state) => state.currentConversation?.conversationID) ?? "";

    useKeyPress("esc", () => {
      closeOverlay();
    });

    const {
      loading: unreadLoading,
      run: getUnreadMembers,
      cancel: cancelGetUnreadMembers,
    } = useRequest(
      (offset?: number) =>
        IMSDK.getGroupMessageReaderList({
          conversationID,
          clientMsgID,
          filter: 1,
          offset: offset ?? 0,
          count: 20,
        }),
      {
        manual: true,
        onSuccess: (data) => {
          hasMoreUnread.current = data.data.length === 20;
          setUnReadData((state) => [...state, ...data.data]);
        },
      },
    );
    const {
      loading: readedLoading,
      run: getReadedMembers,
      cancel: cancelGetReadedMembers,
    } = useRequest(
      (offset?: number) =>
        IMSDK.getGroupMessageReaderList({
          conversationID,
          clientMsgID,
          filter: 0,
          offset: offset ?? 0,
          count: 20,
        }),
      {
        manual: true,
        onSuccess: (data) => {
          hasMoreReaded.current = data.data.length === 20;
          setReadedData((state) => [...state, ...data.data]);
        },
      },
    );

    useEffect(() => {
      const groupMessageHasReadedHander = ({
        data,
      }: WSEvent<GroupMessageReceiptInfo>) => {
        if (data.conversationID !== conversationID) return;
        const usefullData = data.groupMessageReadInfo.filter(
          (receipt) => receipt.clientMsgID === clientMsgID,
        );
        if (!usefullData.length) return;

        const readMembers = usefullData.map((item) => item.readMembers).flat();
        if (!hasMoreReaded.current) {
          setReadedData((prev) => [...prev, ...readMembers]);
        }
        setUnReadData((prev) =>
          prev.filter(
            (member) => !readMembers.some((item) => item.userID === member.userID),
          ),
        );
      };
      setTimeout(() => {
        getReadedMembers();
        getUnreadMembers();
      }, 100);
      IMSDK.on(CbEvents.OnRecvGroupReadReceipt, groupMessageHasReadedHander);
      return () => {
        IMSDK.off(CbEvents.OnRecvGroupReadReceipt, groupMessageHasReadedHander);
        cancelGetReadedMembers();
        cancelGetUnreadMembers();
      };
    }, []);

    const getMoreReadedList = () => {
      if (readedLoading || !hasMoreReaded.current || !conversationID) return;
      getReadedMembers(readedData.length);
    };

    const getMoreUnReadList = () => {
      const conversationID =
        useConversationStore.getState().currentConversation?.conversationID;
      if (unreadLoading || !hasMoreUnread.current || !conversationID) return;
      getUnreadMembers(unReadData.length);
    };

    return (
      <div className="flex h-72 w-[500px] flex-col overflow-hidden rounded-md">
        <div className="flex items-center justify-between bg-[var(--gap-text)] px-4 py-2">
          <span className="font-medium">{t("placeholder.isReadList")}</span>
          <CloseOutlined
            className="cursor-pointer !text-[#8e9aaf]"
            rev={undefined}
            onClick={closeOverlay}
          />
        </div>
        <div className="flex flex-1 px-2">
          <div className="flex flex-1 flex-col">
            <div className="flex items-center px-4 py-2.5">
              <span className="mr-1 text-[var(--primary)]">{unReadCount}</span>
              {t("placeholder.unread")}
            </div>
            {Boolean(unReadData.length) && (
              <Virtuoso
                className="flex-1 overflow-x-hidden"
                data={unReadData}
                computeItemKey={(_, member) => member.userID}
                endReached={getMoreUnReadList}
                components={{
                  Footer: () => (unreadLoading ? <div>loading...</div> : null),
                }}
                itemContent={(_, member) => <MemberItem member={member} />}
              />
            )}
          </div>
          <div className="w-3"></div>
          <div className="flex flex-1 flex-col">
            <div className="flex items-center px-4 py-2.5">
              <span className="mr-1 text-[var(--primary)]">{readedCount}</span>
              {t("placeholder.isRead")}
            </div>
            {Boolean(readedData.length) && (
              <Virtuoso
                className="flex-1 overflow-x-hidden"
                data={readedData ?? []}
                computeItemKey={(_, member) => member.userID}
                endReached={getMoreReadedList}
                components={{
                  Header: () => (readedLoading ? <div>loading...</div> : null),
                }}
                itemContent={(_, member) => <MemberItem member={member} />}
              />
            )}
          </div>
        </div>
      </div>
    );
  },
);

const MemberItem = ({ member }: { member: GroupMemberItem }) => (
  <div className="flex items-center rounded-md px-3 py-2 hover:bg-[var(--primary-active)]">
    <OIMAvatar size={24} src={member.faceURL} text={member.nickname} />
    <div className="ml-3 max-w-[80%] truncate text-xs">{member.nickname}</div>
  </div>
);
