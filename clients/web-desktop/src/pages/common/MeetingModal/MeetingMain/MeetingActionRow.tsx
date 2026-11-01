import { CloseOutlined } from "@ant-design/icons";
import { useLocalParticipant } from "@livekit/components-react";
import { Button, Popover, Switch } from "antd";
import clsx from "clsx";
import { t } from "i18next";
import { Track } from "livekit-client";
import { CbEvents } from "@openim/wasm-client-sdk";
import { WSEvent } from "@/im/types";
import { memo, useCallback, useEffect, useMemo, useState } from "react";

import { message } from "@/AntdGlobalComp";
import meeting_member from "@/assets/images/rtc/meeting_member.png";
import meeting_setting from "@/assets/images/rtc/meeting_setting.png";
import meeting_toggle_camera from "@/assets/images/rtc/meeting_toggle_camera.png";
import meeting_toggle_camera_off from "@/assets/images/rtc/meeting_toggle_camera_off.png";
import meeting_toggle_mic from "@/assets/images/rtc/meeting_toggle_mic.png";
import meeting_toggle_mic_off from "@/assets/images/rtc/meeting_toggle_mic_off.png";
import meeting_toggle_screen from "@/assets/images/rtc/meeting_toggle_screen.png";
import { IMSDK } from "@/layout/MainContentWrap";
import { useUserStore } from "@/store";

import { CloseMeetingContent } from "..";
import { MeetingDetails, UpdateMeetingParams } from "../data";
import styles from "./meeting-main.module.scss";

type MeetingActionRowProps = {
  roomID: string;
  meetingDetails: MeetingDetails;
  updateShowSlider: () => void;
  disconnect: (closeRoom?: boolean) => void;
};
export const MeetingActionRow = memo(
  ({ roomID, meetingDetails, updateShowSlider, disconnect }: MeetingActionRowProps) => {
    const [showSetting, setShowSetting] = useState(false);

    const localParticipantState = useLocalParticipant();
    const localParticipant = localParticipantState.localParticipant;

    useEffect(() => {
      const streamChangeHandler = ({
        data,
      }: WSEvent<{ roomID: string; streamType: string; mute: boolean; userID?:string; muteAll?:boolean }>) => {
        if (data.roomID === meetingDetails.roomID && (data.muteAll || data.userID===useUserStore.getState().selfInfo.userID)) {
          if (data.streamType === "video") {
            operateCamera(data.mute);
          } else {
            operateMicrophone(data.mute);
          }
        }
      };
      IMSDK.on(CbEvents.OnStreamChange, streamChangeHandler);
      return () => {
        IMSDK.off(CbEvents.OnStreamChange, streamChangeHandler);
      };
    }, [meetingDetails.roomID]);

    const isHost = useMemo(
      () => meetingDetails.hostUserID === useUserStore.getState().selfInfo.userID,
      [meetingDetails.hostUserID],
    );

    const actionArr = useMemo(
      () => [
        {
          title: `${
            localParticipantState.isMicrophoneEnabled ? t("close") : t("open")
          }${t("placeholder.microphone")}`,
          icon: localParticipantState.isMicrophoneEnabled
            ? meeting_toggle_mic
            : meeting_toggle_mic_off,
        },
        {
          title: `${localParticipantState.isCameraEnabled ? t("close") : t("open")}${t(
            "placeholder.camera",
          )}`,
          icon: localParticipantState.isCameraEnabled
            ? meeting_toggle_camera
            : meeting_toggle_camera_off,
        },
        {
          title: localParticipantState.isScreenShareEnabled
            ? t("placeholder.endSharing")
            : t("placeholder.sharedScreen"),
          icon: meeting_toggle_screen,
        },
        {
          title: t("placeholder.member"),
          icon: meeting_member,
        },
        {
          title: t("placeholder.setting"),
          icon: meeting_setting,
          hidden: !isHost,
        },
      ],
      [
        localParticipantState.isCameraEnabled,
        localParticipantState.isMicrophoneEnabled,
        localParticipantState.isScreenShareEnabled,
        isHost,
      ],
    );

    const updateShowSetting = useCallback(() => {
      setShowSetting((show) => !show);
    }, []);

    const actionClick = async (idx: number) => {
      switch (idx) {
        case 0:
          if (
            !isHost &&
            !meetingDetails.participantCanUnmuteSelf &&
            !localParticipantState.isMicrophoneEnabled
          ) {
            message.warning(t("toast.adminCloseMicrophone"));
            return;
          }
          await operateMicrophone();
          break;
        case 1:
          if (
            !isHost &&
            !meetingDetails.participantCanEnableVideo &&
            !localParticipantState.isCameraEnabled
          ) {
            message.warning(t("toast.adminCloseCamera"));
            return;
          }
          await operateCamera();
          break;
        case 2:
          if (!isHost && meetingDetails.onlyHostShareScreen) {
            message.warning(t("toast.onlyHostShareScreen"));
            return;
          }
          await operateScreenShare();
          break;
        case 3:
          updateShowSlider();
          break;
        default:
          break;
      }
    };

    const operateCamera = async (flag?: boolean) => {
      if (localParticipant) {
        const enable = flag ?? localParticipantState.isCameraEnabled ?? false;
        await localParticipant.setCameraEnabled(!enable);
      }
    };

    const operateMicrophone = async (flag?: boolean) => {
      if (localParticipant) {
        const enable = flag ?? localParticipantState.isMicrophoneEnabled ?? false;
        await localParticipant.setMicrophoneEnabled(!enable);
      }
    };

    const operateScreenShare = async () => {
      if (localParticipant) {
        const enable = localParticipantState.isScreenShareEnabled ?? false;
        if (!window.electronAPI) {
          await localParticipant.setScreenShareEnabled(!enable);
          return;
        }

        // for electron
        if (enable) {
          const track = localParticipant.getTrackPublication(Track.Source.ScreenShare)?.track;
          if (track) {
            await localParticipant.setScreenShareEnabled(false);
          }
          return;
        }
        try {
          await localParticipant.setScreenShareEnabled(true, { audio: false });
        } catch (error) {
          message.error(t("toast.sharedScreenFailed"));
          console.error(error);
        }
      }
    };

    return (
      <div className={clsx("flex items-center justify-between", styles["row-shadow"])}>
        <div className="flex">
          {actionArr.map((action, idx) => {
            if (action.hidden) return null;
            const Wrapper = (actionEl: JSX.Element) =>
              idx === 4 ? (
                <Popover
                  key={action.title}
                  open={showSetting}
                  onOpenChange={(vis) => setShowSetting(vis)}
                  content={
                    <SettingContent
                      roomID={roomID}
                      meetingDetails={meetingDetails}
                      updateShowSetting={updateShowSetting}
                    />
                  }
                  placement="top"
                  trigger="click"
                >
                  {actionEl}
                </Popover>
              ) : (
                actionEl
              );
            return Wrapper(
              <div
                key={action.title}
                className="mx-3 my-2 flex min-w-[72px] cursor-pointer flex-col items-center text-xs"
                onClick={() => actionClick(idx)}
              >
                <img width={32} src={action.icon} alt="" />
                <div>{action.title}</div>
              </div>,
            );
          })}
        </div>
        <Popover
          open={isHost ? undefined : false}
          content={<CloseMeetingContent disconnect={disconnect} />}
          trigger="click"
          placement="topRight"
        >
          <Button
            className="mr-3 rounded"
            type="primary"
            onClick={() => {
              if (!isHost) {
                disconnect();
              }
            }}
          >
            {t("placeholder.finishMeeting")}
          </Button>
        </Popover>
      </div>
    );
  },
);

type SettingContentProps = {
  roomID: string;
  meetingDetails: MeetingDetails;
  updateShowSetting: () => void;
};
const SettingContent = memo(
  ({ roomID, meetingDetails, updateShowSetting }: SettingContentProps) => {
    const settingList = useMemo(
      () => [
        {
          title: t("placeholder.participantCanUnmuteSelf"),
          value: meetingDetails.participantCanUnmuteSelf,
        },
        {
          title: t("placeholder.participantCanEnableVideo"),
          value: meetingDetails.participantCanEnableVideo,
        },
        {
          title: t("placeholder.onlyHostShareScreen"),
          value: meetingDetails.onlyHostShareScreen,
        },
        {
          title: t("placeholder.onlyHostInviteUser"),
          value: meetingDetails.onlyHostInviteUser,
        },
        {
          title: t("placeholder.joinDisableMicrophone"),
          value: meetingDetails.joinDisableMicrophone,
        },
      ],
      [
        meetingDetails.participantCanUnmuteSelf,
        meetingDetails.participantCanEnableVideo,
        meetingDetails.onlyHostShareScreen,
        meetingDetails.onlyHostInviteUser,
        meetingDetails.joinDisableMicrophone,
      ],
    );

    const roomSettingUpdate = (flag: boolean, idx: number) => {
      const options = {} as UpdateMeetingParams;
      switch (idx) {
        case 0:
          options.participantCanUnmuteSelf = flag;
          break;
        case 1:
          options.participantCanEnableVideo = flag;
          break;
        case 2:
          options.onlyHostShareScreen = flag;
          break;
        case 3:
          options.onlyHostInviteUser = flag;
          break;
        case 4:
          options.joinDisableMicrophone = flag;
          break;
        default:
          break;
      }
      IMSDK.signalingUpdateMeetingInfo({
        ...options,
        roomID,
      });
    };

    return (
      <div className="w-[480px]">
        <div className="flex items-center justify-between bg-[#f1f2f3] px-6 py-4">
          <div>{t("placeholder.meetingSetting")}</div>
          <CloseOutlined
            className="cursor-pointer text-[#a5abb8]"
            onClick={updateShowSetting}
          />
        </div>
        {settingList.map((setting, idx) => (
          <div
            key={setting.title}
            className="flex items-center justify-between px-6 py-4"
          >
            <div>{setting.title}</div>
            <Switch
              checked={setting.value}
              size="small"
              onClick={(v) => roomSettingUpdate(v, idx)}
            />
          </div>
        ))}
      </div>
    );
  },
);
