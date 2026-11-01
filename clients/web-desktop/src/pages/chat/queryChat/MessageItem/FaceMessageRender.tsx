import { toLocalFileURL } from "@/im/local-files";
import { Spin } from "antd";
import { FC } from "react";

import CacheImage from "@/components/CacheImage";

import { IMessageItemProps } from ".";

const min = (a: number, b: number) => (a > b ? b : a);

export type CustomEmojiItem = {
  url: string;
  path?: string;
  width: number;
  height: number;
};

const FaceMessageRender: FC<IMessageItemProps> = ({ message }) => {
  const faceEl: CustomEmojiItem = JSON.parse(message.faceElem.data);

  const minHeight = min(200, faceEl.width) * (faceEl.height / faceEl.width) + 2;
  const adaptedHight = min(minHeight, 640);
  const adaptedWidth = min(faceEl.width, 200);

  const minStyle = { minHeight: `${adaptedHight}px`, minWidth: `${adaptedWidth}px` };

  const getSrc = (item: CustomEmojiItem) => {
    if (window.electronAPI?.fileExists(item.path ?? "")) {
      return toLocalFileURL(item.path);
    }
    return item.url;
  };

  return (
    <div className="relative" style={minStyle}>
      <CacheImage
        rootClassName="message-image cursor-pointer"
        className="max-w-[200px] rounded-md"
        preview={false}
        src={getSrc(faceEl)}
        placeholder={
          <div style={minStyle} className="flex items-center justify-center">
            <Spin />
          </div>
        }
      />
    </div>
  );
};

export default FaceMessageRender;
