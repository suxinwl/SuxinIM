import type { UserListItem } from '@/pages/business_system/user_manage/data';
import { batchSend } from '@/services/server/message_manage';
import { getUserList } from '@/services/server/user_manage';
import type { ActionType, ProColumns } from '@ant-design/pro-components';
import { PageContainer, ProTable } from '@ant-design/pro-components';
import { Button, Checkbox, Col, FormInstance, Input, message, Row, Tag } from 'antd';
import { useMemo, useRef, useState } from 'react';

import { DefaultAvatar, defaultAvatarStr } from '@/utils/avatar';
import styles from './style.less';

export type SelectedRowDataItem = Record<
  number,
  {
    keys: React.Key[];
    items: UserListItem[];
  }
>;

const SendMessage = () => {
  const actionRef = useRef<ActionType>();
  const formRef = useRef<FormInstance>();
  // const [fileList, setFileList] = useState([] as any);
  const [selectedRowData, setSelectedRowData] = useState({} as SelectedRowDataItem);
  const [allSend, setAllSend] = useState(false);
  // const latestSelectData = useLatest(selectedRowData);
  const [current, setCurrent] = useState(1);
  const [content, setContent] = useState('');

  const columns: ProColumns<UserListItem>[] = useMemo(
    () => [
      {
        title: '用户头像',
        dataIndex: 'faceURL',
        valueType: 'avatar',
        key: 'faceURL',
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
        align: 'center',
      },
      {
        title: '用户ID',
        key: 'userID',
        dataIndex: 'userID',
        align: 'center',
        // formItemProps: (form, config) => {
        //   return { ...config, label: null, name: 'userID', style: { marginBottom: 0 } };
        // },
        // renderFormItem: () => {
        //   return (
        //     <>
        //       <div className="font-medium ml-3 mb-4">选择接收用户</div>
        //       <Input
        //         // value={value}
        //         onChange={(e) => formRef.current?.setFieldValue('userID', e.target.value)}
        //         className={styles.search_input}
        //         prefix={<SearchOutlined />}
        //         placeholder="输入用户ID或昵称"
        //         bordered={false}
        //       />
        //     </>
        //   );
        // },
        // editable: false,
      },
    ],
    [],
  );

  // const getPicInfo = (file: RcFile): Promise<HTMLImageElement> => {
  //   return new Promise((resolve) => {
  //     const _URL = window.URL || window.webkitURL;
  //     const img = new Image();
  //     img.onload = function () {
  //       resolve(img);
  //     };
  //     img.src = _URL.createObjectURL(file);
  //   });
  // };

  // const uploadButton = (
  //   <div>
  //     <PlusOutlined />
  //     <div style={{ marginTop: 8 }}>Upload</div>
  //   </div>
  // );

  // const customRequest = (options: UploadRequestOption) => {
  //   switchUpload(options)
  //     .then(async ({ data }) => {
  //       const tmpList = [...fileList];
  //       const { width, height } = await getPicInfo(options.file as RcFile);
  //       tmpList.push({
  //         ...(options.file as RcFile),
  //         uuid: (options.file as RcFile).uid,
  //         url: data.URL,
  //         width,
  //         height,
  //       });
  //       setFileList(tmpList);
  //     })
  // };

  const onSelectChange = (keys: React.Key[], items: UserListItem[]) => {
    const tmpData = { ...selectedRowData };
    tmpData[actionRef.current!.pageInfo!.current] = {
      keys,
      items,
    };
    console.log(keys, tmpData);
    setSelectedRowData(tmpData);
  };

  const getItems = useMemo(
    () =>
      Object.values(selectedRowData)
        .map((data) => data.items)
        .flat(),
    [selectedRowData],
  );

  const onCancelSelect = (value: string) => {
    const tmpData = { ...selectedRowData };
    // eslint-disable-next-line guard-for-in
    for (const key in tmpData) {
      const keyIdx = tmpData[key].keys.findIndex((k) => k === value);
      const itemdIdx = tmpData[key].items.findIndex((item) => item.userID === value);
      if (keyIdx !== -1 && itemdIdx !== -1) {
        tmpData[key].keys.splice(keyIdx, 1);
        tmpData[key].items.splice(itemdIdx, 1);
      }
    }
    setSelectedRowData(tmpData);
    setCurrent(current + 1);
    setTimeout(() => setCurrent(actionRef.current!.pageInfo!.current));
  };

  const send2User = () => {
    // let pictureElem;
    // if (fileList.length > 0) {
    //   pictureElem = {
    //     sourcePicture: {
    //       ...fileList[0],
    //     },
    //     bigPicture: {
    //       ...fileList[0],
    //     },
    //     snapshotPicture: {
    //       ...fileList[0],
    //     },
    //   };
    // }

    batchSend({
      sendID: localStorage.getItem('IMAdminUserID')!,
      isSendAll: allSend,
      recvIDList: getItems.map((item) => item.userID),
      text: content,
    }).then(() => {
      message.success('发送成功！');
    });
    // .catch(() => message.error('发送失败！'));
  };

  return (
    <PageContainer>
      <Row>
        <Col span={13} xxl={15}>
          <div className="font-medium">接收用户</div>
          <div className="w-full bg-white h-[120px] mt-3 mb-8 p-2 rounded-lg overflow-auto custom_scrollbar">
            {getItems.map((user) => (
              <Tag key={user.userID} closable onClose={() => onCancelSelect(user.userID)}>
                {user.nickname}
              </Tag>
            ))}
          </div>

          <div>
            <div className="font-medium">消息内容</div>
            <div className="w-full bg-white h-[350px] mt-3 py-1 rounded-lg relative px-3">
              <Input.TextArea
                className={styles.send_content}
                placeholder="请输入群发内容"
                autoSize={{ maxRows: 8, minRows: 8 }}
                bordered={false}
                value={content}
                onChange={(e) => setContent(e.target.value)}
                showCount
                maxLength={500}
              />
              {/* <Upload
                className="ml-3"
                listType="picture-card"
                accept="image/*,video/*"
                fileList={fileList}
                onRemove={() => setFileList([])}
                customRequest={customRequest}
                maxCount={1}
              >
                {fileList.length > 0 ? null : uploadButton}
              </Upload> */}
              <Button
                onClick={send2User}
                className="!absolute right-5 bottom-8 px-6"
                type="primary"
              >
                发送
              </Button>
            </div>
            <Checkbox
              checked={allSend}
              onChange={(e) => setAllSend(e.target.checked)}
              className="mt-4"
            >
              发送给所有已注册用户
            </Checkbox>
          </div>
        </Col>
        <Col className={styles.send_user_table} offset={1} span={10} xxl={8}>
          {/* <div className="font-medium mt-6 mb-4 ml-8">选择接收用户</div>
          <Input
            className={styles.search_input}
            prefix={<SearchOutlined />}
            placeholder="输入用户ID或昵称"
            bordered={false}
          /> */}
          <ProTable<UserListItem>
            search={{
              optionRender: false,
            }}
            columns={columns}
            rowSelection={{
              // @ts-ignore
              selectedRowKeys: selectedRowData[current]?.keys ?? [],
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
                keyword: params.userID ?? '',
                pagination: {
                  pageNumber: params.current as number,
                  showNumber: params.pageSize as number,
                },
              });
              const tmpData = data.users ?? [];
              tmpData.forEach((user: UserListItem, idx: number) => {
                user.index = idx;
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
            scroll={{ x: 'max-content' }}
            toolbar={{
              actions: [],
              settings: [],
            }}
          />
        </Col>
      </Row>
    </PageContainer>
  );
};

export default SendMessage;
