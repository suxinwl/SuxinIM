// Management requests are authenticated and proxied by enterprise-service.
export const WS_URL = `${location.protocol === 'https:' ? 'wss:' : 'ws:'}//${location.host}/ws/`;
export const API_URL = '/enterprise/v1/admin/im/';
export const CHAT_URL = '/enterprise/v1/admin/business/';
export const ACCOUNT_URL = '/enterprise/v1/admin/chat/';
export const ENTERPRISE_URL = '/enterprise/v1/';
export const OBJECT_STORAGE = 'minio';
