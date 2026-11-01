import { Popover, Spin } from "antd";
import { t } from "i18next";
import { memo, useState } from "react";

import { useDeleteComment } from "@/api/moments";
import { feedbackToast } from "@/utils/common";
import { Comments } from "@/types/moment";

import styles from "./moments-item-render.module.scss";

const CommentRow = ({
  comment,
  workMomentID,
  isSelfMoments,
  selfUserID,
  startComment,
}: {
  comment: Comments;
  workMomentID: string;
  isSelfMoments: boolean;
  selfUserID: string;
  startComment: (comment?: Comments) => void;
}) => {
  const [actionVisible, setActionVisible] = useState(false);

  const { isLoading, mutate } = useDeleteComment();

  const confirmDelete = () => {
    if (isLoading) return;
    mutate(
      { commentID: comment.commentID, workMomentID },
      { onError: (error) => feedbackToast({ error }) },
    );
    setActionVisible(false);
  };

  const commentSuffix = comment.replyUserID
    ? ` ${t("placeholder.reply")} ${comment.replyNickname}`
    : "";
  const canDelete = isSelfMoments || comment.userID === selfUserID;
  return (
    <Popover
      content={
        <Spin spinning={isLoading}>
          <div
            className="cursor-pointer rounded-sm bg-[#313e52] px-2 py-1 text-xs text-white"
            onClick={confirmDelete}
          >
            {t("placeholder.delete")}
          </div>
        </Spin>
      }
      title={null}
      trigger="contextMenu"
      placement="top"
      overlayClassName={styles["action-popover"]}
      open={canDelete ? actionVisible : false}
      onOpenChange={(vis) => setActionVisible(vis)}
    >
      <div
        data-ignore
        className="bg-[#f4f5f7] px-2 py-1 hover:bg-[#e8eaef]"
        onClick={() => startComment(comment)}
      >
        <span
          data-ignore
          className="mr-1.5 text-[var(--moment-text)]"
        >{`${comment.nickname}${commentSuffix}:`}</span>
        <span data-ignore>{comment.content}</span>
      </div>
    </Popover>
  );
};

export default memo(CommentRow);
