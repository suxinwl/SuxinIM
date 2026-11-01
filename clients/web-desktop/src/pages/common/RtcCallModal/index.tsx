import "@livekit/components-styles";

import { LiveKitRoom } from "@livekit/components-react";
import { t } from "i18next";
import { SessionType } from "@openim/wasm-client-sdk";
import {
  forwardRef,
  ForwardRefRenderFunction,
  useCallback,
  useEffect,
  useState,
} from "react";

import DraggableModalWrap from "@/components/DraggableModalWrap";
import { OverlayVisibleHandle, useOverlayVisible } from "@/hooks/useOverlayVisible";
import { IMSDK } from "@/layout/MainContentWrap";
import { useUserStore } from "@/store";
import { feedbackToast } from "@/utils/common";

import { AuthData, InviteData, RtcInviteResults } from "./data";
import { RtcLayout } from "./RtcLayout";

interface IRtcCallModalProps {
  inviteData: InviteData;
}

const RtcCallModal: ForwardRefRenderFunction<
  OverlayVisibleHandle,
  IRtcCallModalProps
> = ({ inviteData }, ref) => {
  const { invitation, isJoin } = inviteData;
  const [connect, setConnect] = useState(false);
  const [isConnected, setIsConnected] = useState(false);
  const [authData, setAuthData] = useState<AuthData>({
    liveURL: "",
    token: "",
  });
  const selfID = useUserStore((state) => state.selfInfo.userID);
  const { isOverlayOpen, closeOverlay } = useOverlayVisible(ref);

  const isRecv = selfID !== invitation?.inviterUserID;
  const isGroup = invitation?.sessionType !== SessionType.Single;

  useEffect(() => {
    if (!isOverlayOpen) return;
    tryInvite();
  }, [isOverlayOpen, isRecv, isGroup]);

  const tryInvite = async () => {
    if (isJoin && invitation?.roomID) {
      IMSDK.signalingGetTokenByRoomID<RtcInviteResults>(invitation.roomID)
        .then(({ data }) => {
          setAuthData(data);
          setTimeout(() => setConnect(true));
        })
        .catch((error) => {
          feedbackToast({ msg: t("toast.joinFailed"), error });
          closeOverlay();
        });
      return;
    }
    if (!isRecv) {
      const funcName = isGroup ? "signalingInviteInGroup" : "signalingInvite";
      try {
        const { data } = await IMSDK[funcName]<RtcInviteResults>({
          invitation,
        });
        setAuthData(data);
        if (isGroup) setTimeout(() => setConnect(true));
      } catch (error) {
        feedbackToast({ msg: t("toast.inviteUserFailed"), error });
        closeOverlay();
      }
    }
  };

  const connectRtc = useCallback((data?: AuthData) => {
    if (data) {
      setAuthData(data);
    }
    setTimeout(() => setConnect(true));
  }, []);

  useEffect(() => {
    const disconnect = () => { setConnect(false); closeOverlay(); };
    window.addEventListener("enterprise-disconnect", disconnect);
    return () => window.removeEventListener("enterprise-disconnect", disconnect);
  }, []);

  return (
    <DraggableModalWrap
      title={null}
      footer={null}
      open={isOverlayOpen}
      closable={false}
      maskClosable={false}
      keyboard={false}
      mask={false}
      centered
      width="auto"
      onCancel={closeOverlay}
      destroyOnClose
      ignoreClasses=".ignore-drag, .no-padding-modal, .cursor-pointer"
      className="no-padding-modal rtc-single-modal"
      wrapClassName="pointer-events-none"
    >
      <div>
        {isOverlayOpen && (
          <LiveKitRoom
            serverUrl={authData.liveURL}
            token={authData.token}
            video={invitation?.mediaType === "video"}
            audio={true}
            connect={connect}
            options={{
              publishDefaults: {
                videoCodec: "vp9",
                backupCodec: { codec: "vp8" },
              },
            }}
            onConnected={() => setIsConnected(true)}
            onDisconnected={() => {
              closeOverlay();
              setIsConnected(false);
              setConnect(false);
            }}
          >
            <RtcLayout
              connect={connect}
              isConnected={isConnected}
              isRecv={isRecv}
              isGroup={isGroup}
              inviteData={inviteData}
              connectRtc={connectRtc}
              closeOverlay={closeOverlay}
            />
          </LiveKitRoom>
        )}
      </div>
    </DraggableModalWrap>
  );
};

export default forwardRef(RtcCallModal);
