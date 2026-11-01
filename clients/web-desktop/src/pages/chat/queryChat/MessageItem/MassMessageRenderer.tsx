import { FC } from "react";

import { IMessageItemProps } from ".";
import styles from "./message-item.module.scss";

const MassMessageRenderer: FC<IMessageItemProps> = ({ message }) => {
  const content = JSON.parse(message.customElem?.data).data;
  return <div className={styles.bubble}>{content.textElem.content}</div>;
};

export default MassMessageRenderer;
