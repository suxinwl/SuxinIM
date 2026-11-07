import "xgplayer/dist/index.min.css";

import { memo, useEffect, useRef } from "react";
import { useProtectedResource } from "@/im/useProtectedResource";
import { t } from "i18next";
import { SimplePlayer } from "xgplayer";
import { I18N } from "xgplayer";
import ZH from "xgplayer/es/lang/zh-cn";
import Error from "xgplayer/es/plugins/error";
import Fullscreen from "xgplayer/es/plugins/fullscreen";
import Mobile from "xgplayer/es/plugins/mobile";
import PC from "xgplayer/es/plugins/pc";
import Play from "xgplayer/es/plugins/play";
import Progress from "xgplayer/es/plugins/progress";
import Start from "xgplayer/es/plugins/start";
import Time from "xgplayer/es/plugins/time";

I18N.use(ZH);

const VideoPlayer = ({
  url,
  autoplay,
  poster,
}: {
  url: string;
  autoplay?: boolean;
  poster?: string;
}) => {
  const resource = useProtectedResource(url, { forceOnLoad: true });
  const posterResource = useProtectedResource(poster);
  const element = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!resource.src || !element.current) return;
    const player = new SimplePlayer({
      el: element.current,
      url: resource.src,
      autoplay,
      poster: posterResource.src,
      plugins: [Start, PC, Mobile, Progress, Play, Time, Error, Fullscreen],
    });
    player.on("error", resource.onError);
    return () => { player.destroy(); };
  }, [resource.src, resource.onError, posterResource.src, autoplay]);

  if (resource.failed) return <span role="status">{t("toast.imageLoadFailed")} <button type="button" onClick={resource.retry}>{t("retry", { defaultValue: "重试" })}</button></span>;
  return <div key={resource.src ?? "pending"} ref={element} id="video_player" />;
};

export default memo(VideoPlayer);
