import type { RouteRecordRaw } from 'vue-router';

export function findPrimaryName(tree: RouteRecordRaw[], targetName: string): string | null {
  const containsRoute = (items: RouteRecordRaw[]): boolean => items.some(
    (item) => item.name === targetName || Boolean(item.children?.length && containsRoute(item.children))
  );
  const primary = tree.find((item) => containsRoute([item]));
  return primary?.name ? String(primary.name) : null;
}

export function findFirstLeaf(item: RouteRecordRaw): RouteRecordRaw | null {
  if (item.meta?.hideInMenu) return null;
  if (!item.children?.length) return item;
  for (const child of item.children) {
    const leaf = findFirstLeaf(child);
    if (leaf) return leaf;
  }
  return null;
}
