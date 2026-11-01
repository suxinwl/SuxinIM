// import type { SelectedRowDataItem } from '@/pages/message_manage/SendMessage';
import type { UserListItem } from '@/pages/business_system/user_manage/data';
import { getSomeGroupMemberList } from '@/services/server/group_manage';
import { getUserList } from '@/services/server/user_manage';
import { DefaultAvatar, defaultAvatarStr } from '@/utils/avatar';
import { SearchOutlined } from '@ant-design/icons';
import type { ActionType, ProColumns } from '@ant-design/pro-components';
import { ProTable } from '@ant-design/pro-components';
import type { ColProps, FormInstance } from 'antd';
import { Col, Input } from 'antd';
import type { ForwardRefRenderFunction } from 'react';
import React, { forwardRef, memo, useImperativeHandle, useMemo, useRef, useState } from 'react';
import styles from './style.less';

export type SelectedRowDataItem = Record<
  number,
  {
    keys: React.Key[];
    items: UserListItem[];
  }
>;

interface SelectUserTableProps {
  colProps?: ColProps;
  selectType?: 'radio' | 'checkbox';
  groupID?: string;
  defaultSelected?: SelectedRowDataItem;
  onSelectedChange?: (data: UserListItem[], selectedRowData: SelectedRowDataItem) => void;
}

export type SelectUserTableHandle = {
  //   SelectedList: UserListItem[];
  cancelSelect: (value: string) => void;
  clearSelect: () => void;
};

const SelectUserTable: ForwardRefRenderFunction<SelectUserTableHandle, SelectUserTableProps> = (
  props,
  ref,
) => {
  const { colProps, selectType, defaultSelected, groupID, onSelectedChange } = props;
  const actionRef = useRef<ActionType>();
  const formRef = useRef<FormInstance>();
  const [selectedRowData, setSelectedRowData] = useState(
    defaultSelected ?? ({} as SelectedRowDataItem),
  );
  const [current, setCurrent] = useState(1);

  const columns: ProColumns<UserListItem>[] = useMemo(
    () => [
      {
        title: '用户头像',
        dataIndex: 'avatar',
        valueType: 'avatar',
        key: 'avatar',
        hideInSearch: true,
        align: 'center',
        render(_, record) {
          return <DefaultAvatar faceURL={record.faceURL} nickname={record.nickname} />;
        },
      },
      {
        title: '昵称',
        key: 'nickname',
        dataIndex: 'nickname',
        hideInSearch: true,
        width: 120,
        ellipsis: true,
        align: 'center',
      },
      {
        title: '是否在群中',
        key: 'inGroup',
        dataIndex: 'inGroup',
        hideInSearch: true,
        align: 'center',
        hideInTable: groupID === '',
        render: (_, record) => (record.inGroup ? '是' : '否'),
      },
      {
        title: '用户ID',
        key: 'userID',
        dataIndex: 'userID',
        align: 'center',
        ellipsis: true,
        formItemProps: (form, config) => {
          return { ...config, label: null, name: 'userID', style: { marginBottom: 0 } };
        },
        renderFormItem: () => {
          return (
            <>
              <div className="font-medium ml-3 mb-4">选择接收用户</div>
              <Input
                // value={value}
                onChange={(e) => {
                  formRef.current?.setFieldValue('userID', e.target.value);
                  formRef.current?.setFieldValue('nickname', e.target.value);
                }}
                className={styles.search_input}
                prefix={<SearchOutlined />}
                placeholder="输入用户ID或昵称"
                bordered={false}
              />
            </>
          );
        },
        editable: false,
      },
    ],
    [],
  );

  const getItems = (data: SelectedRowDataItem) => {
    return Object.values(data)
      .map((data) => data.items)
      .flat();
  };

  const onSelectChange = (keys: React.Key[], items: UserListItem[]) => {
    const tmpData = { ...selectedRowData };
    tmpData[actionRef.current!.pageInfo!.current] = {
      keys,
      items,
    };
    console.log(keys, tmpData);
    setSelectedRowData(tmpData);
    if (onSelectedChange) {
      const data = getItems(tmpData);
      onSelectedChange(data, tmpData);
    }
  };

  const cancelSelect = (value: string) => {
    const tmpData = { ...selectedRowData };
    for (const key in tmpData) {
      if (tmpData.hasOwnProperty(key)) {
        const keyIdx = tmpData[key].keys.findIndex((k) => k === value);
        const itemdIdx = tmpData[key].items.findIndex((item) => item.userID === value);
        if (keyIdx !== -1 && itemdIdx !== -1) {
          tmpData[key].keys.splice(keyIdx, 1);
          tmpData[key].items.splice(itemdIdx, 1);
        }
      }
    }
    setSelectedRowData(tmpData);
    setCurrent(current + 1);
    setTimeout(() => setCurrent(actionRef.current!.pageInfo!.current));
    if (onSelectedChange) {
      const data = getItems(tmpData);
      onSelectedChange(data, selectedRowData);
    }
  };

  const clearSelect = () => {
    actionRef.current?.clearSelected!();
  };

  const getSelectKey = useMemo(
    () => selectedRowData[current]?.keys ?? [],
    [selectedRowData, current],
  );

  useImperativeHandle(ref, () => ({
    cancelSelect,
    clearSelect,
  }));

  return (
    <Col className={styles.send_user_table} {...(colProps ?? {})}>
      <ProTable<UserListItem>
        search={{
          optionRender: false,
        }}
        columns={columns}
        rowSelection={{
          columnTitle: '-',
          selectedRowKeys: getSelectKey,
          type: selectType,
          onChange: onSelectChange,
        }}
        tableAlertRender={false}
        tableAlertOptionRender={false}
        actionRef={actionRef}
        formRef={formRef}
        columnsState={{
          defaultValue: {
            option: {
              fixed: 'right',
            },
          },
          persistenceKey: 'user_table_insearch',
          persistenceType: 'sessionStorage',
        }}
        request={async (params = {}, sort, filter) => {
          console.log(params, sort, filter);
          const { data } = await getUserList({
            ...params,
            keyword: params.userID,
            pagination: {
              pageNumber: params.current as number,
              showNumber: params.pageSize as number,
            },
          });
          const tmpData: UserListItem[] = data.users ?? [];
          let inGroupUserIDs = [] as string[];
          if (groupID) {
            const { data } = await getSomeGroupMemberList({
              groupID,
              userIDs: tmpData.map((item) => item.userID),
            });
            console.log(data);
            inGroupUserIDs = ((data.members as UserListItem[]) || []).map((user) => user.userID);
          }
          tmpData.forEach((user: UserListItem, idx: number) => {
            user.index = idx;
            user.inGroup = inGroupUserIDs.includes(user.userID);
            user.avatar = defaultAvatarStr(user.faceURL);
          });
          return {
            data: tmpData,
            success: true,
            total: data.total,
          };
        }}
        rowKey="userID"
        pagination={{
          pageSize: 10,
          onChange: (page) => {
            console.log(page);
            setCurrent(page);
          },
          showSizeChanger: false,
        }}
        dateFormatter="string"
        // scroll={{ x: 'max-content' }}
        toolbar={{
          actions: [],
          settings: [],
        }}
      />
    </Col>
  );
};

export default memo(forwardRef(SelectUserTable));
