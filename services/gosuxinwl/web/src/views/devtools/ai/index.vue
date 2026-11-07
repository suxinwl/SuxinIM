<template>
  <a-space direction="vertical" fill size="large" class="ai-panel">
    <a-card title="AI 接口配置">
      <a-alert>支持兼容 Chat Completions 的模型接口。填写 API 基地址（通常以 /v1 结尾），保存后可用于字段设计、审查和解释。</a-alert>
      <a-form :model="config" layout="vertical" style="margin-top:16px" @submit-success="save">
        <a-grid :cols="2" :col-gap="20">
          <a-grid-item><a-form-item label="API 基地址" required><a-input v-model="config.endpoint" placeholder="https://api.example.com/v1" /></a-form-item></a-grid-item>
          <a-grid-item><a-form-item label="模型名称" required><a-input v-model="config.model" placeholder="模型提供方的模型名称" /></a-form-item></a-grid-item>
        </a-grid>
        <a-form-item :label="config.keyConfigured ? 'API Key（已保存，留空保留）' : 'API Key'"><a-input-password v-model="apiKey" autocomplete="new-password" /></a-form-item>
        <a-space wrap><a-checkbox v-model="config.allowPrivate">允许本机 / 内网模型接口</a-checkbox><a-checkbox v-model="clearKey">清除已保存的 API Key</a-checkbox><a-button html-type="submit" type="primary" :loading="saving">保存配置</a-button></a-space>
      </a-form>
    </a-card>
    <a-card title="AI 开发助手">
      <a-space direction="vertical" fill>
        <a-radio-group v-model="mode" type="button"><a-radio value="schema">字段设计</a-radio><a-radio value="review">代码审查</a-radio><a-radio value="explain">实现建议</a-radio></a-radio-group>
        <a-textarea v-model="prompt" placeholder="描述需求，或粘贴需要审查的代码" :auto-size="{minRows:4,maxRows:12}" :max-length="32768" />
        <a-button type="primary" :loading="generating" @click="run">提交模型</a-button>
        <a-alert v-if="mode==='schema'">字段设计结果可直接复制使用；安装代码生成插件后，也可导入设计 JSON，再预览并确认生成。</a-alert>
        <pre v-if="answer" class="answer">{{ answer }}</pre>
      </a-space>
    </a-card>
    <a-card title="MCP 与开发工具连接">
      <a-alert>使用 Streamable HTTP（协议 2025-11-25）。令牌按管理员隔离，支持限定权限、到期和撤销。主程序内置 AI 接口；安装相应插件后可扩展代码预览、历史和 Excel 校验。生成、回滚和导入确认在插件页面操作。</a-alert>
      <a-form :model="tokenForm" layout="inline" style="margin:16px 0" @submit-success="create">
        <a-form-item label="名称"><a-input v-model="tokenForm.name" placeholder="例如 IDE 开发助手" /></a-form-item>
        <a-form-item label="有效天数"><a-input-number v-model="tokenForm.days" :min="1" :max="30" /></a-form-item>
        <a-form-item label="权限"><a-checkbox-group v-model="tokenForm.scopes"><a-checkbox value="ai:generate">调用 AI</a-checkbox><a-checkbox value="codegen:read">代码预览 / 历史（可选插件）</a-checkbox><a-checkbox value="excel:read">Excel 模板 / 校验（可选插件）</a-checkbox></a-checkbox-group></a-form-item>
        <a-form-item><a-button html-type="submit" type="primary" :loading="creating">创建连接令牌</a-button></a-form-item>
      </a-form>
      <a-alert v-if="newToken" type="success">新令牌仅本次展示，请复制后保存。<a-button size="mini" @click="newToken=''">隐藏令牌</a-button></a-alert>
      <a-textarea v-if="newToken" :model-value="newToken" readonly style="margin-top:10px" />
      <p>支持 HTTP 自定义请求头的客户端连接配置：</p><pre class="answer">{{ connection }}</pre>
      <a-table :data="items" :pagination="false" row-key="id">
        <template #columns>
          <a-table-column title="名称" data-index="name" />
          <a-table-column title="权限"><template #cell="{record}">{{ record.scopes.join('、') }}</template></a-table-column>
          <a-table-column title="到期时间" data-index="expiresAt" />
          <a-table-column title="状态"><template #cell="{record}"><a-tag :color="record.revoked?'gray':'green'">{{ record.revoked?'已撤销':'有效（以到期时间为准）' }}</a-tag></template></a-table-column>
          <a-table-column title="操作"><template #cell="{record}"><a-popconfirm content="撤销后使用此令牌的客户端将立即失去访问权限" @ok="revoke(record.id)"><a-button status="danger" size="small" :disabled="record.revoked">撤销</a-button></a-popconfirm></template></a-table-column>
        </template>
      </a-table>
      <p>接口目录：GET /admin/developer/ai/interfaces。页面 API 使用当前登录身份；MCP 使用 Authorization: Bearer 开发令牌，兼容 X-Devtools-Token 请求头。</p>
    </a-card>
  </a-space>
</template>
<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { Message } from '@arco-design/web-vue';
import * as api from '@/api/devtools/ai';
const config = reactive<api.AIConfig>({endpoint:'',model:'',allowPrivate:false,keyConfigured:false});
const apiKey=ref(''),clearKey=ref(false),saving=ref(false),generating=ref(false),creating=ref(false);
const mode=ref('schema'),prompt=ref(''),answer=ref(''),newToken=ref('');
const items=ref<api.DeveloperToken[]>([]);
const tokenForm=reactive({name:'IDE 开发助手',days:7,scopes:['ai:generate']});
const connection=computed(()=>JSON.stringify({mcpServers:{gosuxin:{type:'http',url:location.origin+'/mcp',headers:{Authorization:'Bearer '+(newToken.value||'<填写开发令牌>')}}}},null,2));
function failure(e:unknown){Message.error(e instanceof Error?e.message:'操作失败');}
async function load(){try{Object.assign(config,await api.getConfig());items.value=await api.tokens();}catch(e){failure(e);}}
async function save(){saving.value=true;try{await api.saveConfig({endpoint:config.endpoint,model:config.model,allowPrivate:config.allowPrivate,apiKey:apiKey.value,clearKey:clearKey.value});apiKey.value='';clearKey.value=false;await load();Message.success('配置已保存');}catch(e){failure(e);}finally{saving.value=false;}}
async function run(){generating.value=true;try{answer.value=(await api.generate(mode.value,prompt.value)).text;}catch(e){failure(e);}finally{generating.value=false;}}
async function create(){creating.value=true;try{newToken.value=(await api.createToken(tokenForm.name,tokenForm.scopes,tokenForm.days)).token;items.value=await api.tokens();}catch(e){failure(e);}finally{creating.value=false;}}
async function revoke(id:string){try{await api.revokeToken(id);newToken.value='';items.value=await api.tokens();Message.success('令牌已撤销');}catch(e){failure(e);}}
onMounted(load);
</script>
<style scoped>.answer { padding: 16px; background: var(--color-fill-2); border-radius: 6px; }.ai-panel { width:100%; }</style>
