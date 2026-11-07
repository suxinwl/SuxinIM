import { v4 as uuidV4 } from "uuid";

import { IMSDK } from "@/layout/MainContentWrap";
import { ExMessageItem } from "@/store";
import { readLocalImage, readLocalVideo } from "@/im/local-media";
import {withAttachmentSourceName} from '@/im/attachment-name';

export interface FileWithPath extends File {
  path?: string;
}

export function useFileMessage() {
  const getImageMessage = async (file: FileWithPath) => {
    const { width, height } = await getPicInfo(file);
    const baseInfo = {
      uuid: uuidV4(),
      type: file.type,
      size: file.size,
      width,
      height,
      url: URL.createObjectURL(file),
    };
    const sourcePath =
      (await window.electronAPI?.saveFileToDisk({
        file,
        sync: true,
        type: "sentFileCache",
      })) || `/${file.name}`;
    const options = {
      sourcePicture: baseInfo,
      bigPicture: baseInfo,
      snapshotPicture: baseInfo,
      sourcePath,
      file,
    };

    return withAttachmentSourceName((await IMSDK.createImageMessageByFile(options)).data,file.name);
  };

  const getVideoMessage = async (file: FileWithPath, snapShotFile: FileWithPath, duration?: number) => {
    const { width, height } = await getPicInfo(snapShotFile);
    const snapshotPath =
      (await window.electronAPI?.saveFileToDisk({
        sync: true,
        file: snapShotFile,
        type: "sentFileCache",
      })) || `/${snapShotFile.name}`;
    const videoPath =
      (await window.electronAPI?.saveFileToDisk({
        file,
        type: "sentFileCache",
      })) || `/${file.name}`;
    const options = {
      videoFile: file,
      snapshotFile: snapShotFile,
      videoPath,
      duration: duration ?? (await readLocalVideo(file)).duration,
      videoType: file.type,
      snapshotPath,
      videoUUID: uuidV4(),
      videoUrl: "",
      videoSize: file.size,
      snapshotUUID: uuidV4(),
      snapshotSize: snapShotFile.size,
      snapshotUrl: URL.createObjectURL(snapShotFile),
      snapshotWidth: width,
      snapshotHeight: height,
      snapShotType: snapShotFile.type,
    };
    return withAttachmentSourceName((await IMSDK.createVideoMessageByFile(options)).data,file.name);
  };

  const getFileMessage = async (file: FileWithPath) => {
    const filePath =
      (await window.electronAPI?.saveFileToDisk({
        file,
        sync: true,
        type: "sentFileCache",
      })) || `/${file.name}`;
    const options = {
      file: (await window.electronAPI?.getFileByPath(filePath)) || file,
      filePath,
      fileName: file.name,
      uuid: uuidV4(),
      sourceUrl: "",
      fileSize: file.size,
      fileType: file.type,
    };
    return (await IMSDK.createFileMessageByFile(options)).data;
  };

  const createFileMessage = async (file: FileWithPath): Promise<ExMessageItem> => {
    const isImage = file.type.includes("image");
    const isVideo = file.type.startsWith("video/") || file.type.includes("mp4") || (!file.type && /\.mp4$/i.test(file.name));
    if (isImage) {
      return await getImageMessage(file);
    }
    if (isVideo) {
      const { snapshot, duration } = await readLocalVideo(file);
      return await getVideoMessage(file, snapshot, duration);
    }
    return await getFileMessage(file);
  };

  const getPicInfo = readLocalImage;
  const getVideoSnshotFile = async (file: File) => (await readLocalVideo(file)).snapshot;

  return {
    getImageMessage,
    getVideoMessage,
    getFileMessage,
    createFileMessage,
    getPicInfo,
    getVideoSnshotFile,
  };
}
