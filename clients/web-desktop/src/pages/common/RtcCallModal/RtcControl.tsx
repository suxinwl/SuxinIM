import {
  TrackToggle,
  useLocalParticipant,
  useRoomContext,
} from "@livekit/components-react";
import clsx from "clsx";
import { t } from "i18next";
import { ConnectionState, RemoteParticipant, RoomEvent, Track } from "livekit-client";
import { CbEvents } from "@openim/wasm-client-sdk";
import { SessionType } from "@openim/wasm-client-sdk";
import { RtcInvite, WSEvent } from "@/im/types";
import { useEffect, useRef } from "react";

import rtc_accept from "@/assets/images/rtc/rtc_accept.png";
import rtc_camera from "@/assets/images/rtc/rtc_camera.png";
import rtc_camera_off from "@/assets/images/rtc/rtc_camera_off.png";
import rtc_hungup from "@/assets/images/rtc/rtc_hungup.png";
import rtc_mic from "@/assets/images/rtc/rtc_mic.png";
import rtc_mic_off from "@/assets/images/rtc/rtc_mic_off.png";
import { CustomMessageType, RtcMessageStatus } from "@/constants";
import { IMSDK } from "@/layout/MainContentWrap";
import {
  ExMessageItem,
  useConversationStore,
  useMessageStore,
  useUserStore,
} from "@/store";
import { feedbackToast } from "@/utils/common";
import emitter from "@/utils/events";

import { CounterHandle, ForwardCounter } from "./Counter";
import { AuthData, RtcInviteResults } from "./data";

interface IRtcControlProps {
  isWaiting: boolean;
  isRecv: boolean;
  isConnected: boolean;
  showMini: boolean;
  invitation: RtcInvite;
  connectRtc: (data?: AuthData) => void;
  closeOverlay: () => void;
}
export const RtcControl = ({
  isWaiting,
  isRecv,
  isConnected,
  showMini,
  invitation,
  connectRtc,
  closeOverlay,
}: IRtcControlProps) => {
  const room = useRoomContext();
  const localParticipantState = useLocalParticipant();
  const opUserID = useUserStore((state) => state.selfInfo.userID);
  const hasJoin = useRef(false);
  const counterRef = useRef<CounterHandle>(null);
  const pushNewMessage = useMessageStore((state) => state.pushNewMessage);

  const isVideoCall = invitation.mediaType === "video";
  const isSingle = invitation?.sessionType === SessionType.Single;

  useEffect(() => {
    const acceptHandler = ({
      data: {
        invitation: { roomID },
      },
    }: WSEvent<{ invitation: RtcInvite }>) => {
      if (invitation.roomID !== roomID) return;

      if (isSingle) {
        connectRtc();
      } else {
        hasJoin.current = true;
      }
    };
    const rejectHandler = ({
      data: {
        invitation: { roomID },
      },
    }: WSEvent<{ invitation: RtcInvite }>) => {
      if (invitation.roomID !== roomID || !isSingle) return;
      insertRtcMessage(RtcMessageStatus.Refused);
      closeOverlay();
    };
    const hangupHandler = ({
      data: {
        invitation: { roomID },
      },
    }: WSEvent<{ invitation: RtcInvite }>) => {
      if ((!isSingle && !isWaiting) || invitation.roomID !== roomID) return;
      insertRtcMessage(RtcMessageStatus.Successed);
      room.disconnect();
      closeOverlay();
    };
    const timeoutHandler = ({
      data: {
        invitation: { roomID },
      },
    }: WSEvent<{ invitation: RtcInvite }>) => {
      if (invitation.roomID !== roomID) return;

      if (isSingle || !hasJoin.current) {
        insertRtcMessage(RtcMessageStatus.Timeout);
        IMSDK.signalingCancel({
          opUserID,
          invitation,
        });
        if (!isSingle) room.disconnect();
        closeOverlay();
      }
    };
    const cancelHandler = ({
      data: {
        invitation: { roomID },
      },
    }: WSEvent<{ invitation: RtcInvite }>) => {
      if (invitation.roomID !== roomID) return;
      if (!isSingle && !isWaiting) return;
      insertRtcMessage(RtcMessageStatus.Canceled);
      closeOverlay();
    };
    const participantDisconnectedHandler = (remoteParticipant: RemoteParticipant) => {
      if (!isSingle) return;

      const identity = remoteParticipant.identity;
      if (
        identity === invitation.inviterUserID ||
        identity === invitation.inviteeUserIDList[0]
      ) {
        insertRtcMessage(RtcMessageStatus.Successed);
        room.disconnect();
      }
    };
    const handleByOtherDevice = ({
      data: {
        invitation: { roomID },
      },
    }: WSEvent<{ invitation: RtcInvite }>) => {
      if (invitation.roomID !== roomID) return;
      closeOverlay();
      insertRtcMessage(RtcMessageStatus.HandleByOtherDevice);
    };

    IMSDK.on(CbEvents.OnInviteeAccepted, acceptHandler);
    IMSDK.on(CbEvents.OnInviteeRejected, rejectHandler);
    IMSDK.on(CbEvents.OnHangUp, hangupHandler);
    IMSDK.on(CbEvents.OnInvitationCancelled, cancelHandler);
    IMSDK.on(CbEvents.OnInvitationTimeout, timeoutHandler);
    IMSDK.on(CbEvents.OnInviteeAcceptedByOtherDevice, handleByOtherDevice);
    IMSDK.on(CbEvents.OnInviteeRejectedByOtherDevice, handleByOtherDevice);
    room.on(RoomEvent.ParticipantDisconnected, participantDisconnectedHandler);
    return () => {
      IMSDK.off(CbEvents.OnInviteeAccepted, acceptHandler);
      IMSDK.off(CbEvents.OnInviteeRejected, rejectHandler);
      IMSDK.off(CbEvents.OnHangUp, hangupHandler);
      IMSDK.off(CbEvents.OnInvitationCancelled, cancelHandler);
      IMSDK.off(CbEvents.OnInvitationTimeout, timeoutHandler);
      IMSDK.off(CbEvents.OnInviteeAcceptedByOtherDevice, handleByOtherDevice);
      IMSDK.off(CbEvents.OnInviteeRejectedByOtherDevice, handleByOtherDevice);
      room.off(RoomEvent.ParticipantDisconnected, participantDisconnectedHandler);
      hasJoin.current = false;
    };
  }, [isSingle, room, invitation.roomID, isWaiting]);

  const disconnect = () => {
    const data = {
      opUserID,
      invitation,
    };
    if (isWaiting) {
      const funcName = isRecv ? "signalingReject" : "signalingCancel";
      IMSDK[funcName](data);
      insertRtcMessage(isRecv ? RtcMessageStatus.Refused : RtcMessageStatus.Canceled);
      closeOverlay();
      return;
    }
    if (!invitation.groupID) {
      IMSDK.signalingHungUp(data);
    }
    insertRtcMessage(RtcMessageStatus.Successed);
    room.disconnect();
  };

  const acceptInvitation = () => {
    IMSDK.signalingAccept<RtcInviteResults>({
      opUserID,
      invitation,
    })
      .then(({ data }) => connectRtc(data))
      .catch((error) => {
        feedbackToast({ msg: t("toast.byInviteUserFailed"), error });
        closeOverlay();
      });
  };

  const insertRtcMessage = async (status: RtcMessageStatus) => {
    if (invitation.sessionType !== SessionType.Single) return;
    const message = (
      await IMSDK.createCustomMessage({
        data: JSON.stringify({
          customType: CustomMessageType.Call,
          data: {
            duration: counterRef.current?.getTimeStr() ?? "",
            mediaType: invitation.mediaType,
            status,
          },
        }),
        extension: "",
        description: "",
      })
    ).data;
    if (!message) return;

    const fullMessage = (
      await IMSDK.insertSingleMessageToLocalStorage<ExMessageItem>({
        recvID: invitation.inviteeUserIDList[0],
        sendID: invitation.inviterUserID,
        message,
      })
    ).data;

    const conversation = useConversationStore.getState().currentConversation;
    if (
      invitation.inviterUserID === conversation?.userID ||
      invitation.inviteeUserIDList[0] === conversation?.userID
    ) {
      pushNewMessage(fullMessage);
      emitter.emit("CHAT_LIST_SCROLL_TO_BOTTOM", false);
    }
  };

  return (
    <div className="ignore-drag absolute bottom-[6%] z-10 flex justify-center">
      {!isWaiting && (
        <ForwardCounter
          ref={counterRef}
          className={clsx("absolute -top-8", { invisible: showMini })}
          isConnected={isConnected}
        />
      )}
      {!showMini && (
        <>
          {!isWaiting && (
            <TrackToggle
              className="flex cursor-pointer flex-col items-center !justify-start !gap-0 !p-0"
              source={Track.Source.Microphone}
              showIcon={false}
            >
              <img
                width={48}
                src={localParticipantState.isMicrophoneEnabled ? rtc_mic : rtc_mic_off}
                alt=""
              />
              <span className="mt-2 text-xs text-white">
                {t("placeholder.microphone")}
              </span>
            </TrackToggle>
          )}
          <div
            className={clsx("ml-12 flex cursor-pointer flex-col items-center", {
              "mr-12": isVideoCall,
              "!mx-0": !isRecv && isWaiting,
            })}
            onClick={disconnect}
          >
            <img width={48} src={rtc_hungup} alt="" />
            <span
              className={clsx("mt-2 text-xs text-white", {
                "!text-[var(--sub-text)]": isWaiting,
              })}
            >
              {isWaiting ? t("cancel") : t("hangUp")}
            </span>
          </div>
          {isRecv && isWaiting && (
            <div
              className="mx-12 flex cursor-pointer flex-col items-center"
              onClick={acceptInvitation}
            >
              <img width={48} src={rtc_accept} alt="" />
              <span
                className={clsx("mt-2 text-xs text-white", {
                  "!text-[var(--sub-text)]": isWaiting,
                })}
              >
                {t("answer")}
              </span>
            </div>
          )}
          {!isWaiting && isVideoCall && (
            <TrackToggle
              className="flex cursor-pointer flex-col items-center justify-start !gap-0 !p-0"
              source={Track.Source.Camera}
              showIcon={false}
            >
              <img
                width={48}
                src={
                  localParticipantState.isCameraEnabled ? rtc_camera : rtc_camera_off
                }
                alt=""
              />
              <span className="mt-2 text-xs text-white">{t("placeholder.camera")}</span>
            </TrackToggle>
          )}
        </>
      )}
    </div>
  );
};
