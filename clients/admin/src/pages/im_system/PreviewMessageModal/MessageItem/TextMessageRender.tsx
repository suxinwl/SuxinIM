import { FC } from "react";

import Twemoji from "@/components/Twemoji";

import { IMessageItemProps } from ".";
import styles from "./message-item.module.less";
import { MessageType } from "@openim/wasm-client-sdk";

const TextMessageRender: FC<IMessageItemProps> = ({ message }) => {
  let content = message.textElem?.content;

  if (message.contentType === MessageType.QuoteMessage) {
    content = message.quoteElem.text;
  }

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
