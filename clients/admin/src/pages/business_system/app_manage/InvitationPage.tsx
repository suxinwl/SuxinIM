import { getClientConfig, setClientConfig } from '@/services/server/app_manage';
import { PageContainer } from '@ant-design/pro-components';
import { Button, Input, message, Space } from 'antd';
import { useEffect, useRef, useState } from 'react';

interface AppConfig {
  adminURL: string;
}

const ConfigManage = () => {
  const [appConfig, setAppConfig] = useState<AppConfig>({
    adminURL: '',
  });

  const orzConfig = useRef<AppConfig>();

  const getConfig = () => {
    getClientConfig().then((res) => {
      if (!res.data.config) return;
      setAppConfig(res.data.config);
      orzConfig.current = res.data;
    });
  };

  useEffect(() => {
    getConfig();

    return () => {};
  }, []);

  const updateConfig = () => {
    if (orzConfig.current?.adminURL === appConfig.adminURL) {
      return;
    }
    setClientConfig({
      adminURL: appConfig.adminURL,
    })
      .then(() => {
        message.success('设置成功！');
      })
      .catch(() => {
        message.error('设置失败！');
        getClientConfig();
      });
  };

  return (
    <PageContainer>
      <div className="text-base font-medium mb-4">邀请码页配置</div>
      <Space.Compact>
        <Input
          style={{ width: '300px' }}
          value={appConfig.adminURL}
          onChange={(e) =>
            setAppConfig({
              adminURL: e.target.value,
            })
          }
        />
        <Button onClick={updateConfig} type="primary">
          保存
        </Button>
      </Space.Compact>
    </PageContainer>
  );
};

export default ConfigManage;
