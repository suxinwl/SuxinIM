import { crteateApplet } from '@/services/server/app_manage';
import { PlusOutlined, UploadOutlined } from '@ant-design/icons';
import { Button, Form, Input, Modal, Upload } from 'antd';

type AppModalProps = {
  isOpen: boolean;
  isCreate: boolean;
  setIsOpen: (isOpen: boolean) => void;
};

const AppModal = ({ isOpen, isCreate, setIsOpen }: AppModalProps) => {
  console.log(isCreate);
  const [form] = Form.useForm();

  const handleCancel = () => {
    setIsOpen(false);
  };

  const onFinish = async (e: any) => {
    console.log(e);
    await crteateApplet({ ...e, id: '' });
  };

  return (
    <Modal title="应用信息" open={isOpen} footer={null} onCancel={handleCancel}>
      <Form
        labelCol={{ span: 4 }}
        wrapperCol={{ span: 14 }}
        layout="horizontal"
        form={form}
        onFinish={onFinish}
      >
        <Form.Item label="AppID" name="appID">
          <Input placeholder="openim" />
        </Form.Item>
        <Form.Item label="应用名称" name="name">
          <Input placeholder="openim" />
        </Form.Item>
        <Form.Item label="应用描述">
          <Input placeholder="即时通讯" />
        </Form.Item>
        <Form.Item label="应用版本" name="version">
          <Input placeholder="1.1.1" />
        </Form.Item>
        <Form.Item label="应用图标" valuePropName="fileList" name="icon">
          <Upload action="/upload.do" listType="picture-card">
            <div>
              <PlusOutlined />
            </div>
          </Upload>
        </Form.Item>
        <Form.Item label="应用文件" valuePropName="fileList">
          <Upload>
            <Button icon={<UploadOutlined />}>Click to Upload</Button>
          </Upload>
        </Form.Item>
        <Form.Item wrapperCol={{ span: 14, offset: 4 }}>
          <Button type="primary" htmlType="submit">
            上传
          </Button>
        </Form.Item>
      </Form>
    </Modal>
  );
};

export default AppModal;
