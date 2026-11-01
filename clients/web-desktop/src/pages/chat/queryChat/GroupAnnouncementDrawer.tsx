import { Button, Divider, Drawer, Input } from "antd";
import { t } from "i18next";
import { GroupItem, GroupMemberItem } from "@/im/types";
import {
  forwardRef,
  ForwardRefRenderFunction,
  memo,
  useEffect,
  useReducer,
  useState,
} from "react";

import empty_announcement from "@/assets/images/chatSetting/empty_announcement.png";
import OIMAvatar from "@/components/OIMAvatar";
import { useCurrentMemberRole } from "@/hooks/useCurrentMemberRole";
import { OverlayVisibleHandle, useOverlayVisible } from "@/hooks/useOverlayVisible";
import { IMSDK } from "@/layout/MainContentWrap";
import { useConversationStore } from "@/store";
import { feedbackToast, formatBr } from "@/utils/common";
import { formatMessageTime } from "@/utils/imCommon";

const initialState = {
  editing: false,
  loading: false,
};

type ActionType = "editing" | "loading" | "done";

const reducer = (state: typeof initialState, action: { type: ActionType }) => {
  switch (action.type) {
    case "editing":
      return { ...state, editing: true, loading: false };
    case "loading":
      return { ...state, editing: true, loading: true };
    case "done":
      return { ...state, editing: false, loading: false };
    default:
      throw new Error(`Unhandled action type: ${action.type}`);
  }
};

const GroupAnnouncementDrawer: ForwardRefRenderFunction<
  OverlayVisibleHandle,
  unknown
> = (_, ref) => {
  const [notification, setNotification] = useState<string>();
  const [editState, dispatch] = useReducer(reducer, initialState);
  const currentGroup = useConversationStore((state) => state.currentGroupInfo);
  const { isOverlayOpen, closeOverlay } = useOverlayVisible(ref);

  const { isNomal } = useCurrentMemberRole();

  useEffect(() => {
    setNotification(currentGroup?.notification);
  }, [currentGroup?.notification]);

  const editOrPublish = async () => {
    if (!editState.editing) {
      setNotification(currentGroup?.notification);
      dispatch({ type: "editing" });
      return;
    }
    dispatch({ type: "loading" });

    try {
      await IMSDK.setGroupInfo({
        groupID: currentGroup!.groupID,
        notification,
      });
    } catch (error) {
      feedbackToast({ error });
    }
    dispatch({ type: "done" });
  };

  return (
    <Drawer
      title={t("placeholder.groupAnnouncement")}
      placement="right"
      rootClassName="chat-drawer"
      onClose={closeOverlay}
      open={isOverlayOpen}
      afterOpenChange={(vis) => {
        if (!vis) {
          dispatch({ type: "done" });
        }
      }}
      maskClassName="opacity-0"
      maskMotion={{
        visible: false,
      }}
      width={450}
      getContainer={"#chat-container"}
    >
      <div className="relative m-5.5 flex h-full flex-col justify-center">
        <div className="flex flex-1 flex-col overflow-hidden">
          {!editState.editing ? (
            <NotificationContent currentGroup={currentGroup} />
          ) : (
            <Input.TextArea
              showCount
              value={notification}
              maxLength={250}
              spellCheck={false}
              bordered={false}
              placeholder={t("placeholder.pleaseEnter")}
              style={{ height: "72%", resize: "none" }}
              styles={{
                textarea: {
                  padding: 0,
                },
              }}
              onChange={(e) => setNotification(e.target.value)}
            />
          )}

          <div className="h-20" />
        </div>
        {isNomal ? (
          <Divider className="border-1 absolute bottom-6 border-[var(--gap-text)] px-20 !text-xs !text-[var(--sub-text)]">
            {t("placeholder.needManageEdit")}
          </Divider>
        ) : (
          <div className="absolute bottom-6 right-0">
            <Button
              className="px-5"
              type="primary"
              onClick={editOrPublish}
              loading={editState.loading}
            >
              {editState.editing ? t("placeholder.publish") : t("placeholder.edit")}
            </Button>
          </div>
        )}
      </div>
    </Drawer>
  );
};

export default memo(forwardRef(GroupAnnouncementDrawer));

const NotificationContent = ({ currentGroup }: { currentGroup?: GroupItem }) => {
  const [notificationOpUser, setNotificationOpUser] = useState<GroupMemberItem>();

  useEffect(() => {
    if (!currentGroup?.notificationUserID) return;

    const getNotificationOpUser = async () => {
      try {
        const { data } = await IMSDK.getSpecifiedGroupMembersInfo({
          groupID: currentGroup.groupID,
          userIDList: [currentGroup.notificationUserID],
        });
        setNotificationOpUser(data[0]);
      } catch (error) {
        console.error(error);
      }
    };
    getNotificationOpUser();
  }, [currentGroup?.notificationUserID, currentGroup?.groupID]);

  return (
    <>
      {currentGroup?.notification ? (
        <>
          <div className="flex">
            <OIMAvatar
              src={notificationOpUser?.faceURL}
              text={notificationOpUser?.nickname || t("placeholder.administrator")}
            />
            <div className="ml-3">
              <div className="mb-1.5 max-w-[200px] truncate">
                {notificationOpUser?.nickname || t("placeholder.administrator")}
              </div>
              <div className="text-xs text-[var(--sub-text)]">
                {formatMessageTime(Number(currentGroup?.notificationUpdateTime))}
              </div>
            </div>
          </div>
          <div
            className="text-break mt-3 flex-1 overflow-y-auto"
            dangerouslySetInnerHTML={{
              __html: formatBr(currentGroup?.notification || ""),
            }}
          ></div>
        </>
      ) : (
        <div className="flex h-full flex-col items-center justify-center">
          <img width={176} src={empty_announcement} alt="empty_announcement" />
          <div className="mt-9 text-xs text-[var(--sub-text)]">
            {t("placeholder.noGroupAnnouncement")}
          </div>
        </div>
      )}
    </>
  );
};
