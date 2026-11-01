import { getClientConfig, setClientConfig } from '@/services/server/app_manage';
import { InfoCircleOutlined } from '@ant-design/icons';
import { PageContainer } from '@ant-design/pro-components';
import { Button, Checkbox, Input, message, Space, Tooltip } from 'antd';
import { useEffect, useRef, useState } from 'react';

interface AppConfig {
  discoverPageURL?: string;
  allowSendMsgNotFriend?: string;
  needInvitationCodeRegister?: string;
}

const ConfigManage = () => {
  const [appConfig, setAppConfig] = useState<AppConfig>({
    discoverPageURL: '',
  });
  const [loading, setLoading] = useState<boolean>(false);
  const orzConfig = useRef<AppConfig>();

  const getConfig = () => {
    getClientConfig().then((res: any) => {
      if (!res.data.config) return;
      setAppConfig(res.data.config);
      console.log(res.data);
      orzConfig.current = res.data;
      // setAppConfig(res.data);
    });
  };

  useEffect(() => {
    console.log(121);
    getConfig();
    return () => {};
  }, []);

  const updateConfig = () => {
    if (orzConfig.current?.discoverPageURL === appConfig.discoverPageURL) {
      return;
    }
    setClientConfig({
      discoverPageURL: appConfig.discoverPageURL,
      allowSendMsgNotFriend: appConfig.allowSendMsgNotFriend,
      needInvitationCodeRegister: appConfig.needInvitationCodeRegister,
    })
      .then(() => {
        message.success('设置成功！');
        getClientConfig();
      })
      .catch(() => {
        message.error('设置失败！');
      });
  };

  return (
    <PageContainer>
      <div>
        <div className="text-base font-medium mb-4">APP发现页配置</div>
        <Space.Compact>
          <Input
            style={{ width: '300px' }}
            value={appConfig.discoverPageURL}
            onChange={(e) => {
              setAppConfig({
                discoverPageURL: e.target.value,
              });
            }}
            suffix={
              <Tooltip title="URL需配置https/http前缀">
                <InfoCircleOutlined style={{ color: 'rgba(0,0,0,.45)' }} />
              </Tooltip>
            }
            placeholder="URL需配置https/http前缀"
          />
          <Button onClick={updateConfig} type="primary">
            保存
          </Button>
        </Space.Compact>
      </div>

      <div className=" mt-[2rem]">
        <div className="text-base font-medium mb-6 py-5 px-5 bg-white">
          <Checkbox
            disabled={loading}
            checked={appConfig?.allowSendMsgNotFriend === '1' ? true : false}
            onChange={(e) => {
              console.log(e);
              setLoading(true);
              setClientConfig({
                allowSendMsgNotFriend: e.target.checked ? '1' : '0',
              })
                .then(() => {
                  message.success('设置成功！');
                  getConfig();
                })
                .catch(() => {
                  message.error('设置失败！');
                })
                .finally(() => setLoading(false));
            }}
          >
            非好友是否允许发送消息
            <div className=" text-xs text-gray-400">*用户需要修改服务端配置文件并重启</div>
          </Checkbox>
        </div>
      </div>

      <div className="text-base font-medium mb-6 py-5 px-5 bg-white">
        <Checkbox
          className="needInvitationCodeRegister"
          disabled={loading}
          checked={appConfig?.needInvitationCodeRegister === '1' ? true : false}
          onChange={(e) => {
            setLoading(true);
            setClientConfig({
              needInvitationCodeRegister: e.target.checked ? '1' : '0',
            })
              .then(() => {
                message.success('设置成功！');
                getConfig();
              })
              .catch(() => {
                message.error('设置失败！');
              })
              .finally(() => setLoading(false));
          }}
        >
          是否需要邀请码才能注册
        </Checkbox>
      </div>
    </PageContainer>
  );
};

export default ConfigManage;
