import type { Router, LocationQueryRaw } from 'vue-router';
import NProgress from 'nprogress'; // progress bar
import { useUserStore } from '@/store';
import { isLogin } from '@/utils/auth';

export default function setupUserLoginInfoGuard(router: Router) {
  router.beforeEach(async (to) => {
    NProgress.start();
    const userStore = useUserStore();
    if (to.name === 'login') return;
    if (isLogin()) {
      if (userStore.id) {
        return;
      } else {
        try {
          await userStore.info();
          return;
        } catch (error) {
          await userStore.logout();
          return {
            name: 'login',
            query: {
              redirect: to.fullPath,
            } as LocationQueryRaw,
          };
        }
      }
    }else {
      if (to.name === 'login') {
        return;
      }else if(to.query.rouid){
        return;
      }
      //判断是否需要-用外部独立页面，如数据大屏
      return {
        name: 'login',
        query: {
          redirect: to.fullPath,
        } as LocationQueryRaw,
      };
    }
  });
}
