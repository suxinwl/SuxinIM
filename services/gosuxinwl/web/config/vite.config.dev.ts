import { mergeConfig } from 'vite';
import baseConfig from './vite.config.base.ts';

export default mergeConfig(
  {
    mode: 'development',
    server: {
      open: false,
      fs: {
        strict: true,
      },
      host: true,
      port:9600,
      proxy: {
        '/admin': {
          target: 'http://127.0.0.1:19880/admin',//本项目本地 HTTP 后端；9880 为 HTTPS 入口
          changeOrigin: true,
          rewrite: (path: string) => path.replace(/^\/admin/, '')//这里的/需要转义
        }
      }
    },
    plugins: [
    ],
  },
  baseConfig
);
