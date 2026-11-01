import { useDrop, useUpdate } from "ahooks";
import { t } from "i18next";
import { MessageType } from "@openim/wasm-client-sdk";
import { useEffect } from "react";

import { modal } from "@/AntdGlobalComp";
import file_icon from "@/assets/images/messageItem/file_icon.png";
import { IMSDK } from "@/layout/MainContentWrap";
import { ExMessageItem, useConversationStore } from "@/store";
import { bytesToSize } from "@/utils/common";

import { SendMessageParams } from "./useSendMessage";

export function useDropDomOnly({
  domRef,
  sendMessage,
}: {
  domRef?: React.RefObject<HTMLDivElement>;
  sendMessage: (params: SendMessageParams) => void;
}) {
  const update = useUpdate();

  useEffect(() => {
    setTimeout(() => {
      update();
    });
  }, []);

  useDrop(domRef, {
    onDom: ({ message }: { message: ExMessageItem }, e) => {
      const conversationName =
        useConversationStore.getState().currentConversation?.showName;
      const { fileName, fileSize } = getFileData(message);
      modal.confirm({
        title: `${t("placeholder.sendTo")}${conversationName}`,
        icon: null,
        width: 320,
        className: "drop-file-moal",
        centered: true,
        content: (
          <div className="h-[240px] overflow-y-auto border-b border-t border-[var(--gap-text)] p-2">
            <div className="mb-2 flex items-center">
              <img width={38} src={file_icon} alt="file" />
              <div className="ml-3 overflow-hidden">
                <div className="mb-1.5 truncate">{fileName}</div>
                <div className="text-xs text-[var(--sub-text)]">
                  {bytesToSize(fileSize)}
                </div>
              </div>
            </div>
          </div>
        ),
        okText: t("confirm"),
        cancelText: t("cancel"),
        onOk: async () => {
          const newMessage = (await IMSDK.createForwardMessage(message)).data;
          sendMessage({
            message: newMessage,
          });
        },
      });
      e?.preventDefault();
    },
  });
}

const getFileData = (message: ExMessageItem) => {
  if (message.contentType === MessageType.PictureMessage) {
    const idx = message.pictureElem.sourcePath?.lastIndexOf("/") ?? -1;
    return {
      fileName:
        idx > -1
          ? message.pictureElem.sourcePath.slice(idx + 1)
          : t("placeholder.file"),
      fileSize: message.pictureElem.sourcePicture.size,
    };
  }
  if (message.contentType === MessageType.VideoMessage) {
    const idx = message.videoElem.videoPath.lastIndexOf("/");
    return {
      fileName:
        idx > -1 ? message.videoElem.videoPath.slice(idx + 1) : t("placeholder.file"),
      fileSize: message.videoElem.videoSize,
    };
  }
  return {
    fileName: message.fileElem.fileName,
    fileSize: message.fileElem.fileSize,
  };
};
