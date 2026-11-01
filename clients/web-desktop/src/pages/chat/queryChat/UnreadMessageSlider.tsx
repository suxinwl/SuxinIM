import clsx from "clsx";
import { t } from "i18next";
import {
  forwardRef,
  ForwardRefRenderFunction,
  useEffect,
  useImperativeHandle,
  useState,
} from "react";

import db_arrow_down from "@/assets/images/common/db_arrow_down.png";
import { useMessageStore } from "@/store";
import emitter from "@/utils/events";

export type UnreadMessageSliderHandle = {
  updateAppendCount: () => void;
};

const UnreadMessageSlider: ForwardRefRenderFunction<
  UnreadMessageSliderHandle,
  { scrollToBottom: () => void }
> = ({ scrollToBottom }, ref) => {
  const [count, setCount] = useState(0);
  const jumpClientMsgID = useMessageStore((state) => state.jumpClientMsgID);
  const getHistoryMessageListByReq = useMessageStore(
    (state) => state.getHistoryMessageListByReq,
  );
  const updateJumpClientMsgID = useMessageStore((state) => state.updateJumpClientMsgID);
  const clearAppendState = useMessageStore((state) => state.clearAppendState);

  useEffect(() => {
    const addCount = () => {
      setCount((value) => value + 1);
    };
    emitter.on("ADD_NEW_MESSAGE_COUNT", addCount);
    return () => {
      emitter.off("ADD_NEW_MESSAGE_COUNT", addCount);
    };
  }, []);

  const updateAppendCount = () => {
    const appendMessageList = useMessageStore
      .getState()
      .historyMessageList.filter((message) => message.isAppend);
    setCount(appendMessageList.length);
  };

  const clearReadCount = () => {
    clearAppendState();
    setCount(0);
  };

  const clearRead = async () => {
    if (jumpClientMsgID) {
      await getHistoryMessageListByReq();
      updateJumpClientMsgID();
    }

    scrollToBottom();
    clearReadCount();
  };

  useImperativeHandle(
    ref,
    () => ({
      updateAppendCount,
    }),
    [],
  );

  return (
    <div
      style={{
        boxShadow: "0px 6px 16px 1px rgba(142,154,176,0.16)",
        transition: "opacity 0.3s ease",
        opacity: count > 0 ? 1 : 0,
      }}
      className={clsx(
        "shadow-[0px 6px 16px 1px rgba(142,154,176,0.16)] boder-[var(--gap-text)] absolute bottom-3 left-1/2 flex -translate-x-1/2 cursor-pointer items-center justify-center rounded-full border bg-white py-1 pl-2 pr-3",
        {
          "pointer-events-none": count === 0,
        },
      )}
      onClick={clearRead}
    >
      <img width={17} src={db_arrow_down} alt="" />
      <div className="text-xs text-[var(--primary)]">
        {t("placeholder.multipleNewMessages", { count })}
      </div>
    </div>
  );
};

export default forwardRef(UnreadMessageSlider);
