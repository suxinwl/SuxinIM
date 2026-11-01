import { MessageType } from "@openim/wasm-client-sdk";
import { FC } from "react";

import Twemoji from "@/components/Twemoji";
import { formatBr } from "@/utils/common";
import { formatAtText, formatLink } from "@/utils/imCommon";

import { IMessageItemProps } from ".";
import styles from "./message-item.module.scss";

const TextMessageRender: FC<IMessageItemProps> = ({ message }) => {
  let content = message.textElem?.content;

  if (message.contentType === MessageType.QuoteMessage) {
    content = message.quoteElem.text;
  }
  if (message.contentType === MessageType.AtTextMessage) {
    content = formatAtText(message.atTextElem);
  }

  content = formatLink(content);
  content = formatBr(content);

  return (
    <Twemoji>
      <div
        className={styles.bubble}
        dangerouslySetInnerHTML={{ __html: content }}
      ></div>
    </Twemoji>
  );
};

export default TextMessageRender;
