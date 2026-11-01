import { handleFileUpload } from '@/services/admin/upload';
import { update_info } from '@/services/server/user_manage';
import { UploadOutlined } from '@ant-design/icons';
import { PageContainer } from '@ant-design/pro-components';
import { useModel } from '@umijs/max';
import { Avatar, Button, Card, Input, message, Space, Upload } from 'antd';
import { UploadRequestOption } from 'rc-upload/lib/interface';
import { useState } from 'react';

const Profile = () => {
  const { initialState, setInitialState } = useModel('@@initialState');
  const { nickname = '', faceURL = '' } = initialState?.currentUser ?? {};
  const [userInfo, setUserInfo] = useState({
    nickname,
    faceURL,
  });
  const [loading, setLoading] = useState(false);

  const updateUserAvatar = async (url: string) => {
    await update_info({
      ...userInfo,
      userID: localStorage.getItem('IMAdminUserID')!,
      faceURL: url,
    });
    await setInitialState((s) => ({
      ...s,
      currentUser: {
        ...s?.currentUser,
        nickname: userInfo.nickname,
        faceURL: url,
      },
    }));
    setUserInfo({
      nickname: userInfo.nickname,
      faceURL: url,
    });
  };

  const customUpload = async (data: UploadRequestOption) => {
    try {
      const avatarUrl = await handleFileUpload(data);
      if (avatarUrl) {
        updateUserAvatar(avatarUrl);
      }
    } catch (error) {
      console.log(error);
    }
  };

  const updateAdminInfo = async () => {
    setLoading(true);
    try {
      await update_info({
        nickname: userInfo.nickname,
        userID: localStorage.getItem('IMAdminUserID')!,
      });
      await setInitialState((s) => ({
        ...s,
        currentUser: {
          ...s?.currentUser,
          nickname: userInfo.nickname,
          faceURL: userInfo.faceURL,
        },
      }));
      message.success('修改成功');
    } catch (error) {
      message.error('修改失败！');
    } finally {
      setLoading(false);
    }
  };

  return (
    <PageContainer>
      <Card>
        <div className="mb-4 font-medium text-base">头像</div>
        <Upload accept="image/*" customRequest={customUpload} showUploadList={false}>
          <Avatar shape="square" size={110} src={faceURL || '/admin/icons/icon.png'} />
          <div className="text-[#1890FFFF] mt-3">
            <Button icon={<UploadOutlined />}>点击上传</Button>
          </div>
        </Upload>

        <div className="my-4 font-medium text-base">昵称</div>
        <Space.Compact>
          <Input
            value={userInfo.nickname}
            onChange={(e) => {
              setUserInfo({
                ...userInfo,
                nickname: e.target.value,
              });
            }}
            style={{ width: '260px' }}
          />
          <Button loading={loading} onClick={updateAdminInfo} type="primary">
            保存
          </Button>
        </Space.Compact>
      </Card>
    </PageContainer>
  );
};

export default Profile;
