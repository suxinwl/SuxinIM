import { deleBlock, getBlockList } from '@/services/server/user_manage';
import { DefaultAvatar } from '@/utils/avatar';
import type { ActionType, ProColumns } from '@ant-design/pro-components';
import { PageContainer, ProTable } from '@ant-design/pro-components';
import { message, Popconfirm } from 'antd';
import { useRef } from 'react';
import type { BlockListItem } from './data';

const columns: ProColumns<BlockListItem>[] = [
  {
    key: 'index',
    dataIndex: 'index',
    valueType: 'indexBorder',
    width: 48,
  },
  {
    title: '用户头像',
    dataIndex: 'faceURL',
    valueType: 'avatar',
    key: 'faceURL',
    hideInSearch: true,
    align: 'center',
    render(_, record) {
      return <DefaultAvatar faceURL={record.faceURL} nickname={record.nickname} />;
    },
  },
  {
    title: '昵称',
    key: 'nickname',
    dataIndex: 'nickname',
    hideInSearch: true,
    ellipsis: true,
    align: 'center',
  },
  {
    title: '用户ID',
    key: 'userID',
    dataIndex: 'userID',
    editable: false,
    ellipsis: true,
    align: 'center',
    formItemProps: {
      rules: [
        {
          required: true,
          message: '此项为必填项',
        },
      ],
    },
  },
  {
    title: '性别',
    key: 'gender',
    dataIndex: 'gender',
    hideInSearch: true,
    valueType: 'select',
    align: 'center',
    request: async () => {
      return [
        {
          label: '保密',
          value: 0,
        },
        {
          label: '男',
          value: 1,
        },
        {
          label: '女',
          value: 2,
        },
      ];
    },
  },
  // {
  //   title: '生日',
  //   key: 'birth',
  //   dataIndex: 'birth',
  //   valueType: 'date',
  //   hideInSearch: true,
  //   render(dom, entity, index, action, schema) {
  //     const text = entity.birth;
  //     if (!text) {
  //       return '-';
  //     }
  //     if (isMoment(text)) {
  //       return text.format('YYYY-MM-DD');
  //     }
  //     return moment(text * 1000).format('YYYY-MM-DD');
  //   },
  // },
  // {
  //   title: '用户角色',
  //   key: 'level',
  //   dataIndex: 'level',
  //   hideInSearch: true,
  //   valueType: 'select',
  //   align: 'center',
  //   request: async () => {
  //     return [
  //       {
  //         label: '普通用户',
  //         value: 1,
  //       },
  //       {
  //         label: '高级用户',
  //         value: 100,
  //       },
  //     ];
  //   },
  // },
  // {
  //   title: '封禁账号时间',
  //   key: 'disableTimeRange',
  //   dataIndex: 'disableTimeRange',
  //   valueType: 'dateTimeRange',
  //   hideInSearch: true,
  //   editable: false,
  // },
  {
    title: '操作',
    valueType: 'option',
    key: 'option',
    align: 'center',
    render: (text, record, _, action) => (
      <Popconfirm
        title="确定要解封该用户吗？"
        onConfirm={() => {
          deleBlock(record.userID).then(() => {
            action?.reload();
            message.success('操作成功！');
          });
          // .catch(() => message.error('操作失败！'));
        }}
        okText="确定"
        cancelText="取消"
      >
        <a href="#">解封</a>
      </Popconfirm>
    ),
  },
];

const BlockList = () => {
  const actionRef = useRef<ActionType>();

  return (
    <PageContainer>
      <ProTable<BlockListItem>
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
          const { data } = await getBlockList({
            ...params,
            keyword: params.userID,
            pagination: {
              pageNumber: params.current as number,
              showNumber: params.pageSize as number,
            },
          });

          // const tmpData = (data.userInfoList ?? []).map((user: any) => {
          //   // user.disableTimeRange = [user.beginDisableTime, user.endDisableTime];
          //   return { ...user.attribute, ...user.register };
          // });
          return {
            data: data.users,
            success: true,
            total: data.total ?? 0,
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
        rowKey="userID"
        search={{
          labelWidth: 'auto',
        }}
        pagination={{
          pageSize: 10,
          onChange: (page) => console.log(page),
        }}
        dateFormatter="string"
        // scroll={{ x: 'max-content' }}
        toolbar={{
          actions: [],
          settings: [],
        }}
      />
    </PageContainer>
  );
};

export default BlockList;
