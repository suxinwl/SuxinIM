import { getApplet } from '@/services/server/app_manage';
import { ActionType, PageContainer, ProColumns, ProTable } from '@ant-design/pro-components';
import { Button } from 'antd';
import { useRef, useState } from 'react';
import AppModal from './AppModal';

const Workbench = () => {
  const actionRef = useRef<ActionType>();
  const [isOpen, setIsOpen] = useState(true);
  const columns: ProColumns<applet>[] = [
    {
      dataIndex: 'index',
      valueType: 'indexBorder',
      width: 48,
    },
    {
      title: '应用名称',
      dataIndex: 'name',
      key: 'name',
      align: 'center',
    },
    {
      title: 'icon',
      dataIndex: 'icon',
      key: 'icon',
      valueType: 'avatar',
      hideInSearch: true,
      align: 'center',
    },
    {
      title: '版本',
      dataIndex: 'version',
      key: 'version',
      hideInSearch: true,
      align: 'center',
    },
    {
      title: 'APPID',
      key: 'appID',
      dataIndex: 'appID',
      hideInSearch: true,
      align: 'center',
    },
    {
      title: '操作',
      valueType: 'option',
      key: 'option',
      render: (text, record, _, action) => (
        <a
          key="editable"
          onClick={() => {
            action?.startEditable?.(record.id);
          }}
        >
          移除
        </a>
      ),
    },
  ];

  return (
    <PageContainer>
      <AppModal isOpen={isOpen} isCreate={true} setIsOpen={setIsOpen} />

      <ProTable
        columns={columns}
        actionRef={actionRef}
        cardBordered
        request={async (params = {}, sort, filter) => {
          console.log(params, sort, filter);
          const { data } = await getApplet({
            keyword: params.name ?? '',
            pagination: {
              pageNumber: (params.current as number) - 1,
              showNumber: params.pageSize as number,
            },
          });
          return {
            data: data.applets,
            success: true,
            total: data.total,
          };
        }}
        editable={{
          type: 'multiple',
        }}
        rowKey="id"
        search={{
          labelWidth: 'auto',
        }}
        pagination={{
          pageSize: 10,
          onChange: (page) => console.log(page),
        }}
        dateFormatter="string"
        toolBarRender={() => [
          <Button key="button" onClick={() => {}} type="primary">
            新建
          </Button>,
        ]}
      />
    </PageContainer>
  );
};

export default Workbench;
