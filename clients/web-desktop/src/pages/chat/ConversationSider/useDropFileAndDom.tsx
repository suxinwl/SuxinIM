import { useDrop } from "ahooks";
import { t } from "i18next";
import { GroupMemberRole, GroupStatus, MessageType } from "@openim/wasm-client-sdk";
import { ConversationItem } from "@/im/types";
import { useState } from "react";

import { modal } from "@/AntdGlobalComp";
import file_icon from "@/assets/images/messageItem/file_icon.png";
import { IMSDK } from "@/layout/MainContentWrap";
import { ExMessageItem, useUserStore } from "@/store";
import { bytesToSize } from "@/utils/common";

import { useFileMessage } from "../queryChat/ChatFooter/SendActionBar/useFileMessage";
import { useSendMessage } from "../queryChat/ChatFooter/useSendMessage";

export function useDropFileAndDom({
  domRef,
  currentConversation,
}: {
  domRef: React.RefObject<HTMLDivElement>;
  currentConversation: ConversationItem;
}) {
  const [droping, setDroping] = useState(false);
  const { createFileMessage } = useFileMessage();
  const { sendMessage } = useSendMessage();

  const dropEnd = () => {
    if (!droping) return;
    setDroping(false);
  };

  useDrop(domRef.current, {
    onText: (_, e) => {
      e?.preventDefault();
      dropEnd();
    },
    onFiles: async (files, e) => {
      if (!(await getIsCanSendMessage(currentConversation))) {
        dropEnd();
        return;
      }

      if (files.length) {
        modal.confirm({
          title: `${t("placeholder.sendTo")}${currentConversation.showName}`,
          icon: null,
          width: 320,
          centered: true,
          className: "drop-file-moal",
          content: (
            <div className="h-[240px] overflow-y-auto border-b border-t border-[var(--gap-text)] p-2">
              {files.map((file) => (
                <div className="mb-2 flex items-center" key={file.lastModified}>
                  <img width={38} src={file_icon} alt="file" />
                  <div className="ml-3 overflow-hidden">
                    <div className="mb-1.5 truncate">{file.name}</div>
                    <div className="text-xs text-[var(--sub-text)]">
                      {bytesToSize(file.size)}
                    </div>
                  </div>
                </div>
              ))}
            </div>
          ),
          okText: t("confirm"),
          cancelText: t("cancel"),
          onOk: () => {
            files.map(async (file) => {
              const message = await createFileMessage(file);
              sendMessage({
                message: message,
                recvID: currentConversation.userID,
                groupID: currentConversation.groupID,
              });
            });
          },
        });
      }

      dropEnd();
    },
    onUri: (_, e) => {
      e?.preventDefault();
      dropEnd();
    },
    onDom: async ({ message }: { message: ExMessageItem }, e) => {
      if (!(await getIsCanSendMessage(currentConversation))) {
        dropEnd();
        return;
      }
      const { fileName, fileSize } = getFileData(message);
      modal.confirm({
        title: `${t("placeholder.sendTo")}${currentConversation.showName}`,
        icon: null,
        width: 320,
        centered: true,
        className: "drop-file-moal",
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
            recvID: currentConversation.userID,
            groupID: currentConversation.groupID,
          });
        },
      });
      dropEnd();
    },
    onDragEnter: () => {
      if (droping) return;
      setDroping(true);
    },
    onDragLeave: dropEnd,
  });

  return {
    droping,
  };
}

const getIsCanSendMessage = async (conversation: ConversationItem) => {
  if (conversation?.userID) {
    return true;
  }
  const { data: members } = await IMSDK.getSpecifiedGroupMembersInfo({
    groupID: conversation.groupID,
    userIDList: [useUserStore.getState().selfInfo.userID],
  });
  const member = members[0];
  if (!member) {
    return false;
  }

  const { data: groups } = await IMSDK.getSpecifiedGroupsInfo([conversation.groupID]);
  const group = groups[0];

  if (
    (group && group.status === GroupStatus.Dismissed) ||
    (group.status === GroupStatus.Muted && member.roleLevel === GroupMemberRole.Normal)
  ) {
    return false;
  }

  return member.muteEndTime < Date.now();
};

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
