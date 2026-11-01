import { t } from "i18next";
import { GroupItem, GroupMemberItem } from "@/im/types";
import { useEffect, useState } from "react";

import cancel from "@/assets/images/chatHeader/cancel.png";
import speaker from "@/assets/images/chatHeader/speaker.png";
import OIMAvatar from "@/components/OIMAvatar";
import { IMSDK } from "@/layout/MainContentWrap";

import styles from "./chat-header.module.scss";

const GroupAnnouncementCard = ({
  currentGroupInfo,
  conversationID,
  openGroupAnnouncementDrawer,
}: {
  currentGroupInfo?: GroupItem;
  conversationID?: string;
  openGroupAnnouncementDrawer: () => void;
}) => {
  const [notificationOpUser, setNotificationOpUser] = useState<GroupMemberItem>();

  useEffect(() => {
    if (!currentGroupInfo?.notificationUserID) return;

    const getNotificationOpUser = async () => {
      try {
        const { data } = await IMSDK.getSpecifiedGroupMembersInfo({
          groupID: currentGroupInfo.groupID,
          userIDList: [currentGroupInfo.notificationUserID],
        });
        setNotificationOpUser(data[0]);
      } catch (error) {
        console.error(error);
      }
    };
    getNotificationOpUser();
  }, [currentGroupInfo?.notificationUserID, currentGroupInfo?.groupID]);

  const notificationRead = () => {
    if (!conversationID) return;
    IMSDK.resetConversationGroupAtType(conversationID);
  };

  return (
    <div
      className={styles["notice-card"]}
      onDoubleClick={() => {
        openGroupAnnouncementDrawer();
        notificationRead();
      }}
    >
      <div className="flex items-center justify-between">
        <div className="flex items-center">
          <img width={20} src={speaker} alt="spear" />
          <span className="ml-2 text-[var(--primary)]">
            {t("placeholder.groupAnnouncement")}
          </span>
        </div>
        <img
          className="cursor-pointer"
          width={16}
          src={cancel}
          alt="cancel"
          onClick={notificationRead}
        />
      </div>

      <div className="mb-4 line-clamp-2 pt-3">{currentGroupInfo?.notification}</div>

      <div
        className="flex w-fit items-center"
        onClick={() =>
          window.userClick(notificationOpUser?.userID, notificationOpUser?.groupID)
        }
      >
        <OIMAvatar
          size={20}
          src={notificationOpUser?.faceURL}
          text={notificationOpUser?.nickname || t("placeholder.administrator")}
        />
        <div className="ml-2 max-w-[120px] truncate text-xs text-[var(--sub-text)]">
          {notificationOpUser?.nickname || t("placeholder.administrator")}
        </div>
      </div>
    </div>
  );
};

export default GroupAnnouncementCard;
