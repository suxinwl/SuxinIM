import { stringifyQuery, type LocationQuery } from 'vue-router';

const pluginNamePattern = /^[a-z][a-z0-9-]{0,63}$/;
const genericPluginRoute = /^\/runtime-plugins\/([a-z][a-z0-9-]{0,63})(?:\/(.*))?$/;

/** Generic entries start at the plugin root; existing business menus retain their inner SPA path. */
export function runtimePluginAdminTarget(path: string, configuredPlugin?: unknown, query?: LocationQuery): string | null {
  const generic = path.match(genericPluginRoute);
  const plugin = typeof configuredPlugin === 'string' && configuredPlugin ? configuredPlugin : generic?.[1];
  if (!plugin || !pluginNamePattern.test(plugin) || (generic && generic[1] !== plugin)) return null;
  const page = generic ? generic[2] || '' : path.replace(/^\//, '');
  const search = query ? stringifyQuery(query) : '';
  return `/plugins/${plugin}/admin/${page}${search ? `?${search}` : ''}`;
}

/** These business workspaces own their navigation; other runtime plugins remain embedded. */
export function runtimePluginOpensStandalone(target: string): boolean {
  const pathname = target.split(/[?#]/, 1)[0];
  return /^\/plugins\/biz-suite\/admin\/(?:$|overview$|(?:hrm|im|wecom)(?:\/|$))/.test(pathname);
}

type AliasRoute = { path: string; alias?: string | string[]; children?: AliasRoute[] };

/** Keep the old bookmark attached to an authorized menu, never to a public fallback. */
export function addBizSuiteLegacyAlias(routes: AliasRoute[]): void {
  const entries: AliasRoute[] = [];
  const visit = (items: AliasRoute[]) => items.forEach((item) => {
    entries.push(item);
    if (item.children) visit(item.children);
  });
  visit(routes);
  const nativeIM = entries.find(item => item.path === '/communication');
  if (nativeIM) nativeIM.alias = [...(nativeIM.alias ? [nativeIM.alias].flat() : []), '/runtime-plugins/biz-suite/im'];
  const legacy = '/runtime-plugins/biz-suite';
  if (entries.some((item) => item.path === legacy || [item.alias].flat().includes(legacy))) return;
  const entry = entries.find((item) => item.path === `${legacy}/hrm`)
    || entries.find((item) => item.path === `${legacy}/im`);
  if (!entry) return;
  entry.alias = [...(entry.alias ? [entry.alias].flat() : []), legacy];
}


/** Hide host chrome only inside an actual plugin iframe, never from a caller-supplied query. */
export function isEmbeddedRuntimePlugin(pathname: string, embedded: boolean): boolean {
  return embedded && /^\/plugins\/[a-z][a-z0-9-]{0,63}\/admin(?:\/|$)/.test(pathname);
}

