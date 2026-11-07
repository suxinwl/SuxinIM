import { defHttp } from '@/utils/http';
import md5 from 'md5';

export const zones = ['lan', 'internet'] as const;
export const clients = ['web_desktop', 'web_mobile', 'pc', 'android', 'ios'] as const;
export type Zone = typeof zones[number];
export type LogicalClient = typeof clients[number];
export type Choice = 'inherit' | 'allow' | 'deny';
export type PolicyMatrix = Record<Zone, Record<LogicalClient, { login: Choice; save: Choice }>>;
export interface EffectivePolicy { zone: Zone; logicalClient: LogicalClient; login: boolean; preview: boolean; save: boolean; sources: { login: string; save: string }; currentIP: string; policyRevision: number; networkRevision: number; securityVersion: number }
export interface Policy { subject: 'person' | 'role'; subjectID: number; matrix: PolicyMatrix; revision: number; effective?: Record<Zone, Record<LogicalClient, EffectivePolicy>> }
export interface SecurityState { uid: number; mustChangePassword: boolean; locked: boolean; lockedUntil: number; securityVersion: number; recentFailures: number }
export interface LockPolicy { windowSeconds: number; failures: number; lockSeconds: number; revision: number }
export interface ChangeChallenge { reasonCode: 'PASSWORD_CHANGE_REQUIRED'; requiresPasswordChange: true; changeTicket: string; expiresAt: number }
const options = { errorMessageMode: 'message' as const };
const base = '/system/account';
export const getSecurity = (id: number) => defHttp.get<SecurityState>({ url: `${base}/security`, params: { id } }, options);
export const resetPassword = (id: number, password: string) => defHttp.post({ url: `${base}/resetPassword`, params: { id, password: md5(password) } }, options);
export const unlockAccount = (id: number) => defHttp.post({ url: `${base}/unlock`, params: { id } }, options);
export const getLockPolicy = () => defHttp.get<LockPolicy>({ url: `${base}/lockPolicy` }, options);
export const saveLockPolicy = (params: LockPolicy) => defHttp.post<LockPolicy>({ url: `${base}/lockPolicy`, params }, options);
export const getPolicy = (subject: Policy['subject'], id: number) => defHttp.get<Policy>({ url: `/system/${subject === 'role' ? 'role' : 'account'}/accessPolicy`, params: { id } }, options);
export const savePolicy = (subject: Policy['subject'], id: number, policy: Policy) => defHttp.post<Policy>({ url: `/system/${subject === 'role' ? 'role' : 'account'}/accessPolicy`, params: { id, matrix: policy.matrix, revision: policy.revision } }, options);
export const getEffectivePolicy = () => defHttp.get<EffectivePolicy>({ url: '/user/effectivePolicy' }, options);

// Challenge tickets are held in memory only, never in URLs or browser storage.
export async function changeRequiredPassword(changeTicket: string, newPassword: string): Promise<void> {
  const response = await fetch('/common/im-sso/password/change', { method: 'POST', credentials: 'omit', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ changeTicket, newPassword: md5(newPassword) }) });
  const result = await response.json();
  if (!response.ok || result.code !== 0) throw new Error('密码修改未通过，请重新登录后重试');
}
export async function cancelPasswordChange(changeTicket: string): Promise<void> {
  await fetch('/common/im-sso/password/cancel', { method: 'POST', credentials: 'omit', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ changeTicket }) }).catch(() => undefined);
}
