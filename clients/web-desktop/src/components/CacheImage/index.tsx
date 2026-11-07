import { toLocalFileURL } from "@/im/local-files";
import { useProtectedResource } from "@/im/useProtectedResource";
import { Image, ImageProps } from "antd";
import { useMemo, useState } from "react";
import { t } from "i18next";

import { useUserStore } from "@/store";
import { ProtectedPreviewImage } from "@/components/ProtectedPreviewGroup";

const CacheImage = (props: ImageProps) => {
  const resource = useProtectedResource(props.src);
  const previewConfig = typeof props.preview === "object" ? props.preview : {};
  const explicitPreview = useProtectedResource(previewConfig.src);
  const [previewVisible, setPreviewVisible] = useState(false);
  const getSourceUrl = useMemo(() => {
    // Protected media always uses the viewer's authorized HTTPS request. A
    // shared disk-cache hit must not bypass current SID/access validation.
    if (resource.protectedResource) return resource.src;
    const source = resource.src;
    if (!window.electronAPI || source?.match(/^blob:/)) return source;

    if (!source?.match(/^https?:\/\//) && !source?.match(/^file:\/\//)) {
      return toLocalFileURL(source);
    }
    const cachePath = useUserStore.getState().imageCache[source];
    if (cachePath && window.electronAPI?.fileExists(cachePath)) {
      return toLocalFileURL(cachePath);
    }
    return source;
  }, [resource.src, resource.protectedResource]);
  if (resource.failed) return <span role="img" aria-label={t("toast.imageLoadFailed")} className={props.rootClassName}>
    {t("toast.imageLoadFailed")} <button type="button" onClick={resource.retry}>{t("retry", { defaultValue: "重试" })}</button>
  </span>;
  const preview = props.preview === false || (explicitPreview.protectedResource && !explicitPreview.src) ? false : {
    ...previewConfig,
    src: explicitPreview.protectedResource ? explicitPreview.src : previewConfig.src,
    onVisibleChange: (visible: boolean, previous: boolean) => { setPreviewVisible(visible); previewConfig.onVisibleChange?.(visible, previous); },
    imageRender: (node: React.ReactElement, info: Parameters<NonNullable<typeof previewConfig.imageRender>>[1]) => {
      const protectedNode = <ProtectedPreviewImage node={node} source={previewConfig.src ?? props.src} visible={previewConfig.visible ?? previewVisible} />;
      return previewConfig.imageRender ? previewConfig.imageRender(protectedNode, info) : protectedNode;
    },
  };
  return <Image {...props} src={getSourceUrl} preview={preview} onError={event => { if (!resource.onError()) props.onError?.(event); }} />;
};

export default CacheImage;
