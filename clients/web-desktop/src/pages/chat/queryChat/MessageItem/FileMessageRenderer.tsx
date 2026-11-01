import { useDrag } from "ahooks";
import { Spin } from "antd";
import { MessageStatus } from "@openim/wasm-client-sdk";
import { FC, useRef } from "react";

import file_icon from "@/assets/images/messageItem/file_icon.png";
import { useMessageFileDownloadState } from "@/hooks/useMessageFileDownloadState";
import FileDownloadIcon from "@/svg/FileDownloadIcon";
import { bytesToSize } from "@/utils/common";

import { IMessageItemProps } from ".";
import { useMessageUploadProgress } from "./useMessageUploadProgress";

const FileMessageRenderer: FC<IMessageItemProps> = ({ message }) => {
  const { fileElem } = message;
  const dragRef = useRef(null);

  const { progress, downloadState, tryDownload } = useMessageFileDownloadState(message);

  const uploadProgress = useMessageUploadProgress(message.clientMsgID);

  const isSending = message.status === MessageStatus.Sending;
  const isSucceed = message.status === MessageStatus.Succeed;

  // useDrag({ message }, dragRef, {
  // onDragStart: (e) => {
  //   const filePath = message.localEx || message.fileElem?.filePath;
  //   if (filePath && window.electronAPI?.fileExists(filePath)) {
  //     e.preventDefault();
  //     window.electronAPI.ipcInvoke("dragFile", filePath);
  //   }
  // },
  // });

  const showDownloadProgressTypes = ["downloading", "pause", "resume"];
  const showDownloadProgress = showDownloadProgressTypes.includes(downloadState);

  return (
    <Spin spinning={isSending} tip={`${uploadProgress}%`}>
      <div
        ref={dragRef}
        onClick={tryDownload}
        className="flex w-60 cursor-pointer items-center justify-between rounded-md border border-[var(--gap-text)]  p-3"
      >
        <div className="mr-2 flex h-full flex-1 flex-col justify-between overflow-hidden">
          <div data-drag="app-drag" className="line-clamp-2 break-all">
            {fileElem.fileName}
          </div>
          <div data-drag="app-drag" className="text-xs text-[var(--sub-text)]">
            {bytesToSize(fileElem.fileSize)}
          </div>
        </div>
        <div className="relative min-w-[38px]">
          <img width={38} src={file_icon} alt="file" data-drag="app-drag" />
          {isSucceed && downloadState !== "finish" && (
            <div className="absolute left-0 top-0 flex h-full w-full items-center justify-center rounded-md bg-[rgba(0,0,0,.4)]">
              <FileDownloadIcon
                pausing={downloadState === "pause"}
                percent={!showDownloadProgress ? 0 : progress}
              />
            </div>
          )}
        </div>
      </div>
    </Spin>
  );
};

export default FileMessageRenderer;
