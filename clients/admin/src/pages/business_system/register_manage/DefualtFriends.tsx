import {
  addDefaultFriends,
  getDefaultFriends,
  removeDefaultFriends,
} from '@/services/server/register_manage';
import type { ActionType, ProColumns } from '@ant-design/pro-components';
import { PageContainer, ProTable } from '@ant-design/pro-components';
import { Button, message, Popconfirm } from 'antd';
import { useRef, useState } from 'react';
import type { DefualtFriendItem } from './data';
import SelectUserModal, { SelectedListItem, SelectModalOptions } from './SelectUserModal';

const columns: ProColumns<DefualtFriendItem>[] = [
  {
    key: 'index',
    title: '序号',
    dataIndex: 'index',
    valueType: 'indexBorder',
    width: 48,
    align: 'center',
  },
  {
    title: '账号ID',
    dataIndex: 'userID',
    key: 'userID',
    align: 'center',
  },
  {
    title: '账号名称',
    key: 'nickname',
    hideInSearch: true,
    render: (text, record) => <span>{record.user.nickname}</span>,
    align: 'center',
  },
  {
    title: '操作',
    valueType: 'option',
    key: 'option',
    align: 'center',
    render: (text, record, _, action) => (
      <Popconfirm
        key={'delect'}
        title="确定要移除该用户吗？"
        onConfirm={() => {
          removeDefaultFriends([record.userID]).then(() => {
            action?.reload();
          });
        }}
        okText="确定"
        cancelText="取消"
      >
        <a href="#">移除</a>
      </Popconfirm>
    ),
  },
];

const DefualtFriends = () => {
  const actionRef = useRef<ActionType>();
  const [selectModalOptions, setSelectModalOptions] = useState<SelectModalOptions>({
    visible: false,
    selectType: 'member',
  });

  const closeSelectModal = () => {
    setSelectModalOptions({
      visible: false,
      selectType: 'member',
    });
  };

  const selectedCallBack = async (data: SelectedListItem) => {
    const tmpArr = data.data.map((user) => user.userID);
    try {
      await addDefaultFriends(tmpArr);
      actionRef.current?.reload();
      message.success('新增成功！');
    } catch (error) {
      message.error('新增失败！');
    }
  };

  return (
    <PageContainer>
      <ProTable<DefualtFriendItem>
        columns={columns}
        actionRef={actionRef}
        cardBordered
        columnsState={{
          defaultValue: {
            option: {
              fixed: 'right',
            },
          },
        }}
        request={async (params = {}, sort, filter) => {
          console.log(params, sort, filter);
          const { data } = await getDefaultFriends(
            params.current ?? 1,
            params.pageSize ?? 10,
            params.userID,
          );
          // const tmpData = data.chat_logs ?? [];
          return {
            data: data.users,
            success: true,
            total: data.total,
          };
        }}
        rowKey="userID"
        search={{
          labelWidth: 'auto',
        }}
        pagination={{
          pageSize: 10,
          onChange: (page) => console.log(page),
        }}
        dateFormatter="string"
        scroll={{ x: 'max-content' }}
        toolbar={{
          actions: [
            <Button
              key={'add'}
              type="primary"
              // disabled
              onClick={() => {
                setSelectModalOptions({
                  visible: true,
                  selectType: 'member',
                });
              }}
            >
              新增
            </Button>,
          ],
          settings: [],
        }}
      />
      <SelectUserModal
        selectModalOptions={selectModalOptions}
        selectedCallBack={selectedCallBack}
        closeSelectModal={closeSelectModal}
      />
    </PageContainer>
  );
};

export default DefualtFriends;
