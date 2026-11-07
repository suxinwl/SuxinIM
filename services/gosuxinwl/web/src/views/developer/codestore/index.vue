<template>
  <div class="container">
    <page-card breadcrumb scrollPage>
      <div class="market-header">
        <div>
          <h2>本地插件</h2>
          <p>运行包支持直接安装、更新及卸载，主服务持续运行；已有内容和文件保留。</p>
        </div>
        <a-space wrap>
          <a-button :loading="loading" :disabled="!!busyName" @click="refreshMarket">刷新</a-button>
          <a-button :disabled="!!busyName" @click="openPackModal(true, { record: { domian: '', code_token: '', tabwarehouse: 'local' } })">本地代码打包</a-button>
          <a-upload accept=".zip" :disabled="!!busyName" :show-file-list="false" :custom-request="handleInstallLocalCode">
            <template #upload-button><a-button type="primary">安装插件包</a-button></template>
          </a-upload>
        </a-space>
      </div>
      <a-tabs v-model:active-key="activeTab">
        <a-tab-pane key="all" :title="'全部插件（' + plugins.length + '）'" />
        <a-tab-pane key="installed" :title="'已安装（' + installedCount + '）'" />
        <a-tab-pane key="available" title="未安装" />
      </a-tabs>
      <a-alert v-if="loadError" type="error" class="market-error">插件列表加载失败，请点击刷新重试。</a-alert>
      <a-table row-key="name" :data="visiblePlugins" :loading="loading" :pagination="false" :scroll="{ x: 850 }">
        <template #columns>
          <a-table-column title="插件" :width="160">
            <template #cell="{ record }"><strong>{{ record.title }}</strong></template>
          </a-table-column>
          <a-table-column title="版本" data-index="version" :width="95" />
          <a-table-column title="功能" data-index="description" />
          <a-table-column title="状态" :width="110">
            <template #cell="{ record }"><a-tag :color="record.error ? 'red' : record.installed ? 'green' : 'gray'">{{ record.error ? '运行异常' : record.installed ? '已安装' : '未安装' }}</a-tag><div v-if="record.runtime">{{ record.platform }}</div><small v-if="record.error">{{ record.error }}</small></template>
          </a-table-column>
          <a-table-column title="操作" :width="190">
            <template #cell="{ record }">
              <a-space>
                <a-button v-if="record.installed" type="text" :disabled="!!busyName" @click="router.push(record.entry)">进入后台</a-button>
                <a-button v-if="record.installed" type="text" status="danger" :loading="busyName === record.name" :disabled="!!busyName && busyName !== record.name" @click="confirmUninstall(record)">卸载</a-button>
                <a-button v-else type="primary" :loading="busyName === record.name" :disabled="!!busyName && busyName !== record.name" @click="changeInstallation(record, true)">安装</a-button>
              </a-space>
            </template>
          </a-table-column>
        </template>
      </a-table>
      <a-alert class="market-notice" type="info">在线插件市场与购买服务筹备中，本地插件可正常管理。</a-alert>
    </page-card>
    <PackUpCode @register="registerPackModal" />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import type { RequestOption } from '@arco-design/web-vue/es/upload/interfaces';
import { Message, Modal } from '@arco-design/web-vue';
import { useModal } from '/@/components/Modal';
import { getInstallPack, installLocalCode, installCode, uninstallCode } from '@/api/developer/packinstall';
import PackUpCode from './PackUpCode.vue';
import { useAppStore } from '@/store';

interface LocalPlugin {
  name: string;
  title: string;
  version: string;
  description: string;
  entry: string;
  installed: boolean;
  runtime?: boolean;
  active?: boolean;
  platform?: string;
  error?: string;
}

const router = useRouter();
const appStore = useAppStore();
const [registerPackModal, { openModal: openPackModal }] = useModal();
const plugins = ref<LocalPlugin[]>([]);
const loading = ref(false);
const loadError = ref(false);
const busyName = ref('');
const activeTab = ref('all');
const installedCount = computed(() => plugins.value.filter((plugin) => plugin.installed).length);
const visiblePlugins = computed(() => plugins.value.filter((plugin) => activeTab.value === 'all' || (activeTab.value === 'installed' ? plugin.installed : !plugin.installed)));

async function loadPlugins() {
  loading.value = true;
  loadError.value = false;
  try {
    plugins.value = await getInstallPack({ catalog: true });
  } catch {
    loadError.value = true;
  } finally {
    loading.value = false;
  }
}

async function refreshMarket() {
  const refreshed = await appStore.refreshServerRoutes(router);
  await loadPlugins();
  if (!refreshed) Message.warning('插件状态已更新，菜单刷新失败，请点击刷新重试。');
}

async function changeInstallation(plugin: LocalPlugin, installed: boolean) {
  if (busyName.value) return;
  busyName.value = plugin.name;
  try {
    await (installed ? installCode : uninstallCode)({ name: plugin.name });
    Message.success(installed ? plugin.title + '安装成功' : plugin.title + '已卸载，数据和文件已保留');
    await refreshMarket();
  } catch {
    // The shared HTTP handler already displays installation errors.
  } finally {
    busyName.value = '';
  }
}

function confirmUninstall(plugin: LocalPlugin) {
  Modal.confirm({
    title: '卸载' + plugin.title,
    content: '卸载后将关闭功能并移除菜单入口。已有内容、配置和文件会保留，重新安装即可恢复使用。',
    okText: '确认卸载',
    cancelText: '取消',
    onOk: () => changeInstallation(plugin, false),
  });
}

const handleInstallLocalCode = (options: RequestOption) => {
  let cancelled = false;
  (async () => {
    const { onProgress, onError, onSuccess, fileItem } = options;
    busyName.value = 'upload';
    Message.loading({ content: '上传插件包中', id: 'plugin-upload', duration: 0 });
    try {
      if (!fileItem.file) throw new Error('请选择插件包');
      const response = await installLocalCode({ name: 'file', file: fileItem.file as Blob, filename: fileItem.name || '', data: {} }, (event: ProgressEvent) => {
        if (event.total > 0) onProgress(Math.round(event.loaded / event.total * 100), event);
      });
      if (cancelled) return;
      if (!response || response.code !== 0) throw new Error(response?.message || '上传插件包失败');
      Message.loading({ content: '安装插件中', id: 'plugin-upload', duration: 0 });
      await installCode({ name: response.data });
      onSuccess(response);
      Message.success({ content: '插件安装成功', id: 'plugin-upload', duration: 2000 });
      await refreshMarket();
    } catch (error) {
      onError(error as Error);
      Message.error({ content: (error as Error).message || '插件安装失败', id: 'plugin-upload', duration: 3000 });
    } finally {
      busyName.value = '';
    }
  })();
  return { abort() { cancelled = true; busyName.value = ''; Message.info({ content: '上传已取消', id: 'plugin-upload', duration: 1000 }); } };
};

onMounted(loadPlugins);
</script>

<style scoped>
.market-header { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 16px; margin-bottom: 20px; }
.market-header h2 { margin: 0 0 8px; color: var(--color-text-1); }
.market-header p { margin: 0; color: var(--color-text-3); }
.market-notice { margin-top: 24px; }
.market-error { margin-bottom: 16px; }
</style>
