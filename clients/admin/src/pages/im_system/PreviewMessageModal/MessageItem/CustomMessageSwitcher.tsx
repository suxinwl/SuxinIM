import { FC } from "react";

export const CustomMessageType = {
  MassMsg: 903,
};

import { IMessageItemProps } from ".";
import CatchMessageRender from "./CatchMsgRenderer";
import MassMessageRenderer from "./MassMessageRenderer";

const CustomMessageSwitcher: FC<IMessageItemProps> = (props) => {
  const { message } = props;
  const customData = JSON.parse(message.customElem.data);

  if (customData.customType === CustomMessageType.MassMsg) {
    return <MassMessageRenderer {...props} />;
  }
  return <CatchMessageRender />;
};

export default CustomMessageSwitcher;
