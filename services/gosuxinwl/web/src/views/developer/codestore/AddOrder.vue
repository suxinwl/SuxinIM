<template>
  <a-modal v-model:visible="visible" title="Suxin 插件购买" :footer="false">
    <a-result status="info" title="购买服务筹备中" subtitle="将迁移至 Suxin 官网">
      <template #extra><a-link href="https://www.suxinwl.com" target="_blank" rel="noopener noreferrer">访问 Suxin 官网</a-link></template>
    </a-result>
  </a-modal>
</template>
<script setup lang="ts">
import { ref } from 'vue';
const visible = ref(false);
const showModel = (..._args: unknown[]) => { visible.value = true; };
defineExpose({ showModel });
</script>
