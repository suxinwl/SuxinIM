import type { Row } from './model';

export type ResultFilters = { keyword: string; department: string; enabled: string; status: string };
export type EmployeeFilterValues = {
  name: string;
  account: string;
  department: string;
  enabled: 'true' | 'false' | '';
  status: string;
  reason: string;
};

const statusLabels = new Map([
  ['ready', '已同步'], ['pending', '待同步'], ['waiting', '待开通'], ['failed', '同步失败'],
]);
const hiddenKeys = new Set(['authfingerprint', 'password', 'salt', 'token']);

function record(value: unknown): Record<string, unknown> {
  return value !== null && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : {};
}

function text(...values: unknown[]): string {
  for (const value of values) {
    if (typeof value === 'string' && value.trim()) return value.trim();
    if (typeof value === 'number' && Number.isFinite(value)) return String(value);
  }
  return '';
}

function employeeSource(row: Row): Record<string, unknown> {
  // Normally extractResponse has already flattened projection. Keeping the
  // same merge order also supports a result supplied before that step.
  return { ...row, ...record(row.projection) };
}

export function employeeFilterValues(row: Row): EmployeeFilterValues {
  const source = employeeSource(row);
  const profile = record(source.profile);
  const account = text(source.employeeNo, source.account, profile.account, profile.employeeNo);
  const enabled = typeof source.enabled === 'boolean' ? String(source.enabled) as 'true' | 'false'
    : typeof source.willEnable === 'boolean' ? String(source.willEnable) as 'true' | 'false' : '';
  const explicitStatus = text(source.status);
  const status = explicitStatus || (typeof source.willEnable === 'boolean' ? source.willEnable ? 'pending' : 'waiting' : '');
  const reasons = [source.reason, source.error].filter((value): value is string => typeof value === 'string' && !!value.trim()).map(value => value.trim());
  return {
    name: text(source.name, source.nickname, profile.name, profile.nickname),
    account,
    department: text(source.departmentName, profile.departmentName),
    enabled,
    status,
    reason: [...new Set(reasons)].join('；') || (account ? '' : '缺少工号'),
  };
}

function employeeText(row: Row, values: EmployeeFilterValues): string[] {
  const source = employeeSource(row);
  const preview = typeof source.enabled !== 'boolean' && typeof source.willEnable === 'boolean';
  const enabled = values.enabled === '' ? '' : preview
    ? values.enabled === 'true' ? '可开通' : '暂不可开通'
    : values.enabled === 'true' ? '已启用' : '未启用';
  const previewStatus = typeof source.willEnable === 'boolean';
  const status = previewStatus && values.status === 'pending' ? '待开通'
    : previewStatus && values.status === 'waiting' ? '待处理' : statusLabels.get(values.status) || values.status;
  return [values.name, values.account || '未填写', values.department, values.reason, enabled, status];
}

function visibleText(value: unknown, seen = new Set<object>()): string[] {
  if (typeof value === 'string') return [value];
  if (typeof value === 'number' && Number.isFinite(value)) return [String(value)];
  if (typeof value === 'boolean') return [value ? '是' : '否'];
  if (value === null || typeof value !== 'object' || seen.has(value)) return [];
  seen.add(value);
  if (Array.isArray(value)) return value.flatMap(item => visibleText(item, seen));
  return Object.entries(value).flatMap(([key, item]) => {
    const normalized = key.replace(/[_-]/g, '').toLowerCase();
    if (key.toLowerCase() === '__key' || hiddenKeys.has(normalized) || normalized.endsWith('token')) return [];
    return visibleText(item, seen);
  });
}

export function filterRecords(rows: Row[], filters: ResultFilters, employees: boolean): Row[] {
  const keyword = filters.keyword.trim().toLowerCase();
  const department = filters.department.trim();
  const enabled = filters.enabled.trim();
  const status = filters.status.trim();
  const matches = (values: string[]) => !keyword || values.some(value => value.toLowerCase().includes(keyword));
  return rows.filter(row => {
    if (!employees) return matches(visibleText(row));
    const values = employeeFilterValues(row);
    return (!department || values.department === department)
      && (!enabled || values.enabled === enabled)
      && (!status || values.status === status)
      && matches(employeeText(row, values));
  });
}

export function employeeDepartments(rows: Row[]): string[] {
  return [...new Set(rows.map(row => employeeFilterValues(row).department).filter(Boolean))]
    .sort((left, right) => left.localeCompare(right, 'zh-CN'));
}
