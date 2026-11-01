import call_video from '@/assets/images/call_video.png';
import { useModel } from '@umijs/max';
import { Image, Spin } from 'antd';
import { MessageType } from '@openim/wasm-client-sdk';
import { FC } from 'react';
import { IMessageItemProps } from '.';

const min = (a: number, b: number) => (a > b ? b : a);

const MediaMessageRender: FC<IMessageItemProps> = ({ message, showAlbum }) => {
  const { setInitialState } = useModel('@@initialState');

  const isVideoMessage = message.contentType === MessageType.VideoMessage;
  const sourceUrl = isVideoMessage
    ? message.videoElem.snapshotUrl
    : message.pictureElem.snapshotPicture.url;
  const imageHeight = isVideoMessage
    ? message.videoElem.snapshotHeight
    : message.pictureElem.sourcePicture.height;
  const imageWidth = isVideoMessage
    ? message.videoElem.snapshotWidth
    : message.pictureElem.sourcePicture.width;
  const snapshotMaxHeight = isVideoMessage ? 320 : message.pictureElem.snapshotPicture.height;
  const minHeight = min(200, imageWidth) * (imageHeight / imageWidth) + 2;
  const adaptedHight = min(minHeight, snapshotMaxHeight);
  const adaptedWidth = min(imageWidth, 200);

  const previewInAlbum = () => {
    showAlbum?.(message.clientMsgID);
  };

  const showVideo = (url: string) => {
    console.error(url)
    if (!isVideoMessage) return;
    setInitialState((s) => ({
      ...s,
      videoUrl: url,
    }));
  };

  const getShowPreview = () => {
    if (isVideoMessage) {
      return false;
    }
    return showAlbum ? { visible: false } : true;
  };

  const minStyle = { minHeight: `${adaptedHight}px`, minWidth: `${adaptedWidth}px` };

  if (!isVideoMessage) {
    return (
      <div className="relative" style={minStyle}>
        <Image
          rootClassName="message-image cursor-pointer"
          className="max-w-[200px] rounded-md"
          src={sourceUrl}
          preview={getShowPreview()}
          onClick={previewInAlbum}
          placeholder={
            <div style={minStyle} className="flex items-center justify-center">
              <Spin />
            </div>
          }
        />
      </div>
    );
  }

  return (
    <div className="relative" style={minStyle} onClick={() => showVideo(message.videoElem.videoUrl)}>
      <Image
        rootClassName="message-image cursor-pointer"
        className="max-w-[200px] rounded-md"
        src={sourceUrl}
        preview={getShowPreview()}
        onClick={previewInAlbum}
        placeholder={
          <div style={minStyle} className="flex items-center justify-center">
            <Spin />
          </div>
        }
      />
      <div className="video-image-mask">
        <img src={call_video} alt="" width={40} />
      </div>
    </div>
  );
};

export default MediaMessageRender;
