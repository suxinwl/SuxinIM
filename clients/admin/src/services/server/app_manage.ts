import { ACCOUNT_URL } from '@/config';
import { request } from '@umijs/max';

// new
export async function getClientConfig() {
  return request<{
    data: any;
  }>('/client_config/get', {
    method: 'POST',
    data: {},
    headers: {
      isAccount: true,
    },
    baseURL: ACCOUNT_URL,
  });
}

// new
export async function setClientConfig(data: any) {
  return request<{
    data: any;
  }>('/client_config/set', {
    method: 'POST',
    data: {
      config: {
        ...data,
      },
    },
    headers: {
      isAccount: true,
    },
    baseURL: ACCOUNT_URL,
  });
}

// new
export async function getApplet(data: any) {
  return request<{
    data: any;
  }>('/applet/search', {
    method: 'POST',
    data,
    headers: {
      isAccount: true,
    },
    baseURL: ACCOUNT_URL,
  });
}

// new
export async function crteateApplet(data: any) {
  return request<{
    data: any;
  }>('/applet/add', {
    method: 'POST',
    data,
    headers: {
      isAccount: true,
    },
    baseURL: ACCOUNT_URL,
  });
}
