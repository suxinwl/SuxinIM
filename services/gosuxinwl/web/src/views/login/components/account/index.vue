<template>
  <a-form
    ref="formRef"
    :model="form"
    :label-col-style="{ display: 'none' }"
    :wrapper-col-style="{ flex: 1 }"
    size="large"
    @submit="handleLogin"
  >
    <a-form-item field="username" hide-label :rules="[{ required: true, message: $t('login.form.userName.errMsg')}]" :validate-trigger="['change', 'blur']">
      <a-input v-model="form.username" :placeholder="$t('login.form.userName.placeholder')+defAccount.username"  allow-clear  >
        <template #prefix>
          <icon-user />
        </template>
      </a-input>
    </a-form-item>
    <a-form-item field="password" hide-label :rules="[{ required: true, message: $t('login.form.password.errMsg')}]" :validate-trigger="['change', 'blur']">
      <a-input-password v-model="form.password" :placeholder="$t('login.form.password.placeholder')+defAccount.password" allow-clear>
        <template #prefix>
          <icon-lock />
        </template>
      </a-input-password>
    </a-form-item>
    <a-form-item field="captcha" v-if="captchaShow" hide-label :rules="[{ required: true, message: $t('login.form.verification.errMsg')}]" >
      <a-input v-model="form.captcha" :placeholder="$t('login.form.verification.placeholder')" :max-length="4" allow-clear style="flex: 1 1" hide-button/>
      <div class="captcha-container" @click="handleCaptcha">
        <a-tooltip :content="$t('login.form.clickverification')">
         <img :src="captchaImgBase64" alt="验证码" class="captcha" />
        </a-tooltip>
        <div v-if="form.expired" class="overlay">
          <p>{{$t("login.form.verification.expired") }}</p>
        </div>
      </div>
    </a-form-item>
    <a-form-item style="margin-top:-5px;margin-bottom:15px;">
      <a-row justify="space-between" align="center" class="w-full">
        <a-checkbox v-model="loginConfig.rememberMe">记住账号</a-checkbox>
        <a-link>{{$t('login.form.forgetPassword')}}</a-link>
      </a-row>
    </a-form-item>
    <a-form-item>
      <a-space direction="vertical" fill class="w-full">
        <a-button class="btn" type="primary" :loading="loading" html-type="submit" size="large" long>{{$t('login.form.login')}}</a-button>
      </a-space>
    </a-form-item>
  </a-form>
  <RequiredPasswordModal ref="passwordModal" @success="passwordChanged" />
</template>

<script setup lang="ts">
import { redirectRuntimePluginAfterLogin } from '@/utils/runtime-plugin';
import { loginDestination } from '@/utils/login-redirect';
import { ref,reactive,onBeforeUnmount,onMounted} from 'vue';
import { type FormInstance, Message } from '@arco-design/web-vue'
import { useRouter } from 'vue-router';
import { useStorage } from '@vueuse/core'
import { getCaptcha } from '@/api/user';
import { useUserStore } from '@/store';
import { useI18n } from 'vue-i18n';
import RequiredPasswordModal from '../RequiredPasswordModal.vue';
import type { ChangeChallenge } from '@/api/system/security';
const passwordModal=ref<InstanceType<typeof RequiredPasswordModal>>();
const { t } = useI18n();
const loginConfig = useStorage('login-data', {
  rememberMe: true,
  username: "", 
  password: ""
})
loginConfig.value.password='';
//演示用户名和密码默认值提示
const defAccount=ref({username:"",password:""})
const dfAcPass=window?.globalConfig?.DefaultAccountPassword
if(dfAcPass&&dfAcPass.length==2){
  defAccount.value.username = `（演示账号：${dfAcPass[0]}）`
  defAccount.value.password =  `（演示密码：${dfAcPass[1]}）`
}
const formRef = ref<FormInstance>()
const form = reactive({
  username: loginConfig.value.username,
  password: '',
  captcha: "",
  codeid: "",
  expired: false
})

// 验证码过期定时器
const timer=ref() 
const startTimer = (expireTime: number) => {
  if (timer.value) {
    clearTimeout(timer.value)
  }
  const remainingTime = expireTime - Date.now()
  if (remainingTime <= 0) {
    form.expired = true
    return
  }
  timer.value = setTimeout(() => {
    form.expired = true
  }, remainingTime)
}
// 组件销毁时清理定时器
onBeforeUnmount(() => {
  if (timer.value) {
    clearTimeout(timer.value)
  }
})

const captchaImgBase64 = ref()
const captchaShow = ref<boolean>(false)
// 获取验证码
const handleCaptcha = async() => {
  const resdata= await getCaptcha({type:"image"})
  const { id,show, img, expireTime } = resdata
  form.codeid = id
  captchaShow.value= show
  captchaImgBase64.value = img
  form.expired = false
  startTimer(expireTime)
}

const userStore = useUserStore()
const router = useRouter()
const loading = ref(false)
// 登录
const handleLogin = async () => {
  try {
    const isInvalid = await formRef.value?.validate()
    if (isInvalid) return
    loading.value = true
    await userStore.login({
      username: form.username,
      password:form.password,
      captcha: form.captcha,
      codeid: form.codeid
    })
    const { redirect, ...othersQuery } = router.currentRoute.value.query;
    if (await redirectRuntimePluginAfterLogin(redirect)) return;
    await router.replace(loginDestination(redirect, othersQuery, name => router.hasRoute(name)));
    const { rememberMe } = loginConfig.value
    loginConfig.value.username = rememberMe ? form.username : ''
    loginConfig.value.password = ''
    Message.success({content:t('login.form.login.success'),id:"errmsg"})
  } catch (error) {
    const challenge=(error as {challenge?:ChangeChallenge})?.challenge;if(challenge){passwordModal.value?.open(challenge);form.password='';}
    handleCaptcha()
    form.captcha =''
  } finally {
    loading.value = false
  }
}
function passwordChanged(){form.password='';form.captcha='';loginConfig.value.password='';void handleCaptcha();}

onMounted(() => {
  handleCaptcha()
})
</script>

<style lang="less" scoped>
.arco-input-wrapper,
:deep(.arco-select-view-single) {
  height: 40px;
  border-radius: 4px;
  font-size: 13px;
}

.arco-input-wrapper.arco-input-error {
  background-color: rgb(var(--danger-1));
  border-color: rgb(var(--danger-3));
}

.arco-input-wrapper.arco-input-error:hover {
  background-color: rgb(var(--danger-1));
  border-color: rgb(var(--danger-6));
}

.arco-input-wrapper :deep(.arco-input) {
  font-size: 13px;
  color: var(--color-text-1);
}

.arco-input-wrapper:hover {
  border-color: rgb(var(--arcoblue-6));
}

.captcha {
  width: 111px;
  height: 39px;
  margin: 0 0 0 5px;
  background-color: var(--color-neutral-2);
  padding-left: 5px;
  border-radius: 4px;
}

.btn {
  height: 40px;
}

.captcha-container {
  position: relative;
  display: inline-block;
  cursor: pointer;
  height: 39px;
}

.captcha-container {
  position: relative;
  display: inline-block;
}

.overlay {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  background-color: rgba(51, 51, 51, 0.8);
  display: flex;
  justify-content: center;
  align-items: center;
}

.overlay p {
  font-size: 12px;
  color: white;
}
</style>
