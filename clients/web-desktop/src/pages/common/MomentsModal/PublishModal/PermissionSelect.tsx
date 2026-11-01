import { CheckOutlined } from "@ant-design/icons";
import { Button } from "antd";
import clsx from "clsx";
import i18n, { t } from "i18next";
import { useEffect, useState } from "react";

import { CheckListItem } from "@/pages/common/ChooseModal/ChooseBox/CheckItem";
import { MomentPermission } from "@/types/moment";
import emitter, { SelectUserParams } from "@/utils/events";

const permissionMenus = [
  {
    title: t("placeholder.public"),
    subTitle: t("placeholder.publicToast"),
    permission: MomentPermission.Public,
    showAssignUser: false,
  },
  {
    title: t("placeholder.partlyVisible"),
    subTitle: t("placeholder.partlyVisibleToast"),
    permission: MomentPermission.AssignCanSee,
    showAssignUser: true,
  },
  {
    title: t("placeholder.hiddenSome"),
    subTitle: t("placeholder.hiddenSomeToast"),
    permission: MomentPermission.AssignCanNotSee,
    showAssignUser: true,
  },
  {
    title: t("placeholder.privacy"),
    subTitle: t("placeholder.privacyToast"),
    permission: MomentPermission.Private,
    showAssignUser: false,
  },
];

i18n.on("languageChanged", () => {
  permissionMenus[0].title = t("placeholder.public");
  permissionMenus[0].subTitle = t("placeholder.publicToast");
  permissionMenus[1].title = t("placeholder.partlyVisible");
  permissionMenus[1].subTitle = t("placeholder.partlyVisibleToast");
  permissionMenus[2].title = t("placeholder.hiddenSome");
  permissionMenus[2].subTitle = t("placeholder.hiddenSomeToast");
  permissionMenus[3].title = t("placeholder.privacy");
  permissionMenus[3].subTitle = t("placeholder.privacyToast");
});

const PermissionSelect = ({
  permission,
  assignUserList,
  assignGroupList,
  updatePermisson,
  updateShowPrepare,
  updateAssignUserList,
  updateAssignGroupList,
}: {
  permission: MomentPermission;
  assignUserList: CheckListItem[];
  assignGroupList: CheckListItem[];
  updatePermisson: (permission: MomentPermission) => void;
  updateShowPrepare: (isPrepare: boolean) => void;
  updateAssignUserList: (users: CheckListItem[]) => void;
  updateAssignGroupList: (groups: CheckListItem[]) => void;
}) => {
  const [tempPermission, setTempPermission] = useState<MomentPermission>(permission);
  const [tempAssignUserList, setTempAssignUserList] = useState<CheckListItem[]>([
    ...assignUserList,
  ]);
  const [tempAssignGroupList, setTempAssignGroupList] = useState<CheckListItem[]>([
    ...assignGroupList,
  ]);

  const assignList = [...tempAssignUserList, ...tempAssignGroupList];

  useEffect(() => {
    const handleData = (data: SelectUserParams) => {
      if (data.notConversation) return;

      setTempAssignUserList(data.choosedList.filter((item) => Boolean(item.userID)));
      setTempAssignGroupList(data.choosedList.filter((item) => Boolean(item.groupID)));
    };
    emitter.on("SELECT_USER", handleData);
    return () => {
      emitter.off("SELECT_USER", handleData);
    };
  }, []);

  const internalUpdatePermission = (per: MomentPermission) => {
    const isSamePermission = per === tempPermission;
    if (!isSamePermission) setTempPermission(per);
    if (
      per === MomentPermission.AssignCanSee ||
      per === MomentPermission.AssignCanNotSee
    ) {
      emitter.emit("OPEN_CHOOSE_MODAL", {
        type: "SELECT_USER",
        extraData: {
          notConversation: false,
          list: isSamePermission ? assignList : [],
        },
      });
    }
  };

  const confirmUpdate = () => {
    updatePermisson(tempPermission);
    updateAssignUserList(tempAssignUserList);
    updateAssignGroupList(tempAssignGroupList);
    updateShowPrepare(true);
  };

  return (
    <div className="flex flex-1 flex-col">
      <div className="mt-2 bg-white">
        {permissionMenus.map((menu) => (
          <PermissionItem
            key={menu.permission}
            title={menu.title}
            subTitle={menu.subTitle}
            checked={tempPermission === menu.permission}
            showAssignUser={menu.showAssignUser && tempPermission === menu.permission}
            assignList={assignList}
            onClick={() => internalUpdatePermission(menu.permission)}
          />
        ))}
      </div>

      <div className="my-2 flex flex-1 items-end justify-center">
        <Button
          type="primary"
          ghost
          className="m-2 w-full"
          onClick={() => updateShowPrepare(true)}
        >
          {t("cancel")}
        </Button>
        <Button type="primary" className="m-2 w-full" onClick={confirmUpdate}>
          {t("placeholder.save")}
        </Button>
      </div>
    </div>
  );
};

export default PermissionSelect;

const PermissionItem = ({
  title,
  subTitle,
  checked,
  showAssignUser,
  assignList,
  onClick,
}: {
  title: string;
  subTitle: string;
  checked: boolean;
  showAssignUser?: boolean;
  assignList?: CheckListItem[];
  onClick: () => void;
}) => {
  const assignStr = assignList
    ?.map((item) => item.nickname || item.groupName || item.showName)
    .join("、");

  return (
    <div
      className="flex cursor-pointer border-b border-[var(--chat-bubble)] px-3 py-2"
      onClick={onClick}
    >
      <div className="mr-3 pt-3">
        <CheckOutlined
          className={clsx(
            "invisible font-medium text-[#0289fa]",
            checked && "!visible",
          )}
        />
      </div>
      <div>
        <div>{title}</div>
        <div className="mt-1 text-xs text-[var(--sub-text)]">{subTitle}</div>
        {showAssignUser && (
          <div className="mt-1 line-clamp-2 text-xs text-[#0289fa]">{assignStr}</div>
        )}
      </div>
    </div>
  );
};
