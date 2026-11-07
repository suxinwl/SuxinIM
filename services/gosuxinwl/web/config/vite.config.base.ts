import { resolve } from 'path';
import { defineConfig } from 'vite';
import vue from '@vitejs/plugin-vue';
import vueJsx from '@vitejs/plugin-vue-jsx';
import svgLoader from 'vite-svg-loader';
import settings from '../src/config/settings.json' with { type: 'json' };
import { fileURLToPath } from 'node:url';
const configDir = fileURLToPath(new URL('.', import.meta.url));
const { themeColor } = settings;
export default defineConfig({
  plugins: [vue(), vueJsx(), svgLoader({ svgoConfig: {} })],
  resolve: {
    alias: [
      {
        find: '@',
        replacement: resolve(configDir, '../src'),
      },
      {
        find: '/@',
        replacement: resolve(configDir, '../src'),
      },
      {
        find: 'assets',
        replacement: resolve(configDir, '../src/assets'),
      },
      {
        find: 'vue-i18n',
        replacement: 'vue-i18n/dist/vue-i18n.esm-bundler.js', // Resolve the i18n warning issue
      },
      {
        find: 'vue',
        replacement: 'vue/dist/vue.esm-bundler.js', // compile template
      },
    ],
    extensions: ['.ts', '.js'],
  },
  build: {
    target: 'es2020'
  },
  define: {
    'process.env': {},
    __VUE_PROD_HYDRATION_MISMATCH_DETAILS__: 'true'
  },
  css: {
    preprocessorOptions: {
      less: {
        modifyVars: {
          hack: `true; @import (reference) "${resolve(configDir, '../src/assets/style/breakpoint.less')}";`,
          'arcoblue-6': themeColor,//#165DFF
        },
        javascriptEnabled: true,
      },
    },
  },
});
