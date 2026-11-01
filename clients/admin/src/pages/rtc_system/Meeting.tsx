import type { ActionType, ProColumns } from '@ant-design/pro-components';
import { PageContainer, ProTable } from '@ant-design/pro-components';
// import { useModel } from '@umijs/max';
import { deleteMeeting, getMeetingList } from '@/services/server/rct_manage';
import { message, Popconfirm, Space } from 'antd';
import moment from 'moment';
import { useRef, useState } from 'react';
import UserModal from './UserModal';

const Meeting = () => {
  // const { setInitialState } = useModel('@@initialState');
  const actionRef = useRef<ActionType>();
  const [showUser, setShowUser] = useState(false);
  const [userList, setUserList] = useState<User[]>([]);

  const deleteRecord = (id: string) => {
    console.log(id);
    deleteMeeting({ roomIDs: [id] }).then(() => {
      actionRef?.current?.reload();
      message.success('操作成功！');
    });
  };

  const timeDiffFormatted = (time: number) => {
    const diffMilliseconds = Math.abs(time * 1000);
    const duration = moment.duration(diffMilliseconds);
    const formattedTimeDiff = `${duration.hours().toString().padStart(2, '0')}:${duration
      .minutes()
      .toString()
      .padStart(2, '0')}`;

    return formattedTimeDiff;
  };

  const columns: ProColumns[] = [
    {
      key: 'index',
      title: '序号',
      dataIndex: 'index',
      valueType: 'indexBorder',
      width: 48,
      align: 'center',
    },
    {
      title: '通话类型',
      key: 'v1',
      dataIndex: 'v1',
      hideInSearch: true,
      align: 'center',
      render: () => <span>视频会议</span>,
    },
    {
      title: '会议主题',
      key: 'meetingName',
      dataIndex: 'meetingName',
      hideInSearch: true,
      align: 'center',
      render: (_, record) => <span>{record.meetingName}</span>,
    },
    {
      title: '发起者',
      key: 'hostUserNickname',
      dataIndex: 'hostUserNickname',
      hideInSearch: true,
      align: 'center',
      render: (_, record) => <span>{record.hostUserNickname}</span>,
    },
    {
      title: '发起者ID',
      key: 'hostUserID',
      dataIndex: 'hostUserID',
      align: 'center',
      render: (_, record) => <span>{record.hostUserID}</span>,
    },
    {
      title: '会议成员',
      key: 'joinedUsers',
      dataIndex: 'joinedUsers',
      hideInSearch: true,
      align: 'center',
      render: (_, record) => (
        <a
          href="#"
          key="show"
          onClick={() => {
            setUserList(record.joinedUsers);
            setShowUser(true);
          }}
        >
          点击查看
        </a>
      ),
    },
    {
      title: '通话时长',
      key: 'time',
      dataIndex: 'time',
      hideInSearch: true,
      align: 'center',
      render: (_, record) => <span>{timeDiffFormatted(record.endTime - record.startTime)}</span>,
    },
    {
      title: '开始时间',
      key: 'startTime',
      dataIndex: 'startTime',
      hideInSearch: true,
      align: 'center',
      render: (_, record) => (
        <span>{moment(record.startTime * 1000).format('YYYY-MM-DD HH:mm')}</span>
      ),
    },
    {
      title: '结束时间',
      key: 'endTime',
      dataIndex: 'endTime',
      hideInSearch: true,
      align: 'center',
      render: (_, record) => (
        <span>{moment(record.endTime * 1000).format('YYYY-MM-DD HH:mm')}</span>
      ),
    },
    {
      title: '时间',
      key: 'time',
      dataIndex: 'time',
      hideInTable: true,
      align: 'center',
      valueType: 'dateTimeRange',
    },
    {
      title: '操作',
      valueType: 'option',
      key: 'option',
      align: 'center',
      render: (_, record) => (
        <Space>
          {/* <a
            href="#"
            onClick={() =>
              setInitialState((s) => ({
                ...s,
                videoUrl:
                  'http://14.29.213.197:50002/object/7156492014/msg_video_1249fa0cf30bac705d643be24209f42e.mp4',
              }))
            }
          >
            查看
          </a> */}
          <Popconfirm
            key="block"
            title={`确定要删除该记录吗？`}
            onConfirm={() => deleteRecord(record.roomID)}
            okText="确定"
            cancelText="取消"
          >
            <a href="#">删除</a>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <PageContainer>
      <UserModal
        userList={userList}
        title="会议成员"
        open={showUser}
        close={() => {
          setUserList([]);
          setShowUser(false);
        }}
      />
      <ProTable
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
          if (params.time) {
            params.startTime = moment(params.time[0]).unix();
            params.endTime = moment(params.time[1]).unix();
          }
          const { data } = await getMeetingList({
            ...(params as any),
            pagination: {
              pageNumber: params.current as number,
              showNumber: params.pageSize,
            },
          });
          return {
            data: data.meetingRecords,
            success: true,
            total: data.total,
          };
        }}
        rowKey={(record: any) => record.roomID}
        search={{
          labelWidth: 'auto',
        }}
        pagination={{
          pageSize: 10,
          onChange: (page) => console.log(page),
        }}
        dateFormatter="string"
        scroll={{ x: 'max-content' }}
        toolbar={{}}
      />
    </PageContainer>
  );
};

export default Meeting;
