import group_img from '@/assets/images/group.png';
import { GroupStatus } from '@/constants/enum';
import {
  cancelMuteGroup,
  dismissGroup,
  getGroupList,
  muteGroup,
} from '@/services/server/group_manage';
import type { ActionType, ProColumns } from '@ant-design/pro-components';
import { PageContainer, ProTable } from '@ant-design/pro-components';
import { history } from '@umijs/max';
import { Avatar, Button, message, Popconfirm, Space } from 'antd';
import moment from 'moment';
import { useMemo, useRef, useState } from 'react';
import type { GroupListItem } from './data';
import type { DrawerOptions } from './GroupActionDrawer';
import GroupActionDrawer from './GroupActionDrawer';

const GroupList = () => {
  const actionRef = useRef<ActionType>();
  const [drawerOptions, setDrawerOptions] = useState<DrawerOptions>({
    visible: false,
    opType: 'create',
  });

  const reload = () => {
    actionRef.current?.reload();
  };

  const columns: ProColumns<GroupListItem>[] = useMemo(
    () => [
      {
        key: 'index',
        dataIndex: 'index',
        valueType: 'indexBorder',
      },
      {
        title: '群头像',
        dataIndex: 'faceURL',
        valueType: 'avatar',
        key: 'faceURL',
        hideInSearch: true,
        align: 'center',
        render(_, record) {
          return <Avatar shape="square" src={record.groupInfo.faceURL || group_img}></Avatar>;
        },
      },
      {
        title: '群名称',
        key: 'groupName',
        dataIndex: 'groupName',
        hideInSearch: true,
        align: 'center',
        render: (text, record) => <span>{record.groupInfo.groupName}</span>,
      },
      {
        title: '群组ID',
        key: 'groupID',
        dataIndex: 'groupID',
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
        render: (text, record) => <span>{record.groupInfo.groupID}</span>,
      },
      {
        title: '群人数',
        key: 'memberCount',
        dataIndex: 'memberCount',
        hideInSearch: true,
        align: 'center',
        render: (text, record) => <span>{record.groupInfo.memberCount}</span>,
      },
      {
        title: '群主ID',
        key: 'GroupOwnerID',
        dataIndex: 'GroupOwnerID',
        hideInSearch: true,
        align: 'center',
        render: (text, record) => <span>{record.groupInfo.ownerUserID}</span>,
      },
      {
        title: '创建时间',
        key: 'createTime',
        dataIndex: 'createTime',
        hideInSearch: true,
        align: 'center',
        renderText(text, record) {
          return moment(record.groupInfo.createTime).format('YYYY-MM-DD');
        },
      },
      {
        title: '操作',
        valueType: 'option',
        key: 'option',
        align: 'center',
        render: (text, record) => {
          const isMuted = record.groupInfo.status === GroupStatus.Muted;
          return (
            <Space>
              <a
                onClick={() => {
                  history.push('/im_system/group_manage/group_member', {
                    groupInfo: record.groupInfo,
                  });
                  // action?.startEditable?.(record.groupID);
                }}
              >
                群成员
              </a>
              <a
                href="#"
                onClick={() => {
                  setDrawerOptions({
                    visible: true,
                    opType: 'edit',
                    groupInfo: { ...record.groupInfo },
                  });
                }}
              >
                群聊设置
              </a>
              <Popconfirm
                title={`确定要${isMuted ? '关闭' : '开启'}全体禁言吗？`}
                onConfirm={() => {
                  const func = isMuted ? cancelMuteGroup : muteGroup;
                  func(record.groupInfo.groupID).then(() => {
                    message.success('操作成功！');
                    reload();
                  });
                  // .catch(() => message.error('操作失败！'));
                }}
                okText="确定"
                cancelText="取消"
              >
                <a href="#">{isMuted ? '取消禁言' : '全体禁言'}</a>
              </Popconfirm>
              <Popconfirm
                title="确定要解散该群组吗？"
                onConfirm={
                  () =>
                    dismissGroup(record.groupInfo.groupID).then(() => {
                      message.success('操作成功！');
                      reload();
                    })
                  // .catch(() => message.error('操作失败！'))
                }
                okText="确定"
                cancelText="取消"
              >
                <a href="#">解散</a>
              </Popconfirm>
            </Space>
          );
        },
      },
    ],
    [],
  );

  const closeDrawer = () => {
    setDrawerOptions({
      visible: false,
      opType: 'create',
      groupInfo: undefined,
    });
  };

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
          const { data } = await getGroupList({
            ...params,
            groupID: params.groupID ?? '',
            pagination: {
              pageNumber: params.current as number,
              showNumber: params.pageSize as number,
            },
          });
          const tmpData = data.groups ?? [];
          return {
            data: tmpData,
            success: true,
            total: data.total,
          };
        }}
        rowKey={(record: any) => record.groupInfo.groupID}
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
              key="add"
              type="primary"
              onClick={() => {
                setDrawerOptions({
                  visible: true,
                  opType: 'create',
                  groupInfo: undefined,
                });
              }}
            >
              新建群组
            </Button>,
          ],
          settings: [],
        }}
      />
      <GroupActionDrawer reload={reload} drawerOptions={drawerOptions} closeDrawer={closeDrawer} />
    </PageContainer>
  );
};

export default GroupList;
