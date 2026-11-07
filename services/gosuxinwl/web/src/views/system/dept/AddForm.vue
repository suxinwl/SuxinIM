<template>
  <BasicModal v-bind="$attrs" @register="registerModal" :loading="loading" helpMessage="编辑和修改部门" width="900px" :minHeight="500" :title="getTitle" @ok="handleSubmit" @visible-change="handleModalVisibility">
    <a-form ref="formRef" :model="formData" auto-label-width style="padding:10px 20px;">
      <a-form-item field="name" label="部门名称" validate-trigger="input" :rules="[{required:true,message:'请填写部门名称'}]" style="margin-bottom:15px;">
        <a-input v-model="formData.name" placeholder="请填写部门名称" />
      </a-form-item>
      <a-form-item label="上级部门" field="pid" style="margin-bottom:15px;">
        <a-tree-select placeholder="选择上级部门" :data="parntList" 
        :fieldNames="{
            key: 'id',
            title: 'name',
            children: 'children'
          }"
        v-model="formData.pid" @change="() => searchLeaders('')">
        </a-tree-select>
      </a-form-item>
      <a-form-item label="排序" field="weigh" style="margin-bottom:15px;">
        <a-input-number v-model="formData.weigh" placeholder="请填排序" />
      </a-form-item>
      <a-form-item label="部门负责人" field="leader_uid" style="margin-bottom:15px;">
        <a-select data-testid="dept-leader" v-model="formData.leader_uid" :options="leaderOptions" :loading="leaderLoading"
          allow-search allow-clear :filter-option="false" :search-delay="250" @search="searchLeaders" @popup-visible-change="handleLeaderPopup"
          placeholder="搜索员工姓名、工号或手机号">
          <template #empty>{{ leaderError || '没有可选员工，请确认员工已开通企业通讯' }}</template>
        </a-select>
        <template #extra>
          <div v-if="leaderError" data-testid="dept-leader-error" role="alert" style="color:rgb(var(--danger-6));">
            {{ leaderError }} <a-button type="text" size="mini" :disabled="leaderLoading" @click="retryLeaders">重试</a-button>
          </div>
          可选择本企业已开通通讯的员工；开启部门群后，负责人作为群主加入群聊。
        </template>
      </a-form-item>
      <a-form-item label="部门群" field="im_group_enabled" style="margin-bottom:15px;">
        <a-switch data-testid="dept-group" v-model="formData.im_group_enabled" :checked-value="1" :unchecked-value="0">
          <template #checked>开</template>
          <template #unchecked>关</template>
        </a-switch>
        <template #extra>开启后自动创建并同步部门群。关闭后停止自动管理，保留已有群和聊天记录。</template>
      </a-form-item>
      <a-form-item field="remark" label="备注" validate-trigger="input" style="margin-bottom:15px;">
        <a-textarea v-model="formData.remark" placeholder="请填写备注" allow-clear/>
      </a-form-item>
    </a-form>
  </BasicModal>
</template>
<script lang="ts">
  import { defineComponent, ref, computed, unref, onBeforeUnmount} from 'vue';
  import { BasicModal, useModalInner } from '/@/components/Modal';
  import useLoading from '@/hooks/loading';
  import { useI18n } from 'vue-i18n';
  import { cloneDeep } from 'lodash-es';
  //api
  import { getParent, save, getLeaderOptions } from '@/api/system/dept';
  import { IconPicker ,Icon} from '@/components/Icon';
  import { FormInstance,Message } from '@arco-design/web-vue';
  export default defineComponent({
    name: 'AddForm',
    components: { BasicModal,IconPicker,Icon },
    emits: ['success'],
    setup(_, { emit }) {
      const { t } = useI18n();
      const isUpdate = ref(false);
      const parntList = ref([]);
      //表单
      const formRef = ref<FormInstance>();
      //表单字段
      const basedata={
            id:0,
            name: '',
            pid: 0,
            weigh: 1,
            remark:"",
            leader_uid: 0 as number | undefined,
            im_group_enabled: 0,
        }
      const formData = ref(basedata)
      const leaderLoading = ref(false);
      const leaderError = ref('');
      const leaderChoices = ref<{ value: number; label: string }[]>([]);
      const currentLeader = ref({ id: 0, name: '' });
      let leaderRequest = 0;
      let leaderController: AbortController | undefined;
      let leaderLoaded = false;
      let leaderKeyword = '';
      let formActive = false;
      const cancelLeaderSearch = () => {
        ++leaderRequest;
        leaderController?.abort();
        leaderController = undefined;
        leaderLoading.value = false;
      };
      const handleModalVisibility = (visible: boolean) => {
        if (!visible) {
          formActive = false;
          cancelLeaderSearch();
        }
      };
      onBeforeUnmount(() => handleModalVisibility(false));
      const leaderOptions = computed(() => {
        const options = [...leaderChoices.value];
        const selected = Number(formData.value.leader_uid);
        if (selected && selected === currentLeader.value.id && !options.some(item => item.value === selected)) {
          options.unshift({ value: selected, label: currentLeader.value.name || '当前负责人' });
        }
        return options;
      });
      const searchLeaders = async (keyword = '') => {
        if (!formActive) return;
        leaderController?.abort();
        const controller = new AbortController();
        leaderController = controller;
        const request = ++leaderRequest;
        leaderKeyword = keyword;
        leaderLoading.value = true;
        leaderError.value = '';
        try {
          const result = await getLeaderOptions({ keyword, dept_id: formData.value.id,
            parent_id: formData.value.pid, page: 1, pageSize: 100 }, controller.signal);
          if (request !== leaderRequest) return;
          if (!result || !Array.isArray(result.items)) throw new Error('Invalid leader options');
          leaderChoices.value = result.items.filter(item => Number.isSafeInteger(Number(item.id)) && Number(item.id) > 0).map(item => ({ value: Number(item.id),
            label: [item.name, item.employee_no ? `工号 ${item.employee_no}` : '', item.department].filter(Boolean).join(' · ') }));
          leaderLoaded = true;
        } catch {
          if (request === leaderRequest && !controller.signal.aborted) {
            leaderError.value = '部门负责人加载失败，请重试';
          }
        } finally {
          if (request === leaderRequest) {
            leaderLoading.value = false;
            leaderController = undefined;
          }
        }
      };
      const retryLeaders = () => searchLeaders(leaderKeyword);
      const handleLeaderPopup = (visible: boolean) => {
        if (visible && !leaderLoading.value && (!leaderLoaded || leaderError.value)) void retryLeaders();
      };
      const [registerModal, { setModalProps, closeModal }] = useModalInner(async (data) => {
          formRef.value?.resetFields()
          setModalProps({ confirmLoading: false });
          cancelLeaderSearch();
          formActive = true;
          leaderLoaded = false;
          leaderKeyword = '';
          leaderError.value = '';
          leaderChoices.value = [];
          isUpdate.value = !!data?.isUpdate;
          formData.value = { ...cloneDeep(basedata), ...(unref(isUpdate) ? cloneDeep(data.record) : { pid: data.pid }) };
          formData.value.leader_uid = Number(formData.value.leader_uid) || undefined;
          formData.value.im_group_enabled = Number(formData.value.im_group_enabled) === 1 ? 1 : 0;
          currentLeader.value = { id: Number(formData.value.leader_uid) || 0, name: data.record?.leader_name || '' };
          const resultdata = await getParent({id:data.record?data.record.id:0});
          const parntList_df : any=[{id: 0,name: "一级部门",pid: 0}];
          if(resultdata){
            parntList.value=parntList_df.concat(resultdata)
          }else{
            parntList.value=parntList_df
          }
      });
      const getTitle = computed(() => (!unref(isUpdate) ? '新增部门' : '编辑部门'));
     //点击确认
     const { loading, setLoading } = useLoading();
     const handleSubmit = async () => {
      try {
          const res = await formRef.value?.validate();
          if (!res) {
            if (formData.value.im_group_enabled === 1 && !Number(formData.value.leader_uid)) {
              Message.warning('开启部门群前，请先选择已开通通讯的部门负责人');
              return;
            }
            setLoading(true);
            Message.loading({content:"提交中",id:"save",duration:0})
            const { id, name, pid, weigh, remark, im_group_enabled } = formData.value;
            await save({ id, name, pid, weigh, remark, im_group_enabled, leader_uid: Number(formData.value.leader_uid) || 0 });
            Message.success({content:"提交成功",id:"save",duration:2000})
            closeModal()
            emit('success');
            setLoading(false);
          }
        } catch (error) {
          setLoading(false);
          Message.loading({content:"提交中",id:"save",duration:2})
        }
      };
      return { 
        registerModal, 
        getTitle, 
        handleSubmit,
        formRef,
        loading,
        formData,
        parntList,
        leaderOptions,
        leaderLoading,
        leaderError,
        searchLeaders,
        retryLeaders,
        handleLeaderPopup,
        handleModalVisibility,
        t,
        OYoptions:[
          { label: '否', value: 0 },
          { label: '是', value: 1 },
        ],
      };
    },
  });
</script>
