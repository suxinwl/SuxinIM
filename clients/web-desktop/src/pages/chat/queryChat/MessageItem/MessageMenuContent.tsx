import i18n, { t } from "i18next";
import { MessageStatus, MessageType } from "@openim/wasm-client-sdk";
import { memo, useRef } from "react";
import { useCopyToClipboard } from "react-use";

import check from "@/assets/images/messageMenu/check.png";
import copy from "@/assets/images/messageMenu/copy.png";
import emoji from "@/assets/images/messageMenu/emoji.png";
import finder from "@/assets/images/messageMenu/finder.png";
import forward from "@/assets/images/messageMenu/forward.png";
import remove from "@/assets/images/messageMenu/remove.png";
import reply from "@/assets/images/messageMenu/reply.png";
import revoke from "@/assets/images/messageMenu/revoke.png";
import { useCurrentMemberRole } from "@/hooks/useCurrentMemberRole";
import { getSourceData } from "@/hooks/useMessageFileDownloadState";
import { IMSDK } from "@/layout/MainContentWrap";
import {
  ExMessageItem,
  getImageMessageSourceUrl,
  useConversationStore,
  useMessageStore,
  useUserStore,
} from "@/store";
import { feedbackToast } from "@/utils/common";
import emitter from "@/utils/events";
import { isGroupSession } from "@/utils/imCommon";
import { getUserCustomEmojis, setUserCustomEmojis } from "@/utils/storage";

const messageMenuList = [
  {
    idx: 0,
    title: t("placeholder.add"),
    icon: emoji,
    hidden: false,
  },
  {
    idx: 1,
    title: t("placeholder.forward"),
    icon: forward,
    hidden: false,
  },
  {
    idx: 2,
    title: t("placeholder.copy"),
    icon: copy,
    hidden: false,
  },
  {
    idx: 3,
    title: t("placeholder.check"),
    icon: check,
    hidden: false,
  },
  {
    idx: 4,
    title: t("placeholder.reply"),
    icon: reply,
    hidden: false,
  },
  {
    idx: 5,
    title: t("placeholder.revoke"),
    icon: revoke,
    hidden: false,
  },
  {
    idx: 6,
    title: t("placeholder.delete"),
    icon: remove,
    hidden: false,
  },
  {
    idx: 7,
    title: t("placeholder.finder"),
    icon: finder,
    hidden: false,
  },
];

i18n.on("languageChanged", () => {
  messageMenuList[0].title = t("placeholder.add");
  messageMenuList[1].title = t("placeholder.forward");
  messageMenuList[2].title = t("placeholder.copy");
  messageMenuList[3].title = t("placeholder.check");
  messageMenuList[4].title = t("placeholder.reply");
  messageMenuList[5].title = t("placeholder.revoke");
  messageMenuList[6].title = t("placeholder.delete");
  messageMenuList[7].title = t("placeholder.finder");
});

const canCopyTypes = [
  MessageType.TextMessage,
  MessageType.AtTextMessage,
  MessageType.QuoteMessage,
  MessageType.PictureMessage,
];

const canAddPhizTypes = [MessageType.PictureMessage, MessageType.FaceMessage];
const canDownloadTypes = [
  MessageType.PictureMessage,
  MessageType.VideoMessage,
  MessageType.FileMessage,
];

const MessageMenuContent = ({
  message,
  conversationID,
  closeMenu,
}: {
  message: ExMessageItem;
  conversationID: string;
  closeMenu: () => void;
}) => {
  const copying = useRef(false);
  const selfUserID = useUserStore((state) => state.selfInfo.userID);
  const ownerUserID = useConversationStore(
    (state) => state.currentGroupInfo?.ownerUserID,
  );
  const updateCheckMode = useMessageStore((state) => state.updateCheckMode);
  const updateOneMessage = useMessageStore((state) => state.updateOneMessage);
  const deleteOneMessage = useMessageStore((state) => state.deleteOneMessage);
  const updateQuoteMessage = useConversationStore((state) => state.updateQuoteMessage);
  const addRevokedMessage = useConversationStore((state) => state.addRevokedMessage);

  const [_, copyToClipboard] = useCopyToClipboard();
  const { isNomal, isAdmin } = useCurrentMemberRole();

  const getCustomEmojiData = async () => {
    let sourceData = {
      path: "",
      url: "",
      width: 0,
      height: 0,
    };
    if (message.contentType === MessageType.PictureMessage) {
      sourceData = {
        path: message.pictureElem.sourcePath,
        url: message.pictureElem.sourcePicture.url,
        width: message.pictureElem.sourcePicture.width,
        height: message.pictureElem.sourcePicture.height,
      };
    }
    if (message.contentType === MessageType.FaceMessage) {
      const faceEl = JSON.parse(message.faceElem.data);
      sourceData = {
        path: faceEl.path ?? "",
        url: faceEl.url,
        width: faceEl.width,
        height: faceEl.height,
      };
    }
    if (window.electronAPI?.fileExists(sourceData.path)) {
      return sourceData;
    }
    if (window.electronAPI?.fileExists(message.localEx ?? "")) {
      sourceData.path = message.localEx;
      return sourceData;
    }

    const blob = await fetch(sourceData.url).then((response) => response.blob());
    const file = new File([blob], getFileNameFromUrl(sourceData.url), {
      type: blob.type,
    });
    sourceData.path =
      (await window.electronAPI?.saveFileToDisk({
        file,
        type: "fileCache",
        sync: true,
      })) ?? "";
    return sourceData;
  };

  const getFileNameFromUrl = (url: string) => {
    const idx = url.lastIndexOf("/");
    return url.slice(idx + 1);
  };

  const menuClick = (idx: number) => {
    switch (idx) {
      case 0:
        getUserCustomEmojis().then(async (res) => {
          setUserCustomEmojis([...res, await getCustomEmojiData()]);
          feedbackToast({ msg: t("toast.addSuccess") });
        });
        break;
      case 1:
        emitter.emit("OPEN_CHOOSE_MODAL", {
          type: "FORWARD_MESSAGE",
          extraData: message,
        });
        break;
      case 2:
        if (message.contentType === MessageType.PictureMessage) {
          copyImage();
        } else {
          copyToClipboard(getCopyText());
          feedbackToast({ msg: t("toast.copySuccess") });
        }
        break;
      case 3:
        updateOneMessage({ ...message, checked: true });
        updateCheckMode(true);
        break;
      case 4:
        updateQuoteMessage(message);
        break;
      case 5:
        tryRevoke();
        break;
      case 6:
        tryRemove();
        break;
      case 7:
        window.electronAPI?.showInFinder(
          message.localEx || getSourceData(message).path,
        );
        break;
      default:
        break;
    }
    closeMenu();
  };

  const copyImage = async () => {
    let error;
    if (copying.current) return;
    copying.current = true;
    try {
      const sourceUrl = getImageMessageSourceUrl(message);
      let blob = await fetch(sourceUrl).then((response) => response.blob());
      if (blob.type !== "image/png") {
        blob = await convertToPng(blob);
      }

      await navigator.clipboard.write([
        new window.ClipboardItem({
          "image/png": blob,
        }),
      ]);
    } catch (err) {
      error = err;
      console.error(error);
    }
    feedbackToast({
      error,
      msg: error ? t("toast.copyFailed") : t("toast.copySuccess"),
    });
    copying.current = false;
  };

  const tryRevoke = async () => {
    try {
      await IMSDK.revokeMessage({ conversationID, clientMsgID: message.clientMsgID });
      updateOneMessage({
        ...message,
        contentType: MessageType.RevokeMessage,
        notificationElem: {
          detail: JSON.stringify({
            clientMsgID: message.clientMsgID,
            revokeTime: Date.now(),
            revokerID: selfUserID,
            revokerNickname: t("you"),
            revokerRole: 0,
            seq: message.seq,
            sessionType: message.sessionType,
            sourceMessageSendID: message.sendID,
            sourceMessageSendTime: message.sendTime,
            sourceMessageSenderNickname: message.senderNickname,
          }),
        },
      });
      if (
        canCopyTypes.slice(0, 3).includes(message.contentType) &&
        message.sendID === selfUserID
      ) {
        addRevokedMessage({ ...message }, message.quoteElem?.quoteMessage);
      }
    } catch (error) {
      feedbackToast({ error });
    }
  };

  const tryRemove = async () => {
    try {
      await IMSDK.deleteMessage({ clientMsgID: message.clientMsgID, conversationID });
      deleteOneMessage(message.clientMsgID);
    } catch (error) {
      feedbackToast({ error });
    }
  };

  const getCopyText = () => {
    const selection = window.getSelection()?.toString();
    if (selection) return selection;

    if (message.contentType === MessageType.AtTextMessage) {
      return message.atTextElem.text;
    }
    if (message.contentType === MessageType.QuoteMessage) {
      return message.quoteElem.text;
    }
    return message.textElem.content;
  };

  const senderIsOwner = message.sendID === ownerUserID;
  const isSender = message.sendID === selfUserID;
  const moreThanRevokeLimit = message.sendTime < Date.now() - 5 * 60 * 1000;
  const messageIsSuccess = message.status === MessageStatus.Succeed;

  return (
    <div className="p-1">
      {messageMenuList.map((menu) => {
        if (menu.idx === 0 && !canAddPhizTypes.includes(message.contentType)) {
          return null;
        }

        if (menu.idx === 2 && !canCopyTypes.includes(message.contentType)) {
          return null;
        }

        if (
          (menu.idx === 4 || menu.idx === 5) &&
          (!messageIsSuccess || message.contentType === MessageType.CustomMessage)
        ) {
          return null;
        }

        if (menu.idx === 5) {
          if (moreThanRevokeLimit && isNomal) return null;

          if (!isSender && !isGroupSession(message.sessionType)) return null;

          if (isGroupSession(message.sessionType)) {
            if ((isAdmin && senderIsOwner) || (isNomal && !isSender)) {
              return null;
            }
          }
        }

        if (menu.idx === 7) {
          if (!canDownloadTypes.includes(message.contentType) || !window.electronAPI) {
            return null;
          }
          const sourceUrl = message.localEx || getSourceData(message).path;
          if (!sourceUrl || !window.electronAPI?.fileExists(sourceUrl)) {
            return null;
          }
        }
        return (
          <div
            className="flex cursor-pointer items-center rounded px-3 py-2 hover:bg-[var(--primary-active)]"
            key={menu.idx}
            onClick={() => menuClick(menu.idx)}
            onMouseDown={(e) => e.preventDefault()}
          >
            <img className="mr-2 h-3.5" width={14} src={menu.icon} alt={menu.title} />
            <div className="text-xs">{menu.title}</div>
          </div>
        );
      })}
    </div>
  );
};

export default memo(MessageMenuContent);

async function convertToPng(blob: Blob): Promise<Blob> {
  return new Promise((resolve, reject) => {
    const img = new Image();

    img.onload = () => {
      const canvas = document.createElement("canvas");
      canvas.width = img.width;
      canvas.height = img.height;

      const ctx = canvas.getContext("2d");
      ctx?.drawImage(img, 0, 0);

      canvas.toBlob((pngBlob) => {
        resolve(pngBlob!);
      }, "image/png");
    };

    img.onerror = reject;

    img.src = URL.createObjectURL(blob);
  });
}
