<template>
  <div class="host-ai-page">
    <header><h2>AI / MCP 开发助手</h2><p>模型接口、字段设计、代码审查与开发工具连接</p></header>
    <AI />
  </div>
</template>

<script setup lang="ts">
import AI from '@/views/devtools/ai/index.vue';
import { configureHostAITransport } from '@/api/devtools/client';
import { defHttp } from '@/utils/http';

configureHostAITransport((path, options) => {
  const method = (options.method || 'GET').toUpperCase();
  const config = { url: path, params: options.body, timeout: path.endsWith('/generate') ? 100000 : 15000 };
  if (method === 'GET') return defHttp.get<unknown>(config, { errorMessageMode: 'none' });
  if (method === 'POST') return defHttp.post<unknown>(config, { errorMessageMode: 'none' });
  return Promise.reject(new Error('AI 接口请求方式无效'));
});
</script>

<style scoped>
.host-ai-page{padding:20px;color:var(--color-text-1)}.host-ai-page header{margin-bottom:20px}.host-ai-page header h2{margin:0}.host-ai-page header p{color:var(--color-text-3)}
</style>
