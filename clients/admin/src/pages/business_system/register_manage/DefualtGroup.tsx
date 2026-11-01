import {
  addDefaultGroup,
  getuserregistergroupidlist,
  removeDefaultGroup,
} from '@/services/server/register_manage';
import type { ActionType, ProColumns } from '@ant-design/pro-components';
import { PageContainer, ProTable } from '@ant-design/pro-components';
import { Button, Input, message, Popconfirm } from 'antd';
import { useRef, useState } from 'react';
import type { DefualtGroupItem } from './data';

const DefualtGroups = () => {
  const actionRef = useRef<ActionType>();
  const [addInputval, setaddInputval] = useState<string>();

  const columns: ProColumns<DefualtGroupItem>[] = [
    {
      title: '序号',
      key: 'index',
      dataIndex: 'index',
      valueType: 'indexBorder',
      width: 88,
      align: 'center',
    },
    {
      title: '群组ID',
      dataIndex: 'groupID',
      key: 'groupID',
      align: 'center',
      // formItemProps: (form, config) => {
      //   return {
      //     ...config,
      //     label: '群组ID',
      //     name: 'groupID',
      //     style: { marginBottom: 0 },
      //   };
      // },
      renderFormItem: () => {
        return (
          <Input
            value={addInputval}
            onChange={(e) => {
              let val = e.target.value;
              setaddInputval(val);
            }}
            // className={styles.search_input}
            placeholder="输入群组ID"
            bordered={true}
          />
        );
      },
    },
    {
      title: '群组名称',
      key: 'groupName',
      dataIndex: 'groupName',
      align: 'center',
      hideInSearch: true,
      render: (_, record) => <span>{record.groupName}</span>,
    },
    {
      title: '操作',
      valueType: 'option',
      key: 'option',
      align: 'center',
      render: (_, record) => (
        <Popconfirm
          key={'remove'}
          title="确定要移除该群组吗？"
          onConfirm={async () => {
            try {
              let val = [record.groupID];
              await removeDefaultGroup(val);
              actionRef.current?.reload();
              message.success('删除成功！');
            } catch (error) {
              message.error('删除失败！');
            }
          }}
          okText="确定"
          cancelText="取消"
        >
          <a
            onClick={(e) => {
              e.preventDefault();
            }}
            href="#"
          >
            移除
          </a>
        </Popconfirm>
      ),
    },
  ];

  return (
    <PageContainer>
      <ProTable<DefualtGroupItem>
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
          const { data } = await getuserregistergroupidlist(
            params.current ?? 1,
            params.pageSize ?? 10,
          );
          return {
            data: data.groups,
            success: true,
            total: data.total,
          };
        }}
        rowKey="groupID"
        search={{
          labelWidth: 'auto',
          searchText: '添加',
          optionRender: () => [
            <Button
              key={'add'}
              onClick={async () => {
                if (!addInputval) {
                  return;
                }
                try {
                  let groupIDList = [];
                  groupIDList.push(addInputval);
                  await addDefaultGroup(groupIDList);
                  setaddInputval('');
                  actionRef.current?.reload();
                  message.success('新增成功！');
                } catch (error) {
                  message.error('新增失败！');
                }
              }}
              className=" "
              type="primary"
            >
              添加
            </Button>,
          ],
        }}
        pagination={{
          pageSize: 10,
          onChange: (page) => console.log(page),
        }}
        dateFormatter="string"
        scroll={{ x: 'max-content' }}
        toolbar={{
          actions: [],
          settings: [],
        }}
      />
    </PageContainer>
  );
};

export default DefualtGroups;
