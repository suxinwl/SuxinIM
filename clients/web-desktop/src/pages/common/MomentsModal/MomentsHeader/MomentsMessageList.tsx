import { Divider, Empty, Image, Spin } from "antd";
import clsx from "clsx";
import { t } from "i18next";
import { forwardRef, ForwardRefRenderFunction, memo, useImperativeHandle } from "react";
import { Virtuoso } from "react-virtuoso";

import { useLogs } from "@/api/moments";
import liked from "@/assets/images/moments/liked.png";
import OIMAvatar from "@/components/OIMAvatar";
import { WorkMomentLogType } from "@/constants";
import { WorkMoments } from "@/types/moment";
import { formatMomentsTime } from "@/utils/imCommon";

const MomentsMessageList: ForwardRefRenderFunction<
  { refetch: () => void },
  {
    clearLoading: boolean;
    jumpToMoments: (workMomentID: string) => void;
    clearMessageList: () => void;
  }
> = ({ clearLoading, jumpToMoments, clearMessageList }, ref) => {
  const { isFetching, fetchNextPage, refetch, hasNextPage, data } = useLogs();

  const items = data?.pages.flatMap((page) => page.data?.workMoments ?? []);

  const endReached = () => {
    if (hasNextPage && !isFetching) {
      fetchNextPage();
    }
  };

  useImperativeHandle(
    ref,
    () => ({
      refetch,
    }),
    [],
  );

  return (
    <Spin spinning={clearLoading}>
      <div className="flex h-[420px] w-80 flex-col overflow-hidden rounded-lg">
        <div className="flex items-center justify-between px-3 py-2">
          <div />
          <div>{t("placeholder.chat")}</div>
          <div className="cursor-pointer" onClick={clearMessageList}>
            {t("clear")}
          </div>
        </div>

        <div className="flex-1">
          <Virtuoso
            data={items}
            className="no-scrollbar"
            endReached={endReached}
            // computeItemKey={(_, item) => item.conversationID}
            components={{
              Footer: () => (
                <div>
                  {isFetching && (
                    <div className="flex justify-center py-3">
                      <Spin />
                    </div>
                  )}
                  {!isFetching && Boolean(items?.length) && (
                    <Divider className="!m-0 bg-[var(--chat-bubble)] px-20 py-3 !text-xs !font-normal !text-[var(--sub-text)]">
                      {t("empty.noMoreMessage")}
                    </Divider>
                  )}
                </div>
              ),
              EmptyPlaceholder: () =>
                !isFetching ? (
                  <Empty
                    rootClassName="mt-[120px]"
                    image={Empty.PRESENTED_IMAGE_SIMPLE}
                    description={t("empty.noMoments")}
                  />
                ) : null,
            }}
            itemContent={(_, moments) => (
              <MomentsMessageItem moments={moments} jumpToMoments={jumpToMoments} />
            )}
          />
        </div>
      </div>
    </Spin>
  );
};

export default memo(forwardRef(MomentsMessageList));

const getOpUser = (moments: WorkMoments) => {
  if (moments.type === WorkMomentLogType.Like) {
    return {
      faceURL: moments.likeUsers?.[0].faceURL,
      nickname: moments.likeUsers?.[0].nickname,
    };
  }
  if (moments.type === WorkMomentLogType.Commit) {
    return {
      faceURL: moments.comments?.[0].faceURL,
      nickname: moments.comments?.[0].nickname,
    };
  }
  return {
    faceURL: moments.faceURL,
    nickname: moments.nickname,
  };
};

const MomentsMessageItem = memo(
  ({
    moments,
    jumpToMoments,
  }: {
    moments: WorkMoments;
    jumpToMoments: (workMomentID: string) => void;
  }) => {
    const opUser = getOpUser(moments);

    const getContent = () => {
      if (moments.type === WorkMomentLogType.Like) {
        return (
          <img src={liked} width={16} className="inline-block align-middle" alt="" />
        );
      }
      if (moments.type === WorkMomentLogType.Commit) {
        return (
          <>
            {Boolean(moments.comments?.[0].replyUserID) && (
              <>
                <span>{t("placeholder.replied")}</span>
                <span className="text-[var(--moment-text)]">
                  {moments.comments?.[0].replyNickname}:
                </span>
              </>
            )}
            <span>{moments.comments?.[0].content}</span>
          </>
        );
      }
      return t("placeholder.atYou");
    };

    const onlyText = !moments.content.metas?.[0]?.thumb;

    return (
      <div className="flex items-center p-3">
        <OIMAvatar src={opUser.faceURL} text={opUser.nickname} />
        <div className="mx-2 flex-1 overflow-hidden">
          <div className="max-w-[60%] truncate text-[var(--moment-text)]">
            {opUser.nickname}
          </div>
          <div className="truncate">{getContent()}</div>
        </div>
        <div className="mr-1 text-xs text-[var(--sub-text)]">
          {formatMomentsTime(moments.createTime)}
        </div>
        <div
          className={clsx(
            "relative line-clamp-2 h-[42px] w-[42px] cursor-pointer overflow-hidden break-all text-xs",
            onlyText && "bg-[var(--chat-bubble)]",
          )}
          onClick={() => jumpToMoments(moments.workMomentID)}
        >
          <div className="absolute left-0 top-0 flex h-full w-full items-center justify-center">
            <Image
              className="max-w-[42px]"
              src={moments.content.metas?.[0]?.thumb}
              preview={false}
            />
          </div>
          {onlyText ? moments.content.text : ""}
        </div>
      </div>
    );
  },
);
