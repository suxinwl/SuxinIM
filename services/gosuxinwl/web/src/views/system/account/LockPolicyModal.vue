<template><a-modal :modal-style="{maxWidth:'calc(100vw - 24px)'}" v-model:visible="visible" title="连续密码错误锁定策略" :ok-loading="saving" @before-ok="save"><a-alert>仅统计通过验证码后的错误密码。达到阈值锁定并退出所有会话，管理员可提前解锁。</a-alert><a-form :model="form" layout="vertical" style="margin-top:16px"><a-form-item label="统计窗口（分钟）"><a-input-number v-model="form.windowMinutes" :min="1" :max="1440" :precision="0" /></a-form-item><a-form-item label="错误密码次数"><a-input-number v-model="form.failures" :min="1" :max="100" :precision="0" /></a-form-item><a-form-item label="锁定时长（分钟）"><a-input-number v-model="form.lockMinutes" :min="1" :max="10080" :precision="0" /></a-form-item></a-form></a-modal></template>
<script setup lang="ts">
import { reactive, ref } from 'vue'; import { Message } from '@arco-design/web-vue'; import { getLockPolicy, saveLockPolicy } from '@/api/system/security';
const visible=ref(false),saving=ref(false),ready=ref(false);const form=reactive({windowMinutes:10,failures:5,lockMinutes:30,revision:0});
async function open(){ready.value=false;const p=await getLockPolicy();Object.assign(form,{windowMinutes:p.windowSeconds/60,failures:p.failures,lockMinutes:p.lockSeconds/60,revision:p.revision});ready.value=true;visible.value=true;}
async function save(){if(!ready.value)return false;saving.value=true;try{await saveLockPolicy({windowSeconds:form.windowMinutes*60,failures:form.failures,lockSeconds:form.lockMinutes*60,revision:form.revision});Message.success('锁定策略已保存');return true;}catch{return false;}finally{saving.value=false;}}
defineExpose({open});
</script>
