import { toLocalFileURL } from "@/im/local-files";
import { Spin } from "antd";
import {
  FileElem,
  PictureElem,
  SoundElem,
  VideoElem,
} from "@/im/types";
import { FC, memo } from "react";
import { useTranslation } from "react-i18next";

import CacheImage from "@/components/CacheImage";
import OIMAvatar from "@/components/OIMAvatar";
import { useMessageFileDownloadState } from "@/hooks/useMessageFileDownloadState";
import { ExMessageItem, useUserStore } from "@/store";
import FileDownloadIcon from "@/svg/FileDownloadIcon";
import emitter from "@/utils/events";
import { formatMessageTime } from "@/utils/imCommon";

import MessageItemErrorBoundary from "./MessageItem/MessageItemErrorBoundary";

export interface SystemNotificationElem {
  notificationName: string;
  notificationFaceURL: string;
  notificationType: number;
  text: string;
  externalUrl?: string;
  mixType: NotificationMixType;
  pictureElem?: PictureElem;
  soundElem?: SoundElem;
  videoElem?: VideoElem;
  fileElem?: FileElem;
  ex: string;
}

enum NotificationMixType {
  Text,
  TextWithImage,
  TextWithVideo,
  TextWithFile,
  TextWithVoice,
  TextWithVoiceAndImage,
}

const min = (a: number, b: number) => (a > b ? b : a);

const mediaTypes = [
  NotificationMixType.TextWithImage,
  NotificationMixType.TextWithVideo,
];

const SystemNotification: FC<{ message: ExMessageItem }> = ({ message }) => {
  return (
    <div className="relative flex select-text px-5 py-3 pt-9">
      <div className="flex w-full">
        <OIMAvatar
          size={36}
          src={message.senderFaceUrl}
          text={message.senderNickname}
        />

        <div className="ml-3 flex-1">
          <div className="flex leading-4">
            <div
              title={message.senderNickname}
              className="mr-2 max-w-[30%] truncate text-[var(--sub-text)]"
            >
              {message.senderNickname}
            </div>
            <div className="text-[var(--sub-text)]">
              {formatMessageTime(message.sendTime)}
            </div>
          </div>
          <div
            className="mt-2 w-fit max-w-[80%] rounded-md p-2"
            style={{
              boxShadow: "0px 3px 12px 1px rgba(0,0,0,0.16)",
            }}
          >
            <MessageItemErrorBoundary message={message}>
              <NotificationRender message={message} />
            </MessageItemErrorBoundary>
          </div>
        </div>
      </div>
      {message.gapTime && (
        <div className="absolute left-1/2 top-3 -translate-x-1/2 text-xs text-[var(--sub-text)]">
          {formatMessageTime(message.sendTime, true)}
        </div>
      )}
    </div>
  );
};

export default memo(SystemNotification);

const NotificationRender = ({ message }: { message: ExMessageItem }) => {
  const { t } = useTranslation();
  const notificationEl: SystemNotificationElem = JSON.parse(
    message.notificationElem.detail,
  );

  return (
    <>
      {mediaTypes.includes(notificationEl.mixType) && (
        <MediaRow message={message} notificationEl={notificationEl} />
      )}
      <div>{notificationEl.text}</div>
      {notificationEl.externalUrl && (
        <div className="mt-0.5 text-xs text-[#0289FAFF]">
          <a href={notificationEl.externalUrl} target="_blank" rel="noreferrer">
            {t("clickToView")}
          </a>
        </div>
      )}
    </>
  );
};

const playVideo = (path: string) => {
  const sourcePath = window.electronAPI ? toLocalFileURL(path) : path;
  emitter.emit("OPEN_VIDEO_PLAYER", sourcePath);
};

const MediaRow = ({
  notificationEl,
  message,
}: {
  notificationEl: SystemNotificationElem;
  message: ExMessageItem;
}) => {
  const imageCache = useUserStore((state) => state.imageCache);
  const { progress, downloadState, tryDownload } = useMessageFileDownloadState(
    message,
    playVideo,
  );

  const withVideo = notificationEl.mixType === NotificationMixType.TextWithVideo;
  const imageHeight =
    (withVideo
      ? notificationEl.videoElem?.snapshotHeight
      : notificationEl.pictureElem?.sourcePicture.height) ?? 0;
  const imageWidth =
    (withVideo
      ? notificationEl.videoElem?.snapshotWidth
      : notificationEl.pictureElem?.sourcePicture.width) ?? 0;
  const snapshotMaxHeight =
    (withVideo ? 240 : notificationEl.pictureElem?.sourcePicture.height) ?? 0;
  const minHeight = min(240, imageWidth) * (imageHeight / imageWidth);
  const adaptedHight = min(minHeight, snapshotMaxHeight);
  const adaptedWidth = (adaptedHight / imageHeight) * imageWidth;

  const minStyle = { minHeight: `${adaptedHight}px`, minWidth: `${adaptedWidth}px` };

  const imageUrl = withVideo
    ? notificationEl.videoElem?.snapshotUrl
    : notificationEl.pictureElem?.sourcePicture.url;

  const tryPlayVideo = () => {
    if (!withVideo) return;
    if (!window.electronAPI) {
      playVideo(notificationEl.videoElem?.videoUrl ?? "");
    } else {
      tryDownload();
    }
  };

  return (
    <div>
      <div className="relative cursor-pointer" style={minStyle} onClick={tryPlayVideo}>
        <CacheImage
          className="max-h-[240px] rounded-md"
          src={imageCache[imageUrl ?? ""] ?? imageUrl}
          preview={
            withVideo
              ? false
              : {
                  maskClassName: "rounded-md",
                  src: imageCache[imageUrl ?? ""] ?? imageUrl,
                }
          }
          placeholder={
            <div style={minStyle} className="flex items-center justify-center">
              <Spin />
            </div>
          }
        />
        {withVideo && (
          <div
            className="absolute left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 cursor-pointer"
            onClick={tryPlayVideo}
          >
            <FileDownloadIcon
              size={40}
              pausing={downloadState === "pause"}
              finished={downloadState === "finish" || !window.electronAPI}
              percent={progress}
            />
          </div>
        )}
      </div>
      <div className="my-1 h-px bg-[#f3f3f3]" />
    </div>
  );
};
