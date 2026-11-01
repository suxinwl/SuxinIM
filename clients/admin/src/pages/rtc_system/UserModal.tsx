import { DefaultAvatar } from '@/utils/avatar';
import { copy2Text } from '@/utils/common';
import { List, Modal } from 'antd';
import { FC } from 'react';

type UserModalProps = {
  open: boolean;
  title: string;
  userList: User[];
  close: () => void;
};

const UserModal: FC<UserModalProps> = ({ open, title, userList, close }) => {
  return (
    <Modal
      title={title}
      open={open}
      onOk={close}
      onCancel={close}
      mask={false}
      footer={null}
      width={360}
    >
      <div className="max-h-[500px] overflow-auto">
        <List
          dataSource={userList}
          renderItem={(item) => (
            <List.Item>
              <List.Item.Meta
                avatar={<DefaultAvatar faceURL={item.faceURL || ''} nickname={item.nickname} />}
                title={item.nickname}
                description={
                  <span className="cursor-pointer" onClick={() => copy2Text(item.userID)}>
                    {item.userID}
                  </span>
                }
              />
            </List.Item>
          )}
        />
      </div>
    </Modal>
  );
};

export default UserModal;
