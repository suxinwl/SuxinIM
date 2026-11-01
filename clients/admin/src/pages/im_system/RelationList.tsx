import type { FriendListItem } from '@/pages/business_system/user_manage/data';
import { deleteFriend, getFriendList } from '@/services/server/user_manage';
import { DefaultAvatar } from '@/utils/avatar';
import type { ActionType, ProColumns } from '@ant-design/pro-components';
import { PageContainer, ProTable } from '@ant-design/pro-components';
import { message, Popconfirm } from 'antd';
import moment from 'moment';
import { useRef } from 'react';

const columns: ProColumns<FriendListItem>[] = [
  {
    key: 'index',
    dataIndex: 'index',
    valueType: 'indexBorder',
    width: 48,
  },
  {
    title: '用户头像',
    dataIndex: 'avatar',
    valueType: 'avatar',
    key: 'avatar',
    hideInSearch: true,
    align: 'center',
    render(_, record) {
      return (
        <DefaultAvatar faceURL={record.friendUser.faceURL} nickname={record.friendUser.nickname} />
      );
    },
  },
  {
    title: '昵称',
    key: 'nickName',
    dataIndex: 'nickName',
    hideInSearch: true,
    align: 'center',
    render: (_, record) => <span>{record.friendUser.nickname}</span>,
  },
  {
    title: '用户ID',
    key: 'userID',
    dataIndex: 'userID',
    editable: false,
    align: 'center',
    render: (_, record) => <span>{record.friendUser.userID}</span>,
  },
  {
    title: '添加好友时间',
    key: 'createTime',
    dataIndex: 'createTime',
    valueType: 'date',
    hideInSearch: true,
    editable: false,
    align: 'center',
    renderText(text, record) {
      return moment(record.createTime * 1000).format('YYYY-MM-DD');
    },
  },
  {
    title: '操作',
    valueType: 'option',
    key: 'option',
    align: 'center',
    render: (text, record, _, action) => (
      <Popconfirm
        key={'option'}
        title="确定要删除该好友吗？"
        onConfirm={async () => {
          try {
            await deleteFriend({
              ownerUserID: record.ownerUserID,
              friendUserID: record.friendUser.userID,
            });
            message.success('操作成功！');
            action?.reload();
          } catch (error) {
            message.error('操作失败！');
          }
        }}
        okText="确定"
        cancelText="取消"
      >
        <a href="#">删除</a>
      </Popconfirm>
    ),
  },
];

const RelationList = () => {
  const actionRef = useRef<ActionType>();

  return (
    <PageContainer>
      <ProTable<FriendListItem>
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
          const userID = location.search.slice(8);
          const { data } = await getFriendList({
            userID,
            pagination: {
              pageNumber: params.current as number,
              showNumber: params.pageSize as number,
            },
          });
          // await attachInfo(data.friendInfoList);
          return {
            data: data.friendsInfo ?? [],
            success: true,
            total: data.total,
          };
        }}
        editable={{
          type: 'multiple',
          actionRender: (row, config, doms) => {
            return [doms.save, doms.cancel];
          },
          // onSave: (key, record) => {
          //   return updateMerchant(record);
          // },
        }}
        rowKey={(record: any) => record.friendUser.userID}
        search={false}
        // search={{
        //   labelWidth: 'auto',
        // }}
        pagination={{
          pageSize: 10,
          onChange: (page) => console.log(page),
        }}
        dateFormatter="string"
        scroll={{ x: 'max-content' }}
        toolbar={{
          actions: [],
          settings: [],
        }}
      />
    </PageContainer>
  );
};

export default RelationList;
