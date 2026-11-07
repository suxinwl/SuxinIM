<template>
  <section class="runtime-plugin-page" :class="{ 'runtime-plugin-fullscreen': route.meta.onlypage }">
    <a-spin v-if="loading" />
    <a-alert v-else-if="error" type="error">{{ error }}<a-button size="small" @click="openAdmin">重试</a-button></a-alert>
    <iframe v-else-if="src" :src="src" :title="String(route.meta.title || '插件管理')" />
  </section>
</template>

<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue';
import { useRoute } from 'vue-router';
import { bootstrapRuntimePluginAdmin } from '@/utils/runtime-plugin';
import { runtimePluginAdminTarget, runtimePluginOpensStandalone } from '@/utils/runtime-plugin-route';

const route = useRoute();
const loading = ref(false);
const error = ref('');
const src = ref('');
let generation = 0;

async function openAdmin() {
  const current = ++generation;
  const target = runtimePluginAdminTarget(route.path, route.meta.runtimePlugin, route.query);
  src.value = '';
  error.value = '';
  if (!target) { error.value = '插件管理地址无效'; return; }
  loading.value = true;
  try {
    await bootstrapRuntimePluginAdmin(target);
    if (current !== generation) return;
    if (runtimePluginOpensStandalone(target)) {
      window.location.replace(target);
    } else {
      src.value = target;
    }
  } catch (cause) {
    if (current === generation) error.value = cause instanceof Error ? cause.message : '无法打开插件管理，请确认登录和角色权限';
  } finally {
    if (current === generation) loading.value = false;
  }
}
watch(() => [route.fullPath, route.meta.runtimePlugin], openAdmin, { immediate: true });
onBeforeUnmount(() => { generation += 1; });
</script>

<style scoped>
.runtime-plugin-page { display: flex; flex: 1 1 0; min-width: 0; min-height: 0; overflow: hidden; }
.runtime-plugin-fullscreen { width: 100%; height: 100vh; height: 100dvh; min-height: 0; padding: 0; box-sizing: border-box; overflow: hidden; }
iframe { display: block; width: 100%; height: 100%; min-width: 0; min-height: 0; border: 0; }
</style>
