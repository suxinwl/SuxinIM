import { toLocalFileURL, isLocalFileURL, localFilePath } from "@/im/local-files";
import { Image } from "antd";
import { useEffect, useState } from "react";

import CacheImage from "@/components/CacheImage";
import PreviewToolsBar from "@/components/PreviewToolsBar";
import { useMessageStore, useUserStore } from "@/store";
import FileDownloadIcon from "@/svg/FileDownloadIcon";
import { MomentContentType, MomentMeta, WorkMoments } from "@/types/moment";
import { downloadFile, getDownloadTask } from "@/utils/common";
import emitter from "@/utils/events";

const MomentsMediaRow = ({ moments }: { moments: WorkMoments }) => {
  const [previewState, setPreviewState] = useState({
    current: 0,
    visible: false,
  });
  const [previewList, setPreviewList] = useState<string[]>([]);
  const imageCache = useUserStore((state) => state.imageCache);
  const currentTask = useMessageStore((state) =>
    getDownloadTask({
      downloadMap: state.downloadMap,
      compareKey: "workMomentID",
      compareValue: moments.workMomentID,
    }),
  );
  const updateDownloadTask = useMessageStore((state) => state.updateDownloadTask);

  const isTextWithVideo = moments.content.type === MomentContentType.TextWithVideo;
  const isVideoDownloaded = isLocalFileURL(previewList[0]);

  useEffect(() => {
    if (moments.content.metas) {
      setPreviewList(
        moments.content.metas.map((meta) =>
          imageCache[meta.original]
            ? toLocalFileURL(imageCache[meta.original])
            : meta.original,
        ),
      );
    }
    const downloadSuccessHandler = (url: string, filePath: string) => {
      const { workMomentID } = useMessageStore.getState().downloadMap[url];

      if (workMomentID?.split("-")[0] === moments.workMomentID) {
        setPreviewList((list) => {
          const tmpList = [...list];
          tmpList[Number(workMomentID.split("-")[1]) || 0] = toLocalFileURL(filePath);
          return tmpList;
        });
      }
    };

    const unsubscribeSuccess = window.electronAPI?.subscribe(
      "downloadSuccess",
      downloadSuccessHandler,
    );
    return () => {
      unsubscribeSuccess?.();
    };
  }, []);

  const tryPlayVideo = () => {
    if (!isTextWithVideo) return;

    if (isVideoDownloaded || !window.electronAPI) {
      emitter.emit("OPEN_VIDEO_PLAYER", previewList[0]);
      return;
    }
    if (
      currentTask?.downloadState === "downloading" ||
      currentTask?.downloadState === "resume"
    ) {
      window.electronAPI?.ipcInvoke("pauseDownload", currentTask.downloadUrl!);
      updateDownloadTask(currentTask.downloadUrl!, {
        downloadState: "pause",
      });
      return;
    }

    if (currentTask?.downloadState === "pause") {
      window.electronAPI?.ipcInvoke("resumeDownload", currentTask.downloadUrl!);
      updateDownloadTask(currentTask.downloadUrl!, {
        downloadState: "downloading",
      });
      return;
    }
    const sourceUrl = moments.content.metas?.[0].original;
    if (!currentTask && sourceUrl) {
      downloadFile(sourceUrl, {
        isMediaMessage: true,
        showError: true,
        isThumb: true,
        saveType: isTextWithVideo ? "video" : "image",
        workMomentID: moments.workMomentID,
      });
    }
  };

  const downloadOrShowFinder = (current: number) => {
    const currentTask = getDownloadTask({
      downloadMap: useMessageStore.getState().downloadMap,
      compareKey: "workMomentID",
      compareValue: `${moments.workMomentID}-${current}`,
    });

    if (currentTask?.downloadState === "pause") {
      window.electronAPI?.ipcInvoke("resumeDownload", currentTask?.downloadUrl);
      updateDownloadTask(currentTask?.downloadUrl ?? "", {
        downloadState: "downloading",
      });
      return;
    }
    if (!previewList[current].startsWith("http")) {
      window.electronAPI?.showInFinder(localFilePath(previewList[current]));
      return;
    }
    downloadFile(previewList[current], {
      isMediaMessage: true,
      showError: true,
      isThumb: true,
      saveType: isTextWithVideo ? "video" : "image",
      workMomentID: `${moments.workMomentID}-${current}`,
    });
  };

  const checkDownload = (current: number) => {
    if (window.electronAPI && previewList[current].startsWith("http")) {
      setTimeout(
        () =>
          downloadFile(previewList[current], {
            isMediaMessage: true,
            showError: true,
            isThumb: true,
            saveType: isTextWithVideo ? "video" : "image",
            workMomentID: `${moments.workMomentID}-${current}`,
          }),
        200,
      );
    }
  };

  const onPreviewIdxChange = (next: number) => {
    setPreviewState((state) => ({ ...state, current: next }));
    checkDownload(next);
  };

  const getImageUrl = (metas?: MomentMeta) => {
    if (!metas) {
      metas = moments.content.metas?.[0];
    }
    let sourceUrl = imageCache[metas?.thumb ?? ""] || metas?.thumb;
    if (!isTextWithVideo) {
      sourceUrl = imageCache[metas?.original ?? ""] || sourceUrl;
    }
    return sourceUrl as string;
  };

  return (
    <div>
      {
        <div className="grid w-fit grid-cols-3">
          <Image.PreviewGroup
            items={previewList}
            preview={{
              current: previewState.current,
              visible: previewState.visible,
              onVisibleChange: (vis) =>
                setPreviewState((state) => ({ ...state, visible: vis })),
              onChange: onPreviewIdxChange,
              toolbarRender: (originalNode, { current }) => (
                <PreviewToolsBar
                  workMomentID={`${moments.workMomentID}-${current}`}
                  isFileDownloaded={!previewList[current].startsWith("http")}
                  dom={originalNode as JSX.Element}
                  onDownloadOrShowFinder={() => downloadOrShowFinder(current)}
                />
              ),
            }}
          >
            {moments.content.metas?.length === 1 ? (
              <div className="relative w-fit" onClick={tryPlayVideo}>
                <CacheImage
                  rootClassName="cursor-pointer"
                  className="max-w-[200px]"
                  src={getImageUrl()}
                  preview={!isTextWithVideo}
                  onClick={() => onPreviewIdxChange(0)}
                />
                {isTextWithVideo && (
                  <div className="absolute left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 cursor-pointer">
                    <FileDownloadIcon
                      size={40}
                      pausing={currentTask?.downloadState === "pause"}
                      finished={isVideoDownloaded || !window.electronAPI}
                      percent={currentTask?.progress ?? 0}
                    />
                  </div>
                )}
              </div>
            ) : (
              moments.content.metas?.map((meta, idx) => (
                <CacheImage
                  key={idx}
                  rootClassName="cursor-pointer w-[120px] h-[120px] mr-2 mb-2 rounded-sm overflow-hidden"
                  className="!h-full object-cover"
                  src={getImageUrl(meta)}
                  onClick={() => onPreviewIdxChange(idx)}
                />
              ))
            )}
          </Image.PreviewGroup>
        </div>
      }
    </div>
  );
};

export default MomentsMediaRow;
