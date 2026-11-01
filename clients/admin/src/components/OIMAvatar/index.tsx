import { Avatar as AntdAvatar, AvatarProps } from "antd";
import clsx from "clsx";
import { FC, useEffect, useMemo, useState } from "react";

// import default_group from "@/assets/images/contact/group.png";
import default_group from "@/assets/images/group.png";

interface IOIMAvatarProps extends AvatarProps {
  text?: string;
  color?: string;
  bgColor?: string;
  isgroup?: boolean;
  isnotification?: boolean;
  size?: number;
}

const OIMAvatar: FC<IOIMAvatarProps> = (props) => {
  const {
    src,
    text,
    size = 42,
    color = "#fff",
    bgColor = "#2074de",
    isgroup = false,
    isnotification,
  } = props;
  const [errorHolder, setErrorHolder] = useState<string>();
  const getAvatarUrl = useMemo(() => {
    if (src) {
      return src;
    }
    return isgroup ? default_group : undefined;
  }, [src, isgroup, isnotification]);

  const avatarProps = { ...props, isgroup: undefined, isnotification: undefined };

  useEffect(() => {
    if (!isgroup) {
      setErrorHolder(undefined);
    }
  }, [isgroup]);

  const errorHandler = () => {
    if (isgroup) {
      setErrorHolder(default_group);
    }
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
