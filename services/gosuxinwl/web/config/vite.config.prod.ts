import { fileURLToPath } from 'node:url';
import { mergeConfig } from 'vite';
import baseConfig from './vite.config.base.ts';
import configVisualizerPlugin from './plugin/visualizer.ts';
import configArcoResolverPlugin from './plugin/arcoResolver.ts';

export default mergeConfig(
  {
    // 1 配置二级访问目录（位置：suxinweb）
    base: process.env.NODE_ENV === 'production' ? '/suxinweb/' : '',
    // 2 独立部署
    // base: '',
    mode: 'production',
    plugins: [
      // Legacy vite-plugin-compression builds absolute Windows asset names
      // under dist/E:/... with Vite 8. Nginx performs response compression.
      configVisualizerPlugin(),
      configArcoResolverPlugin(),
    ],
    build: {
      rolldownOptions: {
        input: { main: fileURLToPath(new URL('../index.html', import.meta.url)) },
        output: {
          codeSplitting: {
            groups: [
            { name: 'arco', test: /node_modules[\\/]@arco-design[\\/]web-vue/ },
            { name: 'chart', test: /node_modules[\\/](?:echarts|vue-echarts|zrender)[\\/]/ },
            { name: 'vue', test: /node_modules[\\/](?:vue|@vue|vue-router|pinia|@vueuse|vue-i18n|@intlify)[\\/]/ },
            ],
          },
        },
      },
      chunkSizeWarningLimit: 2000,
    },
  },
  baseConfig
);
