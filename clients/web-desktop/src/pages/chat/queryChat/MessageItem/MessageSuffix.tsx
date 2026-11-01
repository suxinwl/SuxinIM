import { ExclamationCircleFilled, LoadingOutlined } from "@ant-design/icons";
import { useLatest } from "ahooks";
import { Spin } from "antd";
import { MessageStatus, MessageType } from "@openim/wasm-client-sdk";
import { FC, useEffect, useState } from "react";

import { IMSDK } from "@/layout/MainContentWrap";
import { ExMessageItem, useMessageStore } from "@/store";

import { useSendMessage } from "../ChatFooter/useSendMessage";
import { IMessageItemProps } from ".";
import styles from "./message-item.module.scss";

const MessageSuffix: FC<IMessageItemProps> = ({ message, conversationID }) => {
  const [count, setCount] = useState(0);
  const latestCount = useLatest(count);
  const [showSending, setShowSending] = useState(false);
  const deleteOneMessage = useMessageStore((state) => state.deleteOneMessage);

  const { sendMessage, updateOneMessage } = useSendMessage();

  const isShowLimitTimer = message.isRead && message.attachedInfoElem?.isPrivateChat;

  useEffect(() => {
    if (message.status !== MessageStatus.Sending) return;
    const timer = setTimeout(() => {
      if (message.status === MessageStatus.Sending) {
        setShowSending(true);
      }
    }, 1000);
    return () => {
      clearTimeout(timer);
    };
  }, [message.status]);

  useEffect(() => {
    if (!isShowLimitTimer) return;
    setCount(message.attachedInfoElem?.burnDuration || 30);
    const timer = setInterval(() => {
      setCount((prev) => {
        if (prev === 0) {
          clearInterval(timer);
          removeMessage();
          return 0;
        }
        return prev - 1;
      });
    }, 1000);
    return () => {
      clearInterval(timer);
    };
  }, [isShowLimitTimer]);

  useEffect(() => {
    const currentConversationID = conversationID;
    return () => {
      if (message.attachedInfoElem?.isPrivateChat && latestCount.current !== 0) {
        removeMessage(currentConversationID);
      }
    };
  }, [message.attachedInfoElem?.isPrivateChat]);

  const removeMessage = async (currentConversationID?: string) => {
    await IMSDK.deleteMessage({
      clientMsgID: message.clientMsgID,
      conversationID: currentConversationID ?? conversationID!,
    });
    deleteOneMessage(message.clientMsgID);
  };

  const reSend = async () => {
    updateOneMessage({ ...message, status: MessageStatus.Sending });
    await checkResendFile(message);
    sendMessage({ message, needPush: false, isResend: true });
  };

  return (
    <div className={styles.suffix}>
      {showSending && message.status === MessageStatus.Sending && (
        <Spin
          className="flex"
          indicator={<LoadingOutlined style={{ fontSize: 16 }} spin rev={undefined} />}
        />
      )}
      {message.status === MessageStatus.Failed && (
        <ExclamationCircleFilled
          className="text-base text-[var(--warn-text)]"
          rev={undefined}
          onClick={reSend}
        />
      )}
      {isShowLimitTimer && (
        <div className="text-xs text-[var(--sub-text)]">{`${count}s`}</div>
      )}
    </div>
  );
};

export default MessageSuffix;

const getSourcePath = (message: ExMessageItem) => {
  if (message.contentType === MessageType.PictureMessage) {
    return {
      uuid: message.pictureElem.sourcePicture.uuid,
      path: message.pictureElem.sourcePath,
    };
  }
  if (message.contentType === MessageType.VideoMessage) {
    return {
      uuid: message.videoElem.videoUUID,
      path: message.videoElem.videoPath,
      snapshotPath: message.videoElem.snapshotPath,
      snapshotUUID: message.videoElem.snapshotUUID,
    };
  }
  return {
    uuid: message.fileElem.uuid,
    path: message.fileElem.filePath,
  };
};

const fileTypes = [
  MessageType.PictureMessage,
  MessageType.VideoMessage,
  MessageType.FileMessage,
];
const checkResendFile = async (message: ExMessageItem) => {
  if (!window.electronAPI || !fileTypes.includes(message.contentType)) return;
  const { uuid, path, snapshotPath, snapshotUUID } = getSourcePath(message);
  const sourceFile = await window.electronAPI?.getFileByPath(path);
  console.log(path, sourceFile);

  let snapshotFile;
  if (snapshotPath) {
    snapshotFile = await window.electronAPI?.getFileByPath(snapshotPath);
  }
  if (!sourceFile || (snapshotPath && !snapshotFile)) return;
  IMSDK.fileMapSet(uuid, sourceFile);
  if (snapshotUUID && snapshotFile) {
    IMSDK.fileMapSet(snapshotUUID, snapshotFile);
  }
};
