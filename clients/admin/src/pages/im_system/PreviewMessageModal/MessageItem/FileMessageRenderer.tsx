import { FC } from 'react';

import file_icon from '@/assets/images/file_icon.png';
import { bytesToSize } from '@/utils/common';

import { IMessageItemProps } from '.';

const FileMessageRenderer: FC<IMessageItemProps> = ({ message }) => {
  const { fileElem } = message;

  return (
    <div className="flex w-60 items-center justify-between rounded-md border border-[var(--gap-text)] p-3">
      <div className="mr-2 flex h-full flex-1 flex-col justify-between overflow-hidden">
        <div className="line-clamp-2 break-all">{fileElem.fileName}</div>
        <div className="text-xs text-[var(--sub-text)]">{bytesToSize(fileElem.fileSize)}</div>
      </div>
      <div className="relative min-w-[38px] cursor-pointer">
        <img width={38} src={file_icon} alt="file" />
      </div>
    </div>
  );
};

export default FileMessageRenderer;
