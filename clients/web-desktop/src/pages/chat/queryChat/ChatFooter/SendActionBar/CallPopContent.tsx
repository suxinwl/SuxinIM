import { t } from "i18next";
import { SessionType } from "@openim/wasm-client-sdk";
import { PublicUserItem } from "@/im/types";
import { memo } from "react";
import { v4 as uuidV4 } from "uuid";

import { modal } from "@/AntdGlobalComp";
import call_audio from "@/assets/images/chatFooter/call_audio.png";
import call_video from "@/assets/images/chatFooter/call_video.png";
import { IMSDK } from "@/layout/MainContentWrap";
import { useConversationStore, useUserStore } from "@/store";
import emitter from "@/utils/events";
import { isGroupSession } from "@/utils/imCommon";

import { CallingRoomData } from "../../ChatHeader/GroupCallingCard";

const callList = [
  {
    idx: 0,
    title: t("placeholder.videoCall"),
    icon: call_video,
  },
  {
    idx: 1,
    title: t("placeholder.voiceCall"),
    icon: call_audio,
  },
];

const CallPopContent = ({
  userInfo,
  callClick,
}: {
  userInfo?: PublicUserItem;
  callClick?: (idx: number) => void;
}) => {
  const prepareCall = async (idx: number) => {
    const conversation = useConversationStore.getState().currentConversation!;
    const mediaType = idx ? "audio" : "video";
    if (!userInfo && isGroupSession(conversation.conversationType)) {
      const { data } = await IMSDK.signalingGetRoomByGroupID<CallingRoomData>(
        conversation.groupID,
      );
      if (data.invitation) {
        modal.confirm({
          title: t("placeholder.hint"),
          content: t("toast.isJoinCalling"),
          onOk: () => {
            emitter.emit("OPEN_RTC_MODAL", {
              invitation: data.invitation,
              participant: data.participant?.[0],
              isJoin: true,
            });
          },
        });
        return;
      }
      emitter.emit("OPEN_CHOOSE_MODAL", {
        type: "RTC_INVITE",
        extraData: mediaType,
      });
      callClick?.(idx);
      return;
    }
    emitter.emit("OPEN_RTC_MODAL", {
      invitation: {
        inviterUserID: useUserStore.getState().selfInfo.userID,
        inviteeUserIDList: [userInfo?.userID ?? conversation.userID],
        groupID: "",
        roomID: uuidV4(),
        timeout: 60,
        mediaType,
        sessionType: SessionType.Single,
        platformID: window.electronAPI?.getPlatform() ?? 5,
      },
      participant: {
        userInfo: {
          nickname: userInfo?.nickname ?? conversation.showName,
          userID: userInfo?.userID ?? conversation.userID,
          faceURL: userInfo?.faceURL ?? conversation.faceURL,
          ex: "",
        },
      },
    });
    callClick?.(idx);
  };
  return (
    <div className="p-1">
      {callList.map((item) => (
        <div
          className="flex cursor-pointer items-center rounded px-3 py-2 text-xs hover:bg-[var(--primary-active)]"
          key={item.title}
          onClick={() => prepareCall(item.idx)}
        >
          <img width={20} src={item.icon} alt="call_video" />
          <div className="ml-3 text-[#515E70]">{item.title}</div>
        </div>
      ))}
    </div>
  );
};
export default memo(CallPopContent);
