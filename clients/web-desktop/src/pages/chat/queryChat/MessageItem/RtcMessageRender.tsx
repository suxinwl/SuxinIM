import clsx from "clsx";
import { SessionType } from "@openim/wasm-client-sdk";
import { FC, memo } from "react";
import { useTranslation } from "react-i18next";
import { v4 as uuidV4 } from "uuid";

import rtc_audio_call from "@/assets/images/messageItem/rtc_audio_call.png";
import rtc_audio_call_rs from "@/assets/images/messageItem/rtc_audio_call_rs.png";
import rtc_video_call from "@/assets/images/messageItem/rtc_video_call.png";
import rtc_video_call_rs from "@/assets/images/messageItem/rtc_video_call_rs.png";
import { RtcMessageStatus } from "@/constants";
import { useConversationStore, useUserStore } from "@/store";
import emitter from "@/utils/events";

import { IMessageItemProps } from ".";
import styles from "./message-item.module.scss";

const RtcMessageRender: FC<IMessageItemProps> = ({ message, isSender, disabled }) => {
  const { t } = useTranslation();

  const messageData = JSON.parse(message.customElem.data).data;

  const reCall = () => {
    const conversation = useConversationStore.getState().currentConversation;
    emitter.emit("OPEN_RTC_MODAL", {
      invitation: {
        inviterUserID: useUserStore.getState().selfInfo.userID,
        inviteeUserIDList: [conversation!.userID],
        groupID: "",
        roomID: uuidV4(),
        timeout: 60,
        mediaType: messageData.mediaType,
        sessionType: SessionType.Single,
        platformID: window.electronAPI?.getPlatform() ?? 5,
      },
      participant: {
        userInfo: {
          nickname: conversation!.showName,
          userID: conversation!.userID,
          faceURL: conversation!.faceURL,
          ex: "",
        },
      },
    });
  };

  const getStatusStr = () => {
    switch (messageData.status) {
      case RtcMessageStatus.Canceled:
        return t("canceled");
      case RtcMessageStatus.Refused:
        return t("application.refused");
      case RtcMessageStatus.Timeout:
        return t("placeholder.callTimeout");
      case RtcMessageStatus.Successed:
        return t("placeholder.callTime");
      case RtcMessageStatus.HandleByOtherDevice:
        return t("placeholder.handleByOtherDevice");
      default:
        return "";
    }
  };

  const getIcon = () => {
    if (isSender) {
      return messageData.mediaType === "video" ? rtc_video_call : rtc_audio_call;
    }
    return messageData.mediaType === "video" ? rtc_video_call_rs : rtc_audio_call_rs;
  };

  return (
    <div
      className={clsx(
        styles.bubble,
        "flex cursor-pointer items-center !py-2",
        !isSender && "flex-row-reverse",
        disabled && "justify-end",
      )}
      onClick={reCall}
    >
      <img width={18} src={getIcon()} alt="" />
      <div className={clsx("ml-1.5 flex", { "ml-0 mr-1.5": !isSender })}>
        <div className="mr-1.5">{getStatusStr()}</div>
        {messageData.status === RtcMessageStatus.Successed && (
          <div>{messageData.duration}</div>
        )}
      </div>
    </div>
  );
};

export default memo(RtcMessageRender);
