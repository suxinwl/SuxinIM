import { Spin } from "antd";
import dayjs from "dayjs";
import i18n, { t } from "i18next";
import { FullUserItemWithCache } from "@/im/types";
import { memo, useEffect, useState } from "react";

import join_meeting from "@/assets/images/rtc/meeting_join.png";
import order_meeting from "@/assets/images/rtc/meeting_order.png";
import quick_meeting from "@/assets/images/rtc/meeting_quick.png";
import OIMAvatar from "@/components/OIMAvatar";
import { IMSDK } from "@/layout/MainContentWrap";
import { useUserStore } from "@/store";
import emitter from "@/utils/events";

import { LaunchStep, MeetingAuthData, MeetingRecordItem } from "./data";

const actionList = [
  {
    icon: join_meeting,
    title: t("placeholder.joinMeeting"),
  },
  {
    icon: quick_meeting,
    title: t("placeholder.quickMeeting"),
  },
  {
    icon: order_meeting,
    title: t("placeholder.orderMeeting"),
  },
];

i18n.on("languageChanged", () => {
  actionList[0].title = t("placeholder.joinMeeting");
  actionList[1].title = t("placeholder.quickMeeting");
  actionList[2].title = t("placeholder.orderMeeting");
});

type MeetingPrepareProps = {
  closeOverlay: () => void;
  updateStep: (step: LaunchStep, extraData?: unknown) => void;
};
export const MeetingPrepare = memo(
  ({ updateStep, closeOverlay }: MeetingPrepareProps) => {
    const selfInfo = useUserStore((state) => state.selfInfo);
    const [meetingList, setMeetingList] = useState<MeetingRecordItem[]>([]);
    const [loading, setLoading] = useState(false);

    useEffect(() => {
      getMeetingRecords();
    }, []);

    const getMeetingRecords = () => {
      setLoading(true);
      IMSDK.signalingGetMeetings<{ meetingInfoList: MeetingRecordItem[] }>()
        .then(async ({ data: { meetingInfoList } }) => {
          meetingInfoList = (meetingInfoList ?? []).filter((item) =>
            Boolean(item.hostUserID),
          );
          if (meetingInfoList.length > 0) {
            const { data } = await IMSDK.getUsersInfoWithCache({
              userIDList: meetingInfoList.map((item) => item.hostUserID),
            });
            const userData = data.map((user) => user.publicInfo);
            meetingInfoList.map((meeting) => {
              const findUser = userData.find(
                (user: any) => user.userID === meeting.hostUserID,
              );
              meeting.hostFaceURL = findUser?.faceURL;
              meeting.hostNickname = findUser?.nickname;
            });
          }
          setMeetingList(meetingInfoList);
        })
        .finally(() => setLoading(false));
    };

    const actionClick = (idx: number) => {
      switch (idx) {
        case 0:
          updateStep(LaunchStep.Join);
          break;
        case 1:
          IMSDK.signalingCreateMeeting<MeetingAuthData>({
            meetingName: `${selfInfo.nickname}${t("placeholder.initiatedMeeting")}`,
            meetingHostUserID: selfInfo.userID,
            startTime: dayjs().unix(),
            meetingDuration: 3600,
            inviteeUserIDList: [],
          })
            .then(({ data }) => {
              IMSDK.signalingUpdateMeetingInfo({
                roomID: data.roomID,
                participantCanUnmuteSelf: true,
              });
              setTimeout(() => {
                getMeetingRecords();
              });
              emitter.emit("OPEN_MEETING_MODAL", data);
              closeOverlay();
            })
            .catch((err) => {
              console.error(err);
            });
          break;
        case 2:
          updateStep(LaunchStep.Order);
          break;
        default:
          break;
      }
    };

    return (
      <>
        <div className="flex w-full justify-around border-b border-[var(--gap-text)] px-5.5 py-3">
          {actionList.map((action, idx) => (
            <div
              key={action.title}
              className="flex cursor-pointer flex-col items-center text-xs"
              onClick={() => actionClick(idx)}
            >
              <img src={action.icon} alt="" width={50} className="mb-2.5" />
              <span>{action.title}</span>
            </div>
          ))}
        </div>
        <Spin className="mt-2.5 w-full" spinning={loading}>
          <div className="h-full overflow-y-auto">
            {meetingList.map((item) => (
              <div
                key={item.roomID}
                className="flex items-center px-3.5 py-2.5 hover:bg-[var(--primary-active)]"
                onClick={() =>
                  updateStep(LaunchStep.HistoryDisplay, {
                    ...item,
                    startTime: dayjs(item.startTime * 1000),
                    duration: item.endTime - item.startTime,
                  })
                }
              >
                <OIMAvatar size={42} src={item.hostFaceURL} text={item.hostNickname} />
                <div className="ml-3 flex flex-col overflow-hidden">
                  <div className="flex">
                    <div className="truncate">{item.meetingName}</div>
                    {Date.now() < item.startTime * 1000 ? (
                      <span className="ml-3 h-5 min-w-[44px] rounded bg-[#1E74DE] px-1 text-xs font-light leading-5 text-white">
                        {t("notStart")}
                      </span>
                    ) : Date.now() < item.endTime * 1000 ? (
                      <span className="ml-3 h-5 min-w-[44px] rounded bg-[#FF9D3C] px-1 text-xs font-light leading-5 text-white">
                        {t("alreadyStarted")}
                      </span>
                    ) : null}
                  </div>
                  <div className="truncate text-xs text-[var(--sub-text)]">{`${dayjs(
                    item.startTime * 1000,
                  ).format("M-DD HH:mm")}-${dayjs(item.endTime * 1000).format(
                    "HH:mm",
                  )} ${t("placeholder.launchPerson")}：${item.hostNickname}`}</div>
                </div>
              </div>
            ))}
          </div>
        </Spin>
      </>
    );
  },
);
