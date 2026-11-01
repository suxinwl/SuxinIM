import 'xgplayer/dist/index.min.css';

import { Modal } from 'antd';
import { memo, useEffect } from 'react';
import { I18N, SimplePlayer } from 'xgplayer';
import ZH from 'xgplayer/es/lang/zh-cn';
import Error from 'xgplayer/es/plugins/error';
import Mobile from 'xgplayer/es/plugins/mobile';
import PC from 'xgplayer/es/plugins/pc';
import Play from 'xgplayer/es/plugins/play';
import Progress from 'xgplayer/es/plugins/progress';
// 引入es目录下的插件
import { useModel } from '@umijs/max';
import Start from 'xgplayer/es/plugins/start';
import Time from 'xgplayer/es/plugins/time';

I18N.use(ZH);

const VideoPlayerModal = () => {
  const { initialState, setInitialState } = useModel('@@initialState');

  useEffect(() => {
    if (!!initialState?.videoUrl) {
      new SimplePlayer({
        id: 'video_player',
        url: initialState?.videoUrl,
        plugins: [Start, PC, Mobile, Progress, Play, Time, Error], // 传入需要组装的插件
      });
    }
  }, [initialState?.videoUrl]);

  const onCancel = () => {
    setInitialState((s: any) => ({
      ...s,
      videoUrl: '',
    }));
  };

  return (
    <Modal
      title={null}
      footer={null}
      // closeIcon={<CloseOutlined className="text-lg font-medium text-gray-400" />}
      open={!!initialState?.videoUrl}
      centered
      onCancel={onCancel}
      maskStyle={{
        opacity: 0,
        transition: 'none',
      }}
      className="no-padding-modal"
      maskTransitionName=""
    >
      <div id="video_player"></div>
    </Modal>
  );
};

export default memo(VideoPlayerModal);
