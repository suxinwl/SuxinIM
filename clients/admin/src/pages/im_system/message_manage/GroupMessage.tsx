import group_img from '@/assets/images/group.png';
import ContentParse from '@/components/ContentParse';
import { MessageTypeOtions } from '@/constants/messageTypeOtions';
import { getChatLogs, revokeMessage } from '@/services/server/message_manage';
import { getConversationID } from '@/utils/common';
import type { ActionType, ProColumns } from '@ant-design/pro-components';
import { PageContainer, ProTable } from '@ant-design/pro-components';
import { Avatar, message, Popconfirm } from 'antd';
import moment from 'moment';
import { useMemo, useRef } from 'react';
import type { GroupListItem } from './data';

const GroupList = () => {
  const actionRef = useRef<ActionType>();

  const nomalMessageTypes = [101, 102, 103, 104, 105, 106, 107, 108, 109, 110, 114, 115, 117];

  const revokeMessageAction = (record: GroupListItem) => {
    if (!nomalMessageTypes.includes(record.contentType)) {
      message.warning('该类型消息不可撤回！');
      return;
    }
    const options = {
      userID: record.sendID,
      conversationID: getConversationID({ groupID: record.groupID }),
      seq: record.seq,
    };
    revokeMessage(options).then(() => {
      message.success('撤回成功！');
      actionRef.current?.reload();
    });
    // .catch(() => message.error('撤回失败！'));
  };

  const columns: ProColumns<GroupListItem>[] = useMemo(
    () => [
      {
        key: 'index',
        dataIndex: 'index',
        valueType: 'indexBorder',
        align: 'center',
      },
      {
        title: '群头像',
        dataIndex: 'groupFaceURL',
        valueType: 'avatar',
        key: 'groupFaceURL',
        hideInSearch: true,
        align: 'center',
        render(_, record) {
          return <Avatar shape="square" src={record.faceURL || group_img}></Avatar>;
        },
      },
      {
        title: '群名称',
        key: 'groupName',
        dataIndex: 'groupName',
        hideInSearch: true,
        align: 'center',
      },
      {
        title: '消息内容',
        dataIndex: 'content',
        key: 'content',
        hideInSearch: true,
        align: 'center',
        ellipsis: true,
        render(_, record) {
          return <ContentParse record={record} />;
        },
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
        title: '群组ID',
        key: 'groupID',
        dataIndex: 'groupID',
        align: 'center',
        editable: false,
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
        title: '群人数',
        key: 'groupMemberCount',
        dataIndex: 'groupMemberCount',
        align: 'center',
        hideInSearch: true,
      },
      {
        title: '群主ID',
        key: 'groupOwner',
        dataIndex: 'groupOwner',
        align: 'center',
        hideInSearch: true,
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
        title: '群类型',
        key: 'groupType',
        dataIndex: 'groupType',
        hideInSearch: true,
        editable: false,
        valueType: 'select',
        align: 'center',
        request: async () => {
          return [
            {
              label: '普通群',
              value: 0,
            },
            {
              label: '超级大群',
              value: 1,
            },
            {
              label: '工作群',
              value: 2,
            },
          ];
        },
      },
      {
        title: '操作',
        valueType: 'option',
        key: 'option',
        align: 'center',
        render: (_, record) => {
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
    ],
    [],
  );

  return (
    <PageContainer>
      <ProTable<GroupListItem>
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
          const { data } = await getChatLogs({
            ...params,
            msgType: 0,
            recvID: params.groupID ?? '',
            sendID: '',
            sessionType: 3,
            pagination: {
              pageNumber: params.current as number,
              showNumber: params.pageSize as number,
            },
          });
          const tmpData = data.chatLogs ?? [];
          return {
            data: tmpData,
            success: true,
            total: data.chatLogsNum,
          };
        }}
        rowKey="serverMsgID"
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
          actions: [],
          settings: [],
        }}
      />
    </PageContainer>
  );
};

export default GroupList;
