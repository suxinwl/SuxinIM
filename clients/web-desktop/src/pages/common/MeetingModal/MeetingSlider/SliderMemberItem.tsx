import { useIsMuted } from "@livekit/components-react";
import { Popover } from "antd";
import clsx from "clsx";
import { t } from "i18next";
import { Participant, Track } from "livekit-client";
import { PublicUserItem } from "@/im/types";
import { memo, useState } from "react";

import meeting_slider_camera from "@/assets/images/rtc/meeting_slider_camera.png";
import meeting_slider_camera_off from "@/assets/images/rtc/meeting_slider_camera_off.png";
import meeting_slider_mic from "@/assets/images/rtc/meeting_slider_mic.png";
import meeting_slider_mic_off from "@/assets/images/rtc/meeting_slider_mic_off.png";
import meeting_slider_more from "@/assets/images/rtc/meeting_slider_more.png";
import OIMAvatar from "@/components/OIMAvatar";
import { IMSDK } from "@/layout/MainContentWrap";
import { useUserStore } from "@/store";

import { UpdateMeetingParams } from "../data";
import styles from "./meeting-slider.module.scss";

type SliderMemberItemProps = {
  roomID: string;
  participant: Participant;
  hostUserID: string;
  beWatchedUserIDList?: string[];
  pinedUserIDList?: string[];
};
export const SliderMemberItem = memo(
  ({
    roomID,
    participant,
    hostUserID,
    beWatchedUserIDList,
    pinedUserIDList,
  }: SliderMemberItemProps) => {
    const selfID = useUserStore((state) => state.selfInfo.userID);
    const [moreActionVisible, setMoreActionVisible] = useState(false);

    const isVideoMuted = useIsMuted({
      participant,
      source: Track.Source.Camera,
    });
    const isAudioMuted = useIsMuted({
      participant,
      source: Track.Source.Microphone,
    });

    const isHost = participant.identity === hostUserID;
    const isSelf = participant.identity === selfID;
    const selfIsHost = hostUserID === selfID;

    const updateMemberCamera = () => {
      if (!selfIsHost) return;
      IMSDK.signalingOperateStream({
        streamType: "video",
        roomID,
        userID: participant.identity,
        mute: !isVideoMuted,
        muteAll: false,
      });
    };

    const updateMemberMic = () => {
      if (!selfIsHost) return;

      IMSDK.signalingOperateStream({
        streamType: "audio",
        roomID,
        userID: participant.identity,
        mute: !isAudioMuted,
        muteAll: false,
      });
    };

    const updatePinUser = () => {
      const options = {} as UpdateMeetingParams;
      if (pinedUserIDList?.includes(participant.identity)) {
        options.reducePinedUserIDList = [participant.identity];
      } else {
        options.addPinedUserIDList = [participant.identity];
      }
      IMSDK.signalingUpdateMeetingInfo({
        ...options,
        roomID,
      });
      setMoreActionVisible(false);
    };

    const updateBeWatchedUser = () => {
      const options = {} as UpdateMeetingParams;
      if (beWatchedUserIDList?.includes(participant.identity)) {
        options.reduceBeWatchedUserIDList = [participant.identity];
      } else {
        options.reduceBeWatchedUserIDList = [...(beWatchedUserIDList ?? [])];
        options.addBeWatchedUserIDList = [participant.identity];
      }
      IMSDK.signalingUpdateMeetingInfo({
        ...options,
        roomID,
      });
      setMoreActionVisible(false);
    };

    const userInfo: PublicUserItem | undefined = JSON.parse(
      participant.metadata ?? "{}",
    ).userInfo;

    return (
      <div
        className={clsx(
          styles["member-item"],
          pinedUserIDList?.includes(participant.identity) &&
            styles["member-item_pined"],
        )}
      >
        <div className="flex flex-1 items-center overflow-hidden">
          <OIMAvatar
            size={38}
            shape="circle"
            src={userInfo?.faceURL}
            text={userInfo?.nickname}
          />
          <div className="ml-3 flex flex-col text-xs">
            <div className="truncate">{userInfo?.nickname}</div>
            {isHost || isSelf ? (
              <div className="truncate text-[var(--sub-text)]">{`
            (${isHost ? t("placeholder.compere") : ""}${isHost && isSelf ? "、" : ""}${
                isSelf ? t("you") : ""
              })
            `}</div>
            ) : null}
          </div>
        </div>
        <div className="flex items-center">
          <img
            className="ml-4 h-4 w-4 cursor-pointer"
            style={{ cursor: !selfIsHost ? "auto" : "pointer" }}
            src={isVideoMuted ? meeting_slider_camera_off : meeting_slider_camera}
            alt=""
            onClick={updateMemberCamera}
          />
          <img
            className="ml-4 h-4 w-4 cursor-pointer"
            style={{ cursor: !selfIsHost ? "auto" : "pointer" }}
            src={isAudioMuted ? meeting_slider_mic_off : meeting_slider_mic}
            alt=""
            onClick={updateMemberMic}
          />
          {selfIsHost && (
            <Popover
              content={
                <div>
                  <div
                    className={clsx(
                      styles["more-action-btn"],
                      "border-b border-b-[rgba(81,94,112,0.1)]",
                    )}
                    onClick={updatePinUser}
                  >
                    {!pinedUserIDList?.includes(participant.identity)
                      ? t("placeholder.sticky")
                      : t("placeholder.removeSticky")}
                  </div>
                  <div
                    className={styles["more-action-btn"]}
                    onClick={updateBeWatchedUser}
                  >
                    {beWatchedUserIDList?.includes(participant.identity)
                      ? t("placeholder.removeWatchUser")
                      : t("placeholder.watchUser")}
                  </div>
                </div>
              }
              open={moreActionVisible}
              onOpenChange={(vis) => setMoreActionVisible(vis)}
              trigger="click"
              placement="bottom"
            >
              <img
                className="ml-4 h-4 w-4 cursor-pointer"
                src={meeting_slider_more}
                alt=""
              />
            </Popover>
          )}
        </div>
      </div>
    );
  },
);
