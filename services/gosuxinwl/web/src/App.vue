<template>
  <a-config-provider :locale="locale">
    <router-view v-if="isRouter"/>
    <global-setting />
  </a-config-provider>
</template>

<script lang="ts" setup>
  import { computed ,nextTick, provide, ref} from 'vue';
  import enUS from '@arco-design/web-vue/es/locale/lang/en-us';
  import zhCN from '@arco-design/web-vue/es/locale/lang/zh-cn';
  import zhTW from '@arco-design/web-vue/es/locale/lang/zh-tw';
  import GlobalSetting from '@/components/global-setting/index.vue';
  import useLocale from '@/hooks/locale';

  const { currentLocale } = useLocale();
  const locale = computed(() => {
    switch (currentLocale.value) {
      case 'en-US':
        document.title=window?.globalConfig.AppTitle_enUS
        return enUS;
      case 'zh-CN':
        document.title=window?.globalConfig.AppTitle_zhCN
        return zhCN;
      case 'zh-TW':
        document.title=window?.globalConfig.AppTitle_zhTW
        return zhTW;
      default:
        document.title=window?.globalConfig.AppTitle_enUS
        return enUS;
    }
  });
  //刷新
  const isRouter = ref(true)
  const reload = () => {
    isRouter.value = false 
    nextTick(() => {isRouter.value = true})
  }
  provide("reload", reload)
</script>
<style lang="less">

</style>
