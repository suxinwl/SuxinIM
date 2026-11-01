import { Layout, Tooltip } from "antd";
import clsx from "clsx";
import i18n, { t } from "i18next";
import { CbEvents } from "@openim/wasm-client-sdk";
import { GroupAtType, OnlineState, Platform, SessionType } from "@openim/wasm-client-sdk";
import { UserOnlineState, WSEvent } from "@/im/types";
import { memo, useCallback, useEffect, useRef, useState } from "react";

import group_member from "@/assets/images/chatHeader/group_member.png";
import group_notice from "@/assets/images/chatHeader/group_notice.png";
import launch_group from "@/assets/images/chatHeader/launch_group.png";
import search_history from "@/assets/images/chatHeader/search_history.png";
import settings from "@/assets/images/chatHeader/settings.png";
import OIMAvatar from "@/components/OIMAvatar";
import { OverlayVisibleHandle } from "@/hooks/useOverlayVisible";
import { IMSDK } from "@/layout/MainContentWrap";
import { useConversationStore, useUserStore } from "@/store";
import emitter from "@/utils/events";
import { isGroupSession } from "@/utils/imCommon";

import GroupAnnouncementDrawer from "../GroupAnnouncementDrawer";
import GroupSetting from "../GroupSetting";
import SearchMessageDrawer from "../SearchMessageDrawer";
import SingleSetting from "../SingleSetting";
import styles from "./chat-header.module.scss";
import GroupAnnouncementCard from "./GroupAnnouncementCard";

const menuList = [
  {
    title: t("placeholder.groupAnnouncement"),
    icon: group_notice,
    idx: 0,
  },
  {
    title: t("placeholder.historyList"),
    icon: search_history,
    idx: 1,
  },
  // {
  //   title: t("placeholder.file"),
  //   icon: file_manage,
  //   idx: 2,
  // },
  {
    title: t("placeholder.createGroup"),
    icon: launch_group,
    idx: 3,
  },
  {
    title: t("placeholder.invitation"),
    icon: launch_group,
    idx: 4,
  },
  {
    title: t("placeholder.setting"),
    icon: settings,
    idx: 5,
  },
];

i18n.on("languageChanged", () => {
  menuList[0].title = t("placeholder.groupAnnouncement");
  menuList[1].title = t("placeholder.historyList");
  menuList[2].title = t("placeholder.createGroup");
  menuList[3].title = t("placeholder.invitation");
  menuList[4].title = t("placeholder.setting");
});

const ChatHeader = () => {
  const singleSettingRef = useRef<OverlayVisibleHandle>(null);
  const groupSettingRef = useRef<OverlayVisibleHandle>(null);
  const searchMessageRef = useRef<OverlayVisibleHandle>(null);
  const groupAnnouncementRef = useRef<OverlayVisibleHandle>(null);

  const currentConversation = useConversationStore(
    (state) => state.currentConversation,
  );
  const currentGroupInfo = useConversationStore((state) => state.currentGroupInfo);
  const inGroup = useConversationStore((state) =>
    Boolean(state.currentMemberInGroup?.groupID),
  );

  // locale re render
  useUserStore((state) => state.appSettings.locale);

  useEffect(() => {
    if (singleSettingRef.current?.isOverlayOpen) {
      singleSettingRef.current?.closeOverlay();
    }
    if (groupSettingRef.current?.isOverlayOpen) {
      groupSettingRef.current?.closeOverlay();
    }
    if (searchMessageRef.current?.isOverlayOpen) {
      searchMessageRef.current?.closeOverlay();
    }
  }, [currentConversation?.conversationID]);

  const openGroupAnnouncementDrawer = useCallback(() => {
    groupAnnouncementRef.current?.openOverlay();
  }, []);

  const menuClick = (idx: number) => {
    switch (idx) {
      case 0:
        groupAnnouncementRef.current?.openOverlay();
        break;
      case 1:
        searchMessageRef.current?.openOverlay();
        break;
      // case 2:
      //   break;
      case 3:
      case 4:
        emitter.emit("OPEN_CHOOSE_MODAL", {
          type: isSingle ? "CRATE_GROUP" : "INVITE_TO_GROUP",
          extraData: isSingle
            ? [{ ...currentConversation }]
            : currentConversation?.groupID,
        });
        break;
      case 5:
        if (isGroupSession(currentConversation?.conversationType)) {
          groupSettingRef.current?.openOverlay();
        } else {
          singleSettingRef.current?.openOverlay();
        }
        break;
      default:
        break;
    }
  };

  const showCard = () => {
    if (isSingle || isNotification) {
      window.userClick(currentConversation?.userID);
      return;
    }
    if (currentGroupInfo) {
      emitter.emit("OPEN_GROUP_CARD", currentGroupInfo);
    }
  };

  const isNotification =
    currentConversation?.conversationType === SessionType.Notification;
  const isSingle = currentConversation?.conversationType === SessionType.Single;

  const hasGroupAnnouncement =
    currentConversation?.groupAtType === GroupAtType.AtGroupNotice;

  return (
    <Layout.Header className="relative border-b border-b-[var(--gap-text)] !bg-white !px-3">
      <div className="flex h-full items-center leading-none">
        <div className="flex flex-1 items-center overflow-hidden">
          <OIMAvatar
            src={currentConversation?.faceURL}
            text={currentConversation?.showName}
            isgroup={Boolean(currentConversation?.groupID)}
            isnotification={isNotification}
            onClick={showCard}
          />
          <div
            className={clsx(
              "ml-3 flex !h-10.5 flex-1 flex-col justify-between overflow-hidden",
              isNotification && "!justify-center",
            )}
          >
            <div className="truncate text-base font-semibold">
              {currentConversation?.showName}
            </div>
            {isSingle && <OnlineOrTypingStatus userID={currentConversation?.userID} />}
            {!isSingle && !isNotification && (
              <div className="flex items-center text-xs text-[var(--sub-text)]">
                <img width={20} src={group_member} alt="member" />
                <span>{currentGroupInfo?.memberCount}</span>
              </div>
            )}
          </div>
        </div>

        {!isNotification && (
          <div className="mr-5 flex">
            {menuList.map((menu) => {
              if (
                (menu.idx === 0 || menu.idx === 4) &&
                (isSingle || (!inGroup && !isSingle))
              ) {
                return null;
              }
              if (menu.idx === 3 && !isSingle) {
                return null;
              }

              return (
                <Tooltip title={menu.title} key={menu.idx}>
                  <img
                    className="ml-5 cursor-pointer"
                    width={20}
                    src={menu.icon}
                    alt=""
                    onClick={() => menuClick(menu.idx)}
                  />
                </Tooltip>
              );
            })}
          </div>
        )}
      </div>
      {hasGroupAnnouncement && (
        <GroupAnnouncementCard
          currentGroupInfo={currentGroupInfo}
          conversationID={currentConversation?.conversationID}
          openGroupAnnouncementDrawer={openGroupAnnouncementDrawer}
        />
      )}
      {/* <GroupCallingCard currentGroupInfo={currentGroupInfo} /> */}
      <SingleSetting ref={singleSettingRef} />
      <GroupSetting ref={groupSettingRef} />
      <SearchMessageDrawer ref={searchMessageRef} />
      <GroupAnnouncementDrawer ref={groupAnnouncementRef} />
    </Layout.Header>
  );
};

export default memo(ChatHeader);

const OnlineOrTypingStatus = ({ userID }: { userID: string }) => {
  const [typing, setTyping] = useState(false);
  const [onlineState, setOnlineState] = useState<UserOnlineState>();
  const timer = useRef<NodeJS.Timeout | null>(null);

  useEffect(() => {
    const userStatusChangeHandler = ({ data }: WSEvent<UserOnlineState>) => {
      if (data.userID === userID) {
        setOnlineState(data);
      }
    };
    const inputStateHandler = ({data}: WSEvent<any>) => {
      if(data.userID!==userID)return;
      setTyping(Boolean(data.platformIDs?.length));
      if(timer.current)clearTimeout(timer.current);
      timer.current=setTimeout(()=>setTyping(false),5000);
    };
    IMSDK.on(CbEvents.OnConversationUserInputStatusChanged,inputStateHandler);
    IMSDK.on(CbEvents.OnUserStatusChanged, userStatusChangeHandler);
    IMSDK.subscribeUsersStatus([userID]).then(({ data }) => setOnlineState(data[0]));
    return () => {
      IMSDK.off(CbEvents.OnUserStatusChanged, userStatusChangeHandler);
      IMSDK.off(CbEvents.OnConversationUserInputStatusChanged,inputStateHandler);
      IMSDK.unsubscribeUsersStatus([userID]);
      setTyping(false);
    };
  }, [userID]);

  useEffect(() => {
    const typingHandler = () => {
      setTyping(true);
      timer.current = setTimeout(() => {
        if (timer.current) {
          clearTimeout(timer.current);
        }
        setTyping(false);
      }, 500);
    };
    emitter.on("TYPING_UPDATE", typingHandler);
    return () => {
      if (timer.current) {
        clearTimeout(timer.current);
      }
      emitter.off("TYPING_UPDATE", typingHandler);
    };
  }, []);

  return (
    <div className="flex items-center">
      {typing ? (
        <p className="text-xs text-[var(--sub-text)]">
          {t("placeholder.typing")} <span className={styles["dot-1"]}>.</span>
          <span className={styles["dot-2"]}>.</span>
          <span className={styles["dot-3"]}>.</span>
        </p>
      ) : (
        <>
          <i
            className={clsx(
              "mr-1.5 inline-block h-[6px] w-[6px] rounded-full bg-[#2ddd73]",
              {
                "bg-[#999]": onlineState?.status === OnlineState.Offline,
              },
            )}
          />
          <span className="text-xs text-[var(--sub-text)]">
            {platformToDetails(onlineState)}
          </span>
        </>
      )}
    </div>
  );
};

const platformMap: Record<Platform, string> = {
  1: "iOS",
  2: "Android",
  3: "Windows",
  4: "MacOSX",
  5: "Web",
  // @ts-ignore
  6: "MiniProgram",
  7: "Linux",
  8: "AndroidPad",
  9: "iPad",
};

const platformToDetails = (state?: UserOnlineState) => {
  if (!state || state.status === OnlineState.Offline) return t("placeholder.offLine");
  let string = "";
  state.platformIDs?.map((platform) => (string += `${platformMap[platform]}/`));
  return `${string.slice(0, -1)}${t("placeholder.online")}`;
};
