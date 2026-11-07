import { Image } from "antd";
import { cloneElement, ComponentProps, ReactElement, useState } from "react";
import { t } from "i18next";
import { useProtectedResource } from "@/im/useProtectedResource";

export function ProtectedPreviewImage({ node, source, visible }: { node: ReactElement; source?: string; visible: boolean }) {
  const resource = useProtectedResource(source, { forceOnLoad: true, enabled: visible });
  if (resource.failed) return <span role="img" aria-label={t("toast.imageLoadFailed")}>
    {t("toast.imageLoadFailed")} <button type="button" onClick={resource.retry}>{t("retry", { defaultValue: "重试" })}</button>
  </span>;
  return cloneElement(node as ReactElement<{ src?: string; onError?: (event: unknown) => void }>, {
    src: resource.src,
    onError: event => { if (!resource.onError()) node.props.onError?.(event); },
  });
}

// Keep Antd's transforms, controls and group protocol, but never let its
// independent items array render an expired sender/viewer grant directly.
export default function ProtectedPreviewGroup(props: ComponentProps<typeof Image.PreviewGroup>) {
  const [internalVisible, setInternalVisible] = useState(false);
  if (props.preview === false) return <Image.PreviewGroup {...props} />;
  const preview = typeof props.preview === "object" ? props.preview : {};
  const visible = preview.visible ?? internalVisible;
  return <Image.PreviewGroup {...props} preview={{
    ...preview,
    onVisibleChange: (value, previous, current) => { setInternalVisible(value); preview.onVisibleChange?.(value, previous, current); },
    imageRender: (node, info) => {
      const item = props.items?.[info.current];
      const source = typeof item === "string" ? item : item?.src ?? info.image.url;
      const protectedNode = <ProtectedPreviewImage key={`${visible}:${info.current}:${source}`} node={node} source={source} visible={visible} />;
      return preview.imageRender ? preview.imageRender(protectedNode, info) : protectedNode;
    },
  }} />;
}
