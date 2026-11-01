import { Empty, Spin } from "antd";
import clsx from "clsx";
import { MessageItem } from "@/im/types";
import { memo, useEffect } from "react";
import { Virtuoso } from "react-virtuoso";

import file_icon from "@/assets/images/messageItem/file_icon.png";
import {
  getSourceData,
  useMessageFileDownloadState,
} from "@/hooks/useMessageFileDownloadState";
import FileDownloadIcon from "@/svg/FileDownloadIcon";
import { formatConversionTime } from "@/utils/imCommon";

import styles from "./index.module.scss";
import { useKeyPage } from "./useKeyPage";
import ViewFileInFinder from "./ViewFileInFinder";

export const FileRender = memo(
  ({
    id,
    message,
    isActive,
  }: {
    id?: string;
    message: MessageItem;
    isActive?: boolean;
  }) => {
    const { progress, downloadState, tryDownload } =
      useMessageFileDownloadState(message);

    const showDownloadProgressTypes = ["downloading", "pause", "resume"];
    const showDownloadProgress = showDownloadProgressTypes.includes(downloadState);

    const viewInFinder = () => {
      const path = message.localEx || getSourceData(message).path;
      if (path) {
        window.electronAPI?.showInFinder(path);
      }
    };

    return (
      <ViewFileInFinder viewInFinder={viewInFinder}>
        <div
          id={id}
          className={clsx(
            "flex items-center rounded px-3 py-2 hover:bg-[var(--primary-active)]",
            {
              "bg-[var(--primary-active)]": isActive,
            },
          )}
        >
          <div className="relative min-w-[38px] cursor-pointer" onClick={tryDownload}>
            <img width={38} src={file_icon} alt="file" data-drag="app-drag" />
            {downloadState !== "finish" && (
              <div className="absolute left-0 top-0 flex h-full w-full items-center justify-center rounded-md bg-[rgba(0,0,0,.4)]">
                <FileDownloadIcon
                  pausing={downloadState === "pause"}
                  percent={!showDownloadProgress ? 0 : progress}
                />
              </div>
            )}
          </div>
          <div className="ml-3 flex-1 select-text overflow-hidden">
            <div className="flex items-center">
              <div className="flex-1 truncate">{message.fileElem.fileName}</div>
              <div className="ml-3 text-xs text-[var(--sub-text)]">
                {formatConversionTime(message.sendTime)}
              </div>
            </div>
            <div className="mt-2 text-xs text-[var(--sub-text)]">
              {message.senderNickname}
            </div>
          </div>
        </div>
      </ViewFileInFinder>
    );
  },
);

const FilePanel = ({
  data,
  loading,
  isActive,
}: {
  data: MessageItem[];
  loading: boolean;
  isActive: boolean;
}) => {
  const { activeIdx, updateIdx } = useKeyPage({
    isActive,
    maxIndex: data.length,
    elPrefix: `#file-item-`,
  });

  useEffect(() => {
    if (loading) {
      updateIdx(-1);
    }
  }, [loading]);

  return (
    <Spin wrapperClassName="h-full" spinning={loading}>
      <div className="flex h-full flex-col px-3">
        <Virtuoso
          className={clsx("flex-1 overflow-x-hidden", styles["virtuoso-wrapper"])}
          data={data}
          components={{
            EmptyPlaceholder: () =>
              loading ? null : (
                <Empty className="mt-[30%]" image={Empty.PRESENTED_IMAGE_SIMPLE} />
              ),
          }}
          itemContent={(idx, message) => (
            <FileRender
              message={message}
              id={`file-item-${idx}`}
              isActive={activeIdx === idx}
            />
          )}
        />
      </div>
    </Spin>
  );
};

export default FilePanel;
