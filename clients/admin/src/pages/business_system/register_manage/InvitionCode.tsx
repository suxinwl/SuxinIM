import { generateInvitationCode, getInvitationCode } from '@/services/server/register_manage';
import { copy2Text } from '@/utils/common';
import type { ActionType, ProColumns } from '@ant-design/pro-components';
import { PageContainer, ProTable } from '@ant-design/pro-components';
import { Button, InputNumber, message, Modal } from 'antd';
import { useRef, useState } from 'react';
import type { InvitationCodeItem } from './data';

const columns: ProColumns<InvitationCodeItem>[] = [
  {
    key: 'index',
    title: '序号',
    dataIndex: 'index',
    valueType: 'indexBorder',
    width: 48,
    align: 'center',
  },
  {
    title: '邀请码',
    dataIndex: 'invitationCode',
    key: 'invitationCode',
    align: 'center',
  },
  {
    title: '使用人昵称',
    key: 'usedUserName',
    dataIndex: 'usedUserName',
    valueType: 'select',
    hideInSearch: true,
    align: 'center',
  },
  {
    title: '使用状态',
    key: 'status',
    dataIndex: 'status',
    valueType: 'select',
    hideInTable: true,
    initialValue: '全部',
    valueEnum: {
      2: '未使用',
      1: '已使用',
      0: '全部',
    },
  },
  {
    title: '使用人手机号',
    key: 'usedUserPhoneNumber',
    dataIndex: 'usedUserPhoneNumber',
    valueType: 'select',
    hideInSearch: true,
    align: 'center',
  },
  {
    title: '操作',
    valueType: 'option',
    key: 'option',
    render: (text, record) => {
      return [
        <a href="#" key="copy" onClick={() => copy2Text(record.invitationCode)}>
          复制
        </a>,
      ];
    },
  },
];

const InvitionCode = () => {
  const actionRef = useRef<ActionType>();
  const [isModalOpen, setIsModalOpen] = useState(false);
  const [genNumber, setGenNumber] = useState(10);

  const showModal = () => {
    setIsModalOpen(true);
  };

  const genCode = () => {
    generateInvitationCode(genNumber).then(() => {
      message.success('生成成功！');
      actionRef.current?.reload();
    });
    // .catch(() => message.error('生成失败！'));
  };

  const handleOk = () => {
    setIsModalOpen(false);
    genCode();
  };

  const handleCancel = () => {
    setIsModalOpen(false);
  };

  return (
    <PageContainer>
      <ProTable<InvitationCodeItem>
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
          if (!params.status) {
            params.status = 0;
          }

          if (params.invitationCode) {
            params.keyword = params.invitationCode;
          }

          const { data } = await getInvitationCode({
            ...(params as any),
            status: Number(params.status) ?? 0,
            userIDs: [],
            pagination: {
              pageNumber: params.current as number,
              showNumber: params.pageSize,
            },
          });
          // const tmpData = data.chat_logs ?? [];
          return {
            data: data.list,
            success: true,
            total: data.total,
          };
        }}
        rowKey="invitationCode"
        search={{
          labelWidth: 'auto',
        }}
        pagination={{
          pageSize: 10,
          onChange: (page) => console.log(page),
        }}
        dateFormatter="string"
        scroll={{ x: 'max-content' }}
        toolbar={{
          actions: [
            <Button key="create" type="primary" onClick={showModal}>
              生成邀请码
            </Button>,
          ],
          settings: [],
        }}
      />
      <Modal title="请选择生成数量" open={isModalOpen} onOk={handleOk} onCancel={handleCancel}>
        <div className="w-full flex justify-center">
          <InputNumber
            min={1}
            max={100}
            value={genNumber}
            onChange={(value) => {
              setGenNumber(value as number);
            }}
          />
        </div>
      </Modal>
    </PageContainer>
  );
};

export default InvitionCode;
