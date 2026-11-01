import { toLocalFileURL, isLocalFileURL, localFilePath } from "@/im/local-files";
import { useLatest } from "ahooks";
import { Image, Spin } from "antd";
import {
  forwardRef,
  ForwardRefRenderFunction,
  memo,
  ReactNode,
  useEffect,
  useImperativeHandle,
  useState,
} from "react";

import PreviewToolsBar from "@/components/PreviewToolsBar";
import VideoPlayer from "@/components/VideoPlayer";
import { useConversationStore, useMessageStore, useUserStore } from "@/store";
import { PreviewGroupItem } from "@/store/type";
import FileDownloadIcon from "@/svg/FileDownloadIcon";
import { downloadFile, getDownloadTask } from "@/utils/common";

const MediaPreview: ForwardRefRenderFunction<
  { showAlbum: (clientMsgID: string) => void },
  { conversationID: string }
> = ({ conversationID }, ref) => {
  const [albumVisible, setAlbumVisible] = useState(false);
  const latestVisible = useLatest(albumVisible);
  const [autoPlay, setAutoPlay] = useState(false);
  const [albumCurrent, setAlbumCurrent] = useState(0);
  const previewImgList = useMessageStore((state) => state.previewImgList);
  const updateDownloadTask = useMessageStore((state) => state.updateDownloadTask);
  const imageCache = useUserStore((state) => state.imageCache);
  const getConversationPreviewImgList = useMessageStore(
    (state) => state.getConversationPreviewImgList,
  );

  useEffect(() => {
    if (previewImgList.length && latestVisible.current) {
      setAlbumCurrent((current) => current + 1);
    }
  }, [previewImgList.length]);

  const downloadOrShowFinder = (current: number) => {
    const { url, videoUrl, clientMsgID } = previewImgList[current];
    const currentTask = getDownloadTask({
      downloadMap: useMessageStore.getState().downloadMap,
      compareKey: "clientMsgID",
      compareValue: clientMsgID,
    });
    if (currentTask?.downloadState === "pause") {
      window.electronAPI?.ipcInvoke("resumeDownload", currentTask?.downloadUrl);
      updateDownloadTask(currentTask?.downloadUrl ?? "", {
        downloadState: "downloading",
      });
      return;
    }
    const sourceUrl = videoUrl ?? url;
    if (!sourceUrl.startsWith("http")) {
      window.electronAPI?.showInFinder(localFilePath(sourceUrl));
      return;
    }
    downloadFile(sourceUrl, {
      clientMsgID,
      conversationID,
      isMediaMessage: true,
      showError: true,
      saveType: videoUrl ? "video" : "image",
    });
  };

  const showAlbum = async (clientMsgID: string) => {
    let current = previewImgList.findIndex((img) => img.clientMsgID === clientMsgID);
    let findItem = previewImgList[current];
    if (current < 0) {
      await getConversationPreviewImgList();
      current = useMessageStore
        .getState()
        .previewImgList.findIndex((img) => img.clientMsgID === clientMsgID);
      findItem = useMessageStore.getState().previewImgList[current];
    }
    if (current < 0) return;
    const sourceUrl = findItem.videoUrl ?? findItem.url;
    if (window.electronAPI && sourceUrl.startsWith("http")) {
      downloadFile(sourceUrl, {
        clientMsgID,
        conversationID,
        isMediaMessage: true,
        showError: true,
        saveType: findItem.videoUrl ? "video" : "image",
      });
    } else {
      setAutoPlay(true);
      setTimeout(() => setAutoPlay(false), 200);
    }
    setAlbumCurrent(current);
    setAlbumVisible(true);
  };

  const onToggle = (next: number) => {
    const sourceUrl = previewImgList[next].videoUrl ?? previewImgList[next].url;
    if (window.electronAPI && sourceUrl.startsWith("http")) {
      downloadFile(sourceUrl, {
        clientMsgID: previewImgList[next].clientMsgID,
        conversationID,
        isMediaMessage: true,
        showError: true,
        saveType: previewImgList[next].videoUrl ? "video" : "image",
      });
    }
    setAlbumCurrent(next);
  };

  useImperativeHandle(ref, () => ({ showAlbum }), [previewImgList]);

  const previewList = previewImgList.map((image) => {
    if (!window.electronAPI || isLocalFileURL(image.url) || !image.thumbUrl) {
      return image.url;
    }
    return imageCache[image.thumbUrl ?? ""]
      ? toLocalFileURL(imageCache[image.thumbUrl ?? ""])
      : image.thumbUrl;
  });

  return (
    <div style={{ display: "none" }}>
      <Image.PreviewGroup
        preview={{
          current: albumCurrent,
          visible: albumVisible,
          toolbarRender: (originalNode, { current }) => {
            const previewItem = previewImgList[current];
            const isFileDownloadedField = previewItem.videoUrl ? "videoUrl" : "url";
            const isFileDownloaded = Boolean(
              isLocalFileURL(previewItem[isFileDownloadedField]),
            );
            return (
              <PreviewToolsBar
                clientMsgID={previewItem.clientMsgID}
                isFileDownloaded={isFileDownloaded}
                dom={originalNode as JSX.Element}
                hiddenImageAction={previewItem.videoUrl !== undefined}
                onDownloadOrShowFinder={() => downloadOrShowFinder(current)}
              />
            );
          },
          imageRender: (originNode, { current }) => (
            <PreviewItemRender
              dom={originNode}
              source={previewImgList[current]}
              autoplay={autoPlay}
            />
          ),
          onVisibleChange: (vis) => setAlbumVisible(vis),
          onChange: onToggle,
        }}
        items={previewList}
      />
    </div>
  );
};

const ForwardMediaPreview = memo(forwardRef(MediaPreview));

export default ForwardMediaPreview;

const PreviewItemRender = ({
  dom,
  source,
  autoplay,
}: {
  dom: ReactNode;
  source: PreviewGroupItem;
  autoplay: boolean;
}) => {
  const updateDownloadTask = useMessageStore((state) => state.updateDownloadTask);
  const currentTask = useMessageStore((state) =>
    getDownloadTask({
      downloadMap: state.downloadMap,
      compareKey: "clientMsgID",
      compareValue: source.clientMsgID,
    }),
  );

  useEffect(() => {
    const el = document.getElementsByClassName(
      "ant-image-preview-img",
    )[0] as HTMLImageElement | null;
    if (!el) return;
    el.addEventListener("load", () => {
      el.style.display = "block";
    });
    if (el.complete) {
      el.style.display = "block";
    }
    return () => {
      if (!el) return;

      el.style.display = "none";
    };
  }, [source.clientMsgID]);

  const tryPlayVideo = () => {
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

    if (!currentTask && source.videoUrl) {
      downloadFile(source.videoUrl, {
        isMediaMessage: true,
        showError: true,
        saveType: "video",
        clientMsgID: source.clientMsgID,
        conversationID:
          useConversationStore.getState().currentConversation?.conversationID,
      });
    }
  };

  if (!source.videoUrl) {
    return (
      <>
        {dom}
        <div className="absolute -z-10 flex items-center justify-center">
          <Spin spinning />
        </div>
      </>
    );
  }

  const isVideoDownloaded = isLocalFileURL(source.videoUrl);

  if (isLocalFileURL(source.videoUrl) || !window.electronAPI) {
    return (
      <div className="relative">
        <VideoPlayer url={source.videoUrl} autoplay={autoplay} />
      </div>
    );
  }

  return (
    <>
      {dom}
      <div
        className="absolute left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 cursor-pointer"
        onClick={tryPlayVideo}
      >
        <FileDownloadIcon
          size={40}
          pausing={currentTask?.downloadState === "pause"}
          finished={isVideoDownloaded || !window.electronAPI}
          percent={currentTask?.progress ?? 0}
        />
      </div>
      <div className="absolute -z-10 flex items-center justify-center">
        <Spin spinning />
      </div>
    </>
  );
};
