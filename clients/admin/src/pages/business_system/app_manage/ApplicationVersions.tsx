import {ACCOUNT_URL,ENTERPRISE_URL} from '@/config';
import {PageContainer} from '@ant-design/pro-components';
import {request} from '@umijs/max';
import {Alert,Button,Card,Form,Input,InputNumber,message,Select,Space,Switch,Table} from 'antd';
import {useEffect,useState} from 'react';
export default function ApplicationVersions(){
 const [form]=Form.useForm();const [versions,setVersions]=useState<any[]>([]);const [loading,setLoading]=useState(false);const [saving,setSaving]=useState(false);const [error,setError]=useState('');
 async function refresh(){setLoading(true);setError('');try{const result=await request(ACCOUNT_URL+'application/page_versions',{method:'POST',data:{platforms:[],pagination:{pageNumber:1,showNumber:100}}});setVersions(result.data?.versions??result.data?.applications??[]);}catch(e:any){setError(e.message??'加载失败');}finally{setLoading(false);}}
 useEffect(()=>{void refresh();},[]);
 async function publish(values:any){setSaving(true);setError('');try{await request(ENTERPRISE_URL+'application/publish',{method:'POST',data:values});message.success('版本发布成功');form.resetFields();await refresh();}catch(e:any){setError(e.message??'发布失败');}finally{setSaving(false);}}
 return <PageContainer title="应用版本管理">
  <Alert type="info" showIcon message="先上传完整安装包，再登记实际大小与 SHA-256。版本和构建号必须递增，已发布文件不可覆盖。" style={{marginBottom:16}}/>
  {error&&<Alert type="error" showIcon message={error}/>}
  <Card title="发布版本" style={{marginBottom:16}}><Form form={form} layout="vertical" onFinish={publish} initialValues={{platform:'windows',force:false}}>
   <Space align="start" wrap>
    <Form.Item name="platform" label="平台" rules={[{required:true}]}><Select style={{width:160}} options={[{value:'windows',label:'Windows x64'},{value:'android',label:'Android ARM64'},{value:'ios',label:'iOS'}]}/></Form.Item>
    <Form.Item name="version" label="语义版本号" rules={[{required:true}]}><Input placeholder="3.8.3"/></Form.Item>
    <Form.Item name="build" label="递增构建号" rules={[{required:true}]}><InputNumber min={1} precision={0}/></Form.Item>
    <Form.Item name="size" label="安装包字节数" rules={[{required:true}]}><InputNumber min={1} precision={0} style={{width:180}}/></Form.Item>
   </Space>
   <Form.Item name="url" label="HTTPS 下载地址" rules={[{required:true},{type:'url'},{pattern:/^https:\/\//,message:'必须使用HTTPS'}]}><Input/></Form.Item>
   <Form.Item name="sha256" label="SHA-256" rules={[{required:true},{pattern:/^[0-9a-fA-F]{64}$/,message:'输入64位十六进制值'}]}><Input/></Form.Item>
   <Form.Item name="text" label="更新说明" rules={[{required:true}]}><Input.TextArea rows={3}/></Form.Item>
   <Form.Item name="force" label="强制更新" valuePropName="checked"><Switch/></Form.Item>
   <Button type="primary" htmlType="submit" loading={saving}>校验元数据并发布</Button>
  </Form></Card>
  <Card title="已登记版本" extra={<Button onClick={()=>void refresh()}>刷新</Button>}><Table loading={loading} dataSource={versions} rowKey="id" pagination={{pageSize:10}} columns={[{title:'平台',dataIndex:'platform'},{title:'版本',dataIndex:'version'},{title:'更新说明',dataIndex:'text'},{title:'强更',dataIndex:'force',render:v=>v?'是':'否'},{title:'当前发布',dataIndex:'latest',render:v=>v?'是':'否'},{title:'下载地址',dataIndex:'url',ellipsis:true}]}/></Card>
 </PageContainer>;
}
