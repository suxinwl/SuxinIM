import { request } from './client';
export interface AIConfig { endpoint: string; model: string; allowPrivate: boolean; keyConfigured: boolean; }
export interface DeveloperToken { id: string; name: string; scopes: string[]; createdAt: string; expiresAt: string; revoked: boolean; }
export const getConfig = () => request<AIConfig>('/ai/config');
export const saveConfig = (value: { endpoint: string; model: string; apiKey: string; allowPrivate: boolean; clearKey: boolean }) => request<void>('/ai/config', { method: 'POST', body: value });
export const generate = (mode: string, prompt: string) => request<{ text: string; model: string }>('/ai/generate', { method: 'POST', body: { mode, prompt } });
export const tokens = () => request<DeveloperToken[]>('/ai/tokens');
export const createToken = (name: string, scopes: string[], days: number) => request<{ token: string; metadata: DeveloperToken }>('/ai/tokens', { method: 'POST', body: { name, scopes, days } });
export const revokeToken = (id: string) => request<void>('/ai/tokens/revoke', { method: 'POST', body: { id } });
