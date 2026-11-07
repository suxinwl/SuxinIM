<template>
  <a-modal :modal-style="{maxWidth:'calc(100vw - 24px)'}" v-model:visible="visible" :title="`账号安全 · ${label}`" :footer="false" :width="560" :mask-closable="!saving" :closable="!saving" @cancel="clear">
    <a-spin :loading="loading" style="width:100%"><template v-if="state">
      <a-descriptions :column="1" bordered>
        <a-descriptions-item label="登录状态">{{ state.locked ? '已锁定' : '未锁定' }}</a-descriptions-item>
        <a-descriptions-item v-if="state.locked" label="自动解锁时间">{{ new Date(state.lockedUntil * 1000).toLocaleString() }}</a-descriptions-item>
        <a-descriptions-item label="窗口内密码错误">{{ state.recentFailures }}</a-descriptions-item>
        <a-descriptions-item label="下次登录修改密码">{{ state.mustChangePassword ? '必须修改' : '无需强制修改' }}</a-descriptions-item>
      </a-descriptions>
      <a-popconfirm content="解锁只清除登录锁定，不启用停用账号，也不恢复旧会话。" @ok="unlock"><a-button v-permission="['unlock']" :disabled="saving || (!state.locked && !state.recentFailures)" style="margin-top:12px">解锁账号</a-button></a-popconfirm>
      <a-divider>管理员重置密码</a-divider>
      <a-alert>重置会退出所有端。用户用新密码验证后必须设置自己的密码，再重新登录。不会自动解除账号锁定。</a-alert>
      <a-form :model="form" layout="vertical" style="margin-top:12px">
        <a-form-item label="临时密码"><a-input-password v-model="form.password" autocomplete="new-password" /></a-form-item>
        <a-form-item label="确认临时密码"><a-input-password v-model="form.confirm" autocomplete="new-password" /></a-form-item>
        <a-button v-permission="['resetPassword']" type="primary" status="danger" :loading="saving" @click="reset">重置并要求首次修改</a-button>
      </a-form>
    </template></a-spin>
  </a-modal>
</template>
<script setup lang="ts">
import { reactive, ref } from 'vue';
import { Message } from '@arco-design/web-vue';
import { getSecurity, resetPassword, unlockAccount, type SecurityState } from '@/api/system/security';
const emit = defineEmits(['success']); const visible = ref(false), loading = ref(false), saving = ref(false), id = ref(0), label = ref('');
const state = ref<SecurityState>(); const form = reactive({ password: '', confirm: '' }); let generation = 0;
function clear() { generation += 1; form.password = ''; form.confirm = ''; state.value = undefined; saving.value = false; }
async function open(uid: number, title: string) { clear(); const run = generation; id.value = uid; label.value = title; visible.value = true; loading.value = true; try { const v = await getSecurity(uid); if (run === generation) state.value = v; } finally { if (run === generation) loading.value = false; } }
async function unlock() { if (saving.value) return; const run = generation, uid = id.value; saving.value = true; try { await unlockAccount(uid); if (run !== generation) return; Message.success('账号已解锁'); emit('success'); const value = await getSecurity(uid); if (run === generation) state.value = value; } finally { if (run === generation) saving.value = false; } }
async function reset() { if (saving.value) return; if (!form.password || form.password !== form.confirm) { Message.error('请输入相同的两次临时密码'); return; } const run = generation, uid = id.value; saving.value = true; try { await resetPassword(uid, form.password); if (run !== generation) return; form.password = ''; form.confirm = ''; Message.success('密码已重置，用户下次登录必须修改'); emit('success'); try { const value = await getSecurity(uid); if (run === generation) state.value = value; } catch { if (run === generation) visible.value = false; } } finally { if (run === generation) saving.value = false; } }
defineExpose({ open });
</script>
