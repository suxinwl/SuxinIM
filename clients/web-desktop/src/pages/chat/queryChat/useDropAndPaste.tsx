import { useDrop } from "ahooks";
import { t } from "i18next";
import { ConversationItem } from "@/im/types";
import { useState } from "react";

import { modal } from "@/AntdGlobalComp";
import file_icon from "@/assets/images/messageItem/file_icon.png";
import { insertAtCursor } from "@/components/EditableDiv";
import { bytesToSize } from "@/utils/common";

import { useFileMessage } from "./ChatFooter/SendActionBar/useFileMessage";
import { useSendMessage } from "./ChatFooter/useSendMessage";

export function useDropAndPaste({
  currentConversation,
  getIsCanSendMessage,
}: {
  currentConversation?: ConversationItem;
  getIsCanSendMessage: () => boolean;
}) {
  const [droping, setDroping] = useState(false);
  const { createFileMessage } = useFileMessage();
  const { sendMessage } = useSendMessage();

  const dropEnd = () => {
    if (!droping) return;
    setDroping(false);
  };

  useDrop(document.getElementById("chat-container"), {
    onText: () => {
      dropEnd();
    },
    onFiles: async (files, e) => {
      if (!getIsCanSendMessage()) return;
      const imageFiles = [] as File[];
      const otherFiles = [] as File[];
      files.map((file) => {
        if (file.type.includes("image")) {
          imageFiles.push(file);
        } else {
          otherFiles.push(file);
        }
      });
      if (imageFiles.length) {
        const images = [] as HTMLImageElement[];
        for await (const file of imageFiles) {
          const image = new Image();
          const objUrl = URL.createObjectURL(file);
          image.setAttribute("class", "image-el");
          image.setAttribute("ondblclick", `screenshotPreview("${objUrl}")`);
          image.src = objUrl;
          images.push(image);
        }
        insertAtCursor(images);
      }

      if (otherFiles.length) {
        modal.confirm({
          title: `${t("placeholder.sendTo")}${currentConversation?.showName}`,
          icon: null,
          width: 320,
          centered: true,
          className: "drop-file-moal",
          content: (
            <div className="h-[240px] overflow-y-auto border-b border-t border-[var(--gap-text)] p-2">
              {otherFiles.map((file) => (
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
            otherFiles.map(async (file) => {
              const message = await createFileMessage(file);
              sendMessage({
                message: message,
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
    onDom: (_, e) => {
      e?.preventDefault();
      dropEnd();
    },
    onDragEnter: (e) => {
      if (e?.dataTransfer.types[0] === "custom") return;
      if (!getIsCanSendMessage()) return;
      setDroping(true);
    },
    onDragLeave: dropEnd,
  });

  return {
    droping,
  };
}
