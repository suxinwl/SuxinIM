import { GroupJoinSource, GroupRole } from '@/constants/enum';
import {
  cancelMuteGroupMember,
  getGroupMemberList,
  invite2Group,
  kickGroupMemebr,
} from '@/services/server/group_manage';
import { getUsersOnlineStatus } from '@/services/server/user_manage';
import { DefaultAvatar, defaultAvatarStr } from '@/utils/avatar';
import type { ActionType, ProColumns } from '@ant-design/pro-components';
import { PageContainer, ProTable } from '@ant-design/pro-components';
import { history, useLocation } from '@umijs/max';
import { Button, message, Popconfirm, Space } from 'antd';
import moment from 'moment';
import { useEffect, useMemo, useRef, useState } from 'react';
import type { GroupInfo, GroupMemebrItem } from './data';
import type { DrawerOptions } from './GroupMemberActionDrawer';
import GroupMemberActionDrawer from './GroupMemberActionDrawer';
import SelectUserModal, { SelectedListItem, SelectModalOptions } from './SelectUserModal';

const GroupMember = () => {
  const actionRef = useRef<ActionType>();
  const location = useLocation();
  const [drawerOptions, setDrawerOptions] = useState<DrawerOptions>({
    opType: 'mute',
    visible: false,
  });
  const [selectModalOptions, setSelectModalOptions] = useState<SelectModalOptions>({
    visible: false,
    selectType: 'member',
  });
  const currentGroup = useRef({} as GroupInfo);

  useEffect(() => {
    currentGroup.current = (location.state as any)?.groupInfo;
    if (!(location.state as any)?.groupInfo) {
      history.push('/im_system/group_manage/group_list');
    }
  }, []);

  console.log(currentGroup);

  const reload = () => {
    actionRef.current?.reload();
  };

  const columns: ProColumns<GroupMemebrItem>[] = useMemo(
    () => [
      {
        key: 'index',
        dataIndex: 'index',
        valueType: 'indexBorder',
        width: 48,
      },
      {
        title: '群员头像',
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
        title: '群员ID',
        key: 'userID',
        dataIndex: 'userID',
        hideInSearch: true,
        align: 'center',
      },
      {
        title: '群员角色',
        key: 'roleLevel',
        dataIndex: 'roleLevel',
        hideInSearch: true,
        editable: false,
        valueType: 'select',
        align: 'center',
        request: async () => {
          return [
            {
              label: '普通群员',
              value: GroupRole.Nomal,
            },
            {
              label: '群管理员',
              value: GroupRole.Admin,
            },
            {
              label: '群主',
              value: GroupRole.Owner,
            },
          ];
        },
      },
      {
        title: '群昵称',
        key: 'nickname',
        dataIndex: 'nickname',
        // hideInSearch: true,
        align: 'center',
      },
      {
        title: '入群时间',
        key: 'joinTime',
        dataIndex: 'joinTime',
        hideInSearch: true,
        align: 'center',
        renderText(text) {
          return moment(text).format('YYYY-MM-DD');
        },
      },
      {
        title: '在线状态',
        key: 'onlineStr',
        dataIndex: 'onlineStr',
        hideInSearch: true,
        editable: false,
        align: 'center',
      },
      {
        title: '入群方式',
        key: 'joinSource',
        dataIndex: 'joinSource',
        hideInSearch: true,
        editable: false,
        valueType: 'select',
        align: 'center',
        request: async () => {
          return [
            {
              label: '邀请入群',
              value: GroupJoinSource.Invitation,
            },
            {
              label: '扫描二维码',
              value: GroupJoinSource.QrCode,
            },
            {
              label: '搜索入群',
              value: GroupJoinSource.Search,
            },
          ];
        },
      },
      {
        title: '操作',
        valueType: 'option',
        key: 'option',
        align: 'center',
        render: (text, record) => {
          const isMuted = record.muteEndTime > moment().unix();
          const isOwner = record.roleLevel === GroupRole.Owner;
          return (
            <Space>
              <a key="editable" onClick={() => {}}>
                查看
              </a>
              <a
                href="#"
                className={isOwner ? ' text-[#bba8a883]' : ''}
                onClick={() => {
                  if (isOwner) return;
                  setDrawerOptions({
                    opType: 'role',
                    visible: true,
                    opMember: record,
                    opGroup: currentGroup.current,
                  });
                }}
              >
                设置身份
              </a>
              <Popconfirm
                title={`确定要关闭禁言吗？`}
                onConfirm={() => {
                  cancelMuteGroupMember({ userID: record.userID, groupID: record.groupID }).then(
                    () => {
                      message.success('操作成功');
                      reload();
                    },
                  );
                  // .catch(() => message.error('操作失败'));
                }}
                disabled={!isMuted || isOwner}
                okText="确定"
                cancelText="取消"
              >
                <a
                  href="#"
                  className={isOwner ? ' text-[#bba8a883]' : ''}
                  onClick={() => {
                    if (isMuted || isOwner) return;
                    setDrawerOptions({
                      opType: 'mute',
                      visible: true,
                      opMember: record,
                    });
                  }}
                >
                  {isMuted ? '取消禁言' : '禁言'}
                </a>
              </Popconfirm>
              <Popconfirm
                title="确定要移除该成员吗？"
                onConfirm={() => {
                  kickGroupMemebr({
                    groupID: record.groupID,
                    kickedUserIDs: [record.userID],
                    reason: '',
                  }).then(() => {
                    message.success('操作成功！');
                    reload();
                  });
                  // .catch(() => message.error('操作失败！'));
                }}
                okText="确定"
                cancelText="取消"
                disabled={isOwner}
              >
                <a className={isOwner ? ' text-[#bba8a883]' : ''} href="#">
                  移除
                </a>
              </Popconfirm>
            </Space>
          );
        },
      },
    ],
    [],
  );

  const closeSelectModal = () => {
    setSelectModalOptions({
      visible: false,
      selectType: 'member',
    });
  };

  const selectedCallBack = async (data: SelectedListItem) => {
    const tmpArr = data.data.map((user) => user.userID);
    try {
      await invite2Group({
        groupID: currentGroup.current.groupID,
        invitedUserIDs: tmpArr,
        reason: '',
      });
      actionRef.current?.reload();
      message.success('邀请成功！');
    } catch (error) {
      message.error('邀请失败！');
    }
  };

  const switchOnline = (oType: string, details: any) => {
    let str = '';
    switch (oType) {
      case 'offline':
        return '离线';
      case 'online':
        details?.map((detail: any) => {
          str += `${detail.platform}/`;
          return null;
        });
        return `${str.slice(0, -1)}在线`;
      default:
        return '';
    }
  };

  const attachInfo = async (memberList: GroupMemebrItem[]) => {
    const { data } = await getUsersOnlineStatus(memberList.map((user) => user.userID));
    (data ?? []).forEach((status: any) => {
      memberList.find((user, idx) => {
        if (user.userID === status.userID) {
          user.index = idx;
          user.avatar = defaultAvatarStr(user.faceURL);
          user.onlineStr = switchOnline('online', status.singlePlatformToken);
        }
        return null;
      });
    });
  };

  const closeDrawer = () => {
    setDrawerOptions({
      opType: 'mute',
      visible: false,
    });
  };

  return (
    <PageContainer>
      <ProTable<GroupMemebrItem>
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
          const { data } = await getGroupMemberList({
            groupID: currentGroup.current.groupID,
            pagination: {
              pageNumber: params.current as number,
              showNumber: params.pageSize as number,
            },
          });
          const tmpData: GroupMemebrItem[] = data.members ?? [];
          await attachInfo(tmpData);
          return {
            data: tmpData,
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
              key="key"
              type="primary"
              onClick={() => {
                setSelectModalOptions({
                  visible: true,
                  selectType: 'member',
                });
              }}
            >
              添加群成员
            </Button>,
          ],
          settings: [],
        }}
      />
      <GroupMemberActionDrawer
        drawerOptions={drawerOptions}
        closeDrawer={closeDrawer}
        reload={reload}
      />
      <SelectUserModal
        groupID={currentGroup.current.groupID}
        selectModalOptions={selectModalOptions}
        selectedCallBack={selectedCallBack}
        closeSelectModal={closeSelectModal}
      />
    </PageContainer>
  );
};

export default GroupMember;
