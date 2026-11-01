import { changeAdminPassword } from '@/services/server/user_manage';
import { PageContainer } from '@ant-design/pro-components';
import { Button, Card, Form, Input, message } from 'antd';
import md5 from 'md5';

type FormFields = {old_password: string; password: string; password2: string};

export default function Password() {
  const [form] = Form.useForm<FormFields>();
  const onFinish = async (values: FormFields) => {
    try {
      await changeAdminPassword({
        userID: localStorage.getItem('IMAdminUserID')!,
        currentPassword: md5(values.old_password),
        newPassword: md5(values.password),
      });
      form.resetFields();
      message.success('修改成功');
    } catch {
      message.error('修改失败，请检查当前密码');
    }
  };
  return <PageContainer><Card>
    <Form form={form} layout="vertical" onFinish={onFinish} autoComplete="off" style={{width: 300}}>
      <Form.Item label="当前密码" name="old_password" rules={[{required: true, message: '请输入当前密码'}]}>
        <Input.Password autoComplete="current-password" />
      </Form.Item>
      <Form.Item label="新密码" name="password" rules={[{required: true, min: 6, message: '请输入至少6位新密码'}]}>
        <Input.Password autoComplete="new-password" />
      </Form.Item>
      <Form.Item label="确认密码" name="password2" dependencies={['password']} rules={[
        {required: true, message: '请再次输入新密码'},
        ({getFieldValue}) => ({validator(_, value) {return !value || getFieldValue('password')===value ? Promise.resolve() : Promise.reject(new Error('两次输入的密码不一致'));}}),
      ]}>
        <Input.Password autoComplete="new-password" />
      </Form.Item>
      <Form.Item><Button type="primary" htmlType="submit" block>保存</Button></Form.Item>
    </Form>
  </Card></PageContainer>;
}
