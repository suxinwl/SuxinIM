<template>
  <div class="container">
    <page-card breadcrumb :isFullscreen="isFullscreen">
      <div class="table-toolbar flex flex-between">
        <div class="left">
          <a-space v-if="viewType=='table'">
            <a-input :style="{width:'200px'}"  v-model="formModel.name" placeholder="搜索部门名称" allow-clear />
            <a-range-picker v-model="formModel.createtime" :style="{width:'230px'}" @change="fetchData"/>
            <a-select v-model="formModel.status"  :options="statusOptions" placeholder="状态" :style="{width:'90px'}" @change="fetchData"/>
            <a-button type="primary" @click="fetchData">
              <template #icon>
                <icon-search />
              </template>
              {{ $t('searchTable.form.search') }}
            </a-button>
            <a-button @click="reset">
              {{ $t('searchTable.form.reset') }}
            </a-button>
          </a-space>
          <div v-else-if="renderData.length>1">
            <a-radio-group v-model="parentDeptId" type="button" size="small" @change="(value) => changeParentDept(Number(value))">
              <a-radio v-for="(item,index) in renderData" :value="index">{{ item.name }}</a-radio>
            </a-radio-group>
          </div>
        </div>
        <div class="right">
          <tabletool :showbtn="tableToolData"
           @create="createData(0)" @refresh="fetchData" @selectdensity="(data)=>size=data" @fullscreen="(data)=>isFullscreen=data"></tabletool>
            <a-switch class="table-tree-view" v-model="viewType"  type="round" checked-value="table" unchecked-value="tree" checked-color="" unchecked-color="rgb(var(--orange-6))">
              <template #checked>
                表格视图
              </template>
              <template #unchecked>
                树结构图
              </template>
            </a-switch>
        </div>
      </div>
      <a-table
        v-if="viewType=='table'"
        row-key="id"
        :loading="loading"
        :pagination="false"
        :columns="(cloneColumns as TableColumnData[])"
        :data="renderData"
        :bordered="{wrapper:true,cell:true}"
        :size="size"
        :default-expand-all-rows="true"
        ref="artable"
        :scroll="{x:'100%'}"
        @change="handleChange" 
      >
        <template #title="{ record }">
          <span v-html="record.spacer" style="padding-right: 5px;color: var(--color-neutral-4);"></span>{{ record.name }}
        </template>
        <template #icon="{ record }">
          <Icon :icon="record.icon" :size="20"></Icon>
        </template>
        <template #createtime="{ record }">
          {{dayjs(record.createtime).format("YYYY-MM-DD")}}
        </template>
        <template #status="{ record }">
          <a-switch type="round" v-model="record.status" :checked-value="0" :unchecked-value="1" @change="handleStatus(record)">
              <template #checked>
                开
              </template>
              <template #unchecked>
                关
              </template>
            </a-switch>
        </template>
        <template #operations="{ record }">
          <a-space>
            <Icon icon="icon-plus-circle-fill" class="iconbtn" :size="18" color="rgb(var(--arcoblue-6))" @click="createData(record.id)" v-if="record.type!=2"></Icon>
            <Icon icon="svgfont-edit-square" class="iconbtn" @click="handleEdit(record)" :size="17" color="#0960bd"></Icon>
            <a-popconfirm content="您确定要删除吗?" @ok="handleDel(record)" position="tr">
              <Icon icon="svgfont-delete" class="iconbtn" :size="17" color="#ed6f6f"></Icon>
            </a-popconfirm>
          </a-space>
        </template>
        <template #imGroup="{ record }">
          <a-tooltip content="开启后自动创建并同步部门群；关闭后保留已有群和聊天记录。">
            <a-switch type="round" :data-dept-group="record.id" :model-value="Number(record.im_group_enabled) === 1"
              :loading="groupSaving.has(String(record.id))"
              :disabled="groupSaving.has(String(record.id))"
              @change="(value) => handleIMGroup(record, Boolean(value))">
              <template #checked>开</template>
              <template #unchecked>关</template>
            </a-switch>
          </a-tooltip>
        </template>
        <template #leader="{ record }">
          {{ record.leader_name || '未设置' }}
        </template>
      </a-table>
      <!-- 组织架构图视图 -->
      <div class="tree-view" v-show="viewType === 'tree'">
        <a-dropdown trigger="contextMenu">
          <vue3TreeOrg
            v-if="renderData.length"
            :data="renderData[parentDeptId]"
            :collapsable="true"
            :horizontal="false"
            :define-menus="menus"
            :expand-all="true"
            :clone-node-drag="false"
            :default-expand-level="999"
            :props="{ id: 'id', parentId: 'pid', label: 'name', children: 'children' }"
            center
            :node-add="handleAdd"
            :node-delete="onDelete"
            :node-edit="onUpdate"
            @on-node-drag-end="nodeDragEnd"
          >
          </vue3TreeOrg>
        </a-dropdown>
      </div>
    </page-card>
    <!--表单-->
    <AddForm  @register="registerModal" @success="handleData"/>
  </div>
</template>

<script lang="ts" setup>
  import 'vue3-tree-org/lib/vue3-tree-org.css'
  import { Vue3TreeOrg } from 'vue3-tree-org'
  import { computed,ref, onMounted,watch, nextTick } from 'vue';
  import useLoading from '@/hooks/loading';
  import { getList,upStatus,upIMGroup,del,dragDept} from '@/api/system/dept';
  import type { TableColumnData } from '@arco-design/web-vue/es/table/interface';
  import cloneDeep from 'lodash/cloneDeep';
  import dayjs from 'dayjs';
  //数据
  import { columns} from './data';
  //表单
  import AddForm from './AddForm.vue';
  import { useModal } from '/@/components/Modal';
  import {Icon} from '@/components/Icon';
  import { Message } from '@arco-design/web-vue';
  import {tabletool,SizeProps,statusOptions} from '/@/components/tabletool';
  const [registerModal, { openModal }] = useModal();
  type Column = TableColumnData & { checked?: true };
  const { loading, setLoading } = useLoading(true);
  const renderData = ref<any[]>([]);
  const cloneColumns = ref<Column[]>([]);
  const isFullscreen = ref(false);
  const size = ref<SizeProps>('large');
  const groupSaving = ref(new Set<string>());
  //树结构图
  //所有节点展开状态
  const parentDeptId = ref(0);
  const viewType = ref("table");
  const tableToolData = computed(() => (viewType.value=="table" ?['create','refresh','selectdensity','fullscreen'] : ['create','refresh','fullscreen']));
  // 组织架构图右键菜单
  const menus = [
    { name: '添加部门', command: 'add' },
    { name: '编辑部门', command: 'edit' },
    { name: '删除部门', command: 'delete' },
  ]
    //查询字段
    const generateFormModel = () => {
    return {
      name: '',
      createtime: [],
      status: '',
    };
  };
  const formModel = ref(generateFormModel());
  const artable=ref()
  const fetchData = async () => {
    setLoading(true);
    try {
      renderData.value= await getList(formModel.value);
      if(artable.value){
        nextTick(()=>{
          artable.value.expandAll()
        })
      }
    } catch (err) {
      // you can report use errorHandler or other
    } finally {
      setLoading(false);
    }
  };
  const reset = () => {
    formModel.value = generateFormModel();
    fetchData();
  };
  //初始化排序
  onMounted(() => {
    fetchData();
  })
  watch(
    () => columns.value,
    (val) => {
      cloneColumns.value = cloneDeep(val);
    },
    { deep: true, immediate: true }
  );
  //添加
  const createData=(id:number)=>{
    openModal(true, {
      isUpdate: false,
      pid:id
    });
  }
  //编辑数据
  const handleEdit=async(record:any)=>{
    openModal(true, {
      isUpdate: true,
      record:record
    });
  }
  //更新数据
  const handleData=async()=>{
    fetchData();
  }
  //排序拖拽
  const handleChange = (_data:any) => {
    renderData.value = _data
  }
  //更新状态
  const handleStatus=async(record:any)=>{
    try {
      Message.loading({content:"更新状态中",id:"upStatus",duration:0})
      await upStatus({id:record.id,status:record.status});
      Message.success({content:"更新状态成功",id:"upStatus",duration:2000})
    }catch (error) {
      record.status=record.status==1?0:1
      Message.loading({content:"更新状态中",id:"upStatus",duration:1})
    } 
  }
  const handleIMGroup = async (record: any, enabled: boolean) => {
    const id = String(record.id);
    if (groupSaving.value.has(id)) return;
    if (enabled && !Number(record.leader_uid)) {
      Message.warning('请先为部门设置已开通通讯的负责人，再开启部门群');
      handleEdit(record);
      return;
    }
    groupSaving.value.add(id);
    try {
      await upIMGroup({ id: Number(record.id), im_group_enabled: enabled ? 1 : 0 });
      record.im_group_enabled = enabled ? 1 : 0;
      Message.success(enabled ? '已开启部门群自动管理' : '已关闭自动管理，已有群和聊天记录保留');
    } catch {
      // The API client displays the server error; keep the original switch value.
    } finally {
      groupSaving.value.delete(id);
    }
  };
  //删除数据
  const handleDel=async(record:any)=>{
    try {
      Message.loading({content:"删除中",id:"del",duration:0})
      await del({ids:[record.id]});
      fetchData();
      Message.success({content:"删除成功",id:"del",duration:2000})
    }catch (error) {
      Message.loading({content:"删除中",id:"del",duration:1})
    } 
  }
  //组织-添加架构
  const handleAdd = (record: any) => {
    createData(record.id)
  }
  // 删除部门
  const onDelete = (record: any) => {
    handleDel(record)
  }
  // 修改部门
  const onUpdate = (record: any) => {
    handleEdit(record)
  }
  // 拖拽
  const nodeDragEnd=async(data:any, isSelf:any)=>{
    if(data&&isSelf&&data.id!=isSelf.id){
      try {
        Message.loading({content:"移动部门中",id:"upStatus",duration:0})
        await dragDept({id:data.id,pid:isSelf.id});
        Message.success({content:"移动部门成功",id:"upStatus",duration:2000})
      }catch (error) {
        Message.loading({content:"移动部门中",id:"upStatus",duration:1})
      } 
    }
  }
  //切换一级分类
  const changeParentDept=async(index:number)=>{
     parentDeptId.value=index
  }
</script>

<script lang="ts">
  export default {
    name: 'dept',//页签是名字和路由name相同则缓存生效
  };
</script>

<style scoped lang="less">
  :deep(.arco-table-th) {
    &:last-child {
      .arco-table-th-item-title {
        margin-left: 16px;
      }
    }
  }
  .action-icon {
    margin-left: 12px;
    cursor: pointer;
  }
  .active {
    color: #0960bd;
    background-color: #e3f4fc;
  }
  .setting {
    display: flex;
    align-items: center;
    width: 200px;
    .title {
      margin-left: 12px;
      cursor: pointer;
    }
  }
  :deep(.general-card > .arco-card-header){
    padding: 10px 16px;
  }
  .iconbtn{
    user-select: none;
    cursor: pointer;
    opacity: .8;
    &:hover{
      opacity: 1;
    }
  }
  .table-tree-view{
    height: 30px;
    line-height: 30px;
    :deep(.arco-switch-handle){
      height: 22px;
      width: 15px;
    }
  }
  .tree-view{
    height: calc(100% - 40px);
    position: relative;
    .zm-tree-org{
      padding: 0;
    }
  }
  .table-toolbar{
    overflow-x: auto;
    white-space: nowrap; 
  }
  .container{
   :hover{
     .table-toolbar{
         scrollbar-width: unset;
      }
    }
  }
  //组织架构
  :deep(.zm-draggable) {
    margin-top: 4px;
  }

  :deep(.zm-tree-org .zoom-container) {
    background-color: var(--color-bg-1);
    color: var(--color-text-1);
  }

  :deep(.tree-org-node__content) {
    background-color: var(--color-bg-2);
    color: var(--color-text-1);
    cursor: pointer;
    position: relative;
  }

  .zm-tree-org {
    background-color: var(--color-bg-1);
    height: calc(100vh - 265px);
  }
  :global(.zm-tree-contextmenu) {
    color: var(--color-text-1) !important;
    position: fixed !important;
    background: var(--color-bg-2) !important;
    box-shadow: 0 4px 10px rgba(0, 0, 0, 0.1) !important;
    border: 1px solid var(--color-border) !important;
    border-radius: 4px !important;
    padding: 4px 0 !important;
    min-width: 120px !important;
    z-index: 999 !important;
     ul {
      background: var(--color-bg-1) !important;
      list-style-type: none !important;
      padding: 10px !important;
      margin: 0 !important;
    }

  .zm-tree-menu-item {
    background-color: var(--color-bg-1) !important;
    padding: 10px 10px !important;
    margin-top: 10px !important;
    cursor: pointer !important;
    transition: background-color 0.1s ease !important;
    list-style: none !important;
  }
  }
  :deep(.tree-org-node__expand){
    background-color: var(--color-bg-1) !important;
  }
</style>
<style >
   .zm-tree-contextmenu li {
      font-size: 13px;
      padding: 10px 10px;
      border-radius: 3px;
  }
</style>
