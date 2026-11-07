<template>
  <div class="communication-page">
    <a-page-header :show-back="false" subtitle="账号与权限由 gosuxinwl 统一管理">
      <template #title><span class="platform-title"><img :src="imLogo" alt="IM" />{{ canManage ? '通讯管理' : '企业即时通讯' }}</span></template>
      <template #extra><a-space><a-button type="primary" :loading="launching" @click="launch">进入通讯</a-button><a-button :loading="initializing" @click="initialize">刷新权限</a-button></a-space></template>
    </a-page-header>
    <a-alert v-if="initialError" type="error" class="notice">{{ initialError }}</a-alert>
    <a-alert v-if="!canManage && !initializing && !initialError" type="info">使用“进入通讯”打开聊天。账号、资料及通讯权限请联系系统管理员维护。</a-alert>
    <a-card v-if="canManage && capabilities?.operations.includes('storage/capacity')" title="数据存储容量" class="notice" :bordered="false">
      <template #extra><a-button size="small" :loading="capacityLoading" @click="loadCapacity">刷新容量</a-button></template>
      <a-alert v-if="capacityError || !capacity?.dataCapacity" :type="capacityError ? 'error' : 'warning'">{{ capacityError || '数据卷挂载尚未核验，容量状态未知，不能据此判断业务存储余量。' }}</a-alert>
      <a-alert v-else-if="capacity.status === 'warning' || capacity.status === 'critical'" :type="capacity.status === 'critical' ? 'error' : 'warning'">数据存储空间{{ capacity.status === 'critical' ? '严重不足' : '接近预警阈值' }}，请联系管理员处理。</a-alert>
      <p v-if="capacity">采样时间：{{ new Date(capacity.measuredAt * 1000).toLocaleString() }}</p>
      <div v-if="capacity" class="capacity-grid"><a-card v-for="disk in capacity.filesystems" :key="disk.kind" size="small" :title="disk.kind === 'mongo' ? '消息数据库' : disk.kind === 'minio' ? '附件存储' : disk.kind === 'service_root' ? '服务根文件系统（仅参考）' : disk.kind">
        <template v-if="disk.measured && disk.dataCapacity && disk.usageRatio !== null"><a-progress :percent="Math.max(0, Math.min(1, disk.usageRatio))" :status="disk.status === 'critical' ? 'danger' : disk.status === 'warning' ? 'warning' : 'normal'" /><p>已用 {{ capacityBytes(disk.usage_bytes) }} / {{ capacityBytes(disk.total_bytes) }}，可用 {{ capacityBytes(disk.available_bytes) }}</p></template>
        <a-tag v-else color="orange">{{ disk.kind === 'service_root' ? '不代表数据卷余量' : '容量未确认' }}</a-tag>
      </a-card></div>
    </a-card>
    <a-tabs v-if="canManage" v-model:active-key="activeSection" @change="changeSection">
      <a-tab-pane v-for="section in sections" :key="section.key" :title="section.title" />
    </a-tabs>
    <a-card v-if="canManage" :bordered="false">
      <a-space wrap class="action-bar">
        <a-select data-testid="operation-select" v-model="selectedOperation" :style="{ width: '260px' }" @change="changeOperation">
          <a-option v-for="operation in availableOperations" :key="operation.name" :value="operation.name">{{ operation.title }}</a-option>
        </a-select>
        <a-tag color="arcoblue">OpenIM {{ capabilities?.version || '3.8' }}</a-tag>
      </a-space>
      <a-alert v-if="selectedOperation === 'employees/provision'" :type="previewed ? 'info' : 'warning'" class="notice">{{ previewed ? '已读取开通预览，请核对员工与冲突后确认。' : '请先执行“预览首次通讯开通”，再确认开通。' }}</a-alert>
      <a-alert v-if="selectedOperation === 'versions/update'" type="info" class="notice">已发布的安装包保持不变；更新说明和强制更新策略可以修改。</a-alert>
      <a-alert v-if="selectedRecordLabel" type="info" class="notice" data-testid="record-selection">
        已选操作对象：{{ selectedRecordLabel }}。切换本模块操作时会自动填入对应字段；提交并确认后才会修改数据。
        <a-button type="text" size="small" @click="clearSelection">清除选择</a-button>
      </a-alert>
      <a-form v-if="currentOperation" :model="form" layout="vertical" @submit-success="submit">
        <div class="form-grid">
          <a-form-item v-for="field in currentOperation.fields" :key="field.key" :field="field.key" :label="field.label" :required="field.required" :data-field="field.key">
            <a-switch v-if="field.kind === 'boolean'" :model-value="Boolean(form[field.key])" @update:model-value="value => form[field.key] = value" />
            <a-input-number v-else-if="field.kind === 'number'" :model-value="typeof form[field.key] === 'number' ? Number(form[field.key]) : undefined" @update:model-value="value => form[field.key] = value" :min="field.min ?? 0" :precision="0" :style="{ width: '100%' }" />
            <a-textarea v-else-if="field.kind === 'text' || field.kind === 'list'" :model-value="String(form[field.key] ?? '')" @update:model-value="value => form[field.key] = value" :auto-size="{ minRows: 2, maxRows: 5 }" :placeholder="field.kind === 'list' ? '多个 ID 使用换行或逗号分隔' : field.placeholder" />
            <a-input v-else :model-value="String(form[field.key] ?? '')" @update:model-value="value => form[field.key] = value" :placeholder="field.placeholder" allow-clear />
          </a-form-item>
        </div>
        <a-space>
          <a-button data-testid="submit-operation" html-type="submit" type="primary" :status="currentOperation.destructive ? 'danger' : undefined" :loading="loading" :disabled="!availableOperations.length || initializing || (selectedOperation === 'config/update' && !configReady)">{{ currentOperation.title }}</a-button>
          <a-button @click="resetForm">重置表单</a-button>
        </a-space>
      </a-form>
      <a-empty v-else description="当前账号没有此模块的管理权限" />
    </a-card>
    <a-card v-if="canManage" class="results" title="操作结果" :bordered="false">
      <a-alert v-if="resultError" type="error" class="notice">{{ resultError }}</a-alert>
      <div v-if="queried && !currentOperation?.mutation" class="result-filters" data-testid="result-filters">
        <a-input v-model="filters.keyword" data-testid="result-keyword" class="filter-keyword" :placeholder="isEmployeeTable ? '搜索姓名、工号、部门或原因' : '筛选当前页内容'" :aria-label="isEmployeeTable ? '搜索员工' : '筛选当前页内容'" allow-clear />
        <template v-if="isEmployeeTable">
          <div class="filter-select"><a-select data-testid="filter-department" :model-value="filters.department || undefined" placeholder="全部部门" aria-label="筛选部门" allow-clear @update:model-value="value => filters.department = typeof value === 'string' ? value : ''">
            <a-option v-for="department in departments" :key="department" :value="department">{{ department }}</a-option>
          </a-select></div>
          <div class="filter-select"><a-select data-testid="filter-enabled" :model-value="filters.enabled || undefined" :placeholder="isEmployeePreview ? '全部开通情况' : '全部启用状态'" aria-label="筛选启用状态" allow-clear @update:model-value="value => filters.enabled = typeof value === 'string' ? value : ''">
            <a-option value="true">{{ isEmployeePreview ? '可开通' : '已启用' }}</a-option>
            <a-option value="false">{{ isEmployeePreview ? '暂不可开通' : '未启用' }}</a-option>
          </a-select></div>
          <div class="filter-select"><a-select data-testid="filter-status" :model-value="filters.status || undefined" placeholder="全部同步状态" aria-label="筛选同步状态" allow-clear @update:model-value="value => filters.status = typeof value === 'string' ? value : ''">
            <a-option v-for="(label, status) in employeeStatusLabels" :key="status" :value="status">{{ label }}</a-option>
          </a-select></div>
        </template>
        <a-button :disabled="!hasResultFilters" @click="clearResultFilters">清空筛选</a-button>
        <span class="filter-summary" data-testid="filter-summary">{{ isEmployeeTable ? '员工记录' : '本页结果' }}：匹配 {{ filteredRows.length }} / {{ rows.length }} 条</span>
      </div>
      <a-table v-if="filteredRows.length" :columns="columns" :data="filteredRows" :loading="loading" :pagination="false" :scroll="{ x: '100%' }" row-key="__key">
        <template #cell="{ record, column }"><span class="cell-value">{{ isEmployeeTable ? employeeCell(record, String(column.dataIndex)) : display(record[column.dataIndex]) }}</span></template>
        <template #use="{ record }"><a-space><a-button v-if="canUseRecord(record)" type="text" size="small" :disabled="loading" title="只将记录填入本模块的操作表单，不会立即修改数据" @click="useRecord(record)">填入操作表单</a-button><a-button v-if="canRemoveDiscovery(record)" type="text" status="danger" size="small" :disabled="loading" @click="removeDiscovery(record)">删除入口</a-button></a-space></template>
      </a-table>
      <a-empty v-else-if="queried && !loading" :description="hasResultFilters && rows.length ? '没有符合筛选条件的记录，请调整或清空筛选' : '没有符合条件的记录'" />
      <a-empty v-else-if="!loading" description="选择功能并查询，结果会显示在这里" />
      <a-space v-if="currentOperation?.paged" class="pager">
        <a-button :disabled="page <= 1 || loading" @click="queryPage(page - 1)">上一页</a-button>
        <span>第 {{ page }} 页<span v-if="total !== null">，共 {{ total }} 条</span></span>
        <a-button :disabled="loading || rows.length < pageSize || (total !== null && page * pageSize >= total)" @click="queryPage(page + 1)">下一页</a-button>
      </a-space>
    </a-card>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue';
import { Message, Modal, type TableColumnData } from '@arco-design/web-vue';
import { communicationOperation, getCommunicationCapabilities, getCommunicationAccess, getCommunicationTicket, type CommunicationCapabilities, type StorageCapacity } from '@/api/communication';

type Field = { key: string; label: string; kind?: 'text' | 'list' | 'number' | 'boolean'; required?: boolean; min?: number; placeholder?: string; initial?: string | number | boolean };
type Operation = { name: string; title: string; fields: Field[]; paged?: boolean; mutation?: boolean; destructive?: boolean };
import { extractResponse, formValue, mergeDiscovery, object, validateHTTPS, type FormValue, type Row } from './model';
import { employeeDepartments, employeeFilterValues, filterRecords, type ResultFilters } from './filters';
const imLogo = `${import.meta.env.BASE_URL}brand/im/logo.png?v=puty3`;
const f = (key: string, label: string, required = false, kind?: Field['kind']): Field => ({ key, label, required, kind });
const group = f('groupID', '群 ID', true);
const user = f('userID', '用户 ID', true);
const period = [f('startTime', '开始时间（Unix 秒）', false, 'number'), f('endTime', '结束时间（Unix 秒）', false, 'number')];
const sections = [
  { key: 'employees', title: '员工同步' },
  { key: 'groups', title: '群与成员' }, { key: 'messages', title: '聊天管理' },
  { key: 'users', title: '在线用户' }, { key: 'friends', title: '好友关系' },
  { key: 'rtc', title: '通话与会议' }, { key: 'versions', title: '客户端版本' },
  { key: 'discovery', title: '发现页' }, { key: 'config', title: '客户端配置' },
];
const operations: Operation[] = [
  { name: 'employees/list', title: '查询员工同步状态', fields: [] },
  { name: 'employees/preview', title: '预览首次通讯开通', fields: [] },
  { name: 'employees/provision', title: '确认首次通讯开通', mutation: true, fields: [f('confirm', '确认按预览结果开通员工通讯', true, 'boolean')] },
  { name: 'employees/retry', title: '重试同步任务', mutation: true, fields: [] },
  { name: 'groups/search', title: '查询群', fields: [f('keyword', '群名称或 ID')], paged: true },
  { name: 'groups/members', title: '查询群成员', fields: [group], paged: true },
  { name: 'groups/create', title: '创建群', mutation: true, fields: [f('groupName', '群名称', true), f('ownerUserID', '群主用户 ID', true), f('memberUserIDs', '成员用户 ID', true, 'list'), f('notification', '公告', false, 'text')] },
  { name: 'groups/info', title: '修改群资料', mutation: true, fields: [group, f('groupName', '群名称'), f('notification', '公告', false, 'text'), f('introduction', '群介绍', false, 'text')] },
  { name: 'groups/notice', title: '设置群公告', mutation: true, fields: [group, f('notice', '公告', true, 'text')] },
  { name: 'groups/mute', title: '设置全群禁言', mutation: true, fields: [group, f('muted', '启用全群禁言', false, 'boolean')] },
  { name: 'groups/members/add', title: '邀请群成员', mutation: true, fields: [group, f('invitedUserIDs', '受邀用户 ID', true, 'list')] },
  { name: 'groups/members/remove', title: '移除群成员', mutation: true, destructive: true, fields: [group, f('kickedUserIDs', '移除用户 ID', true, 'list')] },
  { name: 'groups/members/mute', title: '设置成员禁言', mutation: true, fields: [group, user, { ...f('mutedSeconds', '禁言秒数（0 为解除）', true, 'number'), initial: 600 }] },
  { name: 'groups/members/role', title: '设置成员角色', mutation: true, fields: [group, user, { ...f('roleLevel', '角色（20 普通成员／60 管理员）', true, 'number'), initial: 20 }] },
  { name: 'groups/members/nickname', title: '设置群昵称', mutation: true, fields: [group, user, f('nickname', '群昵称', true)] },
  { name: 'groups/owner', title: '转让群主', mutation: true, fields: [group, f('oldOwnerUserID', '当前群主 ID', true), f('newOwnerUserID', '新群主 ID', true)] },
  { name: 'groups/dismiss', title: '解散群', mutation: true, destructive: true, fields: [group] },
  { name: 'messages/search', title: '查询聊天记录', fields: [f('sendID', '发送者 ID'), f('recvID', '接收者 ID'), f('groupID', '群 ID'), f('keyword', '关键词'), ...period], paged: true },
  { name: 'messages/revoke', title: '撤回消息', mutation: true, destructive: true, fields: [f('conversationID', '会话 ID', true), f('seq', '消息序号', true, 'number'), user] },
  { name: 'messages/notification', title: '发送通知', mutation: true, fields: [f('sendID', '发送者 ID', true), f('recvID', '接收者 ID'), f('groupID', '群 ID'), f('sendAll', '发送给所有用户', false, 'boolean'), f('content', '通知内容', true, 'text')] },
  { name: 'users/online', title: '查询在线状态', fields: [f('userIDs', '用户 ID', true, 'list')] },
  { name: 'users/kick', title: '强制下线', mutation: true, destructive: true, fields: [user, { ...f('platformID', '平台 ID（0 为所有平台）', true, 'number'), initial: 0 }] },
  { name: 'friends/search', title: '查询好友关系', fields: [user], paged: true },
  { name: 'friends/add', title: '添加好友关系', mutation: true, fields: [f('ownerUserID', '用户 ID', true), f('friendUserID', '好友用户 ID', true)] },
  { name: 'friends/remove', title: '删除好友关系', mutation: true, destructive: true, fields: [f('ownerUserID', '用户 ID', true), f('friendUserID', '好友用户 ID', true)] },
  { name: 'rtc/status', title: '查看通话服务状态', fields: [] },
  { name: 'rtc/records/search', title: '查询通话与会议记录', fields: [f('userID', '用户 ID'), ...period], paged: true },
  { name: 'rtc/records/delete', title: '删除通话记录', mutation: true, destructive: true, fields: [f('recordIDs', '记录 ID', true, 'list')] },
  { name: 'versions/search', title: '查询客户端版本', fields: [f('platform', '平台（windows／android／ios／web）')], paged: true },
  ...(['create', 'update'] as const).map(action => ({ name: `versions/${action}`, title: action === 'create' ? '登记客户端版本' : '修改客户端版本', mutation: true, fields: [...(action === 'update' ? [f('id', '版本记录 ID', true)] : []), f('platform', '平台', true), f('version', '版本号', true), f('downloadURL', '下载地址', true), f('build', '构建号', true, 'number'), f('size', '安装包字节数', true, 'number'), f('sha256', '安装包 SHA-256（64位）', true), f('force', '强制更新', false, 'boolean'), f('description', '更新说明', false, 'text')] })),
  { name: 'versions/delete', title: '删除版本登记', mutation: true, destructive: true, fields: [f('id', '版本记录 ID', true)] },
  { name: 'discovery/list', title: '查询发现页', fields: [] },
  { name: 'discovery/save', title: '保存发现页入口', mutation: true, fields: [f('id', '入口 ID'), f('title', '标题', true), f('url', '链接', true), f('icon', '图标地址'), f('enabled', '启用', false, 'boolean'), f('sort', '排序', false, 'number')] },
  { name: 'config/get', title: '查看客户端配置', fields: [] },
  { name: 'config/update', title: '修改客户端配置', mutation: true, fields: [f('webTitle', 'Web 标题'), f('appName', '应用名称'), f('logoURL', '应用 Logo 地址'), f('privacyURL', '隐私政策地址'), f('userAgreementURL', '用户协议地址'), f('allowAddFriend', '允许添加好友', false, 'boolean'), f('discoverEnabled', '启用发现页', false, 'boolean')] },
];
const activeSection = ref('groups');
const selectedOperation = ref('groups/search');
const capabilities = ref<CommunicationCapabilities>();
const capacity=ref<StorageCapacity>();const capacityLoading=ref(false);const capacityError=ref('');let capacityEpoch=0;
function capacityBytes(v:number|null){if(v===null||!Number.isFinite(v)||v<0)return '未知';const units=['B','KiB','MiB','GiB','TiB'];let n=v,i=0;while(n>=1024&&i<units.length-1){n/=1024;i++;}return `${n.toFixed(i?1:0)} ${units[i]}`;}
async function loadCapacity(){const run=++capacityEpoch;capacityLoading.value=true;capacityError.value='';try{const value=await communicationOperation('storage/capacity',{});if(run!==capacityEpoch||!canManage.value)return;const top=object(value);const raw=Array.isArray(top.filesystems)?top:object(top.data);if(!Array.isArray(raw.filesystems)||typeof raw.dataCapacity!=='boolean')throw new Error('Invalid capacity');capacity.value=raw as unknown as StorageCapacity;}catch{if(run===capacityEpoch)capacityError.value='容量状态暂不可用，请稍后刷新';}finally{if(run===capacityEpoch)capacityLoading.value=false;}}
const canManage = ref(false);
const launching = ref(false);
const initializing = ref(false);
const loading = ref(false);
const initialError = ref('');
const resultError = ref('');
const form = reactive<Record<string, FormValue>>({});
const rows = ref<Row[]>([]);
const filters = reactive<ResultFilters>({ keyword: '', department: '', enabled: '', status: '' });
const queried = ref(false);
const page = ref(1);
const pageSize = 20;
const total = ref<number | null>(null);
const selectedRecord = ref<Record<string, unknown>>({});
const previewed = ref(false);
const configReady = ref(false);
const configBaseline = ref<Record<string, FormValue>>({});
let requestEpoch = 0;
let initializationEpoch = 0;
const selectedSection = ref('');
const availableOperations = computed(() => operations.filter(op => op.name.startsWith(`${activeSection.value}/`) && capabilities.value?.operations.includes(op.name)));
const currentOperation = computed(() => availableOperations.value.find(op => op.name === selectedOperation.value));
const names: Record<string, string> = { groupID: '群 ID', groupName: '群名称', ownerUserID: '群主', memberCount: '成员数', userID: '用户 ID', nickname: '昵称', notification: '公告', introduction: '介绍', roleLevel: '群角色', sendID: '发送者', recvID: '接收者', content: '内容', contentType: '消息类型', conversationID: '会话 ID', seq: '消息序号', sendTime: '发送时间', status: '状态', platform: '平台', platformID: '平台 ID', version: '版本', downloadURL: '下载地址', id: '记录 ID', createdAt: '创建时间', title: '标题', url: '链接', enabled: '启用', sort: '排序', account: '工号', employeeNo: '工号', name: '姓名', departmentName: '部门', reason: '待处理原因', willEnable: '可开通', error: '同步异常', desiredVersion: '目标版本', appliedVersion: '已同步版本', build: '构建号', size: '包大小', sha256: 'SHA-256', description: '更新说明', text: '更新说明', force: '强制更新', webTitle: 'Web标题', appName: '应用名称', allowAddFriend: '允许加好友', discoverEnabled: '启用发现页' };
const isEmployeeTable = computed(() => activeSection.value === 'employees' && ['employees/list', 'employees/preview'].includes(selectedOperation.value));
const isEmployeePreview = computed(() => selectedOperation.value === 'employees/preview');
const employeeStatusLabels = computed(() => ({ ready: '已同步', pending: isEmployeePreview.value ? '待开通' : '待同步', waiting: isEmployeePreview.value ? '待处理' : '待开通', failed: '同步失败' }));
const departments = computed(() => employeeDepartments(rows.value));
const hasResultFilters = computed(() => Boolean(filters.keyword.trim() || filters.department || filters.enabled || filters.status));
const filteredRows = computed(() => filterRecords(rows.value, filters, isEmployeeTable.value));
const selectedRecordLabel = computed(() => selectedSection.value === activeSection.value && Object.keys(selectedRecord.value).length ? display(selectedRecord.value.groupName || selectedRecord.value.nickname || selectedRecord.value.name || selectedRecord.value.title || selectedRecord.value.version || selectedRecord.value.userID || selectedRecord.value.groupID || selectedRecord.value.id) : '');
const employeeColumns: TableColumnData[] = [
  { title: '姓名', dataIndex: 'employeeName', width: 140 },
  { title: '工号', dataIndex: 'employeeNo', width: 120 },
  { title: '部门', dataIndex: 'employeeDepartment', width: 160 },
  { title: '启用状态', dataIndex: 'employeeEnabled', width: 110 },
  { title: '同步状态', dataIndex: 'employeeSyncStatus', width: 110 },
  { title: '待处理原因', dataIndex: 'employeeReason', width: 320 },
  { title: '已同步版本', dataIndex: 'appliedVersion', width: 110 },
  { title: '目标版本', dataIndex: 'desiredVersion', width: 110 },
].map(column => ({ ...column, slotName: 'cell', tooltip: true }));
const columns = computed<TableColumnData[]>(() => [...(isEmployeeTable.value ? employeeColumns : Array.from(new Set(rows.value.flatMap(row => Object.keys(row)))).filter(key => key !== '__key').slice(0, 14).map(key => ({ title: names[key] || key, dataIndex: key, slotName: 'cell', ellipsis: true, tooltip: true, width: key === 'content' ? 320 : 180 }))), ...(filteredRows.value.some(row => canUseRecord(row) || canRemoveDiscovery(row)) ? [{ title: '操作', slotName: 'use', width: 230, fixed: 'right' as const }] : [])]);
function employeeCell(row: Row, key: string): string {
  const values = employeeFilterValues(row);
  if (key === 'employeeName') return values.name || '—';
  if (key === 'employeeNo') return values.account || '未填写';
  if (key === 'employeeDepartment') return values.department || '—';
  if (key === 'employeeEnabled') {
    if (typeof row.enabled !== 'boolean' && typeof row.willEnable === 'boolean') return row.willEnable ? '可开通' : '暂不可开通';
    return values.enabled === 'true' ? '已启用' : values.enabled === 'false' ? '未启用' : '—';
  }
  if (key === 'employeeSyncStatus') {
    return employeeStatusLabels.value[values.status as keyof typeof employeeStatusLabels.value] || display(row.status);
  }
  if (key === 'employeeReason') return values.reason || '—';
  return display(row[key]);
}
function display(value: unknown): string {
  if (value == null) return '—';
  if (typeof value === 'boolean') return value ? '是' : '否';
  if (Array.isArray(value)) return value.map(display).join('、');
  if (typeof value === 'object') {
    const obj = value as Record<string, unknown>;
    return String(obj.nickname || obj.groupName || obj.text || obj.content || obj.userID || obj.id || Object.entries(obj).map(([key, item]) => `${names[key] || key}：${display(item)}`).join('；'));
  }
  return String(value);
}
function resetForm() {
  Object.keys(form).forEach(key => delete form[key]);
  currentOperation.value?.fields.forEach(field => { form[field.key] = field.initial ?? (field.kind === 'boolean' ? false : field.kind === 'number' ? undefined : ''); });
  if (selectedOperation.value === 'discovery/save') form.enabled = true;
  if (selectedSection.value === activeSection.value) for (const field of currentOperation.value?.fields || []) if (selectedRecord.value[field.key] !== undefined) form[field.key] = formValue(field.key, selectedRecord.value[field.key]);
  page.value = 1;
}
function clearResultFilters() { Object.assign(filters, { keyword: '', department: '', enabled: '', status: '' }); }
function clearSelection() { selectedRecord.value = {}; selectedSection.value = ''; resetForm(); }
async function changeOperation() {
  const epoch = ++requestEpoch;
  loading.value = false; configReady.value = false;
  resetForm(); clearResultFilters(); resultError.value = ''; rows.value = []; queried.value = false; total.value = null;
  if (selectedOperation.value !== 'config/update') return;
  loading.value = true;
  try {
    const response = await communicationOperation('config/get', {});
    if (epoch !== requestEpoch) return;
    const saved = object(object(response).config || response);
    for (const field of currentOperation.value?.fields || []) if (saved[field.key] !== undefined) form[field.key] = formValue(field.key, saved[field.key]);
    configBaseline.value = { ...form };
    configReady.value = true;
  } catch (error) { if (epoch === requestEpoch) resultError.value = error instanceof Error ? error.message : '无法读取当前配置，请重试'; }
  finally { if (epoch === requestEpoch) loading.value = false; }
}
function changeSection() { selectedRecord.value = {}; selectedSection.value = ''; selectedOperation.value = availableOperations.value[0]?.name || ''; void changeOperation(); }
function extract(data: unknown): Row[] { const result = extractResponse(data); total.value = result.total; return result.rows; }
function payload() {
  const params: Record<string, unknown> = {};
  for (const field of currentOperation.value?.fields || []) {
    const value = form[field.key];
    if (selectedOperation.value === 'config/update') {
      if (value !== configBaseline.value[field.key]) params[field.key] = value ?? '';
      continue;
    }
    if (field.required && (value === '' || value === undefined || value === null)) throw new Error(`请填写${field.label}`);
    if (value === '' || value === undefined || value === null) continue;
    params[field.key] = field.kind === 'list' ? String(value).split(/[\s,，]+/).filter(Boolean) : value;
  }
  if (currentOperation.value?.paged) Object.assign(params, { pageNumber: page.value, showNumber: pageSize });
  if (selectedOperation.value === 'config/update' && !Object.keys(params).length) throw new Error('配置没有变化');
  if (selectedOperation.value === 'groups/create') params.groupType = 2;
  if (selectedOperation.value === 'employees/provision' && (!previewed.value || params.confirm !== true)) throw new Error('请先预览员工清单，再勾选确认开通');
  if (selectedOperation.value === 'messages/notification' && !params.sendAll && !params.recvID && !params.groupID) throw new Error('请指定接收用户、群或发送给所有用户');
  if (params.startTime && params.endTime && Number(params.startTime) > Number(params.endTime)) throw new Error('开始时间不能晚于结束时间');
  if (selectedOperation.value === 'versions/create' || selectedOperation.value === 'versions/update') {
    if (Number(params.build) <= 0 || Number(params.size) <= 0 || !/^[a-fA-F0-9]{64}$/.test(String(params.sha256))) throw new Error('请填写有效构建号、包大小及64位SHA-256');
    params.sha256 = String(params.sha256).toLowerCase();
  }
  for (const key of ['downloadURL', 'privacyURL', 'userAgreementURL']) validateHTTPS(params[key], names[key] || key);
  if (selectedOperation.value === 'discovery/save') validateHTTPS(params.url, '发现页链接');
  if (selectedOperation.value === 'discovery/save') return { items: [params] };
  return params;
}
async function execute(operation = selectedOperation.value, params = payload(), epoch = requestEpoch) {
  if (epoch !== requestEpoch || loading.value) return;
  loading.value = true; resultError.value = ''; total.value = null;
  const mutation = currentOperation.value?.mutation;
  try {
    if (operation === 'discovery/save') {
      const existing = extractResponse(await communicationOperation('discovery/list', {})).rows;
      if (epoch !== requestEpoch) return;
      params = { items: mergeDiscovery(existing, (params.items as Record<string, unknown>[])[0]) };
    }
    const response = await communicationOperation(operation, params);
    if (epoch !== requestEpoch) return;
    rows.value = extract(response); queried.value = true;
    if (operation === 'employees/preview') previewed.value = true;
    if (operation === 'employees/provision') previewed.value = false;
    if (operation === 'config/update') configBaseline.value = { ...form };
    if (mutation) Message.success('操作已完成');
  } catch (error) { if (epoch === requestEpoch) resultError.value = error instanceof Error ? error.message : '操作失败，请重试'; }
  finally { if (epoch === requestEpoch) loading.value = false; }
}
function submit() {
  if (loading.value) return;
  if (currentOperation.value?.paged && !currentOperation.value.mutation) page.value = 1;
  let params: Record<string, unknown>;
  try { params = payload(); } catch (error) { Message.warning(error instanceof Error ? error.message : '请检查输入'); return; }
  const operation = selectedOperation.value, epoch = requestEpoch;
  if (currentOperation.value?.mutation) Modal.confirm({ title: `确认${currentOperation.value.title}`, content: form.sendAll ? '此通知将发送给所有用户，请核对内容后确认。' : '请确认目标记录及填写内容，操作将立即生效。', okButtonProps: { status: currentOperation.value.destructive ? 'danger' : 'normal' }, onOk: () => execute(operation, params, epoch) });
  else void execute(operation, params, epoch);
}
function queryPage(value: number) {
  const previous = page.value;
  page.value = value;
  try { void execute(selectedOperation.value, payload(), requestEpoch); }
  catch (error) { page.value = previous; Message.warning(error instanceof Error ? error.message : '请检查查询条件'); }
}
function canUseRecord(row: Row): boolean {
  return availableOperations.value.some(operation => operation.fields.some(field => row[field.key] != null && formValue(field.key, row[field.key]) !== undefined));
}
function canRemoveDiscovery(row: Row): boolean { return activeSection.value === 'discovery' && Boolean(row.id) && availableOperations.value.some(operation => operation.name === 'discovery/save'); }
function useRecord(row: Row) {
  if (!canUseRecord(row) || loading.value) return;
  selectedRecord.value = { ...row }; selectedSection.value = activeSection.value;
  resetForm();
  for (const field of currentOperation.value?.fields || []) if (row[field.key] !== undefined) form[field.key] = formValue(field.key, row[field.key]);
  if (row.groupID) form.groupID = String(row.groupID);
  if (row.userID) form.userID = String(row.userID);
  Message.info('已填入操作表单，可切换本模块操作；提交并确认后才会修改数据');
}
function removeDiscovery(row: Row) {
  const epoch = requestEpoch;
  Modal.confirm({ title: `删除发现页入口“${String(row.title || row.id)}”`, content: '只删除当前入口，其他条目保留。', okButtonProps: { status: 'danger' }, onOk: async () => {
    if (epoch !== requestEpoch || loading.value) return;
    loading.value = true;
    try {
      const existing = extractResponse(await communicationOperation('discovery/list', {})).rows;
      if (epoch !== requestEpoch) return;
      const items = existing.filter(item => item.id !== row.id).map(({ __key, ...item }) => item);
      const response = await communicationOperation('discovery/save', { items });
      if (epoch !== requestEpoch) return;
      rows.value = extract(response); Message.success('入口已删除');
    } catch (error) { if (epoch === requestEpoch) resultError.value = error instanceof Error ? error.message : '删除失败，请重试'; }
    finally { if (epoch === requestEpoch) loading.value = false; }
  }});
}
async function initialize() {
  const epoch = ++initializationEpoch;
  ++requestEpoch;
	++capacityEpoch;capacity.value=undefined;capacityError.value='';
  initializing.value = true; initialError.value = ''; canManage.value = false;
  try {
    const access = await getCommunicationAccess();
    if (epoch !== initializationEpoch) return;
    canManage.value = access.permissions.some(permission => ['biz-suite.im.manage', 'biz-suite.im.*', '*'].includes(permission));
    if (!canManage.value) { capabilities.value = undefined; rows.value = []; return; }
    const response = await getCommunicationCapabilities();
    if (epoch !== initializationEpoch) return;
    capabilities.value = { ...response, operations: [...response.operations, 'employees/list', 'employees/preview', 'employees/provision', 'employees/retry'] };
    if(response.operations.includes('storage/capacity'))void loadCapacity();
    if (!availableOperations.value.some(op => op.name === selectedOperation.value)) selectedOperation.value = availableOperations.value[0]?.name || '';
    void changeOperation();
  } catch (error) { if (epoch === initializationEpoch) initialError.value = error instanceof Error ? error.message : '暂时无法读取管理权限'; }
  finally { if (epoch === initializationEpoch) initializing.value = false; }
}
async function launch() {
  launching.value = true;
  try {
    const result = await getCommunicationTicket();
    const url = new URL(result.url);
    if (!['https://192.168.10.11:9885', 'https://im.szputy.com:9885'].includes(url.origin)) throw new Error('通讯入口地址无效');
    window.location.assign(url.href);
  } catch (error) { Message.error(error instanceof Error ? error.message : '暂时无法进入通讯'); }
  finally { launching.value = false; }
}
let favicon: HTMLLinkElement | null = null;
let originalFavicon = '';
let addedFavicon = false;
onMounted(() => {
  favicon = document.querySelector<HTMLLinkElement>('link[rel="icon"]');
  if (!favicon) { favicon = document.createElement('link'); favicon.rel = 'icon'; document.head.appendChild(favicon); addedFavicon = true; }
  originalFavicon = favicon.href;
  favicon.type = 'image/png';
  favicon.href = `${import.meta.env.BASE_URL}brand/im/favicon-32x32.png?v=puty3`;
  initialize();
});
onBeforeUnmount(() => { ++requestEpoch; ++initializationEpoch; ++capacityEpoch; if (addedFavicon) favicon?.remove(); else if (favicon) favicon.href = originalFavicon; });
</script>

<style scoped>
.communication-page { padding: 16px; }
.platform-title { display: inline-flex; align-items: center; gap: 10px; }
.platform-title img { width: 40px; height: 40px; object-fit: contain; }
.action-bar { margin-bottom: 20px; }
.form-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(230px, 1fr)); gap: 0 20px; }
.results { margin-top: 16px; }
.notice { margin-bottom: 16px; }
.capacity-grid { display:grid;grid-template-columns:repeat(auto-fit,minmax(240px,1fr));gap:12px;margin-top:12px; }
.pager { margin-top: 16px; }
.cell-value { white-space: pre-wrap; overflow-wrap: anywhere; }
.result-filters { display: flex; align-items: center; flex-wrap: wrap; gap: 12px; margin-bottom: 16px; }
.filter-keyword { width: 300px; max-width: 100%; }
.filter-select { width: 180px; max-width: 100%; }
.filter-summary { color: var(--color-text-3); }
@media (max-width: 600px) { .filter-keyword, .filter-select { width: 100%; } }
</style>
