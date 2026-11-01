import { useMount } from 'ahooks';
import clsx from 'clsx';
import { FC, useRef, useState } from 'react';

import VoiceIcon from '@/svg/VoiceIcon';
import { message as toast } from 'antd';

import { IMessageItemProps } from '.';
import styles from './message-item.module.less';

const VoiceMessageRender: FC<IMessageItemProps> = ({ message, isSender }) => {
  const audioEl = useRef<HTMLAudioElement>(new Audio());
  const [isPlaying, setIsPlaying] = useState(false);
  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  let isPaused = false;

  useMount(() => {
    audioEl.current.src = message.soundElem.sourceUrl;
    audioEl.current.onended = () => {
      setIsPlaying(false);
      isPaused = false;
    };
    audioEl.current.onpause = () => {
      setIsPlaying(false);
      isPaused = true;
    };
    audioEl.current.onplay = () => {
      setIsPlaying(true);
    };
  });

  const playAudio = () => {
    if (isPlaying) {
      audioEl.current?.pause();
      setIsPlaying(false);
    } else {
      audioEl.current?.play().catch(() => toast.error('play audio failed'));
      isPaused = false;
    }
  };

  return (
    <div
      className={clsx(
        styles.bubble,
        'flex cursor-pointer items-center !py-2',
        !isSender && 'flex-row-reverse',
      )}
      onClick={playAudio}
    >
      <VoiceIcon style={{ transform: isSender ? 'rotateY(180deg)' : 'none' }} playing={isPlaying} />
      <span className={isSender ? 'mr-1' : 'ml-1'}>{`${message.soundElem.duration} ‘’`}</span>
    </div>
  );
};

export default VoiceMessageRender;
