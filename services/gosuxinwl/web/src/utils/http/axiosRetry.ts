import type { AxiosError, AxiosInstance, InternalAxiosRequestConfig } from 'axios';
type RetryConfig = InternalAxiosRequestConfig & { __retryCount?: number };
export function shouldRetryRequest(error: AxiosError): boolean {
  const config = error.config as RetryConfig | undefined;
  const options = config?.requestOptions?.retryRequest;
  return !!config && config.method?.toUpperCase() === 'GET' && options?.isOpenRetry === true &&
    error.code !== 'ERR_CANCELED' && config.signal?.aborted !== true &&
    (!error.response || error.response.status >= 500) && (config.__retryCount || 0) < options.count;
}
/** Retry idempotent requests using the original request config, including network failures. */
export class AxiosRetry {
  retry(instance: AxiosInstance, error: AxiosError) {
    const config = error.config as RetryConfig | undefined;
    const options = config?.requestOptions?.retryRequest;
    if (!config || !options?.isOpenRetry) return Promise.reject(error);
    config.__retryCount ??= 0;
    if (config.__retryCount >= options.count) return Promise.reject(error);
    config.__retryCount += 1;
    return new Promise((resolve) => setTimeout(resolve, options.waitTime)).then(() => instance.request(config));
  }
}
