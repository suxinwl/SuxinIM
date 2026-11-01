import ContentParse from '@/components/ContentParse';
import { SessionType } from '@/constants/enum';
import { MessageTypeOtions } from '@/constants/messageTypeOtions';
import { getChatLogs, revokeMessage } from '@/services/server/message_manage';
import { getConversationID } from '@/utils/common';
import type { ActionType, ProColumns } from '@ant-design/pro-components';
import { PageContainer, ProTable } from '@ant-design/pro-components';
import { message, Popconfirm } from 'antd';
import moment from 'moment';
import { useRef } from 'react';
import type { UserMessageItem } from './data';

const SessionTypeOtions = [
  {
    label: '单聊',
    value: SessionType.Single,
  },
  // {
  //   label: '普通群',
  //   value: SessionType.Group,
  // },
  // {
  //   label: '工作群',
  //   value: SessionType.SuperGroup,
  // },
  {
    label: '系统通知',
    value: SessionType.Notification,
  },
];

const UserMessage = () => {
  const actionRef = useRef<ActionType>();

  const nomalMessageTypes = [101, 102, 103, 104, 105, 106, 107, 108, 109, 110, 114, 115, 117];

  const revokeMessageAction = async (record: UserMessageItem) => {
    if (!nomalMessageTypes.includes(record.contentType)) {
      message.warning('该类型消息不可撤回！');
      return;
    }
    const options = {
      userID: record.sendID,
      conversationID: getConversationID({
        sendID: record.sendID,
        recvID: record.recvID,
        isNotification: record.sessionType === SessionType.Notification,
      }),
      seq: record.seq,
    };
    revokeMessage(options).then(() => {
      message.success('撤回成功！');
      actionRef.current?.reload();
    });
    // .catch(() => message.error('撤回失败！'));
  };

  const columns: ProColumns<UserMessageItem>[] = [
    {
      key: 'index',
      dataIndex: 'index',
      valueType: 'indexBorder',
      align: 'center',
    },
    {
      title: '消息内容',
      dataIndex: 'content',
      key: 'content',
      hideInSearch: true,
      align: 'center',
      render(_, record) {
        return <ContentParse record={record} />;
      },
    },
    {
      title: '发送者昵称',
      key: 'senderNickname',
      dataIndex: 'senderNickname',
      hideInSearch: true,
      align: 'center',
    },
    {
      title: '发送者ID',
      key: 'sendID',
      dataIndex: 'sendID',
      editable: false,
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
    // {
    //   title: '接收者昵称',
    //   key: 'gender',
    //   dataIndex: 'gender',
    //   hideInSearch: true,
    //   align: 'center',
    // },
    {
      title: '接收者ID',
      key: 'recvID',
      dataIndex: 'recvID',
      align: 'center',
    },
    {
      title: '会话类型',
      key: 'sessionType',
      dataIndex: 'sessionType',
      valueType: 'select',
      request: async () => SessionTypeOtions,
      align: 'center',
      initialValue: 1,
    },
    {
      title: '消息类型',
      key: 'contentType',
      dataIndex: 'contentType',
      editable: false,
      valueType: 'select',
      request: async () => MessageTypeOtions,
      align: 'center',
    },
    {
      title: '时间',
      key: 'sendTime',
      dataIndex: 'sendTime',
      valueType: 'date',
      // hideInSearch: true,
      editable: false,
      align: 'center',
      initialValue: moment().format('YYYY-MM-DD'),
      render(_, entity) {
        return moment(entity.createTime).format('YYYY-MM-DD h:mm:ss');
      },
    },
    {
      title: '操作',
      valueType: 'option',
      key: 'option',
      align: 'center',
      render: (text, record) => {
        const isDisable = !nomalMessageTypes.includes(record.contentType);
        return (
          <Popconfirm
            title="确定要撤回该条消息吗？"
            onConfirm={() => revokeMessageAction(record)}
            okText="确定"
            cancelText="取消"
            disabled={isDisable}
          >
            <a style={isDisable ? { color: '#666' } : {}} href="#">
              撤回
            </a>
          </Popconfirm>
        );
      },
    },
  ];

  return (
    <PageContainer>
      <ProTable<UserMessageItem>
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
        request={async (params: any, sort, filter) => {
          console.log(params, sort, filter);
          const { data } = await getChatLogs({
            ...params,
            msgType: (params.contentType as number) ?? 0,
            recvID: (params.recvID as string) ?? '',
            sendID: (params.sendID as string) ?? '',
            pagination: {
              pageNumber: params.current as number,
              showNumber: params.pageSize as number,
            },
          });
          return {
            data: data.chatLogs ?? [],
            success: true,
            total: data.chatLogsNum,
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
        rowKey="serverMsgID"
        search={{
          labelWidth: 'auto',
        }}
        pagination={{
          defaultPageSize: 10,
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

export default UserMessage;
