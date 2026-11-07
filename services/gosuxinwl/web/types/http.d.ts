import 'axios';
import type { RequestOptions } from './axios';
declare module 'axios' {
  interface AxiosRequestConfig { requestOptions?: RequestOptions; }
}
