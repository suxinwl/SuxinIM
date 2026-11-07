<template>
  <a-drawer :width="450" :visible="showDrawer" :ok-loading="loading || optionsLoading" @ok="handleSubmit" @cancel="handleCancel" unmountOnClose>
      <template #title>{{getTitle}}</template>
      <div class="drawer-box">
        <a-form ref="formRef" :model="formData" auto-label-width>
          <a-form-item field="name" label="角色名称" validate-trigger="input" :rules="[{required:true,message:'请填写角色名称'}]" style="margin-bottom:15px;">
            <a-input v-model="formData.name" placeholder="请填写角色名称" allow-clear/>
          </a-form-item>
          <a-form-item label="上级角色" field="pid" style="margin-bottom:15px;">
            <a-tree-select placeholder="选择上级角色" :data="parntList"
            :fieldNames="{
                key: 'id',
                title: 'name',
                children: 'children'
              }"
            v-model="formData.pid" @change="handleUpRolsdata">
            </a-tree-select>
          </a-form-item>
          <a-form-item field="remark" label="备注" validate-trigger="input" style="margin-bottom:15px;">
            <a-textarea v-model="formData.remark" placeholder="请填写备注" allow-clear/>
          </a-form-item>
           <a-form-item field="data_access" label="数据权限" style="margin-bottom:5px;">
            <a-radio-group v-model="formData.data_access" :options="OYoptions" />
           </a-form-item>
           <a-form-item field="menu" label="菜单分配" style="margin-bottom:5px;">
            <div class="rule_data">
              <div class="haeder">
                  <a-checkbox v-if="!isRules" v-model="IsChecked" @change="toggleChecked" style="margin-right: 15px;"> {{ IsChecked ? '取消' : '全选' }}</a-checkbox>
                  <a-checkbox v-model="onExpanded" @change="toggleExpanded">{{ onExpanded? '收起' : '展开'  }}</a-checkbox>
              </div>
              <div class="treebox">
                <a-checkbox-group v-model="formData.btns">
                <a-tree
                    :show-line="true"
                    :checkable="!isRules"
                    v-model:checked-keys="formData.menu"
                    :fieldNames="{
                      key: 'id',
                      title: 'title',
                      children: 'children',
                    }"
                    ref="ruleTreeRef"
                    @check="onCheck"
                    :data="menutreeData"
                  >
                   <!--标题-->
                   <template #title="record" >
                    <template v-if="record.checkable">
                      {{ record.title }}
                    </template>
                     <template v-else>
                      <a-tooltip  v-for="item in record.btn_rules" :content="item.des?item.des:item.title"  position="top" mini>
                       <a-checkbox :value="item.id">{{ item.title }}</a-checkbox>
                      </a-tooltip>
                      </template>
                    </template>
                  </a-tree>
                </a-checkbox-group>
              </div>
            </div>
           </a-form-item>
        </a-form>
      </div>
    </a-drawer>
</template>
<script lang="ts">
  import { defineComponent, ref,nextTick, computed, unref} from 'vue';
  import useLoading from '@/hooks/loading';
  //api
  import { getParent,getMenuList, save,RuleItem } from '@/api/system/role';
  import { Icon} from '@/components/Icon';
  import { FormInstance,Message } from '@arco-design/web-vue';
  import { TreeItem } from './data';
  import { createRoleForm, roleButtonsAfterMenuCheck } from './form-state';


  export default defineComponent({
    name: 'AddDrawer',
    components: { Icon },
    emits: ['success'],
    setup(_, { emit }) {
      const showDrawer = ref(false);
      const isUpdate = ref(false);
      const parntList = ref<RuleItem[]>([]);
      //选择按钮权限
      const btnRuleIds=ref<number []>([])//全部按钮数据id
      //表单
      const formRef = ref<FormInstance>();
      //表单字段
      const formData = ref(createRoleForm());
      const optionsLoading = ref(false);
      //菜单权限
      const isRules=ref(false)
      const IsChecked=ref(false)
      const onExpanded=ref(false)
      const ruleTreeRef = ref();
      const menutreeData = ref<TreeItem[]>([]);
      //打开弹框
      const openDrawer=async(item:any)=>{
        formData.value=createRoleForm();
        isRules.value=false;
        IsChecked.value=false;
        onExpanded.value=false;
        btnRuleIds.value=[];
        menutreeData.value=[];
        parntList.value=[];
        isUpdate.value = !!item?.isUpdate;
        showDrawer.value = true;
        optionsLoading.value=true;
        try {
          formData.value=createRoleForm(unref(isUpdate) ? item.record : null);
          isRules.value=unref(isUpdate) && item.record.rules === '*';
          const resultdata = await getParent({id:item.record?item.record.id:0});
          if(resultdata&&resultdata.length>0){
            parntList.value=resultdata
            if (!unref(isUpdate)) {
              formData.value.pid=resultdata[0].id
            }else{
              if(item.record.pid==0){
                const parntList_df : any=[{id: 0,name: "一级角色",pid: 0,locale:""}];
                parntList.value=parntList_df.concat(resultdata)
              }
            }
          }else{
            parntList.value=[]
          }
          //获取菜单
          await getRuls(formData.value.pid);
        } catch (error) {
          Message.error(error instanceof Error ? error.message : '角色信息加载失败');
          showDrawer.value=false;
        } finally {
          optionsLoading.value=false;
        }
      }
      //选择上级角色同时更新菜单数据
      const handleUpRolsdata=()=>{
        formData.value.menu=[];
        formData.value.btns=[];
        IsChecked.value=false;
        getRuls(formData.value.pid)
      }
      const getRuls=async(pid:number)=>{
        const menudata =await getMenuList({pid:pid})
         menutreeData.value=(menudata.list) as any as TreeItem[]
         btnRuleIds.value=(menudata.btn_rule_ids) as any as number[]
          nextTick(()=>{
            onExpanded.value=true
            ruleTreeRef.value?.expandAll(onExpanded.value)
          })
      }
      const getTitle = computed(() => (!unref(isUpdate) ? '新增角色' : '编辑角色'));
     //点击确认
     const { loading, setLoading } = useLoading();
     const handleSubmit = async () => {
      if (loading.value || optionsLoading.value) return;
      try {
          const res = await formRef.value?.validate();
          if (!res) {
            setLoading(true);
            Message.loading({content:"提交数据中",id:"save",duration:0})
            await save(unref(formData));
            Message.success({content:"数据提交成功",id:"save",duration:2000})
            emit('success');
            showDrawer.value = false;
            setLoading(false);
          }
        } catch (error) {
          setLoading(false);
          Message.loading({content:"提交数据中",id:"save",duration:1})
        }
      };
      const handleCancel = () => {
        showDrawer.value = false;
      }
      //权限菜单
     //全选
     const toggleChecked=(value:any) =>{
       ruleTreeRef.value.checkAll(value)
       //按钮权限
       if(value){
        formData.value.btns=btnRuleIds.value
       }else{
        formData.value.btns=[]
       }
      }
      //展开
      const toggleExpanded=(value:any) =>{
          ruleTreeRef.value.expandAll(value)
      }
      //选中
      const onCheck=(newCheckedKeys:any, event:any)=>{
        formData.value.menu=newCheckedKeys
        formData.value.btns=roleButtonsAfterMenuCheck(
          formData.value.btns, event.checked === true, event.node?.btnids
        )
      }
      return { 
        showDrawer,handleCancel,openDrawer,
        getTitle, 
        handleSubmit,
        formRef,handleUpRolsdata,
        loading,
        optionsLoading,
        formData,
        parntList,
        OYoptions:[
          { label: '自己', value: 0 },
          { label: '自己及子权限', value: 1 },
          { label: '全部', value: 2 },
        ],
        menutreeData,
        onCheck,
        toggleChecked,
        toggleExpanded,
        ruleTreeRef,onExpanded,
        IsChecked,
        isRules,//是否是超级权限
      };
    },
  });
</script>
<style lang="less" scoped>
.drawer-box{
  .rule_data{
    width: 100%;
    .haeder{
      border-bottom:var(--color-neutral-4) solid 1px;
    }
  }
}
</style>
