import { AllowType } from "@openim/wasm-client-sdk";
import { useEffect } from "react";
import { Outlet, useLocation, useNavigate } from "react-router-dom";

import { useConversationStore, useUserStore } from "@/store";
import emitter from "@/utils/events";
import { checkNotificationPermission } from "@/utils/imCommon";
import { getImageCache, getIMToken, getIMUserID } from "@/utils/storage";

import { useAutoUpdate } from "./useAutoUpdate";

export { ImFacade as IMSDK } from "@/im/ImFacade";

export const MainContentWrap = () => {
  const updateAppSettings = useUserStore((state) => state.updateAppSettings);
  const getAppConfigByReq = useUserStore((state) => state.getAppConfigByReq);
  const initImageCache = useUserStore((state) => state.initImageCache);

  const navigate = useNavigate();
  const location = useLocation();

  const updateGate = useAutoUpdate();

  useEffect(() => {
    const loginCheck = async () => {
      const IMToken = await getIMToken();
      const IMUserID = await getIMUserID();
      if (!IMToken || !IMUserID) {
        navigate("/login");
        return;
      }
    };

    loginCheck();
  }, [location.pathname]);

  useEffect(() => {
    window.userClick = (userID?: string, groupID?: string) => {
      if (!userID || userID === "AtAllTag") return;

      const currentGroupInfo = useConversationStore.getState().currentGroupInfo;

      if (groupID && currentGroupInfo?.lookMemberInfo === AllowType.NotAllowed) {
        return;
      }

      emitter.emit("OPEN_USER_CARD", {
        userID,
        groupID,
        isSelf: userID === useUserStore.getState().selfInfo.userID,
        notAdd:
          Boolean(groupID) &&
          currentGroupInfo?.applyMemberFriend === AllowType.NotAllowed,
      });
    };
  }, []);

  useEffect(() => {
    const initSettingStore = async () => {
      if (!window.electronAPI) return;
      updateAppSettings({
        closeAction:
          (await window.electronAPI?.ipcInvoke("getKeyStore", {
            key: "closeAction",
          })) || "miniSize",
      });
      const cache = await getImageCache();
      initImageCache(cache);
      window.electronAPI?.ipcInvoke("main-win-ready");
    };

    initSettingStore();
    getAppConfigByReq();
    checkNotificationPermission();
  }, []);

  return updateGate || <Outlet />;
};
