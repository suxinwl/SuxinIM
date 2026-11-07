export interface RequestOptions {
  method?: string;
  body?: unknown;
}

type HostAITransport = (path: string, options: RequestOptions) => Promise<unknown>;
let hostAITransport: HostAITransport | undefined;

// The built-in AI page supplies the host's authenticated HTTP transport.
export function configureHostAITransport(transport: HostAITransport): void {
  hostAITransport = transport;
}

export async function request<T>(path: string, options: RequestOptions = {}): Promise<T> {
  if (!path.startsWith('/ai/')) throw new Error('AI 接口地址无效');
  if (!hostAITransport) throw new Error('请从 GoSuxin 后台打开 AI / MCP 页面');
  return await hostAITransport('/developer/ai' + path.slice(3), options) as T;
}
