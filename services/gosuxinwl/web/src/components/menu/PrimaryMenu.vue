<template>
  <div class="primary-menu" :class="{ 'primary-dark': appStore.menuDark }">
    <div class="primary-menu-logo">
      <img :src="brandLogo" alt="GoSuxin" />
    </div>
    <div class="primary-menu-list">
      <div
        v-for="item in primaryMenu"
        :key="String(item.name)"
        role="button"
        tabindex="0"
        :aria-pressed="selectedPrimaryName === item.name"
        class="primary-menu-item"
        :class="{ active: selectedPrimaryName === item.name }"
        @click="handleSelect(item)"
        @keydown.enter.prevent="handleSelect(item)"
        @keydown.space.prevent="handleSelect(item)"
      >
        <Icon v-if="item.meta?.icon" :icon="String(item.meta.icon)" :size="20" />
        <span class="primary-menu-title">
          {{ routeTitle(item.meta, t, hasMenuTranslation) }}
        </span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
  import { useI18n } from 'vue-i18n';
  import { routeTitle } from '@/utils/route-title';
  import { useRoute, useRouter } from 'vue-router';
  import type { RouteRecordRaw } from 'vue-router';
  import { useAppStore, useUserStore } from '@/store';
  import { openWindow, regexUrl } from '@/utils';
  import { Icon } from '@/components/Icon';
  import useDualMenu from './use-dual-menu';

  const { t, te } = useI18n();
  const hasMenuTranslation = (key: string) => te(key) || te(key, 'zh-CN') || t(key) !== key;
  const router = useRouter();
  const route = useRoute();
  const appStore = useAppStore();
  const userStore = useUserStore();
  const brandLogo = import.meta.env.BASE_URL + 'logo.png?v=puty3';
  const { primaryMenu, selectedPrimaryName, findFirstLeaf, findPrimaryNameByRoute } = useDualMenu();

  const goto = (item: RouteRecordRaw) => {
    if (regexUrl.test(item.path)) {
      openWindow(item.path);
      return;
    }
    const externalComponent: unknown = item.component;
    if (item.meta?.isExt && typeof externalComponent === 'string') {
      if (regexUrl.test(externalComponent)) openWindow(externalComponent);
      return;
    }
    if (item.meta?.onlypage) {
      const href = router.resolve({
        path: item.path,
        query: {
          tenant_id: userStore.id,
          business_id: userStore.id,
          ...(!item.meta.requiresAuth ? { rouid: String(item.meta.id || '') } : {}),
        },
      });
      openWindow(href.href);
      return;
    }
    if (route.name === item.name && !item.meta?.hideInMenu && !item.meta?.activeMenu) return;
    if (item.name) router.push({ name: item.name });
  };

  const handleSelect = (item: RouteRecordRaw) => {
    selectedPrimaryName.value = String(item.name || '');
    if (!item.children?.length) {
      goto(item);
      return;
    }
    if (findPrimaryNameByRoute(String(route.name || '')) !== item.name) {
      const firstLeaf = findFirstLeaf(item);
      if (firstLeaf) goto(firstLeaf);
    }
  };
</script>

<style lang="less" scoped>
  .primary-menu {
    display: flex;
    flex-direction: column;
    height: 100%;
    // background-color: #001529;
  }
  .primary-dark{
    background-color: #001529 !important;
     .primary-menu-item {
       color: rgba(255, 255, 255, 0.65);
        &:hover {
          color: #fff;
          background-color: rgba(255, 255, 255, 0.08);
        }
        &.active {
          color: #fff;
          background-color: rgb(var(--primary-6));
        }
     }
  }
  .primary-menu-logo {
    display: flex;
    align-items: center;
    justify-content: center;
    height: 50px;
    flex-shrink: 0;

    img {
      height: 28px;
    }
  }

  .primary-menu-list {
    flex: 1;
    overflow-y: auto;
    overflow-x: hidden;
    padding: 4px 0;
  }

  .primary-menu-item {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    padding: 10px 2px;
    margin: 2px 4px;
    border-radius: 4px;
    cursor: pointer;
    color: var(--color-neutral-10);
    transition: all 0.2s;
    user-select: none;

    &:hover {
      color: var(--color-neutral-8);
      background-color: var(--color-neutral-2);
    }

    &.active {
      color: rgb(var(--primary-6));
      background-color: var(--color-primary-light-1);
    }
  }

  .primary-menu-title {
    margin-top: 4px;
    font-size: 12px;
    line-height: 1.3;
    text-align: center;
    width: 100%;
    padding: 0 2px;
    word-break: keep-all;
  }
</style>
