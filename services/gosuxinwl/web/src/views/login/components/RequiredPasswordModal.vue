<template><a-modal :modal-style="{maxWidth:'calc(100vw - 24px)'}" v-model:visible="visible" title="首次登录需要修改密码" :ok-loading="saving" :mask-closable="false" @before-ok="save" @cancel="cancel"><a-alert>账号已验证，但尚未登录。请设置自己的新密码；完成后需要重新登录。</a-alert><a-form :model="form" layout="vertical" style="margin-top:16px"><a-form-item label="新密码"><a-input-password v-model="form.password" autocomplete="new-password" /></a-form-item><a-form-item label="确认新密码"><a-input-password v-model="form.confirm" autocomplete="new-password" /></a-form-item></a-form><p>验证有效至 {{ expiry }}</p></a-modal></template>
<script setup lang="ts">
import { computed, onBeforeUnmount, reactive, ref } from 'vue';import { Message } from '@arco-design/web-vue';
import { changeRequiredPassword, cancelPasswordChange, type ChangeChallenge } from '@/api/system/security';
const emit=defineEmits(['success']);const visible=ref(false),saving=ref(false),expiresAt=ref(0);const form=reactive({password:'',confirm:''});let ticket='';
const expiry=computed(()=>new Date(expiresAt.value*1000).toLocaleTimeString());
function clear(){ticket='';form.password='';form.confirm='';}
function open(c:ChangeChallenge){clear();ticket=c.changeTicket;expiresAt.value=c.expiresAt;visible.value=true;}
function cancel(){const old=ticket;clear();if(old)void cancelPasswordChange(old);}
async function save(){if(!form.password||form.password!==form.confirm){Message.error('请输入相同的两次新密码');return false;}if(Date.now()>=expiresAt.value*1000){Message.error('验证已过期，请重新登录');cancel();return true;}saving.value=true;try{await changeRequiredPassword(ticket,form.password);clear();Message.success('密码已修改，请使用新密码重新登录');emit('success');return true;}catch{Message.error('修改未通过，请检查密码或重新验证');return false;}finally{saving.value=false;}}
onBeforeUnmount(cancel);defineExpose({open});
</script>
