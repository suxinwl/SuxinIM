import { Empty, Spin } from "antd";
import clsx from "clsx";
import { SessionType } from "@openim/wasm-client-sdk";
import { MessageItem } from "@/im/types";
import {
  forwardRef,
  ForwardRefRenderFunction,
  memo,
  useEffect,
  useImperativeHandle,
  useRef,
} from "react";
import { Virtuoso } from "react-virtuoso";

import JumpToMessageWrap from "@/components/JumpToMessageWrap";
import OIMAvatar from "@/components/OIMAvatar";
import { formatConversionTime, getConversationContent } from "@/utils/imCommon";

import { ChatLogsItem } from ".";
import styles from "./index.module.scss";
import { useKeyPage } from "./useKeyPage";

export type ChatLogsReslut = Partial<ChatLogsItem & MessageItem>;

export const ChatLogsRender = memo(
  ({
    id,
    isActive,
    result,
    conversationID,
    onClick,
    closeOverlay,
  }: {
    id?: string;
    isActive?: boolean;
    result: ChatLogsReslut;
    conversationID?: string;
    onClick?: (result: ChatLogsReslut) => void;
    closeOverlay?: () => void;
  }) => {
    const jumpWrapRef = useRef<{ jumpToHistory: () => Promise<void> }>(null);
    const isConversation = Boolean(result.conversationID);

    return (
      <JumpToMessageWrap
        ref={jumpWrapRef}
        message={result as MessageItem}
        conversationID={conversationID!}
        disabled={isConversation}
        afterJump={closeOverlay}
      >
        <div
          id={id}
          className={clsx(
            "flex rounded px-3 py-2 hover:bg-[var(--primary-active)]",
            { "bg-[var(--primary-active)]": isActive },
            { "cursor-pointer": isConversation },
          )}
          onClick={() => onClick?.(result)}
          onDoubleClick={() => !isConversation && jumpWrapRef.current?.jumpToHistory()}
        >
          <div className="relative min-w-[38px]">
            <OIMAvatar
              src={result.faceURL}
              text={result.senderNickname || result.showName}
              isgroup={result.conversationType === SessionType.WorkingGroup}
            />
          </div>
          <div className="ml-3 flex-1 overflow-hidden">
            <div className="flex items-center">
              <div className="flex-1 truncate">
                {result.senderNickname || result.showName}
              </div>
              {!isConversation && (
                <div className="ml-3 text-xs text-[var(--sub-text)]">
                  {formatConversionTime(result.sendTime ?? 0)}
                </div>
              )}
            </div>
            <div className="mt-2 select-text text-xs text-[var(--sub-text)]">
              {result.description ||
                getConversationContent({ ...result, groupID: "" } as MessageItem)}
            </div>
          </div>
        </div>
      </JumpToMessageWrap>
    );
  },
);

const ChatLogsPanel: ForwardRefRenderFunction<
  {
    updateIdx: (idx: number) => void;
  },
  {
    data: ChatLogsItem[];
    loading: boolean;
    isActive: boolean;
    closeOverlay: () => void;
  }
> = ({ data, loading, isActive, closeOverlay }, ref) => {
  const { activeIdx, updateIdx } = useKeyPage({
    isActive,
    maxIndex: data.length,
    elPrefix: `#conversation-item-`,
  });

  useEffect(() => {
    if (loading) {
      updateIdx(-1);
    } else if (data.length && (activeIdx < 0 || activeIdx >= data.length)) {
      updateIdx(0);
    }
  }, [loading, data, activeIdx]);

  useImperativeHandle(ref, () => ({ updateIdx }));

  return (
    <div className="flex h-full px-3">
      <Spin wrapperClassName="h-full flex-1" spinning={loading}>
        <Virtuoso
          className={clsx("h-full overflow-x-hidden", styles["virtuoso-wrapper"])}
          data={data}
          components={{
            EmptyPlaceholder: () =>
              loading ? null : (
                <Empty className="mt-[30%]" image={Empty.PRESENTED_IMAGE_SIMPLE} />
              ),
          }}
          computeItemKey={(_, item) => item.conversationID}
          itemContent={(idx, result) => (
            <ChatLogsRender
              isActive={activeIdx === idx}
              id={`conversation-item-${idx}`}
              result={result}
              onClick={() => updateIdx(idx)}
            />
          )}
        />
      </Spin>
      <div className="mx-3 h-full border-r border-[var(--gap-text)]" />
      <Virtuoso
        className={clsx("h-full flex-1 overflow-x-hidden", styles["virtuoso-wrapper"])}
        data={data[activeIdx]?.messageList ?? []}
        computeItemKey={(_, item) => item.clientMsgID}
        itemContent={(_, message) => (
          <ChatLogsRender
            result={message}
            conversationID={data[activeIdx].conversationID}
            closeOverlay={closeOverlay}
          />
        )}
      />
    </div>
  );
};

export default forwardRef(ChatLogsPanel);
