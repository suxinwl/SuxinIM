import {attachmentSourceName,attachmentMessageSourceName} from "@/im/attachment-name";
import { useLatest, useMount } from "ahooks";
import { MessageType } from "@openim/wasm-client-sdk";
import { ConversationItem } from "@/im/types";
import { useCallback, useEffect, useState } from "react";

import { IMSDK } from "@/layout/MainContentWrap";
import { SystemNotificationElem } from "@/pages/chat/queryChat/SystemNotification";
import { ExMessageItem, useConversationStore, useMessageStore } from "@/store";
import { SaveType } from "@/store/type";
import { downloadFile, getDownloadTask } from "@/utils/common";

export const getSourceData = (message: ExMessageItem) => {
  if (message.contentType === MessageType.PictureMessage) {
    return {
      url: message.pictureElem.sourcePicture.url,
      path: message.pictureElem.sourcePath,
      saveType: "image",
      fileName:attachmentMessageSourceName(message),
      fileSize:message.pictureElem.sourcePicture.size,
    };
  }
  if (message.contentType === MessageType.VideoMessage) {
    return {
      url: message.videoElem.videoUrl,
      path: message.videoElem.videoPath,
      saveType: "video",
      fileName:attachmentMessageSourceName(message),
      fileSize:message.videoElem.videoSize,
    };
  }
  if (message.contentType === MessageType.OANotification) {
    const notificationEl: SystemNotificationElem = JSON.parse(
      message.notificationElem.detail,
    );
    return {
      url: notificationEl.videoElem?.videoUrl ?? "",
      path: notificationEl.videoElem?.videoPath ?? "",
      saveType: "video",
    };
  }
  return {
    url: message.fileElem.sourceUrl,
    path: message.fileElem.filePath,
    saveType: "file",
    fileName:message.fileElem.fileName,
    fileSize:message.fileElem.fileSize,
  };
};

const mediaTypes = [MessageType.VideoMessage, MessageType.PictureMessage];

export function useMessageFileDownloadState(
  message: ExMessageItem,
  customCallback?: (path: string) => void,
) {
  const [previewPath, setPreviewPath] = useState<string>();
  const latestPreviewPath = useLatest(previewPath);

  const updateDownloadTask = useMessageStore((state) => state.updateDownloadTask);
  const currentTask = useMessageStore((state) =>
    getDownloadTask({
      downloadMap: state.downloadMap,
      compareKey: "clientMsgID",
      compareValue: message.clientMsgID,
    }),
  );
  const latestCurrentTask = useLatest(currentTask);
  const latestMessage = useLatest(message);

  useMount(() => {
    checkIsDownload();
  });

  useEffect(() => {
    if (currentTask?.downloadState === "finish") {
      checkIsDownload();
    }
  }, [currentTask?.downloadState]);

  const checkIsDownload = async () => {
    const { data } = await IMSDK.findMessageList([
      {
        conversationID:
          useConversationStore.getState().currentConversation?.conversationID ?? "",
        clientMsgIDList: [latestMessage.current.clientMsgID],
      },
    ]);
    const message = data.findResultItems?.[0].messageList[0];
    if (!message) return;

    const { path } = getSourceData(message);
    if (window.electronAPI?.fileExists(path)) {
      setPreviewPath(path);
      return;
    }
    if (message.localEx && window.electronAPI?.fileExists(message.localEx)) {
      setPreviewPath(message.localEx);
    }
  };

  const tryDownload = useCallback(async () => {
    if (latestPreviewPath.current) {
      const callback = customCallback ?? window.electronAPI?.openFile;
      callback?.(latestPreviewPath.current);
      return;
    }

    if (
      latestCurrentTask.current?.downloadState === "downloading" ||
      latestCurrentTask.current?.downloadState === "resume"
    ) {
      window.electronAPI?.ipcInvoke(
        "pauseDownload",
        latestCurrentTask.current.downloadUrl!,
      );
      updateDownloadTask(latestCurrentTask.current.downloadUrl!, {
        downloadState: "pause",
      });
      return;
    }

    if (latestCurrentTask.current?.downloadState === "pause") {
      window.electronAPI?.ipcInvoke(
        "resumeDownload",
        latestCurrentTask.current.downloadUrl!,
      );
      updateDownloadTask(latestCurrentTask.current.downloadUrl!, {
        downloadState: "downloading",
      });
      return;
    }

    if (!latestCurrentTask.current) {
      const { url, saveType } = getSourceData(latestMessage.current);
      const source=getSourceData(latestMessage.current);
      let conversationID =
        useConversationStore.getState().currentConversation?.conversationID;
      if (!conversationID) {
        conversationID = await IMSDK.getOneConversation({
          sessionType: latestMessage.current.sessionType,
          sourceID: latestMessage.current.groupID || latestMessage.current.sendID,
        }).then((res) => res.data.conversationID);
      }
      downloadFile(url, {
        saveType: saveType as SaveType,
        showError: true,
        clientMsgID: latestMessage.current.clientMsgID,
        isMediaMessage: mediaTypes.includes(message.contentType),
        conversationID,
        fileName:'fileName' in source?source.fileName:attachmentSourceName(undefined,source.path),
        fileSize:'fileSize' in source?Number(source.fileSize):undefined,
      });
    }
  }, []);

  return {
    progress: currentTask?.progress ?? 0,
    downloadState: previewPath ? "finish" : currentTask?.downloadState ?? "cancel",
    tryDownload,
    checkIsDownload,
  };
}
