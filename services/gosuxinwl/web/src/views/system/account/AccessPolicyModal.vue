<template>
  <a-modal :modal-style="{maxWidth:'calc(100vw - 24px)'}" v-model:visible="visible" :title="`${subject === 'role' ? '角色' : '个人'}登录与保存权限 · ${label}`" :width="860" :ok-loading="saving" :ok-button-props="{ disabled: !policy || loading }" @before-ok="save" @cancel="close">
    <a-spin :loading="loading" style="width: 100%">
      <a-alert>登录是访问前提。在线预览不受保存开关限制；个人明确允许或拒绝覆盖角色授权并集，继承则使用角色结果。保存将使受影响账号重新登录。</a-alert>
      <a-alert v-if="subject === 'role' && policy?.revision === 0" type="warning" style="margin-top:12px">当前角色尚未配置，不参与已有角色矩阵并集；所有角色都未配置时才兼容默认全部允许。下方是首次保存建议值，保存后此角色开始参与授权并集。</a-alert>
      <p v-if="current">当前来源 {{ current.currentIP }} · {{ current.zone === 'lan' ? '内网' : '公网' }} · {{ names[current.logicalClient] }}</p>
      <a-tabs v-if="policy" v-model:active-key="zone">
        <a-tab-pane v-for="z in zones" :key="z" :title="z === 'lan' ? '内网' : '公网'">
          <div class="policy-scroll"><table class="policy-table"><thead><tr><th>客户端</th><th>登录</th><th>保存附件</th><th v-if="subject === 'person'">当前生效</th></tr></thead><tbody>
            <tr v-for="client in clients" :key="client"><td>{{ names[client] }}</td>
              <td><a-select v-model="policy.matrix[z][client].login" :options="choices" /></td>
              <td><a-select v-model="policy.matrix[z][client].save" :options="choices" /></td>
              <td v-if="subject === 'person'">{{ effectiveText(z, client) }}</td>
            </tr>
          </tbody></table></div>
        </a-tab-pane>
      </a-tabs>
    </a-spin>
  </a-modal>
</template>
<script setup lang="ts">
import { computed, ref } from 'vue';
import { Message } from '@arco-design/web-vue';
import { clients, zones, getPolicy, savePolicy, getEffectivePolicy, type Policy, type Zone, type LogicalClient, type EffectivePolicy } from '@/api/system/security';
const emit = defineEmits(['success']);
const visible = ref(false), loading = ref(false), saving = ref(false);
const subject = ref<Policy['subject']>('person'), label = ref(''), id = ref(0), zone = ref<Zone>('lan');
const policy = ref<Policy>(), current = ref<EffectivePolicy>();
let generation = 0;
const names: Record<LogicalClient, string> = { web_desktop: '电脑网页', web_mobile: '手机网页', pc: 'PC 客户端', android: 'Android', ios: 'iOS' };
const choices = computed(() => [...(subject.value === 'person' ? [{ label: '继承角色', value: 'inherit' }] : []), { label: '允许', value: 'allow' }, { label: '拒绝', value: 'deny' }]);
function effectiveText(z: Zone, c: LogicalClient) { const v = policy.value?.effective?.[z]?.[c]; return v ? `登录${v.login ? '允许' : '拒绝'} / 保存${v.save ? '允许' : '拒绝'}（${v.sources.login === 'person' ? '个人' : v.sources.login === 'roles' ? '角色' : '默认'}）` : '保存后重新读取生效结果'; }
function close() { generation += 1; policy.value = undefined; }
async function open(value: number, title: string, kind: Policy['subject'] = 'person') {
  const run = ++generation; id.value = value; label.value = title; subject.value = kind; policy.value = undefined; current.value = undefined; visible.value = true; loading.value = true;
  try { const [p, e] = await Promise.all([getPolicy(kind, value), getEffectivePolicy()]); if (run !== generation) return; policy.value = p; current.value = e; zone.value = e.zone; } finally { if (run === generation) loading.value = false; }
}
async function save() { if (!policy.value || loading.value) return false; saving.value = true; try { await savePolicy(subject.value, id.value, policy.value); Message.success('权限已保存，受影响账号需重新登录'); emit('success'); close(); return true; } catch { return false; } finally { saving.value = false; } }
defineExpose({ open });
</script>
<style scoped>.policy-scroll{overflow-x:auto}.policy-table{border-collapse:collapse;width:100%;min-width:600px;margin:12px 0}.policy-table th,.policy-table td{text-align:left;padding:10px;border-bottom:1px solid var(--color-border-2)}.policy-table td:nth-child(2),.policy-table td:nth-child(3){width:150px}</style>
