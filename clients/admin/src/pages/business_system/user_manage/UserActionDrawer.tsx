import { forceLogout, registerUser } from '@/services/server/user_manage';
import {
  Avatar,
  Button,
  Checkbox,
  DatePicker,
  Drawer,
  Form,
  Input,
  message,
  Select,
  Space,
  Upload,
} from 'antd';
import type { CheckboxValueType } from 'antd/es/checkbox/Group';
import md5 from 'md5';
import type { FC } from 'react';
import { memo, useEffect, useState } from 'react';
import type { DrawerOptions } from './UserList';

import avatar_upload from '@/assets/images/avatar_upload.png';
import { countryCode } from '@/utils/areaCode';

import { handleFileUpload } from '@/services/admin/upload';
import { avatarList } from '@/utils/avatar';
import { UploadRequestOption } from 'rc-upload/lib/interface';
import styles from './style.less';

type UserActionProps = {
  drawerOptions: DrawerOptions;
  closeDrawer: () => void;
  reload: () => void;
};

const KickPage = memo(({ drawerOptions, closeDrawer, reload }: UserActionProps) => {
  const [checkOptions, setCheckOptions] = useState([] as any);

  let checkedValue = [] as number[];

  const getOptions = () => {
    const tmpArr = [];
    if (drawerOptions.selectUser?.onlineStr?.includes('Web')) {
      tmpArr.push({
        label: 'Web端',
        value: 5,
      });
    }
    if (drawerOptions.selectUser?.onlineStr?.includes('Android')) {
      tmpArr.push({
        label: 'Android',
        value: 2,
      });
    }
    if (drawerOptions.selectUser?.onlineStr?.includes('IOS')) {
      tmpArr.push({
        label: 'IOS',
        value: 1,
      });
    }
    if (drawerOptions.selectUser?.onlineStr?.includes('Windows')) {
      tmpArr.push({
        label: 'Windows',
        value: 3,
      });
    }
    if (drawerOptions.selectUser?.onlineStr?.includes('Linux')) {
      tmpArr.push({
        label: 'Linux',
        value: 7,
      });
    }
    if (drawerOptions.selectUser?.onlineStr?.includes('OSX')) {
      tmpArr.push({
        label: 'OSX',
        value: 4,
      });
    }
    setCheckOptions(tmpArr);
  };

  useEffect(() => {
    if (drawerOptions.selectUser) {
      getOptions();
    }
  }, [drawerOptions.selectUser]);

  const onChange = (checkedValues: CheckboxValueType[]) => {
    checkedValue = checkedValues as any;
  };

  const kickOut = () => {
    if (checkedValue.length === 0) {
      message.warning('请先选择平台');
      return;
    }
    const promissArr = [] as any;
    checkedValue.forEach((value) => {
      promissArr.push(forceLogout({ userID: drawerOptions.selectUser!.userID, platformID: value }));
    });
    Promise.all(promissArr).then(() => {
      message.success('操作成功！');
      reload();
      closeDrawer();
    });
    // .catch(() => message.error('操作失败！'));
  };

  return (
    <div className="d px-3">
      <div className="d flex border-b items-center pb-5">
        {/* <Avatar
          shape="square"
          size={55}
          className="min-w-[55px]"
          src={drawerOptions.selectUser?.avatar}
        /> */}
        {(drawerOptions.selectUser?.avatar || '').length ? (
          <Avatar
            shape="square"
            size={55}
            className="min-w-[55px]"
            src={
              avatarList.find((avator) => avator.name === drawerOptions.selectUser?.avatar)?.src ||
              drawerOptions.selectUser?.avatar
            }
          ></Avatar>
        ) : (
          <Avatar
            shape="square"
            style={{ backgroundColor: '#4096FF' }}
            size={55}
            className="min-w-[55px]"
          >
            {drawerOptions.selectUser?.nickname.split('')[0]}
          </Avatar>
        )}
        <div className="flex flex-col justify-evenly ml-3 h-[55px]">
          <div className="truncate max-w-[200px]">{drawerOptions.selectUser?.nickname}</div>
          <div>{drawerOptions.selectUser?.userID}</div>
        </div>
      </div>
      <div className="pt-5">
        <Checkbox.Group
          className="flex flex-col kick_check"
          options={checkOptions}
          onChange={onChange}
        />
      </div>
      <div className="w-full text-right">
        <Button type="primary" onClick={kickOut}>
          强制下线
        </Button>
      </div>
    </div>
  );
});

type AddUserFormFields = {
  nickname: string;
  gender: number;
  birth: moment.Moment;
  phoneNumber: string;
  password: string;
  areaCode: string;
};

const AddUserPage = memo(({ closeDrawer }: { closeDrawer: () => void }) => {
  const [uploadUrl, setUploadUrl] = useState('');
  const onFinish = (values: any) => {
    registerUser({
      ...values,
      deviceID: '',
      platform: 1,
      user: {
        nickname: values.nickname,
        faceURL: uploadUrl || `ic_avatar_0${Math.ceil(Math.random() * 6)}`,
        birth: values.birth?.valueOf(),
        gender: values.gender ?? 0,
        areaCode: `+${values.areaCode}`,
        phoneNumber: values.phoneNumber,
        password: md5(values.password),
      },
    }).then(() => {
      message.success('操作成功！');
      closeDrawer();
    });
  };

  const customUpload = async (data: UploadRequestOption) => {
    try {
      const avatarUrl = await handleFileUpload(data);
      if (avatarUrl) {
        setUploadUrl(avatarUrl);
      }
    } catch (error) {
      console.log(error);
    }
  };

  return (
    <div>
      <div>
        <div className="w-full text-center mb-12">
          <div className="flex flex-col">
            <Upload accept="image/*" customRequest={customUpload} showUploadList={false}>
              <Avatar shape="square" size={55} src={uploadUrl || avatar_upload} />
              <div className="text-[#1890FFFF] mt-3">点击上传</div>
            </Upload>
          </div>
        </div>
        <Form<AddUserFormFields>
          name="basic"
          labelCol={{ span: 5 }}
          wrapperCol={{ span: 19 }}
          onFinish={onFinish}
          autoComplete="off"
          initialValues={{
            areaCode: '86',
          }}
        >
          <Form.Item
            label="昵称"
            name="nickname"
            rules={[{ required: true, message: '请输入用户昵称!' }]}
          >
            <Input placeholder="请输入用户昵称" />
          </Form.Item>

          <Form.Item label="性别" name="gender">
            <Select>
              <Select.Option value={1}>男</Select.Option>
              <Select.Option value={2}>女</Select.Option>
            </Select>
          </Form.Item>

          <Form.Item label="生日" name="birth">
            <DatePicker />
          </Form.Item>

          <Form.Item className={styles.no_mb} label={'手机号'}>
            <Space.Compact>
              <Form.Item name="areaCode">
                <Select onSearch={() => {}} showSearch className="!w-24">
                  {countryCode.map((country) => (
                    <Select.Option
                      key={country.phone_code}
                      value={country.phone_code}
                    >{`+${country.phone_code}`}</Select.Option>
                  ))}
                </Select>
              </Form.Item>
              <Form.Item
                name="phoneNumber"
                rules={[{ required: true, message: '请输入用户手机号!' }]}
              >
                <Input placeholder={'请输入用户手机号'} />
              </Form.Item>
            </Space.Compact>
          </Form.Item>

          <Form.Item
            label="密码"
            name="password"
            rules={[{ required: true, message: '请输入用户密码!' }]}
          >
            <Input.Password placeholder="请输入用户密码" />
          </Form.Item>

          <div className="w-full text-right">
            <Button className="px-6" type="primary" htmlType="submit">
              添加
            </Button>
          </div>
        </Form>
      </div>
    </div>
  );
});

const UserActionDrawer: FC<UserActionProps> = (props) => {
  const { drawerOptions, closeDrawer } = props;

  console.log(drawerOptions);

  return (
    <Drawer
      title={drawerOptions.selectUser ? '强制用户下线' : '添加新用户'}
      placement="right"
      closable={false}
      onClose={closeDrawer}
      open={drawerOptions.visible}
    >
      {drawerOptions.selectUser ? (
        <KickPage {...props} />
      ) : (
        <AddUserPage closeDrawer={closeDrawer} />
      )}
    </Drawer>
  );
};

export default UserActionDrawer;
