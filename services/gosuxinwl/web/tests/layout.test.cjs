const assert = require('node:assert/strict');
const { test } = require('node:test');
const path = require('node:path');
const Module = require('node:module');
const { buildSync } = require('esbuild');

function loadSource(relative) {
  const filename = path.resolve(__dirname, '..', relative);
  const result = buildSync({
    entryPoints: [filename],
    tsconfig: path.resolve(__dirname, '../tsconfig.json'),
    bundle: true,
    write: false,
    platform: 'node',
    format: 'cjs',
  });
  const compiled = new Module(filename, module);
  compiled.filename = filename;
  compiled.paths = module.paths;
  compiled._compile(result.outputFiles[0].text, filename);
  return compiled.exports;
}

const { restoreSettings } = loadSource('src/store/modules/app/settings.ts');
const { findPrimaryName, findFirstLeaf } = loadSource('src/components/menu/menu-tree.ts');

test('old saved settings receive layout defaults without losing preferences', () => {
  const settings = restoreSettings(JSON.stringify({ menuWidth: 260, footer: false, theme: 'dark' }));
  assert.equal(settings.layout, 'left');
  assert.equal(settings.tabMode, 'googlecard');
  assert.equal(settings.menuWidth, 260);
  assert.equal(settings.footer, false);
  assert.equal(settings.theme, 'dark');
});

test('legacy top menu migrates and a saved layout takes precedence', () => {
  assert.equal(restoreSettings('{"topMenu":true}').layout, 'top');
  const columns = restoreSettings('{"layout":"columns","topMenu":true,"menuCollapse":true,"tabMode":"rounded"}');
  assert.equal(columns.topMenu, false);
  assert.equal(columns.menuCollapse, false);
  assert.equal(columns.tabMode, 'rounded');
});

test('invalid preferences cannot prevent startup or create unsupported tabs', () => {
  for (const input of [null, '{broken', 'null', '[]', '42', '{"layout":"missing","tabMode":"missing"}']) {
    const settings = restoreSettings(input);
    assert.equal(settings.layout, 'left');
    assert.equal(settings.tabMode, 'googlecard');
  }
});

test('refresh loads current server menus even when old settings contain route state', () => {
  const settings = restoreSettings(JSON.stringify({
    menuCollapse: true,
    theme: 'dark',
    serverMenu: [{ name: 'old-menu' }],
    serverRoute: [{ name: 'old-menu', path: '/old-menu' }],
    isDynamicAddedRoute: true,
  }));
  assert.equal(settings.menuCollapse, true);
  assert.equal(settings.theme, 'dark');
  assert.deepEqual(settings.serverMenu, []);
  assert.deepEqual(settings.serverRoute, []);
  assert.equal(settings.isDynamicAddedRoute, false);
});

const leaf = (name, extra = {}) => ({ name, path: `/${name}`, meta: {}, ...extra });
const tree = [
  leaf('home'),
  leaf('system', { children: [
    leaf('hidden', { meta: { hideInMenu: true } }),
    leaf('organization', { children: [leaf('roles')] }),
  ] }),
];

test('nested routes select their primary menu and unknown routes do not', () => {
  assert.equal(findPrimaryName(tree, 'roles'), 'system');
  assert.equal(findPrimaryName(tree, 'system'), 'system');
  assert.equal(findPrimaryName(tree, 'home'), 'home');
  assert.equal(findPrimaryName(tree, 'missing'), null);
  assert.equal(findPrimaryName([], 'roles'), null);
});

test('primary navigation skips hidden leaves and handles empty groups', () => {
  assert.equal(findFirstLeaf(tree[1]).name, 'roles');
  assert.equal(findFirstLeaf(tree[0]).name, 'home');
  assert.equal(findFirstLeaf(leaf('empty', { children: [leaf('hidden', { meta: { hideInMenu: true } })] })), null);
});

test('unmounting one route subscriber preserves other layout subscribers', () => {
  const { listenerRouteChange, setRouteEmitter, removeRouteListener } = loadSource('src/utils/route-listener.ts');
  const seen = [];
  const stopFirst = listenerRouteChange(() => seen.push('first'));
  const stopSecond = listenerRouteChange(() => seen.push('second'));
  setRouteEmitter({ name: 'home' });
  stopFirst();
  setRouteEmitter({ name: 'roles' });
  assert.deepEqual(seen, ['first', 'second', 'second']);
  stopSecond();
  removeRouteListener();
  let stale = false;
  listenerRouteChange(() => { stale = true; });
  assert.equal(stale, false);
});


const fs = require('node:fs');
const { build } = require('esbuild');
const { parse, compileScript } = require('@vue/compiler-sfc');
const { createSSRApp, h } = require('vue');
const { renderToString } = require('@vue/server-renderer');
const { createRouter, createMemoryHistory } = require('vue-router');
const { isEmbeddedRuntimePlugin } = loadSource('src/utils/runtime-plugin-route.ts');

async function loadLayoutFixture(relative, source) {
  const filename = path.resolve(__dirname, '..', relative);
  const widget = (importPath) => importPath.includes('page-layout') ? 'business-content'
    : importPath.includes('PrimaryMenu') ? 'primary-menu'
      : importPath.includes('/menu/') ? 'menu'
        : importPath.includes('navbar') ? 'navbar'
          : importPath.includes('tab-bar') ? 'tabbar' : 'footer';
  const fixtureModules = {
    '@/store': 'export const useAppStore = () => globalThis.__runtimeLayoutFixture.store; export const useUserStore = () => globalThis.__runtimeLayoutFixture.user;',
    'vue-router': `export { stringifyQuery } from ${JSON.stringify(require.resolve('vue-router'))}; export const useRouter = () => ({ push() { throw new Error("unexpected permission navigation"); } }); export const useRoute = () => ({ name: "privatecode", meta: { requiresAuth: true } });`,
    '@/hooks/locale': 'import { ref } from "vue"; export default () => ({ currentLocale: ref("zh-CN") });',
    '@/hooks/permission': 'export default () => ({ accessRouter: () => true });',
    '@/hooks/responsive': 'export default () => { globalThis.__runtimeLayoutFixture.responsiveCalls += 1; };',
    '@/components/menu/use-dual-menu': 'import { ref } from "vue"; export default () => ({ secondaryMenu: ref([]), hasSecondaryMenu: ref(true) });',
    '@/utils/auth': 'export const isLogin = () => globalThis.__runtimeLayoutFixture.loggedIn;',
    'nprogress': 'export default { start() {}, done() {} };',
  };
  const bundled = await build({
    stdin: { contents: source ?? fs.readFileSync(filename, 'utf8'), sourcefile: filename, resolveDir: path.dirname(filename), loader: 'ts' },
    tsconfig: path.resolve(__dirname, '../tsconfig.json'), bundle: true, write: false,
    platform: 'node', format: 'cjs', external: ['vue'],
    plugins: [{
      name: 'layout-fixtures',
      setup(builder) {
        builder.onResolve({ filter: /./ }, (args) => {
          if (fixtureModules[args.path]) return { path: args.path, namespace: 'fixture' };
          if (args.path.endsWith('.vue')) return { path: widget(args.path), namespace: 'widget' };
          return undefined;
        });
        builder.onLoad({ filter: /./, namespace: 'fixture' }, (args) => ({ contents: fixtureModules[args.path], loader: 'js', resolveDir: path.dirname(filename) }));
        builder.onLoad({ filter: /./, namespace: 'widget' }, (args) => ({
          contents: `import { h } from "vue"; export default { name: "Widget", setup() { return () => h("div", { "data-layout-widget": ${JSON.stringify(args.path)} }, "business page"); } };`, loader: 'js',
        }));
      },
    }],
  });
  const compiled = new Module(filename, module);
  compiled.filename = filename;
  compiled.paths = module.paths;
  compiled._compile(bundled.outputFiles[0].text, filename);
  return compiled.exports.default;
}

let actualLayout;
async function renderActualLayout(pathname, embedded, settings = {}) {
  const previousWindow = global.window;
  const previousFixture = global.__runtimeLayoutFixture;
  const self = {};
  const fixture = { responsiveCalls: 0, user: { id: 1 }, store: {
    layout: 'columns', topMenu: false, menu: true, hideMenu: false,
    navbar: true, tabBar: true, footer: true, menuCollapse: false, menuWidth: 220,
    menuDark: false, ...settings,
  } };
  global.__runtimeLayoutFixture = fixture;
  global.window = { location: { pathname, search: '?embedded=1&runtimeFrontend=1' }, self, top: embedded ? {} : self, globalConfig: { AppTitle_zhCN: 'GoSuxin' } };
  Object.defineProperty(global.window, 'parent', { get() { throw new Error('layout must not read parent login state'); } });
  try {
    if (!actualLayout) {
      const filename = path.resolve(__dirname, '../src/layout/default-layout.vue');
      const { descriptor } = parse(fs.readFileSync(filename, 'utf8'), { filename });
      actualLayout = await loadLayoutFixture('src/layout/default-layout.vue', compileScript(descriptor, { id: 'runtime-layout-test', inlineTemplate: true }).content);
    }
    const app = createSSRApp(actualLayout);
    const layoutElement = { inheritAttrs: false, setup(_props, { slots, attrs }) { return () => h('section', attrs, slots.default?.()); } };
    for (const name of ['a-layout', 'a-layout-content', 'a-layout-sider', 'a-drawer']) app.component(name, layoutElement);
    return { html: await renderToString(app), responsiveCalls: fixture.responsiveCalls };
  } finally {
    if (previousWindow === undefined) delete global.window; else global.window = previousWindow;
    if (previousFixture === undefined) delete global.__runtimeLayoutFixture; else global.__runtimeLayoutFixture = previousFixture;
  }
}

test('content mode requires an actual embedded plugin admin path', () => {
  assert.equal(isEmbeddedRuntimePlugin('/plugins/privatecode/admin/privatecode', true), true);
  assert.equal(isEmbeddedRuntimePlugin('/plugins/privatecode/admin', true), true);
  assert.equal(isEmbeddedRuntimePlugin('/plugins/privatecode/admin/privatecode', false), false);
  for (const pathname of ['/suxinweb/privatecode', '/runtime-plugins/privatecode', '/plugins/privatecode/api/admin', '/plugins/privatecode/administrator', '/plugins/privatecode/administer', '/plugins/Invalid/admin/', '/plugins//admin/']) {
    assert.equal(isEmbeddedRuntimePlugin(pathname, true), false, pathname);
  }
});

test('actual plugin iframe layouts render business content without duplicate host chrome', async () => {
  for (const [plugin, settings] of [
    ['privatecode', { layout: 'columns' }], ['ebook', { layout: 'left' }],
    ['analysis', { layout: 'top', topMenu: true }], ['inventory', { layout: 'columns', hideMenu: true }],
  ]) {
    const { html, responsiveCalls } = await renderActualLayout(`/plugins/${plugin}/admin/page`, true, settings);
    assert.match(html, /layout-runtime-content/);
    assert.match(html, /data-layout-widget="business-content"/);
    for (const name of ['navbar', 'menu', 'primary-menu', 'tabbar', 'footer']) {
      assert.doesNotMatch(html, new RegExp(`data-layout-widget="${name}"`), `${plugin}: ${name}`);
    }
    assert.doesNotMatch(html, /layout-sider|layout-columns|padding-left|margin-left/);
    assert.equal(responsiveCalls, 0, 'an embedded content page must not change host layout preferences');
  }
});

test('direct plugin access and ordinary host pages retain their full navigation', async () => {
  for (const [pathname, embedded, settings] of [
    ['/plugins/privatecode/admin/privatecode', false, { layout: 'columns' }],
    ['/plugins/analysis/admin/analysis', false, { layout: 'left' }],
    ['/suxinweb/privatecode', true, { layout: 'columns' }],
  ]) {
    const { html, responsiveCalls } = await renderActualLayout(pathname, embedded, settings);
    assert.doesNotMatch(html, /layout-runtime-content/);
    for (const name of ['navbar', 'menu', 'tabbar', 'footer', 'business-content']) {
      assert.match(html, new RegExp(`data-layout-widget="${name}"`));
    }
    assert.equal(responsiveCalls, 1);
  }
});

test('an iframe still uses its own login guard instead of a parent session', async () => {
  const previousFixture = global.__runtimeLayoutFixture;
  let infoCalls = 0;
  global.__runtimeLayoutFixture = { loggedIn: false, user: { id: 0, info: async () => { infoCalls += 1; }, logout: async () => {} } };
  try {
    const setupGuard = await loadLayoutFixture('src/router/guard/userLoginInfo.ts');
    const router = createRouter({ history: createMemoryHistory('/plugins/privatecode/admin/'), routes: [
      { path: '/login', name: 'login', component: {}, meta: { requiresAuth: false } },
      { path: '/privatecode', name: 'privatecode', component: {}, meta: { requiresAuth: true } },
    ] });
    setupGuard(router);
    await router.push('/privatecode?filter=active#details');
    assert.equal(router.currentRoute.value.name, 'login');
    assert.equal(router.currentRoute.value.query.redirect, '/privatecode?filter=active#details');
    assert.equal(infoCalls, 0);
    global.__runtimeLayoutFixture.loggedIn = true;
    await router.push('/privatecode');
    assert.equal(router.currentRoute.value.name, 'privatecode');
    assert.equal(infoCalls, 1, 'the child must fetch and validate its own user information');
  } finally {
    if (previousFixture === undefined) delete global.__runtimeLayoutFixture; else global.__runtimeLayoutFixture = previousFixture;
  }
});
