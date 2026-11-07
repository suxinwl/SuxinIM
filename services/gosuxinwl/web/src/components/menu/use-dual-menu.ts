import { ref, computed, watch } from 'vue';
import { useRoute } from 'vue-router';
import type { RouteRecordRaw } from 'vue-router';
import useMenuTree from './use-menu-tree';
import { findPrimaryName, findFirstLeaf } from './menu-tree';

const selectedPrimaryName = ref<string>('');

export default function useDualMenu() {
  const { menuTree } = useMenuTree();
  const route = useRoute();

  const primaryMenu = computed<RouteRecordRaw[]>(() => menuTree.value || []);

  const selectedPrimary = computed(() =>
    primaryMenu.value.find((item) => item.name === selectedPrimaryName.value)
  );

  const secondaryMenu = computed(() => {
    const primary = selectedPrimary.value;
    if (!primary?.children?.length) return [];
    return primary.children;
  });

  const hasSecondaryMenu = computed(() => secondaryMenu.value.length > 0);

  const findPrimaryNameByRoute = (targetName: string) =>
    findPrimaryName(primaryMenu.value, targetName);

  const syncPrimaryFromRoute = (routeName: string) => {
    const primaryName = findPrimaryNameByRoute(routeName);
    if (primaryName) {
      selectedPrimaryName.value = primaryName;
    } else if (!primaryMenu.value.some((item) => item.name === selectedPrimaryName.value)) {
      selectedPrimaryName.value = String(primaryMenu.value[0]?.name || '');
    }
  };

  watch(
    [primaryMenu, () => route.name, () => route.meta.activeMenu],
    () => {
      syncPrimaryFromRoute(String(route.meta.activeMenu || route.name || ''));
    },
    { immediate: true }
  );

  return {
    primaryMenu,
    secondaryMenu,
    hasSecondaryMenu,
    selectedPrimaryName,
    selectedPrimary,
    findFirstLeaf,
    findPrimaryNameByRoute,
    syncPrimaryFromRoute,
  };
}
