import { Alert, Modal } from "antd";
import { memo } from "react";
const ConfigModal = ({visible, close}: {visible: boolean; close: () => void}) => {
  return <Modal title="通讯连接" open={visible} onCancel={close} footer={null}>
    <Alert message="网页版使用当前访问地址。内外网均使用 HTTPS。" type="info"/>
  </Modal>;
};
export default memo(ConfigModal);
