import { toLocalFileURL } from "@/im/local-files";
import { Image, ImageProps } from "antd";
import { useMemo } from "react";

import { useUserStore } from "@/store";
import { downloadFile } from "@/utils/common";

const CacheImage = (props: ImageProps) => {
  const getSourceUrl = useMemo(() => {
    if (!window.electronAPI || props.src?.match(/^blob:/)) return props.src;

    if (!props.src?.match(/^https?:\/\//) && !props.src?.match(/^file:\/\//)) {
      return toLocalFileURL(props.src);
    }
    const cachePath = useUserStore.getState().imageCache[props.src];
    if (cachePath && window.electronAPI?.fileExists(cachePath)) {
      return toLocalFileURL(cachePath);
    }
    if (props.src) {
      downloadFile(props.src, {
        isThumb: true,
        saveType: "image",
        randomName: true,
      });
    }
    return props.src;
  }, [props.src]);
  return <Image {...props} src={getSourceUrl} />;
};

export default CacheImage;
