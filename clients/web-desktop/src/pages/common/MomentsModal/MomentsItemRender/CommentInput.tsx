import { useClickAway } from "ahooks";
import { Button, Input } from "antd";
import { t } from "i18next";
import {
  forwardRef,
  ForwardRefRenderFunction,
  memo,
  useImperativeHandle,
  useRef,
  useState,
} from "react";

import { useCreateComment } from "@/api/moments";
import { Comments } from "@/types/moment";

import { feedbackToast } from "@/utils/common";

export interface CommentState {
  visible: boolean;
  text: string;
  replyItem: Comments | null;
}

export type CommentInputHandle = {
  updateCommentState: (state: CommentState) => void;
};
const CommentInput: ForwardRefRenderFunction<
  CommentInputHandle,
  { workMomentID: string }
> = ({ workMomentID }, ref) => {
  const [commentState, setCommentState] = useState<CommentState>({
    visible: false,
    text: "",
    replyItem: null,
  });

  const wrapperRef = useRef<HTMLDivElement>(null);
  const { mutateAsync, isLoading } = useCreateComment();
  const pending = useRef(false);

  useClickAway((event) => {
    const el: HTMLElement | null = event.target as HTMLElement | null;

    if (commentState.visible && !pending.current && !el?.hasAttribute("data-ignore")) {
      setCommentState({
        visible: false,
        text: "",
        replyItem: null,
      });
    }
  }, wrapperRef);

  const onKeyUp = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === "Escape") {
      setCommentState({
        visible: false,
        text: "",
        replyItem: null,
      });
    }
  };

  const confirmComment = async () => {
    const content = commentState.text.trim();
    if (!content || pending.current) return;
    pending.current = true;
    try {
      await mutateAsync({
        workMomentID,
        content,
        replyUserID: commentState.replyItem?.userID ?? "",
      });
      setCommentState({ visible: false, text: "", replyItem: null });
    } catch (error) {
      // Preserve the draft and reply target so a failed send can be retried.
      feedbackToast({ error });
    } finally {
      pending.current = false;
    }
  };

  useImperativeHandle(
    ref,
    () => ({
      updateCommentState: (state: CommentState) => setCommentState(state),
    }),
    [],
  );

  return (
    <div
      ref={wrapperRef}
      style={{
        maxHeight: commentState.visible ? "999px" : "0",
        transition: "max-height 0.5s ease",
      }}
      className="relative flex cursor-pointer flex-col items-end overflow-hidden bg-[#f4f5f7]"
    >
      <div className="w-full p-2">
        <Input.TextArea
          autoSize={{ minRows: 4, maxRows: 4 }}
          maxLength={50}
          disabled={isLoading}
          value={commentState.text}
          onChange={(e) => setCommentState({ ...commentState, text: e.target.value })}
          onKeyUp={onKeyUp}
          placeholder={
            commentState.replyItem
              ? `${t("placeholder.reply")}${commentState.replyItem.nickname}`
              : t("placeholder.comment")
          }
        />
      </div>
      <Button
        size="small"
        className="absolute bottom-4 right-4 mt-2 text-right"
        type="primary"
        onClick={confirmComment}
        loading={isLoading}
        disabled={!commentState.text.trim()}
      >
        {t("placeholder.send")}
      </Button>
    </div>
  );
};

export default memo(forwardRef(CommentInput));
