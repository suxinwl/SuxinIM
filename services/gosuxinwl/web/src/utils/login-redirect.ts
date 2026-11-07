import type { LocationQueryRaw, RouteLocationRaw } from 'vue-router';

export function loginDestination(
  requested: unknown,
  query: LocationQueryRaw,
  hasRoute: (name: string) => boolean,
): RouteLocationRaw {
  if (typeof requested === 'string') {
    // Preserve the entire local path, including plugin name, query and hash.
    if (/^\/(?!\/)/.test(requested) && !/[\\\u0000-\u0020]/.test(requested) &&
        !/^\/login(?:[/?#]|$)/.test(requested)) return requested;
    // Existing bookmarks may still contain a registered route name.
    if (requested !== 'login' && requested !== 'notFound' && hasRoute(requested)) {
      return { name: requested, query };
    }
  }
  return { name: 'home', query };
}
