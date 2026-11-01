import { GroupRole } from '@/constants/enum';
import { muteGroupMember, transferGroup, updateGroupMemebr } from '@/services/server/group_manage';
import { history } from '@umijs/max';
import type { RadioChangeEvent } from 'antd';
import { Drawer, Input, message, Radio, Space } from 'antd';
import Button from 'antd/es/button';
import type { FC } from 'react';
import { useState } from 'react';
import type { GroupInfo, GroupMemebrItem } from './data';

type GroupMemberActionProps = {
  drawerOptions: DrawerOptions;
  closeDrawer: () => void;
  reload: () => void;
};

export type DrawerOptions = {
  opType: 'mute' | 'role';
  opMember?: GroupMemebrItem;
  opGroup?: GroupInfo;
  visible: boolean;
};

const MuteMemberPage: FC<GroupMemberActionProps> = (props) => {
  const { drawerOptions, closeDrawer, reload } = props;
  const [value, setValue] = useState<number>();
  const [customSec, setCustomSec] = useState<number>();

  const onChange = (e: RadioChangeEvent) => {
    console.log('radio checked', e.target.value);
    setValue(e.target.value);
  };

  const comfirmMute = () => {
    console.log(value, customSec);
    const sec = customSec ?? value;
    if ((sec ?? 0) < 60) {
      message.error('请设置至少60s！');
      return;
    }
    muteGroupMember({
      mutedSeconds: sec!,
      userID: drawerOptions.opMember!.userID,
      groupID: drawerOptions.opMember!.groupID,
    })
      .then(() => {
        message.success('操作成功！');
        reload();
      })
      // .catch(() => message.error('操作失败！'))
      .finally(() => closeDrawer());
  };

  return (
    <div>
      <div>
        <div className="font-semibold mb-6">
          {drawerOptions.opType === 'mute' ? '禁言' : '设置群身份'}
        </div>
        <Radio.Group onChange={onChange} value={value}>
          <Space direction="vertical">
            <Radio value={600}>10分钟</Radio>
            <Radio value={3600}>1小时</Radio>
            <Radio value={43200}>12小时</Radio>
            <Radio value={86400}>1天</Radio>
          </Space>
        </Radio.Group>
      </div>
      <div>
        <div className="my-6 font-semibold">自定义时长</div>
        <Input
          type="number"
          placeholder="请输入自定义时长"
          value={customSec}
          onChange={(e) => setCustomSec(Number(e.target.value))}
        />
      </div>
      <div className="w-full text-right mt-6">
        <Button onClick={comfirmMute} type="primary" className="w-[82px]">
          确定
        </Button>
      </div>
    </div>
  );
};

const SetMemberRolePage: FC<GroupMemberActionProps> = (props) => {
  const { drawerOptions, closeDrawer, reload } = props;
  const [value, setValue] = useState<GroupRole>(drawerOptions.opMember!.roleLevel);

  const onChange = (e: RadioChangeEvent) => {
    console.log('radio checked', e.target.value);
    setValue(e.target.value);
  };

  const comfirmRole = () => {
    console.log(drawerOptions);
    const func =
      value === GroupRole.Owner
        ? transferGroup({
            oldOwnerUserID: drawerOptions.opGroup!.ownerUserID,
            newOwnerUserID: drawerOptions.opMember!.userID,
            groupID: drawerOptions.opGroup!.groupID,
          })
        : updateGroupMemebr({ members: [{ ...drawerOptions.opMember!, roleLevel: value }] });
    func
      .then(() => {
        message.success('操作成功!');
        reload();
        history.push('/im_system/group_manage/group_list');
      })
      // .catch(() => message.error('操作失败！'))
      .finally(() => {
        closeDrawer();
      });
  };

  return (
    <div>
      <div className="font-semibold mb-6">
        {drawerOptions.opType === 'mute' ? '禁言' : '设置群身份'}
      </div>
      <Radio.Group onChange={onChange} value={value}>
        <Space direction="vertical" size="middle">
          <Radio value={GroupRole.Nomal}>普通成员</Radio>
          <Radio value={GroupRole.Admin}>管理员</Radio>
          <Radio value={GroupRole.Owner}>群主</Radio>
        </Space>
      </Radio.Group>
      {value !== drawerOptions.opMember?.roleLevel && (
        <div className="w-full text-right mt-6">
          <Button onClick={comfirmRole} type="primary" className="w-[82px]">
            确定
          </Button>
        </div>
      )}
    </div>
  );
};

const GroupMemberActionDrawer: FC<GroupMemberActionProps> = (props) => {
  const { drawerOptions, closeDrawer } = props;

  return (
    <Drawer placement="right" closable={false} onClose={closeDrawer} open={drawerOptions.visible}>
      {drawerOptions.opType === 'mute' ? (
        <MuteMemberPage {...props} />
      ) : (
        <SetMemberRolePage {...props} />
      )}
    </Drawer>
  );
};

export default GroupMemberActionDrawer;
