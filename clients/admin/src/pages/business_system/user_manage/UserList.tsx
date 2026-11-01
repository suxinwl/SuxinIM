import { handleFileUpload } from '@/services/admin/upload';
import {
  getUserList,
  getUsersOnlineStatus,
  resetUserPassword,
  updateBlock,
  updateUserInfo,
} from '@/services/server/user_manage';
import { DefaultAvatar, defaultAvatarStr } from '@/utils/avatar';
import type { ActionType, ProColumns } from '@ant-design/pro-components';
import { PageContainer, ProTable } from '@ant-design/pro-components';
import { Button, Input, InputRef, message, Modal, Popconfirm, Space, Upload } from 'antd';
import md5 from 'md5';
import moment from 'moment';
import { UploadRequestOption } from 'rc-upload/lib/interface';
import { useMemo, useRef, useState } from 'react';
import type { UserListItem } from './data';
import UserActionDrawer from './UserActionDrawer';

export type DrawerOptions = {
  visible: boolean;
  selectUser: UserListItem | undefined;
};

const UserList = () => {
  const actionRef = useRef<ActionType>();
  const [drawerOptions, setDrawerOptions] = useState<DrawerOptions>({
    visible: false,
    selectUser: undefined,
  });
  const [resetPwdData, setResetPwdData] = useState({
    userID: '',
    modalVisible: false,
    confirmLoading: false,
  });
  const passwordRef = useRef<InputRef>(null);

  const customUpload = async (userID: string, data: UploadRequestOption) => {
    try {
      const avatarUrl = await handleFileUpload(data);
      if (avatarUrl) {
        await updateUserInfo({
          userID,
          faceURL: avatarUrl,
        });
        actionRef.current?.reload();
      }
    } catch (error) {
      console.log(error);
    }
  };

  const columns: ProColumns<UserListItem>[] = useMemo(
    () => [
      {
        key: 'index',
        dataIndex: 'index',
        valueType: 'indexBorder',
        width: 48,
      },
      {
        title: '用户ID/手机号',
        key: 'keyword',
        dataIndex: 'keyword',
        editable: false,
        hideInTable: true,
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
          return (
            <Upload
              accept="image/*"
              customRequest={(option) => customUpload(record.userID, option)}
              showUploadList={false}
              className="cursor-pointer"
            >
              <DefaultAvatar faceURL={record.faceURL} nickname={record.nickname} />
            </Upload>
          );
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
        hideInSearch: true,
        editable: false,
        align: 'center',
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
      {
        title: '手机号',
        key: 'phoneNumber',
        dataIndex: 'phoneNumber',
        hideInSearch: true,
        editable: false,
        align: 'center',
      },
      {
        title: '用户角色',
        key: 'level',
        dataIndex: 'level',
        hideInSearch: true,
        valueType: 'select',
        align: 'center',
        request: async () => {
          return [
            {
              label: '普通用户',
              value: 1,
            },
            {
              label: '高级用户',
              value: 100,
            },
          ];
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
        title: '操作',
        valueType: 'option',
        key: 'option',
        align: 'center',
        render: (text, record, _, action) => {
          return (
            <Space>
              <a
                key="editable"
                onClick={() => {
                  action?.startEditable?.(record.userID);
                }}
              >
                编辑
              </a>
              <a
                href="#"
                key="reset"
                onClick={() => {
                  setResetPwdData({
                    userID: record.userID,
                    modalVisible: true,
                    confirmLoading: false,
                  });
                }}
              >
                重置密码
              </a>
              <Popconfirm
                key="block"
                title={`确定要${record.forbidden ? '解封' : '禁用'}该用户吗？`}
                onConfirm={() => {
                  updateBlock(record.userID).then(() => {
                    action?.reload();
                    message.success('操作成功！');
                  });
                  // .catch(() => message.error('操作失败！'));
                }}
                okText="确定"
                cancelText="取消"
              >
                <a href="#">{record.forbidden ? '解封' : '禁用'}</a>
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
      selectUser: undefined,
    });
  };

  const reload = () => {
    actionRef.current?.reload();
  };

  const handleOk = () => {
    if (!passwordRef.current?.input?.value) {
      return;
    }
    setResetPwdData({
      ...resetPwdData,
      confirmLoading: true,
    });
    resetUserPassword({
      userID: resetPwdData.userID,
      newPassword: md5(passwordRef.current?.input?.value),
    })
      .then(() => {
        message.success('设置成功！');
        setResetPwdData({
          userID: '',
          modalVisible: false,
          confirmLoading: false,
        });
      })
      .catch(() => {
        setResetPwdData({
          userID: '',
          modalVisible: false,
          confirmLoading: false,
        });
      });
  };

  const handleCancel = () => {
    setResetPwdData({
      userID: '',
      modalVisible: false,
      confirmLoading: false,
    });
  };

  const switchOnline = (oType: string, details: any) => {
    switch (oType) {
      case 'offline':
        return '离线';
      case 'online': {
        const platforms = Array.from(new Set<string>((Array.isArray(details) ? details : [])
          .map((detail: any) => detail.platform)
          .filter((platform: unknown): platform is string => typeof platform === 'string' && platform.length > 0)));
        return platforms.length ? `${platforms.join('/')}在线` : '离线';
      }
      default:
        return '';
    }
  };

  const attachInfo = async (userList: UserListItem[]) => {
    userList.forEach((user, index) => {
      user.index = index;
      user.avatar = defaultAvatarStr(user.faceURL);
      user.onlineStr = '离线';
    });
    const { data } = await getUsersOnlineStatus(userList.map((user) => user.userID));
    (data ?? []).forEach((status: any) => {
      userList.find((user, idx) => {
        if (user.userID === status.userID) {
          user.index = idx;
          user.avatar = defaultAvatarStr(user.faceURL);
          user.onlineStr = switchOnline('online', status.singlePlatformToken ?? status.detailPlatformStatus);
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
            keyword: params.keyword as string,
            pagination: {
              pageNumber: params.current as number,
              showNumber: params.pageSize as number,
            },
          });
          const tmpData = data.users ?? [];
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
          onSave: (key, record) => {
            const options = { ...record };
            if (record.birth) {
              options.birth = moment(record.birth).valueOf();
            }
            return updateUserInfo(options);
          },
        }}
        rowKey="userID"
        search={{
          labelWidth: 'auto',
        }}
        pagination={{
          defaultPageSize: 10,
          showQuickJumper: true,
          onChange: (page) => console.log(page),
        }}
        dateFormatter="number"
        scroll={{ x: 'max-content' }}
        toolbar={{
          actions: [
            <Button
              key="key"
              type="primary"
              onClick={() => {
                setDrawerOptions({
                  visible: true,
                  selectUser: undefined,
                });
              }}
            >
              添加新用户
            </Button>,
          ],
          settings: [],
        }}
      />
      <UserActionDrawer closeDrawer={closeDrawer} reload={reload} drawerOptions={drawerOptions} />
      {resetPwdData.modalVisible && (
        <Modal
          title="输入密码"
          open={resetPwdData.modalVisible}
          confirmLoading={resetPwdData.confirmLoading}
          onOk={handleOk}
          onCancel={handleCancel}
        >
          <div className="w-full flex justify-center">
            <Input.Password ref={passwordRef} placeholder="请输入新密码" />
          </div>
        </Modal>
      )}
    </PageContainer>
  );
};

export default UserList;
