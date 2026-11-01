import type { UserListItem } from '@/pages/business_system/user_manage/data';
import UserActionDrawer from '@/pages/business_system/user_manage/UserActionDrawer';
import { getUserList } from '@/services/server/im_user_manage';
import { getUsersOnlineStatus, updateUserInfo } from '@/services/server/user_manage';
import { DefaultAvatar, defaultAvatarStr } from '@/utils/avatar';
import type { ActionType, ProColumns } from '@ant-design/pro-components';
import { PageContainer, ProTable } from '@ant-design/pro-components';
import { history } from '@umijs/max';
import { Space } from 'antd';
import moment from 'moment';
import { useMemo, useRef, useState } from 'react';
import PreviewMessageModal from './PreviewMessageModal/PreviewMessageModal';

export type DrawerOptions = {
  visible: boolean;
  selectUser: UserListItem | undefined;
};

const UserList = () => {
  const actionRef = useRef<ActionType>();
  const [selectedUserID, setSelectUserID] = useState('');
  const [drawerOptions, setDrawerOptions] = useState<DrawerOptions>({
    visible: false,
    selectUser: undefined,
  });

  const columns: ProColumns<UserListItem>[] = useMemo(
    () => [
      {
        title: '序号',
        key: 'index',
        dataIndex: 'index',
        valueType: 'indexBorder',
        width: 48,
        align: 'center',
      },
      {
        title: '用户头像',
        dataIndex: 'faceURL',
        valueType: 'avatar',
        key: 'faceURL',
        hideInSearch: true,
        editable: false,
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
        align: 'center',
      },
      {
        title: '用户ID',
        key: 'userID',
        dataIndex: 'userID',
        editable: false,
        hideInSearch: true,
        align: 'center',
      },
      {
        title: '在线状态',
        key: 'onlineStr',
        dataIndex: 'onlineStr',
        hideInSearch: true,
        align: 'center',
      },
      {
        title: '操作',
        valueType: 'option',
        key: 'option',
        align: 'center',
        render: (_, record) => (
          <Space>
            <a
              key={'onlineStr'}
              href="#"
              onClick={() => {
                if (record.onlineStr?.length) {
                  setDrawerOptions({
                    visible: true,
                    selectUser: record,
                  });
                }
              }}
            >
              强制下线
            </a>
            <a
              key="friend"
              onClick={() =>
                history.push('/im_system/user_manage/relation_list?userID=' + record.userID)
              }
            >
              关系链
            </a>
            {/* 203服务器新版本 先不做 */}
            <a key="msg" onClick={() => setSelectUserID(record.userID)}>
              会话
            </a>
          </Space>
        ),
      },
    ],
    [],
  );

  const closeDrawer = () => {
    setDrawerOptions({
      visible: false,
      selectUser: undefined,
    });
  };

  const reload = () => {
    actionRef.current?.reload();
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

  const attachInfo = async (userList: UserListItem[]) => {
    const { data } = await getUsersOnlineStatus(userList.map((user) => user.userID));
    (data ?? []).forEach((status: any) => {
      userList.find((user, idx) => {
        if (user.userID === status.userID) {
          user.index = idx;
          user.avatar = defaultAvatarStr(user.faceURL);
          user.onlineStr = switchOnline('online', status.singlePlatformToken);
        }
        return null;
      });
    });
  };

  return (
    <PageContainer>
      <ProTable<UserListItem>
        columns={columns}
        actionRef={actionRef}
        cardBordered
        toolbar={{
          actions: [],
          settings: [],
        }}
        search={false}
        columnsState={{
          defaultValue: {
            option: {
              fixed: 'right',
            },
          },
        }}
        request={async (params = {}, sort, filter) => {
          console.log(params, sort, filter);
          const { data } = await getUserList({
            keyword: params.userID ?? '',
            pagination: {
              pageNumber: params.current as number,
              showNumber: params.pageSize as number,
            },
          });
          const tmpData = data.users ?? [];
          await attachInfo(tmpData);
          console.log(data.users);
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
          onSave: (key, record) => {
            const options = { ...record };
            if (record.birth) {
              options.birth = moment(record.birth).unix();
            }
            return updateUserInfo(options);
          },
        }}
        rowKey="userID"
        pagination={{
          defaultPageSize: 10,
          showQuickJumper: true,
          onChange: (page) => console.log(page),
        }}
        dateFormatter="number"
        scroll={{ x: 'max-content' }}
      />
      {selectedUserID && (
        <PreviewMessageModal
          userID={selectedUserID}
          closeModal={()=>setSelectUserID('')}
        />
      )}
      <UserActionDrawer closeDrawer={closeDrawer} reload={reload} drawerOptions={drawerOptions} />
    </PageContainer>
  );
};

export default UserList;
