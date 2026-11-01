import login_bg from '@/assets/images/login_bg.png';
import { adminLogin } from '@/services/server/login';
import { LockOutlined, UserOutlined } from '@ant-design/icons';
import { history, useIntl, useModel } from '@umijs/max';
import { Button, Checkbox, Form, Input, message } from 'antd';
import md5 from 'md5';

type FormField = {
  account: string;
  secrect: string;
};

const Login = () => {
  const intl = useIntl();
  const { setInitialState } = useModel('@@initialState');
  const onFinish = async (value: FormField) => {
    try {
      // 登录
      const res: any = await adminLogin(value.account, md5(value.secrect));
      if (res.errCode !== 0) {
        throw Error(res.errMsg);
      }

      localStorage.setItem('level', res.data.level);
      localStorage.setItem('IMAccountToken', res.data.adminToken);
      localStorage.removeItem('IMAdminToken');
      localStorage.setItem('IMAdminAccount', value.account);
      localStorage.setItem('IMAdminUserID', res.data.adminUserID);

      await setInitialState((s: any) => ({
        ...s,
        currentUser: res.data,
        isAdmin: res.data.level === 100,
      }));

      message.success('登录成功！');
      setTimeout(() => {
        history.push('/business_system/user_manage/user_list');
      }, 500);
      return;
    } catch (error) {
      console.log(error);
    }
  };

  return (
    <div className="w-screen h-screen flex justify-center items-center">
      <div className="flex">
        <img src={login_bg} alt="" />
        <div className="flex flex-col ml-24">
          <div className="text-2xl font-medium mb-16">
            {intl.formatMessage({ id: 'login.welcome' })}
          </div>
          <Form
            name="basic"
            className="w-[364px]"
            onFinish={onFinish}
            initialValues={{
              check: true,
              // account: localStorage.getItem('IMAdminAccount') ?? '',
              // account: 'openIM123456',
              // secrect: 'openIM123456',
            }}
            autoComplete="off"
            requiredMark={false}
            size="large"
          >
            <Form.Item
              name="account"
              rules={[
                { required: true, message: intl.formatMessage({ id: 'login.account.required' }) },
              ]}
            >
              <Input
                prefix={<UserOutlined className="text-[#4686fc]" />}
                placeholder={intl.formatMessage({ id: 'login.account' })}
              />
            </Form.Item>

            <Form.Item
              name="secrect"
              rules={[
                { required: true, message: intl.formatMessage({ id: 'login.secrect.required' }) },
              ]}
            >
              <Input.Password
                height={40}
                prefix={<LockOutlined className="text-[#4686fc]" />}
                placeholder={intl.formatMessage({ id: 'login.secrect' })}
              />
            </Form.Item>

            <Form.Item wrapperCol={{ span: 24 }}>
              <Button className="w-full" type="primary" htmlType="submit">
                {intl.formatMessage({ id: 'login' })}
              </Button>
            </Form.Item>

            <Form.Item name="check" valuePropName="checked">
              <Checkbox>
                {intl.formatMessage({ id: 'login.agreement' })}
                <span className="text-[#4686fc]">
                  {intl.formatMessage({ id: 'login.agreement.service' })}
                </span>
                {intl.formatMessage({ id: 'and' })}
                <span className="text-[#4686fc]">
                  {intl.formatMessage({ id: 'login.agreement.privacy' })}
                </span>
              </Checkbox>
            </Form.Item>
          </Form>
        </div>
      </div>
    </div>
  );
};

export default Login;
