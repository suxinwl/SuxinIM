import { useParticipants, useTracks } from "@livekit/components-react";
import { useSize } from "ahooks";
import { Popover } from "antd";
import dayjs from "dayjs";
import { t } from "i18next";
import { RoomEvent, Track } from "livekit-client";
import {
  forwardRef,
  ForwardRefRenderFunction,
  memo,
  useCallback,
  useEffect,
  useImperativeHandle,
  useMemo,
  useRef,
  useState,
} from "react";

import meeting_details from "@/assets/images/rtc/meeting_details.png";

import { MeetingDetails, secondsToTime } from "../data";
import { AllInOneVideoGrid } from "./AllInOneVideoGrid";
import { MeetingActionRow } from "./MeetingActionRow";
import { NomalVideoGrid } from "./NomalVideoGrid";

type MeetingMainProps = {
  roomID: string;
  showMax: boolean;
  showSlider: boolean;
  meetingDetails: MeetingDetails;
  updateShowSlider: () => void;
  disconnect: (closeRoom?: boolean) => void;
};

export type MeetingMainHandler = {
  focusUserID?: string;
  toggleAllInOne: (identity: string) => void;
};

const MeetingMain: ForwardRefRenderFunction<MeetingMainHandler, MeetingMainProps> = (
  { roomID, showSlider, showMax, meetingDetails, updateShowSlider, disconnect },
  ref,
) => {
  const [focusUserID, setFocusUserID] = useState<string>();
  const wrapRef = useRef(null);
  const size = useSize(wrapRef);

  const participantLength = useParticipants({
    updateOnlyOn: [RoomEvent.ParticipantConnected, RoomEvent.ParticipantDisconnected],
  }).length;
  const isOnlyAudio =
    useTracks([Track.Source.Camera, Track.Source.ScreenShare]).length === 0;

  useEffect(() => {
    setFocusUserID(meetingDetails.beWatchedUserIDList?.[0]);
  }, [meetingDetails.beWatchedUserIDList]);

  const detailsContent = useMemo(
    () => (
      <div className="ignore-drag p-2.5">
        <div className="mb-2.5">{meetingDetails.meetingName}</div>
        <div className="mb-2.5 text-xs text-[var(--sub-text)]">{`${t(
          "placeholder.meetingID",
        )}：${meetingDetails.roomID}`}</div>
        <div className="mb-2.5 text-xs text-[var(--sub-text)]">{`${t(
          "time.startTime",
        )}： ${dayjs(meetingDetails.startTime * 1000).format(
          "YYYY-MM-DD HH:mm",
        )}`}</div>
        <div className="text-xs text-[var(--sub-text)]">{`${t(
          "placeholder.meetingTime",
        )}： ${
          // @ts-ignore
          secondsToTime[meetingDetails.endTime - meetingDetails.startTime] ?? ""
        }`}</div>
      </div>
    ),
    [meetingDetails.roomID],
  );

  const toggleAllInOne = useCallback((identity: string) => {
    setFocusUserID(identity);
  }, []);

  useImperativeHandle(ref, () => ({
    focusUserID,
    toggleAllInOne,
  }));

  const mainWidth = showMax
    ? `calc(100vw - ${showSlider ? 327 : 0}px - 40px)`
    : "960px";
  const mainHeight = showMax ? "calc(100vh - 40px)" : "650px";

  return (
    <div
      className="flex max-h-[85vh] flex-col"
      style={{ width: mainWidth, height: mainHeight }}
    >
      <div className="app-no-drag h-7 text-center leading-7">
        <span>{t("placeholder.meeting")}</span>
      </div>
      <div className="ignore-drag flex justify-between bg-[rgba(24,144,255,0.05)] px-3 py-1">
        <div>{meetingDetails.meetingName}</div>
        <Counter />
        <div className="flex items-center">
          <span>{t("placeholder.someInMeeting", { count: participantLength })}</span>
          <Popover
            content={detailsContent}
            trigger="click"
            placement="bottom"
            // arrow={false}
          >
            <img
              width={14}
              src={meeting_details}
              alt=""
              className="ml-2 cursor-pointer"
            />
          </Popover>
        </div>
      </div>
      <div
        className="ignore-drag relative flex flex-1 overflow-hidden"
        style={{
          backgroundColor: !isOnlyAudio && focusUserID ? "#343030" : "#fff",
        }}
        ref={wrapRef}
      >
        {focusUserID && !isOnlyAudio ? (
          <AllInOneVideoGrid
            hostUserID={meetingDetails.hostUserID}
            allInOneUserID={focusUserID}
            toggleAllInOne={toggleAllInOne}
          />
        ) : (
          <NomalVideoGrid
            isOnlyAudio={isOnlyAudio}
            hostUserID={meetingDetails.hostUserID}
            wrapHeight={size?.height}
            toggleAllInOne={toggleAllInOne}
          />
        )}
      </div>
      <MeetingActionRow
        roomID={roomID}
        meetingDetails={meetingDetails}
        updateShowSlider={updateShowSlider}
        disconnect={disconnect}
      />
    </div>
  );
};

export const ForwardMeetingMain = memo(forwardRef(MeetingMain));

const Counter = memo(() => {
  const [time, setTime] = useState(dayjs().format("YYYY-MM-DD HH:mm:ss"));

  useEffect(() => {
    const timer = setInterval(() => {
      setTime(dayjs().format("YYYY-MM-DD HH:mm:ss"));
    }, 1000);
    return () => clearInterval(timer);
  }, []);

  return <div>{time}</div>;
});
