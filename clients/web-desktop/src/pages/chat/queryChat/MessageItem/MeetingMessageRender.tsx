import { useRequest } from "ahooks";
import dayjs from "dayjs";
import { FC, useEffect } from "react";
import { useTranslation } from "react-i18next";

import { message as toast } from "@/AntdGlobalComp";
import meeting_arrow from "@/assets/images/messageItem/meeting_arrow.png";
import meeting_icon from "@/assets/images/messageItem/meeting_icon.png";
import { IMSDK } from "@/layout/MainContentWrap";
import { MeetingAuthData } from "@/pages/common/MeetingManageModal/data";
import { MeetingInvitation, secondsToTime } from "@/pages/common/MeetingModal/data";
import emitter from "@/utils/events";

import { IMessageItemProps } from ".";

const MeetingMessageRender: FC<IMessageItemProps> = ({ message }) => {
  const { t } = useTranslation();

  const meetingInfo = JSON.parse(message.customElem.data).data as MeetingInvitation;
  const { runAsync, loading, cancel } = useRequest(
    IMSDK.signalingJoinMeeting<MeetingAuthData>,
    {
      manual: true,
    },
  );

  useEffect(() => {
    return () => {
      cancel();
    };
  }, []);

  const joinMeeting = () => {
    if (loading) return;
    runAsync(meetingInfo.id)
      .then(({ data }) => emitter.emit("OPEN_MEETING_MODAL", data))
      .catch((err) => {
        const isend = err.errMsg.includes("roomIsNotExist");
        toast.warning(isend ? t("toast.meetingOver") : t("toast.joinMeetingFailed"));
        console.error(err);
      });
  };

  return (
    <div className="w-60 rounded-md border border-[var(--gap-text)] px-3 py-2">
      <div className="mb-1 flex items-center">
        <img src={meeting_icon} alt="" />
        <div className="ml-2 truncate">{meetingInfo.subject}</div>
      </div>
      <ul className="ml-6 list-disc text-sm">
        <li className="py-1">{`${t("time.startTime")}：${dayjs(
          meetingInfo.start * 1000,
        ).format("M-DD HH:mm")}`}</li>
        <li className="py-1">{`${t("placeholder.meetingTime")}：${
          // @ts-ignore
          secondsToTime[meetingInfo.duration]
        }`}</li>
        <li className="py-1">{`${t("placeholder.meetingID")}：${meetingInfo.id}`}</li>
      </ul>
      <div
        className="mt-1 flex cursor-pointer items-center justify-center"
        onClick={joinMeeting}
      >
        <div className="text-xs text-[var(--primary)]">
          {t("placeholder.joinMeeting")}
        </div>
        <img src={meeting_arrow} alt="" />
      </div>
    </div>
  );
};

export default MeetingMessageRender;
