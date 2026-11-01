import type { SelectUserTableHandle } from '@/components/SelectUserTable';
import SelectUserTable from '@/components/SelectUserTable';
import type { UserListItem } from '@/pages/business_system/user_manage/data';
import { DefaultAvatar } from '@/utils/avatar';
import { Button, Checkbox, Col, Modal, Row, Space } from 'antd';
import type { FC } from 'react';
import { memo, useEffect, useRef, useState } from 'react';
import type { SelectedRowDataItem } from '../message_manage/SendMessage';

type SelectUserModalProps = {
  groupID?: string;
  selectModalOptions: SelectModalOptions;
  closeSelectModal: () => void;
  selectedCallBack?: (data: SelectedListItem, type: 'owner' | 'admin' | 'member') => void;
};

export type SelectedListItem = {
  data: UserListItem[];
  selectedRowData?: SelectedRowDataItem;
};

export type SelectModalOptions = {
  visible: boolean;
  selectType: 'owner' | 'admin' | 'member';
  preSelectData?: SelectedListItem;
};

const SelectUserModal: FC<SelectUserModalProps> = ({
  groupID,
  selectModalOptions,
  closeSelectModal,
  selectedCallBack,
}) => {
  const [selectedList, setSelectedList] = useState<SelectedListItem>({
    data: [],
    selectedRowData: {},
  });
  const selectTableRef = useRef<SelectUserTableHandle>(null);

  useEffect(() => {
    if (selectModalOptions.visible && selectModalOptions.preSelectData) {
      setSelectedList({
        data: selectModalOptions.preSelectData.data,
        selectedRowData: selectModalOptions.preSelectData.selectedRowData,
      });
    }
  }, [selectModalOptions.visible]);

  const onSelectedChange = (data: UserListItem[], selectedRowData: SelectedRowDataItem) => {
    setSelectedList({
      data,
      selectedRowData,
    });
    console.log(selectedRowData);
  };
  const cancelAll = () => {};

  const closeAndRestState = () => {
    setSelectedList({
      data: [],
      selectedRowData: {},
    });
    selectTableRef.current?.clearSelect();
    closeSelectModal();
  };

  const comfirmSelect = () => {
    selectedCallBack?.(selectedList, selectModalOptions.selectType);
    closeAndRestState();
  };

  return (
    <Modal
      closable={false}
      bodyStyle={{ padding: 0 }}
      width="80%"
      footer={null}
      destroyOnClose
      open={selectModalOptions.visible}
      onCancel={closeAndRestState}
    >
      <Row>
        <SelectUserTable
          groupID={groupID}
          ref={selectTableRef}
          defaultSelected={selectModalOptions.preSelectData?.selectedRowData}
          selectType={selectModalOptions.selectType === 'owner' ? 'radio' : 'checkbox'}
          onSelectedChange={onSelectedChange}
          colProps={{ span: 12 }}
        />
        <Col span={12}>
          <div className="flex flex-col h-full">
            <div className="font-medium mt-6 mb-4 ml-8">
              已选择群成员：{selectedList.data.length}人
            </div>
            <div className="flex justify-between mx-8">
              <div>
                <Checkbox
                  onChange={(e) => {
                    if (e.target.checked) {
                      cancelAll();
                    }
                  }}
                  checked={selectedList.data.length !== 0}
                />
                <span className="ml-2">头像</span>
              </div>
              <div>用户昵称</div>
              <div>用户ID</div>
            </div>
            <div className="flex-1 overflow-y-auto custom_scrollbar">
              {selectedList.data.map((user) => (
                <div key={user.userID} className="flex justify-between mx-8 my-3">
                  <div>
                    <Checkbox
                      checked
                      className="mr-2"
                      onChange={(e) => {
                        if (!e.target.checked) {
                          selectTableRef.current?.cancelSelect(user.userID);
                        }
                      }}
                    />
                    {/* <Avatar className="!ml-2" shape="square" src={user.avatar} size={30} /> */}
                    <DefaultAvatar faceURL={user.faceURL} nickname={user.nickname} />;
                  </div>
                  <div className="max-w-[100px] truncate ml-[42px]">{user.nickname}</div>
                  <div className="max-w-[100px] truncate">{user.userID}</div>
                </div>
              ))}
            </div>
            <div className="w-full h-14 text-right pr-8">
              <Space size="middle">
                <Button onClick={closeSelectModal}>取消</Button>
                <Button onClick={comfirmSelect} type="primary">
                  确定
                </Button>
              </Space>
            </div>
          </div>
        </Col>
      </Row>
    </Modal>
  );
};

export default memo(SelectUserModal);
