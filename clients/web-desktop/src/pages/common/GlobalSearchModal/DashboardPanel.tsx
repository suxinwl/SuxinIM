import { RightOutlined } from "@ant-design/icons";
import { Empty, Spin } from "antd";
import clsx from "clsx";
import { t } from "i18next";
import { SessionType } from "@openim/wasm-client-sdk";
import {
  FriendUserItem,
  GroupItem,
  MessageItem,
} from "@/im/types";
import { useCallback } from "react";

import { useConversationToggle } from "@/hooks/useConversationToggle";
import { useUserStore } from "@/store";

import { ChatLogsItem, SearchData, TabKey } from ".";
import { ChatLogsRender } from "./ChatLogsPanel";
import { ContactItem, ContactRender } from "./ContactPanel";
import { FileRender } from "./FilePanel";
import { useKeyPage } from "./useKeyPage";

const ShowMoreAction = ({ onClick }: { onClick: () => void }) => (
  <div className="flex cursor-pointer items-center" onClick={onClick}>
    <span className="text-[var(--primary)]">{t("placeholder.viewMore")}</span>
    <RightOutlined className="text-[var(--primary)]" />
  </div>
);

const DashboardPanel = ({
  friends,
  groups,
  chatLogs,
  historyFiles,
  isActive,
  toggleTab,
  closeOverlay,
  toggleChatLogActive,
}: {
  friends: SearchData<FriendUserItem>;
  groups: SearchData<GroupItem>;
  chatLogs: SearchData<ChatLogsItem>;
  historyFiles: SearchData<MessageItem>;
  isActive: boolean;
  toggleTab: (key: TabKey) => void;
  closeOverlay: () => void;
  toggleChatLogActive: (idx: number) => void;
}) => {
  const { toSpecifiedConversation } = useConversationToggle();

  const { activeIdx } = useKeyPage({
    isActive,
    maxIndex:
      friends.data.slice(0, 3).length +
      groups.data.slice(0, 3).length +
      chatLogs.data.slice(0, 3).length +
      historyFiles.data.slice(0, 3).length,
    elPrefix: "#dashboard-item-",
    callback: (idx) => {
      const tmpArr = [
        ...friends.data.slice(0, 3),
        ...groups.data.slice(0, 3),
        ...chatLogs.data.slice(0, 3),
        ...historyFiles.data.slice(0, 3),
      ];
      const select = tmpArr[idx];
      if (!select) return;

      if (
        (select as ContactItem).ownerUserID ||
        (select as ContactItem).creatorUserID
      ) {
        contactJumpToConversation(select as ContactItem);
      }
      if ((select as ChatLogsItem).conversationID) {
        messageJumpToConversation(select as ChatLogsItem);
      }
    },
  });

  const contactJumpToConversation = useCallback((item: ContactItem) => {
    toSpecifiedConversation({
      sourceID: item.userID || item.groupID || "",
      sessionType: item.groupID ? SessionType.WorkingGroup : SessionType.Single,
    });
    closeOverlay();
  }, []);

  const messageJumpToConversation = useCallback((item: ChatLogsItem) => {
    const idx = chatLogs.data.findIndex(
      (logs) => logs.conversationID === item.conversationID,
    );
    toggleTab("ChatLogs");
    setTimeout(() => toggleChatLogActive(idx));
  }, []);

  const loading =
    friends.loading || groups.loading || chatLogs.loading || historyFiles.loading;
  const isEmpty = !(
    friends.data.length +
    groups.data.length +
    chatLogs.data.length +
    historyFiles.data.length
  );
  const initailGroupIdx = friends.data.slice(0, 3).length;
  const initailChatLogsIdx = initailGroupIdx + groups.data.slice(0, 3).length;
  const initailHistoryFilesIdx = initailChatLogsIdx + chatLogs.data.slice(0, 3).length;

  return (
    <Spin wrapperClassName="h-full" spinning={loading}>
      <div className="h-full overflow-y-auto p-3">
        {isEmpty && <Empty className="mt-[25%]" image={Empty.PRESENTED_IMAGE_SIMPLE} />}
        {friends.data.length > 0 && (
          <div>
            <div className="mx-3 my-1 flex justify-between">
              <div>{t("placeholder.contacts")}</div>
              {friends.data.length > 3 && (
                <ShowMoreAction onClick={() => toggleTab("Friends")} />
              )}
            </div>
            {friends.data.slice(0, 3).map((friend, idx) => (
              <ContactRender
                key={friend.userID}
                id={`dashboard-item-${idx}`}
                isActive={idx === activeIdx}
                item={friend}
                onClick={contactJumpToConversation}
              />
            ))}
          </div>
        )}

        {groups.data.length > 0 && (
          <div
            className={clsx("mt-2 border-[var(--gap-text)]", {
              "border-t": initailGroupIdx,
            })}
          >
            <div className="mx-3 mb-1 mt-3 flex justify-between">
              <div>{t("placeholder.myGroup")}</div>
              {groups.data.length > 3 && (
                <ShowMoreAction onClick={() => toggleTab("Groups")} />
              )}
            </div>
            {groups.data.slice(0, 3).map((group, idx) => (
              <ContactRender
                key={group.groupID}
                id={`dashboard-item-${initailGroupIdx + idx}`}
                isActive={activeIdx === initailGroupIdx + idx}
                item={group}
                onClick={contactJumpToConversation}
              />
            ))}
          </div>
        )}

        {chatLogs.data.length > 0 && (
          <div
            className={clsx("mt-2 border-[var(--gap-text)]", {
              "border-t": initailChatLogsIdx,
            })}
          >
            <div className="mx-3 mb-1 mt-3 flex justify-between">
              <div>{t("placeholder.messageHistory")}</div>
              {chatLogs.data.length > 3 && (
                <ShowMoreAction onClick={() => toggleTab("ChatLogs")} />
              )}
            </div>
            {chatLogs.data.slice(0, 3).map((chatLog, idx) => (
              <ChatLogsRender
                key={chatLog.conversationID}
                id={`dashboard-item-${initailChatLogsIdx + idx}`}
                isActive={activeIdx === initailChatLogsIdx + idx}
                result={chatLog}
                onClick={messageJumpToConversation as any}
              />
            ))}
          </div>
        )}

        {historyFiles.data.length > 0 && (
          <div
            className={clsx("mt-2 border-[var(--gap-text)]", {
              "border-t": initailHistoryFilesIdx,
            })}
          >
            <div className="mx-3 mb-1 mt-3 flex justify-between">
              <div>{t("placeholder.document")}</div>
              {historyFiles.data.length > 3 && (
                <ShowMoreAction onClick={() => toggleTab("HistoryFiles")} />
              )}
            </div>
            {historyFiles.data.slice(0, 3).map((fileMessage, idx) => (
              <FileRender
                key={fileMessage.clientMsgID}
                id={`dashboard-item-${initailHistoryFilesIdx + idx}`}
                isActive={activeIdx === initailHistoryFilesIdx + idx}
                message={fileMessage}
              />
            ))}
          </div>
        )}
      </div>
    </Spin>
  );
};

export default DashboardPanel;
