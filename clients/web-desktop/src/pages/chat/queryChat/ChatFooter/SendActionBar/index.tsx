import { DownOutlined } from "@ant-design/icons";
import { Popover, PopoverProps, Upload } from "antd";
import { TooltipPlacement } from "antd/es/tooltip";
import clsx from "clsx";
import i18n, { t } from "i18next";
import { UploadRequestOption } from "rc-upload/lib/interface";
import { memo, ReactNode, useCallback, useEffect, useState } from "react";
import React from "react";

import { message as antdMessage } from "@/AntdGlobalComp";
import card from "@/assets/images/chatFooter/card.png";
import cut from "@/assets/images/chatFooter/cut.png";
import emoji from "@/assets/images/chatFooter/emoji.png";
import file from "@/assets/images/chatFooter/file.png";
import image from "@/assets/images/chatFooter/image.png";
import rtc from "@/assets/images/chatFooter/rtc.png";
import video from "@/assets/images/chatFooter/video.png";
import { insertAtCursor } from "@/components/EditableDiv";
import { ExMessageItem } from "@/store";
import { base64toFile } from "@/utils/common";
import emitter from "@/utils/events";

import { SendMessageParams } from "../useSendMessage";
import CallPopContent from "./CallPopContent";
import CutPopContent from "./CutPopContent";
import EmojiPopContent from "./EmojiPopContent";

const sendActionList = [
  {
    title: t("placeholder.emoji"),
    icon: emoji,
    key: "emoji",
    accept: undefined,
    comp: <EmojiPopContent />,
    placement: "topLeft",
  },
  {
    title: t("placeholder.screenshot"),
    icon: cut,
    key: "cut",
    accept: undefined,
    comp: <CutPopContent />,
    placement: "bottomLeft",
  },
  {
    title: t("placeholder.image"),
    icon: image,
    key: "image",
    accept: "image/*",
    comp: null,
    placement: undefined,
  },
  {
    title: t("placeholder.video"),
    icon: video,
    key: "video",
    accept: ".mp4",
    comp: null,
    placement: undefined,
  },
  {
    title: t("placeholder.card"),
    icon: card,
    key: "card",
    accept: undefined,
    comp: null,
    placement: undefined,
  },
  {
    title: t("placeholder.file"),
    icon: file,
    key: "file",
    accept: "*",
    comp: null,
    placement: undefined,
  },
  {
    title: t("placeholder.call"),
    icon: rtc,
    key: "rtc",
    accept: undefined,
    comp: <CallPopContent />,
    placement: "top",
  },
];

i18n.on("languageChanged", () => {
  sendActionList[0].title = t("placeholder.emoji");
  sendActionList[1].title = t("placeholder.screenshot");
  sendActionList[2].title = t("placeholder.image");
  sendActionList[3].title = t("placeholder.video");
  sendActionList[4].title = t("placeholder.card");
  sendActionList[5].title = t("placeholder.file");
  sendActionList[6].title = t("placeholder.call");
});

const SendActionBar = ({
  updateHtml,
  inputFocus,
  sendMessage,
  createFileMessage,
}: {
  updateHtml: () => void;
  inputFocus: () => void;
  sendMessage: (params: SendMessageParams) => Promise<void>;
  createFileMessage: (file: File) => Promise<ExMessageItem>;
}) => {
  const [visibleState, setVisibleState] = useState({
    emoji: false,
    cut: false,
    rtc: false,
  });

  useEffect(() => {
    const dispose = window.electronAPI?.subscribe(
      "captureResponse",
      (base64?: string) => {
        inputFocus();
        if (!base64) return;
        const file = base64toFile(base64);
        const objUrl = URL.createObjectURL(file);
        const image = new Image();
        image.setAttribute("class", "image-el");
        image.setAttribute("ondblclick", `screenshotPreview("${objUrl}")`);
        image.src = objUrl;
        insertAtCursor([image]);
        updateHtml();
        inputFocus();
      },
    );
    return () => {
      dispose?.();
    };
  }, []);

  const callClick = useCallback(
    () => setVisibleState({ cut: false, rtc: false, emoji: false }),
    [],
  );
  const cutWithoutWindow = useCallback(() => {
    window.electronAPI?.ipcInvoke("capture", true);
    setVisibleState({ cut: false, rtc: false, emoji: false });
  }, []);

  const actionClick = (key: string) => {
    if (key === "card") {
      emitter.emit("OPEN_CHOOSE_MODAL", {
        type: "SELECT_CARD",
      });
    }
    if (key === "cut") {
      window.electronAPI?.ipcInvoke("capture");
    }
  };

  const fileHandle = async (options: UploadRequestOption) => {
    const fileEl = options.file as File;
    if (fileEl.size === 0) {
      antdMessage.warning(t("empty.fileContentEmpty"));
      return;
    }
    const message = await createFileMessage(fileEl);
    sendMessage({
      message,
    });
  };

  return (
    <div className="flex items-center px-4.5 pt-2">
      {sendActionList.map((action) => {
        const isCut = action.key === "cut";

        const popProps: PopoverProps = {
          placement: action.placement as TooltipPlacement,
          content:
            action.comp &&
            React.cloneElement(action.comp as React.ReactElement, {
              updateHtml,
              callClick,
              cutWithoutWindow,
            }),
          title: null,
          arrow: false,
          trigger: "click",
          // @ts-ignore
          open: action.key ? visibleState[action.key] : false,
          onOpenChange: (visible) =>
            setVisibleState((state) => {
              const tmpState = { ...state };
              // @ts-ignore
              tmpState[action.key] = visible;
              return tmpState;
            }),
        };

        if (isCut) {
          return window.electronAPI ? (
            <div
              className="mr-5 flex cursor-pointer items-center last:mr-0"
              key={action.key}
              onClick={() => actionClick(action.key)}
            >
              <img src={action.icon} width={20} alt={action.title} />
              <Popover {...popProps}>
                <DownOutlined
                  className="ml-px scale-75 text-xs"
                  rev={undefined}
                  onClick={(e) => e.stopPropagation()}
                />
              </Popover>
            </div>
          ) : null;
        }

        return (
          <ActionWrap
            popProps={popProps}
            key={action.key}
            accept={action.accept}
            fileHandle={fileHandle}
          >
            <div
              className={clsx("flex cursor-pointer items-center last:mr-0", {
                "mr-5": !action.accept,
              })}
              onClick={() => actionClick(action.key)}
            >
              <img src={action.icon} width={20} alt={action.title} />
            </div>
          </ActionWrap>
        );
      })}
    </div>
  );
};

export default memo(SendActionBar);

const ActionWrap = ({
  accept,
  popProps,
  children,
  fileHandle,
}: {
  accept?: string;
  children: ReactNode;
  popProps?: PopoverProps;
  fileHandle: (options: UploadRequestOption) => void;
}) => {
  return accept ? (
    <Upload
      showUploadList={false}
      customRequest={fileHandle}
      accept={accept}
      multiple
      className="mr-5 flex"
    >
      {children}
    </Upload>
  ) : (
    <Popover {...popProps}>{children}</Popover>
  );
};
