<script lang="tsx">
  import { defineComponent, ref, h, compile, computed, watch, onUnmounted } from 'vue';
  import type { PropType } from 'vue';
  import { useI18n } from 'vue-i18n';
  import { routeTitle } from '@/utils/route-title';
  import { useRoute, useRouter, RouteRecordRaw } from 'vue-router';
  import type { RouteMeta, RouteLocationNormalized } from 'vue-router';
  import { useAppStore,useUserStore } from '@/store';
  import { listenerRouteChange } from '@/utils/route-listener';
  import { openWindow, regexUrl } from '@/utils';
  import useMenuTree from './use-menu-tree';
  import {Icon} from '@/components/Icon';
  export default defineComponent({
    emit: ['collapse'],
    props: {
      menuData: {
        type: Array as PropType<RouteRecordRaw[]>,
        default: undefined,
      },
      showCollapseButton: {
        type: Boolean,
        default: undefined,
      },
    },
    setup(props) {
      const { t, te } = useI18n();
      const hasMenuTranslation = (key: string) => te(key) || te(key, 'zh-CN') || t(key) !== key;
      const appStore = useAppStore();
      const useUser = useUserStore();
      const router = useRouter();
      const route = useRoute();
      const { menuTree } = useMenuTree();
      const displayMenuTree = computed(
        () => props.menuData ?? menuTree.value
      );
      const shouldShowCollapseButton = computed(() => {
        if (props.showCollapseButton !== undefined) {
          return props.showCollapseButton;
        }
        return appStore.device !== 'mobile';
      });
      const collapsed = computed({
        get() {
          if (appStore.device === 'desktop') return appStore.menuCollapse;
          return false;
        },
        set(value: boolean) {
          appStore.updateSettings({ menuCollapse: value });
        },
      });

      const topMenu = computed(() => appStore.topMenu);
      const openKeys = ref<string[]>([]);
      const selectedKey = ref<string[]>([]);

      const goto = (item: RouteRecordRaw) => {
        // Open external link
        if (regexUrl.test(item.path)) {//path外部链接
          openWindow(item.path);
          selectedKey.value = [item.name as string];
          return;
        }else if (item.meta&&item.meta.isExt&&regexUrl.test(item.component as any)) {//isExt外部链接
          openWindow(item.component as any);
          selectedKey.value = [item.name as string];
          return;
        }else if (item.meta?.onlypage) {//独立页面-数据大屏
          const href = router.resolve({
            path: item.path,
            query: {
              tenant_id: useUser.id,
              business_id: useUser.id,
              ...(!item.meta?.requiresAuth ? { rouid: String(item.meta?.id || '') } : {}),
            },
          });
          openWindow(href.href);
          selectedKey.value = [item.name as string];
          return;
        }
        // Eliminate external link side effects
        const { hideInMenu, activeMenu } = (item.meta || {}) as RouteMeta;
        if (route.name === item.name && !hideInMenu && !activeMenu) {
          selectedKey.value = [item.name as string];
          return;
        }
        // Trigger router change
        if(item.name){
          router.push({
            name: item.name,
          });
        }
      };
      const findMenuOpenKeys = (target: string) => {
        const result: string[] = [];
        let isFind = false;
        const backtrack = (item: RouteRecordRaw, keys: string[]) => {
          if (item.name === target) {
            isFind = true;
            result.push(...keys);
            return;
          }
          if (item.children?.length) {
            item.children.forEach((el) => {
              backtrack(el, [...keys, el.name as string]);
            });
          }
        };
        displayMenuTree.value.forEach((el: RouteRecordRaw) => {
          if (isFind) return; // Performance optimization
          backtrack(el, [el.name as string]);
        });
        return result;
      };
      const syncMenu = (newRoute: RouteLocationNormalized) => {
        const { requiresAuth, activeMenu, hideInMenu } = newRoute.meta;
        if (requiresAuth && (!hideInMenu || activeMenu)) {
          const menuOpenKeys = findMenuOpenKeys(
            (activeMenu || newRoute.name) as string
          );

          const keySet = new Set([...menuOpenKeys, ...openKeys.value]);
          openKeys.value = [...keySet];

          const selected = activeMenu || menuOpenKeys[menuOpenKeys.length - 1];
          selectedKey.value = selected ? [String(selected)] : [];
        }
      };
      const stopRouteListener = listenerRouteChange(syncMenu, true);
      onUnmounted(stopRouteListener);
      watch(displayMenuTree, () => syncMenu(route), { immediate: true });
      const setCollapse = (val: boolean) => {
        if (appStore.device === 'desktop')
          appStore.updateSettings({ menuCollapse: val });
      };

      const renderSubMenu = () => {
        function travel(_route: RouteRecordRaw[], nodes = []) {
          if (_route) {
            _route.forEach((element) => {
              //过来隐藏菜单
              if(element?.meta?.hideInMenu) return
              // This is demo, modify nodes as needed
              const icon = element?.meta?.icon
                ? () => h(Icon,{icon: element?.meta?.icon,size:18})
                : null;
              const node =
                element?.children && element?.children.length !== 0 ? (
                  <a-sub-menu
                    key={element?.name}
                    v-slots={{
                      icon,
                      title: () => element?.meta?.locale && hasMenuTranslation(element.meta.locale) ? h(compile(t(element.meta.locale))) : routeTitle(element.meta, t, hasMenuTranslation),
                    }}
                  >
                    {travel(element?.children)}
                  </a-sub-menu>
                ) : (
                  <a-menu-item
                    key={element?.name}
                    v-slots={{ icon }}
                    onClick={() => goto(element)}
                  >
                    {routeTitle(element.meta, t, hasMenuTranslation)}
                  </a-menu-item>
                );
              nodes.push(node as never);
            });
          }
          return nodes;
        }
        return travel(displayMenuTree.value);
      };

      return () => (
        <a-menu
          mode={topMenu.value ? 'horizontal' : 'vertical'}
          v-model:collapsed={collapsed.value}
          v-model:open-keys={openKeys.value}
          show-collapse-button={shouldShowCollapseButton.value}
          accordion={appStore.menuAccordion}
          auto-open={false}
          selected-keys={selectedKey.value}
          auto-open-selected={true}
          level-indent={26}
          style="height: 100%;width:100%;"
          onCollapse={setCollapse}
        >
          {renderSubMenu()}
        </a-menu>
      );
    },
  });
</script>

<style lang="less" scoped>
  :deep(.arco-menu-inner) {
    overflow: hidden;
    &:hover{
      overflow: auto;
    }
    .arco-menu-inline-header {
      display: flex;
      align-items: center;
    }
    .arco-menu-item-inner,.arco-menu-title{
      user-select: none;
    }
    .arco-icon {
      &:not(.arco-icon-down) {
        font-size: 18px;
      }
    }
    .arco-menu-icon{
      margin-right: 8px !important;
    }
  }
</style>
