import {requiresResourceGrant,resolvePublicBrandResource} from "@/im/resources";
import { useProtectedResource } from "@/im/useProtectedResource";
import default_user from "@/assets/images/puty-avatar.png";
import { toLocalFileURL } from "@/im/local-files";
import { Avatar as AntdAvatar, AvatarProps } from "antd";
import clsx from "clsx";
import * as React from "react";
import { useMemo } from "react";

// import default_group from "@/assets/images/contact/group.png";
import default_group from "@/assets/images/contact/my_groups.png";
import { useMessageStore, useUserStore } from "@/store";
import { avatarList, getDefaultAvatar } from "@/utils/avatar";

const default_avatars = avatarList.map((item) => item.name);

interface IOIMAvatarProps extends AvatarProps {
  text?: string;
  color?: string;
  bgColor?: string;
  isgroup?: boolean;
  isnotification?: boolean;
  size?: number;
}

const OIMAvatar: React.FC<IOIMAvatarProps> = (props) => {
  const {
    src: originalSrc,
    text,
    size = 42,
    color = "#fff",
    bgColor = "#2074de",
    isgroup = false,
    isnotification,
  } = props;
  const src=typeof originalSrc==="string"?resolvePublicBrandResource(originalSrc):originalSrc;
  const resource = useProtectedResource(typeof src === "string" ? src : undefined);
  const [errorHolder, setErrorHolder] = React.useState<string>();

  const getAvatarUrl = useMemo(() => {
    if (src) {
      if(typeof src==="string" && requiresResourceGrant(src)) return resource.src || (isgroup?default_group:default_user);
      if (default_avatars.includes(src as string))
        return getDefaultAvatar(src as string);

      if (!window.electronAPI) return src;

      const cachePath = useUserStore.getState().imageCache[src as string];
      if (cachePath && window.electronAPI?.fileExists(cachePath)) {
        return toLocalFileURL(cachePath);
      }
      return src;
    }
    return isgroup ? default_group : default_user;
  }, [src, isgroup, isnotification,resource.src]);

  const avatarProps = { ...props, isgroup: undefined, isnotification: undefined };

  React.useEffect(() => {
    if (!isgroup) {
      setErrorHolder(undefined);
    }
  }, [isgroup, src, resource.src]);

  const errorHandler = () => {
    if (resource.onError()) return false;
    setErrorHolder(isgroup ? default_group : default_user);
  };

  return (
    <AntdAvatar
      style={{
        backgroundColor: bgColor,
        minWidth: `${size}px`,
        minHeight: `${size}px`,
        lineHeight: `${size - 2}px`,
        color,
      }}
      shape="square"
      {...avatarProps}
      className={clsx(
        {
          "cursor-pointer": Boolean(props.onClick),
        },
        props.className,
      )}
      src={errorHolder ?? getAvatarUrl}
      onError={errorHandler as any}
    >
      {text}
    </AntdAvatar>
  );
};

export default OIMAvatar;
