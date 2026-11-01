import { FC } from "react";

import { CustomMessageType } from "@/constants";

import { IMessageItemProps } from ".";
import CatchMessageRender from "./CatchMsgRenderer";
import MassMessageRenderer from "./MassMessageRenderer";
import MeetingMessageRender from "./MeetingMessageRender";
import RtcMessageRender from "./RtcMessageRender";

const CustomMessageSwitcher: FC<IMessageItemProps> = (props) => {
  const { message } = props;
  const customData = JSON.parse(message.customElem.data);
  if (customData.customType === CustomMessageType.Call) {
    return <RtcMessageRender {...props} />;
  }
  if (customData.customType === CustomMessageType.MeetingInvitation) {
    return <MeetingMessageRender {...props} />;
  }
  if (customData.customType === CustomMessageType.MassMsg) {
    return <MassMessageRenderer {...props} />;
  }
  return <CatchMessageRender />;
};

export default CustomMessageSwitcher;
