import { Badge, Popover } from "antd";
import { t } from "i18next";
import {
  forwardRef,
  ForwardRefRenderFunction,
  memo,
  useCallback,
  useImperativeHandle,
  useRef,
  useState,
} from "react";

import { MomentsClearType, useClearUnreadMoments } from "@/api/moments";
import top_back from "@/assets/images/moments/top_back.png";
import top_close from "@/assets/images/moments/top_close.png";
import top_message from "@/assets/images/moments/top_message.png";
import top_publish from "@/assets/images/moments/top_publish.png";
import top_refresh from "@/assets/images/moments/top_refresh.png";
import { OverlayVisibleHandle } from "@/hooks/useOverlayVisible";
import { useUserStore } from "@/store";

import PublishModal from "../PublishModal";
import MomentsMessageList from "./MomentsMessageList";
import PublishMenu from "./PublishMenu";

interface IMomentsHeaderProps {
  canBack: boolean;
  activeUserID?: string;
  refetch: () => void;
  closeOverlay: () => void;
  backPreviousState: () => void;
  jumpToMoments: (workMomentID: string) => void;
}

const MomentsHeader: ForwardRefRenderFunction<
  { showMessages: () => void },
  IMomentsHeaderProps
> = (
  { canBack, activeUserID, refetch, closeOverlay, backPreviousState, jumpToMoments },
  ref,
) => {
  const messageListRef = useRef<{ refetch: () => void }>(null);
  const publishModalRef = useRef<OverlayVisibleHandle>(null);
  const [popoverState, setPopoverState] = useState({
    messageList: false,
    actions: false,
    withVideo: false,
  });
  const workMomentsUnreadCount = useUserStore((state) => state.workMomentsUnreadCount);
  const updateWorkMomentsUnreadCount = useUserStore(
    (state) => state.updateWorkMomentsUnreadCount,
  );
  const { isLoading: clearLoading, mutateAsync: clearUnreadMoments } =
    useClearUnreadMoments();

  const preparePublish = useCallback((withVideo = false) => {
    publishModalRef.current?.openOverlay();
    setPopoverState({
      messageList: false,
      actions: false,
      withVideo,
    });
  }, []);

  const jumpToMomentsAndClose = useCallback((workMomentID: string) => {
    jumpToMoments(workMomentID);
    setPopoverState({
      messageList: false,
      actions: false,
      withVideo: false,
    });
  }, []);

  const messageListOpenChange = (visible: boolean) => {
    if (visible && workMomentsUnreadCount) {
      messageListRef.current?.refetch();
      clearUnreadMoments(MomentsClearType.Count);
      updateWorkMomentsUnreadCount();
    }
    setPopoverState((state) => ({ ...state, messageList: visible }));
  };

  const clearMessageList = useCallback(() => {
    clearUnreadMoments(MomentsClearType.List).then(messageListRef.current?.refetch);
  }, []);

  useImperativeHandle(
    ref,
    () => ({ showMessages: () => messageListOpenChange(true) }),
    [workMomentsUnreadCount],
  );

  return (
    <div className="absolute top-0 z-10 flex w-full bg-gradient-to-r from-[#0e2941] to-[#021b30] px-3 py-2">
      <div className="flex items-center">
        <div
          className="flex h-6 w-6 cursor-pointer items-center justify-center"
          onClick={closeOverlay}
        >
          <img width={12} src={top_close} alt="" />
        </div>
        {canBack && (
          <div
            className="flex h-6 w-6 cursor-pointer items-center justify-center"
            onClick={backPreviousState}
          >
            <img width={9} src={top_back} alt="" />
          </div>
        )}
      </div>
      <div className="ml-3 flex-1 text-center text-white">
        {t("placeholder.moments")}
      </div>
      <div className="flex items-center">
        <img
          className="cursor-pointer"
          width={24}
          src={top_refresh}
          alt=""
          onClick={refetch}
        />
        <Popover
          content={
            <MomentsMessageList
              ref={messageListRef}
              clearLoading={clearLoading}
              clearMessageList={clearMessageList}
              jumpToMoments={jumpToMomentsAndClose}
            />
          }
          trigger="click"
          placement="bottomRight"
          overlayClassName="ignore-drag"
          title={null}
          arrow={false}
          open={popoverState.messageList}
          onOpenChange={messageListOpenChange}
        >
          <Badge size="small" offset={[-6, 2]} dot={Boolean(workMomentsUnreadCount)}>
            <img className="mx-2 cursor-pointer" width={24} src={top_message} alt="" />
          </Badge>
        </Popover>
        <Popover
          content={<PublishMenu preparePublish={preparePublish} />}
          trigger="click"
          placement="bottom"
          overlayClassName="ignore-drag"
          title={null}
          arrow={false}
          open={popoverState.actions}
          onOpenChange={(vis) =>
            setPopoverState((state) => ({ ...state, actions: vis }))
          }
        >
          <img className="cursor-pointer" width={24} src={top_publish} alt="" />
        </Popover>
      </div>
      <PublishModal
        ref={publishModalRef}
        withVideo={popoverState.withVideo}
        activeUserID={activeUserID}
      />
    </div>
  );
};

export default memo(forwardRef(MomentsHeader));
