import { toLocalFileURL, isLocalFileURL, localFilePath } from "@/im/local-files";
import ProtectedPreviewGroup from "@/components/ProtectedPreviewGroup";
import { useEffect, useState, useSyncExternalStore } from "react";
import { authorizeResource, requiresResourceGrant } from "@/im/resources";
import { getResourceSessionVersion, subscribeResourceSession } from "@/im/resource-session";
import { momentMediaSource } from "@/api/moments-media";

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
  const sessionVersion = useSyncExternalStore(subscribeResourceSession, getResourceSessionVersion);
  const previewKey = JSON.stringify([sessionVersion, moments.workMomentID, moments.content.metas?.map(meta => momentMediaSource(meta, "original"))]);
  const [previews, setPreviews] = useState<{ key: string; items: string[] }>();
  const previewList = previews?.key === previewKey ? previews.items : [];
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
    let live = true;
    void Promise.all((moments.content.metas ?? []).map(async meta => {
      try {
        const source = momentMediaSource(meta, "original"), url = await authorizeResource(source);
        return !requiresResourceGrant(source) && imageCache[source] ? toLocalFileURL(imageCache[source]) : url;
      } catch { return ""; }
    })).then(items => { if (live) setPreviews({ key: previewKey, items }); });
    const downloadSuccessHandler = (url: string, filePath: string) => {
      const { workMomentID } = useMessageStore.getState().downloadMap[url] ?? {};

      if (workMomentID?.split("-")[0] === moments.workMomentID) {
        const index = Number(workMomentID.split("-")[1]) || 0;
        const meta = moments.content.metas?.[index];
        const source = meta && momentMediaSource(meta, "original");
        if (!source) return;
        // A local file must not bypass access checks after a SID/policy change.
        void authorizeResource(source, { force: true }).then(() => {
          if (!live) return;
          setPreviews(current => {
            if (current?.key !== previewKey) return current;
            const items = [...current.items]; items[index] = toLocalFileURL(filePath);
            return { key: previewKey, items };
          });
        }).catch(() => {});
      }
    };

    const unsubscribeSuccess = window.electronAPI?.subscribe(
      "downloadSuccess",
      downloadSuccessHandler,
    );
    return () => {
      live = false;
      unsubscribeSuccess?.();
    };
  }, [previewKey]);

  const tryPlayVideo=()=>{if(isTextWithVideo && previewList[0])emitter.emit("OPEN_VIDEO_PLAYER",previewList[0]);};

  const downloadOrShowFinder = (current: number) => {
    if (!previewList[current]) return;
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

  const onPreviewIdxChange = (next: number) => {
    setPreviewState((state) => ({ ...state, current: next }));

  };

  const getImageUrl = (metas?: MomentMeta) => {
    if (!metas) {
      metas = moments.content.metas?.[0];
    }
    // Protected images go through CacheImage's viewer authorization, even when
    // an older sender URL happens to exist in the shared disk-cache index.
    let sourceUrl = metas && momentMediaSource(metas, "thumb");
    if (!requiresResourceGrant(sourceUrl || "")) sourceUrl = imageCache[sourceUrl ?? ""] || sourceUrl;
    if (!isTextWithVideo) {
      const original = metas && momentMediaSource(metas, "original");
      sourceUrl = original && requiresResourceGrant(original) ? original : imageCache[original ?? ""] || sourceUrl;
    }
    return sourceUrl as string;
  };

  return (
    <div>
      {
        <div className="grid w-fit grid-cols-3">
          <ProtectedPreviewGroup
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
                  isFileDownloaded={Boolean(previewList[current]) && !previewList[current].startsWith("http")}
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
                      finished={true}
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
          </ProtectedPreviewGroup>
        </div>
      }
    </div>
  );
};

export default MomentsMediaRow;
