import { CloseOutlined, LeftOutlined } from "@ant-design/icons";
import i18n, { t } from "i18next";
import {
  forwardRef,
  ForwardRefRenderFunction,
  memo,
  useCallback,
  useMemo,
  useState,
} from "react";

import { message } from "@/AntdGlobalComp";
import meeting_delete from "@/assets/images/rtc/meeting_delete.png";
import meeting_share from "@/assets/images/rtc/meeting_share.png";
import DraggableModalWrap from "@/components/DraggableModalWrap";
import { CustomMessageType } from "@/constants";
import { OverlayVisibleHandle, useOverlayVisible } from "@/hooks/useOverlayVisible";
import { IMSDK } from "@/layout/MainContentWrap";
import { useUserStore } from "@/store";
import emitter from "@/utils/events";

import {
  displayTypes,
  LaunchStep,
  MeetingAuthData,
  NewMeetingInfo,
  notBackTypes,
} from "./data";
import { MeetingDisplay } from "./MeetingDisplay";
import { MeetingPrepare } from "./MeetingPrepare";
import { OrderOrJoinMeeting } from "./OrderOrJoinMeeting";

const MeetingManageModal: ForwardRefRenderFunction<OverlayVisibleHandle, unknown> = (
  _,
  ref,
) => {
  const [step, setStep] = useState(LaunchStep.Prepare);
  const [newMeetingInfo, setNewMeetingInfo] = useState<NewMeetingInfo>(
    {} as NewMeetingInfo,
  );
  const selfInfo = useUserStore((state) => state.selfInfo);
  const [joinLoading, setJoinLoading] = useState(false);

  const { isOverlayOpen, closeOverlay } = useOverlayVisible(ref);

  const updateStep = useCallback((next: LaunchStep, extraData?: unknown) => {
    if (extraData) {
      setNewMeetingInfo(extraData as NewMeetingInfo);
    }
    setStep(next);
  }, []);

  const joinMeeting = useCallback((roomID: string) => {
    setJoinLoading(true);
    IMSDK.signalingJoinMeeting<MeetingAuthData>(roomID)
      .then(({ data }) => {
        emitter.emit("OPEN_MEETING_MODAL", data);
        closeOverlay();
      })
      .catch((err) => {
        const isend = err.errMsg.includes("roomIsNotExist");
        message.warning(isend ? t("toast.meetingOver") : t("toast.joinMeetingFailed"));
        console.error(err);
      })
      .finally(() => setJoinLoading(false));
  }, []);

  const inviteMember = () => {
    const meetingInfo = {
      inviterFaceURL: selfInfo.faceURL,
      id: newMeetingInfo.roomID,
      duration: newMeetingInfo.duration,
      inviterNickname: selfInfo.nickname,
      inviterUserID: selfInfo.userID,
      subject: newMeetingInfo.meetingName,
      start: newMeetingInfo.startTime.unix(),
    };
    emitter.emit("OPEN_CHOOSE_MODAL", {
      type: "MEETING_INVITE",
      extraData: {
        data: JSON.stringify({
          customType: CustomMessageType.MeetingInvitation,
          data: meetingInfo,
        }),
        extension: "",
        description: "",
      },
    });
  };

  const back = () => {
    setStep(LaunchStep.Prepare);
  };

  const deleteMeeting = () => {
    IMSDK.signalingCloseRoom(newMeetingInfo.roomID).then(() => back());
  };

  const MainRender = () => {
    switch (step) {
      case LaunchStep.Prepare:
        return <MeetingPrepare closeOverlay={closeOverlay} updateStep={updateStep} />;
      case LaunchStep.Order:
        return <OrderOrJoinMeeting isLaunch={true} updateStep={updateStep} />;
      case LaunchStep.Join:
        return (
          <OrderOrJoinMeeting joinMeeting={joinMeeting} joinLoading={joinLoading} />
        );
      case LaunchStep.Display:
      case LaunchStep.HistoryDisplay:
        return (
          <MeetingDisplay
            meetingInfo={newMeetingInfo}
            isHost={newMeetingInfo.hostUserID === selfInfo.userID}
            joinLoading={joinLoading}
            updateStep={updateStep}
            joinMeeting={joinMeeting}
          />
        );
      case LaunchStep.Update:
        return (
          <OrderOrJoinMeeting
            isLaunch={true}
            updateStep={updateStep}
            meetingInfo={newMeetingInfo}
          />
        );
      default:
        break;
    }
  };

  const modalTitle = useMemo(() => {
    switch (step) {
      case LaunchStep.Prepare:
        return t("placeholder.meeting");
      case LaunchStep.Join:
        return t("placeholder.joinMeeting");
      case LaunchStep.Order:
        return t("placeholder.orderMeeting");
      case LaunchStep.Display:
        return t("placeholder.orderSuccess");
      case LaunchStep.HistoryDisplay:
        return t("placeholder.meetingDetail");
      case LaunchStep.Update:
        return t("placeholder.changeMeetingDetail");
      default:
        return "";
    }
  }, [step, i18n.language]);

  const ignoreClasses = `.ant-form, .ignore-drag, .cursor-pointer`;

  const canShare =
    displayTypes.includes(step) &&
    (newMeetingInfo.hostUserID === selfInfo.userID ||
      !newMeetingInfo.onlyHostInviteUser);
  const canDelete =
    displayTypes.includes(step) && newMeetingInfo.hostUserID === selfInfo.userID;

  return (
    <DraggableModalWrap
      className={"no-padding-modal meeting-manage-modal"}
      ignoreClasses={ignoreClasses}
      closable={false}
      footer={null}
      mask={false}
      width={360}
      centered
      open={isOverlayOpen}
      onCancel={closeOverlay}
      afterClose={() => {
        back();
        if (joinLoading) setJoinLoading(false);
      }}
      title={null}
    >
      <div className="flex h-[550px] flex-col">
        <div className="relative my-3 w-full text-center">
          <span>{modalTitle}</span>
          <div className="text-[var(--sub-text)]] absolute -top-1 right-3 flex items-center">
            {canShare ? (
              <img
                width={14}
                src={meeting_share}
                alt=""
                onClick={inviteMember}
                className="cursor-pointer"
              />
            ) : null}
            {canDelete ? (
              <img
                className="ml-3 cursor-pointer"
                width={14}
                src={meeting_delete}
                alt=""
                onClick={deleteMeeting}
              />
            ) : null}
            {!displayTypes.includes(step) ? (
              <CloseOutlined onClick={closeOverlay} className="cursor-pointer" />
            ) : null}
          </div>
          {!notBackTypes.includes(step) ? (
            <div
              className="text-[var(--sub-text)]] absolute left-3 top-0 flex cursor-pointer items-center"
              onClick={back}
            >
              <LeftOutlined className="mr-1" />
              <span className="text-xs">{t("placeholder.getBack")}</span>
            </div>
          ) : null}
        </div>
        <MainRender />
      </div>
    </DraggableModalWrap>
  );
};

export default memo(forwardRef(MeetingManageModal));
