<template>
  <a-layout
    class="layout layout-default"
    :class="{ mobile: appStore.hideMenu, 'layout-columns': isColumnsLayout && !runtimeContentOnly, 'layout-runtime-content': runtimeContentOnly }"
  >
    <!-- Embedded plugin pages share the outer host navigation. -->
    <template v-if="runtimeContentOnly">
      <a-layout-content class="content-page runtime-content-page">
        <PageLayout />
      </a-layout-content>
    </template>
    <!-- 双列布局：左侧双栏全高，导航栏在右侧内容区顶部 -->
    <template v-else-if="isColumnsLayout">
      <a-layout-sider
        v-if="renderMenu"
        v-show="!hideMenu"
        class="layout-sider layout-sider-primary"
        :width="primaryMenuWidth"
        :hide-trigger="true"
      >
        <PrimaryMenu />
      </a-layout-sider>
      <a-layout-sider
        v-if="renderMenu && hasSecondaryMenu"
        v-show="!hideMenu"
        class="layout-sider layout-sider-secondary"
        :width="menuWidth"
        :style="{ left: `${primaryMenuWidth}px` }"
        :hide-trigger="true"
      >
        <div class="secondary-menu-header">
          <span class="secondary-menu-title">{{ AppTitle }}</span>
        </div>
        <div
          class="menu-wrapper secondary-menu-wrapper"
          :class="{ 'app-menu-dark': appStore.layout!='columns'&&appStore.menuDark }"
        >
          <Menu :menu-data="secondaryMenu" :show-collapse-button="false" />
        </div>
      </a-layout-sider>
      <a-drawer
        v-if="hideMenu"
        :visible="drawerVisible"
        placement="left"
        :footer="false"
        mask-closable
        :closable="false"
        @cancel="drawerCancel"
      >
        <Menu />
      </a-drawer>
      <a-layout class="layout-main" :style="columnsMainStyle">
        <div v-if="navbar" class="layout-navbar-inner" ref="navbarRef">
          <NavBar />
        </div>
        <TabBar v-if="appStore.tabBar" class="layout-tabbar" />
        <a-layout-content class="content-page">
          <PageLayout />
        </a-layout-content>
        <Footer v-if="footer" />
      </a-layout>
    </template>

    <!-- 默认/顶部布局 -->
    <template v-else>
      <div v-if="navbar" class="layout-navbar" ref="navbarRef">
        <NavBar />
      </div>
      <a-layout-sider
        v-if="renderMenu"
        v-show="!hideMenu"
        class="layout-sider"
        breakpoint="xl"
        :collapsed="collapsed"
        :collapsible="true"
        :width="menuWidth"
        :style="{ paddingTop: navbar ? navbarHeight : '' }"
        :hide-trigger="true"
        @collapse="setCollapsed"
      >
        <div class="menu-wrapper" :class="{ 'app-menu-dark': appStore.layout!='columns'&&appStore.menuDark }">
          <Menu />
        </div>
      </a-layout-sider>
      <a-drawer
        v-if="hideMenu"
        :visible="drawerVisible"
        placement="left"
        :footer="false"
        mask-closable
        :closable="false"
        @cancel="drawerCancel"
      >
        <Menu />
      </a-drawer>
      <a-layout class="layout-content" :style="paddingStyle">
        <TabBar v-if="appStore.tabBar" class="layout-tabbar" />
        <a-layout-content class="content-page">
          <PageLayout />
        </a-layout-content>
        <Footer v-if="footer" />
      </a-layout>
    </template>
  </a-layout>
</template>

<script lang="ts" setup>
  import { ref, computed, watch, provide, onMounted, nextTick } from 'vue';
  import { useRouter, useRoute } from 'vue-router';
  import { useAppStore, useUserStore } from '@/store';
  import useLocale from '@/hooks/locale';
  import NavBar from '@/components/navbar/index.vue';
  import Menu from '@/components/menu/index.vue';
  import PrimaryMenu from '@/components/menu/PrimaryMenu.vue';
  import useDualMenu from '@/components/menu/use-dual-menu';
  import Footer from '@/components/footer/index.vue';
  import TabBar from '@/components/tab-bar/index.vue';
  import usePermission from '@/hooks/permission';
  import useResponsive from '@/hooks/responsive';
  import PageLayout from './page-layout.vue';
  import { isEmbeddedRuntimePlugin } from '@/utils/runtime-plugin-route';

  const PRIMARY_MENU_WIDTH = 88;
  const runtimeContentOnly = isEmbeddedRuntimePlugin(window.location.pathname, window.self !== window.top);

  const isInit = ref(false);
  const navbarRef = ref();
  const appStore = useAppStore();
  const userStore = useUserStore();
  const router = useRouter();
  const route = useRoute();
  const permission = usePermission();
  const { currentLocale } = useLocale();
  if (!runtimeContentOnly) useResponsive(true);

  const { secondaryMenu, hasSecondaryMenu } = useDualMenu();

  const navbarHeight = `50px`;
  const navbar = computed(() => appStore.navbar);
  const isColumnsLayout = computed(
    () => appStore.layout === 'columns' && !appStore.topMenu
  );
  const renderMenu = computed(() => appStore.menu && !appStore.topMenu);
  const hideMenu = computed(() => appStore.hideMenu);
  const footer = computed(() => appStore.footer);
  const primaryMenuWidth = PRIMARY_MENU_WIDTH;
  const menuWidth = computed(() => {
    return appStore.menuCollapse ? 48 : appStore.menuWidth;
  });
  const collapsed = computed(() => {
    return appStore.menuCollapse;
  });
  const totalSiderWidth = computed(() => {
    if (!renderMenu.value || hideMenu.value) return 0;
    if (isColumnsLayout.value) {
      let width = primaryMenuWidth;
      if (hasSecondaryMenu.value) width += menuWidth.value;
      return width;
    }
    return menuWidth.value;
  });
  const columnsMainStyle = computed(() => ({
    marginLeft: totalSiderWidth.value
      ? `${totalSiderWidth.value}px`
      : undefined,
  }));
  const paddingStyle = computed(() => {
    const paddingLeft =
      renderMenu.value && !hideMenu.value
        ? { paddingLeft: `${menuWidth.value}px` }
        : {};
    const paddingTop = navbar.value ? { paddingTop: navbarHeight } : {};
    return { ...paddingLeft, ...paddingTop };
  });
  const AppTitle = computed(() => {
    switch (currentLocale.value) {
      case 'zh-CN':
        return window?.globalConfig?.AppTitle_zhCN;
      case 'zh-TW':
        return window?.globalConfig?.AppTitle_zhTW;
      case 'en-US':
        return window?.globalConfig?.AppTitle_enUS;
      default:
        return window?.globalConfig?.AppTitle_enUS;
    }
  });
  const setCollapsed = (val: boolean) => {
    if (!isInit.value) return;
    appStore.updateSettings({ menuCollapse: val });
  };
  watch(
    () => userStore.id,
    (roleValue) => {
      if (roleValue && !permission.accessRouter(route))
        router.push({ name: 'notFound' });
    }
  );
  const drawerVisible = ref(false);
  const drawerCancel = () => {
    drawerVisible.value = false;
  };
  provide('toggleDrawerMenu', () => {
    drawerVisible.value = !drawerVisible.value;
  });
  onMounted(() => {
    isInit.value = true;
    nextTick(() => {
      navbarRef;
    });
  });
</script>

<style scoped lang="less">
  @nav-size-height: 50px;
  @layout-max-width: 1100px;

  .layout {
    width: 100%;
    height: 100%;
  }
  .layout-default {
    flex-direction: row;
    &-right {
      overflow: hidden;
    }
  }

  .layout-runtime-content {
    flex-direction: column;
    min-width: 0;
    background-color: var(--color-fill-2);

    .runtime-content-page {
      flex: 1;
      min-height: 0;
      overflow: auto;
    }
  }

  // 双列布局：右侧主区域
  .layout-columns {
    .layout-main {
      flex: 1;
      min-width: 0;
      height: 100%;
      display: flex;
      flex-direction: column;
      overflow: hidden;
      background-color: var(--color-fill-2);
      transition: margin-left 0.2s cubic-bezier(0.34, 0.69, 0.1, 1);
    }

    .layout-navbar-inner {
      flex-shrink: 0;
      height: @nav-size-height;
      z-index: 100;
      position: relative;
    }

    .content-page {
      flex: 1;
      overflow-y: auto;
      overflow-x: hidden;
      display: flex;
      flex-direction: column;
    }
  }

  .layout-navbar {
    position: fixed;
    top: 0;
    left: 0;
    z-index: 1001;
    width: 100%;
    height: @nav-size-height;
  }
  .layout-sider {
    position: fixed;
    top: 0;
    left: 0;
    z-index: 1000;
    height: 100%;
    transition: all 0.2s cubic-bezier(0.34, 0.69, 0.1, 1);
    &::after {
      position: absolute;
      top: 0;
      right: -1px;
      display: block;
      width: 1px;
      height: 100%;
      background-color: var(--color-border);
      content: '';
    }

    > :deep(.arco-layout-sider-children) {
      overflow-y: hidden;
    }
  }

  .layout-sider-secondary {
    z-index: 999;
  }

  .secondary-menu-header {
    display: flex;
    align-items: center;
    height: 50px;
    padding: 0 16px;
    flex-shrink: 0;
    border-bottom: 1px solid var(--color-border);
    overflow: hidden;
    white-space: nowrap;
    background-color: var(--color-bg-2);
  }

  .secondary-menu-title {
    font-size: 16px;
    font-weight: 500;
    color: var(--color-text-1);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .secondary-menu-wrapper {
    height: calc(100% - 50px);
  }

  .menu-wrapper {
    height: 100%;
    overflow: auto;
    overflow-x: hidden;
    :deep(.arco-menu) {
      ::-webkit-scrollbar {
        width: 12px;
        height: 4px;
      }

      ::-webkit-scrollbar-thumb {
        border: 4px solid transparent;
        background-clip: padding-box;
        border-radius: 7px;
        background-color: var(--color-text-4);
      }

      ::-webkit-scrollbar-thumb:hover {
        background-color: var(--color-text-3);
      }
    }
  }
  //内容区
  .layout-content {
    background-color: var(--color-fill-2);
    transition: padding 0.2s cubic-bezier(0.34, 0.69, 0.1, 1);
    width: 100%;
    height: 100%;
    display: flex;
    flex-direction: column;
    overflow: hidden;
    position: relative;
    .content-page {
      overflow-y: auto;
      overflow-x: hidden;
      display: flex;
      flex-direction: column;
    }
  }
  // 深色菜单主题色变量
  .app-menu-dark {
    background-color: #001529 !important;
    :deep(.arco-menu-light) {
      background-color: transparent;
    }
    :deep(.arco-menu-item) {
      color: var(--color-neutral-4);
      background-color: transparent;
      &:hover {
        background-color: rgba(255, 255, 255, 0.08);
      }
    }
    :deep(.arco-menu-inline-header) {
      background-color: transparent;
      color: var(--color-neutral-4);
      &:hover {
        background-color: rgba(255, 255, 255, 0.08);
      }
    }
    :deep(.arco-menu-pop-header) {
      background-color: transparent;
      color: var(--color-neutral-4);
      &:hover {
        background-color: rgba(255, 255, 255, 0.08);
      }
    }
    :deep(.arco-menu-item.arco-menu-selected) {
      color: rgb(var(--primary-6));
      background-color: rgba(255, 255, 255, 0.08);
    }
    :deep(.arco-menu-inline-header.arco-menu-selected) {
      color: rgb(var(--primary-6));
    }
    :deep(.arco-menu-collapse-button) {
      background-color: rgba(255, 255, 255, 0.08);
      &:hover {
        background-color: rgba(255, 255, 255, 0.2);
      }
    }
  }
  body[arco-theme='dark'] {
    .app-menu-dark {
      :deep(.arco-menu-item) {
        color: var(--color-neutral-8);
      }
      :deep(.arco-menu-inline-header) {
        color: var(--color-neutral-8);
      }
      :deep(.arco-menu-item.arco-menu-selected) {
        color: rgb(var(--primary-6));
      }
      :deep(.arco-menu-inline-header.arco-menu-selected) {
        color: rgb(var(--primary-6));
      }
    }
  }
</style>
