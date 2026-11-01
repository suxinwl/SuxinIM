import { useHistoryTravel } from "ahooks";
import { Alert, Button, Empty, Spin } from "antd";
import { t } from "i18next";
import { CbEvents } from "@openim/wasm-client-sdk";
import { WSEvent } from "@/im/types";
import {
  forwardRef,
  ForwardRefRenderFunction,
  memo,
  useCallback,
  useEffect,
  useRef,
} from "react";
import { useQueryClient } from "react-query";
import { Virtuoso, VirtuosoHandle } from "react-virtuoso";

import { getMomentsByID, useMomentDetails, useUserMoments } from "@/api/moments";
import {
  refreshMomentQueries,
  uniqueMoments,
  updateMomentCaches,
} from "@/api/moments-cache";
import { feedbackToast } from "@/utils/common";
import DraggableModalWrap from "@/components/DraggableModalWrap";
import OIMAvatar from "@/components/OIMAvatar";
import { OverlayVisibleHandle, useOverlayVisible } from "@/hooks/useOverlayVisible";
import { IMSDK } from "@/layout/MainContentWrap";
import { useUserStore } from "@/store";
import { WorkMoments } from "@/types/moment";

import MomentsHeader from "./MomentsHeader";
import MomentsItemRender from "./MomentsItemRender";

export interface RouteTravel {
  nickname?: string;
  faceURL?: string;
  userID?: string;
  moments?: WorkMoments;
  updateMoments?: boolean;
}

const MomentsModalNew: ForwardRefRenderFunction<OverlayVisibleHandle> = (_, ref) => {
  const { isOverlayOpen, closeOverlay } = useOverlayVisible(ref);
  const virtuoso = useRef<VirtuosoHandle>(null);
  const headerRef = useRef<{ showMessages: () => void }>(null);
  const workMomentsUnreadCount = useUserStore((state) => state.workMomentsUnreadCount);

  const {
    value: routeTravel,
    setValue: setRouteTravel,
    backLength,
    go,
  } = useHistoryTravel<RouteTravel>({});
  const { isFetching, isError, fetchNextPage, hasNextPage, refetch, data } =
    useUserMoments(
      isOverlayOpen && !routeTravel?.moments?.workMomentID,
      routeTravel?.userID,
    );
  const detail = useMomentDetails(isOverlayOpen, routeTravel?.moments);
  const backStep = useRef(-1);

  const workMoments = uniqueMoments(data?.pages);

  const queryClient = useQueryClient();

  useEffect(() => {
    if (!isOverlayOpen) return;
    const changed = () => {
      refreshMomentQueries(queryClient);
      void useUserStore.getState().getWorkMomentsUnreadCount();
    };
    const customMessageHandler = ({
      data: event,
    }: WSEvent<{ key: string; data: string }>) => {
      if (!event?.key?.includes("wm_")) return;
      try {
        const moment = JSON.parse(event.data).body as WorkMoments;
        if (moment?.workMomentID && moment.content)
          updateMomentCaches(queryClient, moment);
      } catch {
        /* Legacy notifications may omit a full moment snapshot. */
      }
      changed();
    };
    // The 3.8 enterprise service uses this event rather than the old wm_ envelope.
    IMSDK.on("OnWorkMomentChanged", changed);
    IMSDK.on(CbEvents.OnRecvCustomBusinessMessage, customMessageHandler);
    return () => {
      IMSDK.off("OnWorkMomentChanged", changed);
      IMSDK.off(CbEvents.OnRecvCustomBusinessMessage, customMessageHandler);
    };
  }, [isOverlayOpen, queryClient]);

  const endReached = () => {
    if (!isFetching && hasNextPage) {
      fetchNextPage();
    }
  };

  const updateRouteTravel = useCallback(
    (travel: RouteTravel) => {
      if (
        travel.userID &&
        (travel.userID === routeTravel?.userID ||
          travel.userID === routeTravel?.moments?.userID)
      )
        return;
      if (
        travel.moments?.workMomentID &&
        !travel.updateMoments &&
        travel.moments?.workMomentID === routeTravel?.moments?.workMomentID
      )
        return;

      if (travel.updateMoments && travel.moments) {
        updateMomentCaches(queryClient, travel.moments);
        return;
      }
      backStep.current = -1;

      setRouteTravel({ ...travel });
    },
    [routeTravel, queryClient, setRouteTravel],
  );

  const backPreviousState = useCallback(() => go(backStep.current), []);

  const jumpToMoments = useCallback((workMomentID: string) => {
    getMomentsByID(workMomentID)
      .then(({ data: { workMoment } }) => {
        setRouteTravel({ moments: workMoment });
      })
      .catch((error) => feedbackToast({ error }));
  }, []);

  return (
    <DraggableModalWrap
      open={isOverlayOpen}
      title={null}
      footer={null}
      width={550}
      onCancel={closeOverlay}
      closeIcon={null}
      centered
      className="no-padding-modal"
      mask={false}
      ignoreClasses=".ignore-drag, .cursor-pointer, .ant-modal-wrap"
    >
      <div className="relative h-[80vh]">
        <MomentsHeader
          ref={headerRef}
          canBack={Boolean(backLength)}
          activeUserID={routeTravel?.userID}
          refetch={routeTravel?.moments ? detail.refetch : refetch}
          closeOverlay={closeOverlay}
          jumpToMoments={jumpToMoments}
          backPreviousState={backPreviousState}
        />
        {routeTravel?.moments?.workMomentID ? (
          <div className="pt-10">
            {detail.isError ? (
              <Alert
                type="error"
                showIcon
                message={t("toast.accessFailed")}
                action={
                  <Button size="small" onClick={() => void detail.refetch()}>
                    {t("retry", { defaultValue: "重试" })}
                  </Button>
                }
              />
            ) : (
              <MomentsItemRender
                moments={detail.data ?? routeTravel.moments}
                activeUserID={routeTravel?.userID}
                backPreviousState={backPreviousState}
                updateRouteTravel={updateRouteTravel}
              />
            )}
          </div>
        ) : (
          <Virtuoso
            data={workMoments}
            ref={virtuoso}
            className="no-scrollbar"
            endReached={endReached}
            computeItemKey={(_, moments) => moments.workMomentID}
            components={{
              Header: () => (
                <>
                  {/* The old background asset is a captured loading screen, not
                      a cover. Keep the feed compact instead of showing it forever. */}
                  <div
                    className={
                      routeTravel?.userID ? "bg-[#f4f5f7] px-5 pb-4 pt-14" : "h-10"
                    }
                  >
                    {routeTravel?.userID && (
                      <div className="flex items-center">
                        <OIMAvatar
                          size={48}
                          src={routeTravel?.faceURL}
                          text={routeTravel?.nickname}
                        />
                        <div className="ml-2 max-w-[200px] truncate font-medium">
                          {routeTravel?.nickname}
                        </div>
                      </div>
                    )}
                  </div>
                  {isError && (
                    <Alert
                      type="error"
                      showIcon
                      message={t("toast.accessFailed")}
                      action={
                        <Button size="small" onClick={() => void refetch()}>
                          {t("retry", { defaultValue: "重试" })}
                        </Button>
                      }
                    />
                  )}
                  {Boolean(workMomentsUnreadCount) && (
                    <div className="my-3 flex items-center justify-center">
                      <div
                        className="cursor-pointer rounded-md bg-[#b6bbc2] px-3 py-1 text-sm text-white"
                        onClick={() => headerRef.current?.showMessages()}
                      >
                        {t("placeholder.multipleNewMessages", {
                          count: workMomentsUnreadCount,
                        })}
                      </div>
                    </div>
                  )}
                </>
              ),
              Footer: () =>
                isFetching && (
                  <div className="mt-[20%] flex justify-center">
                    <Spin />
                  </div>
                ),
              EmptyPlaceholder: () =>
                !isFetching &&
                !isError && (
                  <Empty
                    image={Empty.PRESENTED_IMAGE_SIMPLE}
                    description={t("empty.noMoments")}
                  />
                ),
            }}
            itemContent={(_, moments) => (
              <MomentsItemRender
                moments={moments}
                activeUserID={routeTravel?.userID}
                backPreviousState={backPreviousState}
                updateRouteTravel={updateRouteTravel}
              />
            )}
          />
        )}
      </div>
    </DraggableModalWrap>
  );
};

export default memo(forwardRef(MomentsModalNew));
