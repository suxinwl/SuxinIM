import { ArrowsAltOutlined } from "@ant-design/icons";
import type { TrackReferenceOrPlaceholder } from "@livekit/components-core";
import {
  AudioTrack,
  LiveKitRoom,
  TrackLoop,
  TrackRefContext,
  useParticipants,
  useRoomContext,
  useTracks,
} from "@livekit/components-react";
import { Button, Modal, Popover } from "antd";
import { t } from "i18next";
import { ConnectionState, RoomEvent, Track } from "livekit-client";
import {
  FC,
  forwardRef,
  ForwardRefRenderFunction,
  memo,
  useCallback,
  useEffect,
  useRef,
  useState,
} from "react";
import Draggable, { DraggableData, DraggableEvent } from "react-draggable";

import { modal } from "@/AntdGlobalComp";
import meeting_close from "@/assets/images/rtc/meeting_close.png";
import meeting_hidden from "@/assets/images/rtc/meeting_hidden.png";
import meeting_max from "@/assets/images/rtc/meeting_max.png";
import meeting_max_cancel from "@/assets/images/rtc/meeting_max_cancel.png";
import { OverlayVisibleHandle, useOverlayVisible } from "@/hooks/useOverlayVisible";
import { IMSDK } from "@/layout/MainContentWrap";
import { useUserStore } from "@/store";
import { enterpriseRequest } from "@/im/enterprise";

import { MeetingAuthData } from "../MeetingManageModal/data";
import { MeetingDetails } from "./data";
import { ForwardMeetingMain, MeetingMainHandler } from "./MeetingMain";
import { VideoMemberItem } from "./MeetingMain/VideoMemberItem";
import { MeetingSlider } from "./MeetingSlider";
import { sortParticipants } from "./sorting";

type MeetingModalProps = {
  authData?: MeetingAuthData;
};
const MeetingModal: ForwardRefRenderFunction<
  OverlayVisibleHandle,
  MeetingModalProps
> = ({ authData }, ref) => {
  const [connect, setConnect] = useState(false);
  const [isConnected, setIsConnected] = useState(false);
  const draRef = useRef<HTMLDivElement>(null);
  const [bounds, setBounds] = useState({
    left: 0,
    top: 0,
    bottom: 0,
    right: 0,
  });
  const [positionOffset, setPositionOffset] = useState({ x: 0, y: 0 });
  const { isOverlayOpen, closeOverlay } = useOverlayVisible(ref);

  const disconnectFlag = useRef(false);

  useEffect(() => {
    if (isOverlayOpen) {
      setConnect(true);
    }
  }, [isOverlayOpen]);

  const onStart = (event: DraggableEvent, uiData: DraggableData) => {
    const { clientWidth, clientHeight } = window?.document?.documentElement ?? {};
    const targetRect = draRef.current!.getBoundingClientRect();
    setBounds({
      left: -targetRect?.left + uiData?.x,
      right: clientWidth - (targetRect?.right - uiData?.x),
      top: -targetRect?.top + uiData?.y,
      bottom: clientHeight - (targetRect?.bottom - uiData?.y),
    });
  };

  const forceCenter = useCallback(() => {
    const styleStr = draRef.current?.style.transform ?? "";
    const idx = styleStr.lastIndexOf("(");
    const str = styleStr.slice(idx + 1, styleStr.length - 1);
    const arr = str.replace(new RegExp("px", "g"), "").split(", ");
    setPositionOffset({
      x: 0 - Number(arr[0]),
      y: 0 - Number(arr[1]),
    });
  }, []);

  const updateDisconnectFlag = useCallback((flag: boolean) => {
    disconnectFlag.current = flag;
  }, []);

  const onDisconnected = () => {
    setIsConnected(false);
    setConnect(false);
    if (disconnectFlag.current) {
      closeOverlay();
      return;
    }
    modal.confirm({
      title: t("placeholder.hint"),
      content: t("toast.meetingIsOver"),
      onOk: closeOverlay,
    });
  };

  const ignoreClasses = `.cursor-pointer, .ignore-drag`;

  useEffect(() => {
    const disconnect = () => { setConnect(false); closeOverlay(); };
    window.addEventListener("enterprise-disconnect", disconnect);
    return () => window.removeEventListener("enterprise-disconnect", disconnect);
  }, []);

  return (
    <Modal
      className={"no-padding-modal meeting-modal"}
      wrapClassName="pointer-events-none"
      closable={false}
      footer={null}
      mask={false}
      width={"auto"}
      maskClosable={false}
      keyboard={false}
      centered
      title={null}
      open={isOverlayOpen}
      onCancel={closeOverlay}
      afterClose={() => {
        updateDisconnectFlag(false);
      }}
      styles={{
        mask: {
          opacity: 0,
          transition: "none",
        },
      }}
      maskTransitionName=""
      destroyOnClose
      modalRender={(modal) => (
        <Draggable
          positionOffset={positionOffset}
          allowAnyClick={true}
          disabled={false}
          bounds={bounds}
          onStart={(event, uiData) => onStart(event, uiData)}
          cancel={ignoreClasses}
          enableUserSelectHack={false}
        >
          <div ref={draRef}>{modal}</div>
        </Draggable>
      )}
    >
      <LiveKitRoom
        serverUrl={authData?.liveURL}
        token={authData?.token}
        video={false}
        audio={false}
        connect={connect}
        options={{
          publishDefaults: {
            videoCodec: "vp9",
            backupCodec: { codec: "vp8" },
          },
        }}
        onConnected={() => setIsConnected(true)}
        onDisconnected={onDisconnected}
      >
        <MeetingLayout
          roomID={authData?.roomID}
          isConnected={isConnected}
          forceCenter={forceCenter}
          closeOverlay={closeOverlay}
          updateDisconnectFlag={updateDisconnectFlag}
        />
      </LiveKitRoom>
    </Modal>
  );
};

export default memo(forwardRef(MeetingModal));

interface IMeetingLayout {
  roomID?: string;
  isConnected: boolean;
  forceCenter: () => void;
  closeOverlay: () => void;
  updateDisconnectFlag: (flag: boolean) => void;
}
const MeetingLayout: FC<IMeetingLayout> = ({
  roomID,
  isConnected,
  forceCenter,
  closeOverlay,
  updateDisconnectFlag,
}) => {
  const [showSlider, setShowSlider] = useState(false);
  const [showMini, setShowMini] = useState(false);
  const [showMax, setShowMax] = useState(false);

  const [meetingDetails, setMeetingDetails] = useState<MeetingDetails>({
    beWatchedUserIDList: null,
  } as unknown as MeetingDetails);

  const meetingMainRef = useRef<MeetingMainHandler>(null);

  const selfID = useUserStore((state) => state.selfInfo.userID);
  const room = useRoomContext();

  useEffect(() => {
    if (!isConnected || !roomID) return;
    let disposed=false;
    const refresh=async()=>{
      const {data}=await enterpriseRequest<{meetingInfo:MeetingDetails}>("/signaling/get_signal_invitation_info",{roomID});
      if(!disposed)setMeetingDetails(data.meetingInfo);
    };
    const onMeetingChanged=({data}:{data:{roomID:string}})=>{
      if(data.roomID===roomID)void refresh().catch(error=>console.error("Meeting state refresh failed",error));
    };
    IMSDK.on("OnMeetingInfoChanged",onMeetingChanged);
    void refresh().catch(error=>console.error("Meeting state refresh failed",error));
    return ()=>{disposed=true;IMSDK.off("OnMeetingInfoChanged",onMeetingChanged);};
  }, [isConnected, roomID]);

  const updateShowSlider = useCallback(() => {
    setShowSlider((show) => !show);
  }, []);

  const updateShowMax = useCallback(() => {
    setShowMax((show) => !show);
  }, []);

  const updateShowMini = useCallback(() => {
    setShowMini((show) => {
      if (show) {
        forceCenter();
      }
      return !show;
    });
  }, []);

  const disconnect = useCallback(
    (closeRoom?: boolean) => {
      if (room.state === ConnectionState.Disconnected) {
        closeOverlay();
        return;
      }
      updateDisconnectFlag(true);
      room.disconnect();
      if (closeRoom) {
        IMSDK.signalingCloseRoom(meetingDetails.roomID);
      }
    },
    [meetingDetails.roomID, room],
  );

  const isHost = selfID === meetingDetails.hostUserID;

  return (
    <div className="relative flex">
      {!showMini ? (
        <>
          <MeetingTopBar
            isHost={isHost}
            showMax={showMax}
            updateShowMini={updateShowMini}
            updateShowMax={updateShowMax}
            disconnect={disconnect}
          />
          {/* {isConnecting ? <Spin tip={t("placeholder.connecting")} size="default" /> : null} */}
          <ForwardMeetingMain
            ref={meetingMainRef}
            roomID={meetingDetails.roomID}
            showSlider={showSlider}
            showMax={showMax}
            meetingDetails={meetingDetails}
            updateShowSlider={updateShowSlider}
            disconnect={disconnect}
          />

          {showSlider && (
            <MeetingSlider
              isHost={isHost}
              roomID={meetingDetails.roomID}
              meetingDetails={meetingDetails}
              updateShowSlider={updateShowSlider}
            />
          )}
        </>
      ) : (
        <MiniMeetingWin
          hostUserID={meetingDetails.hostUserID}
          updateShowMini={updateShowMini}
        />
      )}
    </div>
  );
};

const MiniMeetingWin = memo(
  ({
    hostUserID,
    updateShowMini,
  }: {
    hostUserID: string;
    updateShowMini: () => void;
  }) => {
    const audioTracks = useTracks([Track.Source.Microphone]);
    const tracks = useTracks([
      { source: Track.Source.Camera, withPlaceholder: true },
      { source: Track.Source.ScreenShare, withPlaceholder: false },
    ]).reduce((acc: TrackReferenceOrPlaceholder[], cur) => {
      const idx = acc.findIndex(
        (track) => track.participant.identity === cur.participant.identity,
      );
      if (idx > -1 && cur.source === Track.Source.ScreenShare) {
        acc[idx] = cur;
      }
      if (idx < 0) {
        acc.push(cur);
      }
      return acc;
    }, []);
    const participants = useParticipants();
    const sortedParticipants = sortParticipants(participants);
    const activeTrack = tracks.find(
      (track) => track.participant.identity === sortedParticipants[0].identity,
    );

    return (
      <div className="flex h-[136px] w-[220px] flex-col bg-[#2e3030]">
        {activeTrack && (
          <VideoMemberItem
            className="!m-0"
            hostUserID={hostUserID}
            track={activeTrack}
          />
        )}
        <ArrowsAltOutlined
          className="absolute bottom-1 right-1 cursor-pointer text-lg text-[#999]"
          onClick={updateShowMini}
        />
        <TrackLoop tracks={audioTracks}>
          <TrackRefContext.Consumer>
            {(track) => track?.publication ? <AudioTrack trackRef={{...track,publication:track.publication}} /> : null}
          </TrackRefContext.Consumer>
        </TrackLoop>
      </div>
    );
  },
);

interface IMeetingTopBarProps {
  isHost: boolean;
  showMax: boolean;
  updateShowMini: () => void;
  updateShowMax: () => void;
  disconnect: () => void;
}
const MeetingTopBar: FC<IMeetingTopBarProps> = ({
  isHost,
  showMax,
  updateShowMax,
  updateShowMini,
  disconnect,
}) => {
  const audioTracks = useTracks([Track.Source.Microphone]);
  return (
    <div className="app-no-drag absolute right-4 top-0 z-10 flex h-7 items-center">
      <div
        className="flex h-3 cursor-pointer items-center leading-3"
        onClick={updateShowMini}
      >
        <img src={meeting_hidden} alt="" className="h-px w-[11px] cursor-pointer" />
      </div>
      <img
        src={showMax ? meeting_max_cancel : meeting_max}
        alt=""
        className="mx-4 h-3 w-[11px] cursor-pointer"
        onClick={updateShowMax}
      />
      <Popover
        open={isHost ? undefined : false}
        content={<CloseMeetingContent disconnect={disconnect} />}
        trigger="click"
        placement="bottom"
      >
        <img
          src={meeting_close}
          alt=""
          className="h-2.5 w-2.5 cursor-pointer"
          onClick={() => {
            if (!isHost) {
              disconnect();
            }
          }}
        />
      </Popover>
      <TrackLoop tracks={audioTracks}>
        <TrackRefContext.Consumer>
          {(track) => track?.publication ? <AudioTrack trackRef={{...track,publication:track.publication}} /> : null}
        </TrackRefContext.Consumer>
      </TrackLoop>
    </div>
  );
};

type CloseMeetingContentProps = {
  disconnect: (closeRoom?: boolean) => void;
};
export const CloseMeetingContent = memo(({ disconnect }: CloseMeetingContentProps) => {
  return (
    <div className="flex flex-col p-2.5">
      <Button
        className="mb-1.5 px-10 py-1"
        danger
        type="primary"
        onClick={() => disconnect(true)}
      >
        {t("placeholder.finishMeeting")}
      </Button>
      <Button className="px-10 py-1" danger onClick={() => disconnect()}>
        {t("placeholder.leaveMeeting")}
      </Button>
    </div>
  );
});
