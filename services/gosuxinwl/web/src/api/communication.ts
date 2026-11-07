import { defHttp } from '@/utils/http';

export interface CommunicationCapabilities {
  operations: string[];
  version: string;
  endpoints?: { hrm: string; im: string };
}
export interface StorageCapacity {
  dataCapacity: boolean; status: 'unknown'|'ok'|'warning'|'critical'; measuredAt: number;
  filesystems: { kind: string; path: string; fsType: string; measured: boolean; dataCapacity: boolean; usage_bytes: number|null; available_bytes: number|null; total_bytes: number|null; usageRatio: number|null; status: string }[];
}

const base = '/plugins/biz-suite/api/admin/im/manager/openim';
const options = { joinPrefix: false, isRootUrl: false, isTransformResponse: false, errorMessageMode: 'message' as const };
interface Envelope<T> { code: number; data: T; msg?: string; message?: string }
function unwrap<T>(response: Envelope<T>): T {
  if (response.code !== 0) throw new Error(response.msg || response.message || '通讯操作失败，请重试');
  return response.data;
}

export async function getCommunicationCapabilities() {
  return unwrap(await defHttp.get<Envelope<CommunicationCapabilities>>({ url: `${base}/capabilities` }, options));
}

export async function communicationOperation(operation: string, params: Record<string, unknown>) {
  // The caller selects a declared operation; server-side capabilities and RBAC
  // remain authoritative. This never accepts a destination URL or admin token.
  const employeePaths: Record<string, string> = { 'employees/list': 'employees', 'employees/preview': 'preview', 'employees/provision': 'provision', 'employees/retry': 'retry' };
  const path = employeePaths[operation];
  if (path) {
    const response = operation === 'employees/list' || operation === 'employees/preview'
      ? await defHttp.get<Envelope<unknown>>({ url: `${base}/${path}` }, options)
      : await defHttp.post<Envelope<unknown>>({ url: `${base}/${path}`, params }, options);
    return unwrap(response);
  }
  return unwrap(await defHttp.post<Envelope<unknown>>({ url: `${base}/operations/${operation}`, params }, options));
}

export async function getCommunicationAccess() {
  return unwrap(await defHttp.get<Envelope<{ permissions: string[] }>>({ url: '/plugins/biz-suite/api/admin/access' }, options));
}

export async function getCommunicationTicket() {
  return unwrap(await defHttp.post<Envelope<{ url: string }>>({ url: '/plugins/biz-suite/api/admin/im/openim/ticket', params: {} }, options));
}
