import { FC, useEffect, useState } from "react";

import speaker from "@/assets/images/speaker.png";

import { IMessageItemProps } from ".";
import { GroupItem } from "@openim/wasm-client-sdk";

interface GroupAnnouncementElem {
  group: GroupItem;
}

const AnnouncementRenderer: FC<IMessageItemProps> = ({
  message,
  getAnnouncementPusher,
}) => {
  const [notificationElem, setNotificationElem] = useState<GroupAnnouncementElem>({
    group: {} as GroupItem,
  });
  useEffect(() => {
    try {
      const data = JSON.parse(message.notificationElem.detail) as GroupAnnouncementElem;
      setNotificationElem(data);
      getAnnouncementPusher?.(data.group.notificationUserID);
    } catch (error) {
      console.log(error);
      console.log(message);
    }
  }, []);
  return (
    <div className="min-w-[240px] rounded-md border border-[var(--gap-text)] px-3 py-2.5">
      <div className="mb-1 flex items-center">
        <img width={20} src={speaker} alt="spear" />
        <span className="ml-2 text-[var(--primary)]">群公告</span>
      </div>
      <div className="text-break">{notificationElem.group.notification}</div>
    </div>
  );
};

export default AnnouncementRenderer;
