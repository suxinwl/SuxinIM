import { deleteVideoCall, getVideoCallList } from '@/services/server/rct_manage';
import type { ActionType, ProColumns } from '@ant-design/pro-components';
import { PageContainer, ProTable } from '@ant-design/pro-components';
// import { useModel } from '@umijs/max';
import { message, Popconfirm, Space } from 'antd';
import moment from 'moment';
import { useRef, useState } from 'react';
import UserModal from './UserModal';

const VideoCall = () => {
  // const { setInitialState } = useModel('@@initialState');
  const actionRef = useRef<ActionType>();
  const [showUser, setShowUser] = useState(false);
  const [userList, setUserList] = useState<User[]>([]);

  const deleteRecord = (id: string) => {
    console.log(id);
    deleteVideoCall({ sIDs: [id] }).then(() => {
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
      .padStart(2, '0')}:${duration.seconds().toString().padStart(2, '0')}`;

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
      key: 'mediaType',
      dataIndex: 'mediaType',
      hideInSearch: true,
      align: 'center',
      render: (_, record) => <span>{record.mediaType === 'video' ? '视频通话' : '语音通话'}</span>,
    },
    {
      title: '会话类型',
      key: 'sessionType',
      dataIndex: 'sessionType',
      valueType: 'select',
      valueEnum: {
        1: '单聊',
        3: '群聊',
      },
      align: 'center',
      initialValue: '1',
    },
    {
      title: '发起者昵称',
      key: 'inviterUserNickname',
      dataIndex: 'inviterUserNickname',
      hideInSearch: true,
      align: 'center',
      render: (_, record) => <span>{record.inviterUserNickname}</span>,
    },
    {
      title: '发起者ID',
      key: 'inviterUserID',
      dataIndex: 'inviterUserID',
      align: 'center',
      render: (_, record) => <span>{record.inviterUserID}</span>,
    },
    // {
    //   title: '接收者昵称',
    //   key: 'v3.2',
    //   dataIndex: 'v3.2',
    //   hideInSearch: true,
    //   hideInTable: sessionType === '2',
    //   align: 'center',
    //   render: () => <span>关羽</span>,
    // },
    // {
    //   title: '接收者ID',
    //   key: 'v3.1',
    //   dataIndex: 'v3.1',
    //   align: 'center',
    //   hideInTable: sessionType === '2',
    //   render: () => <span>546100</span>,
    // },
    {
      title: '成员',
      key: 'inviterUsers',
      dataIndex: 'inviterUsers',
      hideInSearch: true,
      align: 'center',
      render: (_, record) => (
        <a
          href="#"
          key="show"
          onClick={() => {
            setUserList(record.inviterUsers);
            setShowUser(true);
          }}
        >
          点击查看
        </a>
      ),
    },
    {
      title: '通话时长',
      key: 'v5',
      dataIndex: 'v5',
      hideInSearch: true,
      align: 'center',
      render: (_, record) => <span>{timeDiffFormatted(record.endTime - record.createTime)}</span>,
    },
    // {
    //   title: '结束时间',
    //   key: 'endTime',
    //   dataIndex: 'endTime',
    //   hideInSearch: true,
    //   align: 'center',
    //   render: (_, record) => (
    //     <span>{moment(record.endTime * 1000).format('YYYY-MM-DD HH:mm')}</span>
    //   ),
    // },
    {
      title: '开始时间',
      key: 'createTime',
      dataIndex: 'createTime',
      hideInSearch: true,
      align: 'center',
      render: (_, record) => (
        <span>{moment(record.createTime * 1000).format('YYYY-MM-DD HH:mm')}</span>
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
            onConfirm={() => deleteRecord(record.sID)}
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
        title="参与成员"
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

          // if (params.sessionType) {
          //   setSessionType(params.sessionType);
          // }

          if (params.time) {
            params.startTime = moment(params.time[0]).unix();
            params.endTime = moment(params.time[1]).unix();
          }

          if (params.inviterUserID) {
            params.sendID = params.inviterUserID;
          }

          const { data } = await getVideoCallList({
            ...(params as any),
            sesstionType: Number(params.sessionType),
            sessionType: Number(params.sessionType),
            pagination: {
              pageNumber: params.current as number,
              showNumber: params.pageSize,
            },
          });
          return {
            data: data.signalRecords || [],
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

export default VideoCall;
