import {
  DownloadOutlined,
  FolderOpenOutlined,
  LoadingOutlined,
  PauseOutlined,
} from "@ant-design/icons";
import React, { FC } from "react";

import { useMessageStore } from "@/store";
import { getDownloadTask } from "@/utils/common";

interface IPreviewToolsBarProps {
  dom: JSX.Element;
  clientMsgID?: string;
  workMomentID?: string;
  isFileDownloaded: boolean;
  hiddenImageAction?: boolean;
  onDownloadOrShowFinder: () => void;
}
const PreviewToolsBar: FC<IPreviewToolsBarProps> = ({
  dom,
  clientMsgID,
  workMomentID,
  isFileDownloaded,
  hiddenImageAction = false,
  onDownloadOrShowFinder,
}) => {
  const currentTask = useMessageStore((state) =>
    getDownloadTask({
      downloadMap: state.downloadMap,
      compareKey: "clientMsgID",
      compareValue: clientMsgID,
    }),
  );

  const renderIcon = () => {
    if (currentTask?.downloadState === "pause") {
      return <PauseOutlined />;
    }
    if (currentTask?.isMediaMessage && !isFileDownloaded) {
      return <LoadingOutlined spin />;
    }
    return isFileDownloaded ? <FolderOpenOutlined /> : <DownloadOutlined />;
  };

  const action = (
    <div
      key="download-or-show-finder"
      className="ant-image-preview-operations-operation"
      onClick={currentTask?.isMediaMessage ? undefined : onDownloadOrShowFinder}
    >
      {renderIcon()}
    </div>
  );

  if (hiddenImageAction) return React.cloneElement(dom, {}, [action]);

  return React.cloneElement(dom, {}, [dom?.props.children, action]);
};

export default PreviewToolsBar;
