import { cloneDeep } from 'lodash-es';
import type { AppRouteRecordRaw } from '@/router/types';

/** A page cannot redirect back to its own route: Vue Router applies it repeatedly. */
export function removePageSelfRedirect(route: AppRouteRecordRaw): void {
  const redirect = route.redirect;
  const samePath = (path: string) => path.replace(/\/+$/, '') === route.path.replace(/\/+$/, '');
  if (
    (typeof redirect === 'string' && samePath(redirect)) ||
    (redirect && typeof redirect === 'object' &&
      (('path' in redirect && typeof redirect.path === 'string' && samePath(redirect.path)) ||
        ('name' in redirect && redirect.name === route.name)))
  ) {
    delete route.redirect;
  }
}

/** Keep a root menu page inside the application layout without redirecting its child to itself. */
export function wrapRootPageRoute(route: AppRouteRecordRaw, layout: AppRouteRecordRaw['component']): void {
  const page = cloneDeep(route);
  removePageSelfRedirect(page);
  route.children = [page];
  route.component = layout;
  route.name = `${route.name}Parent`;
  route.path = '';
  route.meta = { ...route.meta, single: true, affix: false };
}
