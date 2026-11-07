<template>
  <BasicModal v-bind="$attrs" @register="registerModal" :ok-text="okText" :isPadding="false" :maskClosable="false" :loading="loading" width="1100px" :minHeight="600" :title="getTitle" @ok="handleSubmit">
    <div style="height:100%;">
        <a-tabs class="tabs-wrap" type="card-gutter" size="large" justify v-model:active-key="currentindex">
          <a-tab-pane :key="1" title="代码打包">
            <div class="formbox" v-if="currentindex==1">
              <a-form ref="formRef" :model="formData" auto-label-width >
                <a-row :gutter="16">
                  <a-col :span="12">
                    <a-form-item field="name" label="包名" :rules="[{required:true,message:'请填写包名'}]" >
                        <a-input v-model="formData.name" placeholder="请填包名、插件标识（以字母开头、数字命名）" @input="filterName" allow-clear @change="handlePackNameChange"/>
                    </a-form-item>
                    <a-form-item field="title" label="名称" :rules="[{required:true,message:'请填写包名称'}]" >
                        <a-input v-model="formData.title" placeholder="请填包名称（标题）" allow-clear/>
                    </a-form-item>
                    <a-form-item field="version" label="版本号" :rules="[{required:true,message:'请填写版本号'}]" >
                        <a-input v-model="formData.version" placeholder="请填版本号" allow-clear/>
                    </a-form-item>
                    <a-form-item field="packtables" label="数据表" style="margin-bottom: 10px;">
                      <a-select v-model="formData.packtables" placeholder="选择选择导入的数据表" allow-search multiple allow-clear>
                        <template #label="{ data }">
                            <span>{{data?.value}}</span>
                          </template>
                          <a-option v-for="item in tablelist" :value="item.name">{{ item.name+" "+item.title }}</a-option>
                      </a-select>
                    </a-form-item>
                    <a-form-item field="des" label="描述" style="margin-bottom: 10px;">
                        <a-textarea v-model="formData.des" placeholder="请填说明" :auto-size="{minRows:2,maxRows:3}" allow-clear/>
                    </a-form-item>
                    <a-form-item field="menujson" label="后台菜单" style="margin-bottom: 10px;">
                      <div class="menubox">
                        <div class="menutabar">
                            <a-radio-group type="button" v-model="bmenuboxbar">
                              <a-radio value="selectemenu">选择菜单</a-radio>
                              <a-radio value="editmenu">编辑菜单</a-radio>
                            </a-radio-group>
                          </div>
                          <div class="selectemenu" v-if="bmenuboxbar=='selectemenu'">
                              <a-tree-select
                                v-model="formData.menutree" 
                                :allow-search="true"
                                :allow-clear="true"
                                :tree-checkable="true"
                                :data="MenutreeData"
                                :loading="loadingMenu"
                                placeholder="选择系统菜单"
                                :fieldNames="{
                                    key: 'id',
                                    title: 'title',
                                    children: 'children'
                                  }"
                                @change="handleMarkeMenuBusiness"
                                ></a-tree-select>
                          </div>
                          <div class="parambox" v-else>
                            <CodeEditor v-model:value="formData.menujson" :mode="modeValue" />
                          </div>
                      </div>
                    </a-form-item>
                    <a-form-item field="installcover" label="安装方式" style="margin-bottom: 10px;">
                      <a-radio-group v-model="formData.installcover">
                          <a-radio :value="false">新模块安装</a-radio>
                          <a-radio :value="true">覆盖原有代码（如登录界面）</a-radio>
                        </a-radio-group>
                    </a-form-item>
                    <a-form-item field="isModTidy" label="更新依赖" style="margin-bottom: 10px;">
                      <a-checkbox v-model="formData.isModTidy" :value="true"><strong>安装时</strong>Go后端是否拉取缺失依赖(插件使用新的依赖时选上)</a-checkbox>
                    </a-form-item>
                  <a-form-item field="commandLines" label="前端依赖" style="margin-bottom: 10px;" help="只需填写依赖包名，去掉yarn add或npm install">
                      <a-input v-model="formData.commandLines" placeholder="如果有新package依赖包则填写运行命令(多个用,分隔)" allow-clear/>
                    </a-form-item>
                  </a-col>
                  <a-col :span="12">
                  <div class="borderbox borderbox-scroll" >
                      <div class="borderItem" >
                        <div class="formGoDir">
                          <div class="go-code-list">
                            <div class="file-dir-list flex flex-middle" v-for="(item,index) in formData.goFiles">
                              <div class="file-dir-text flex_1"><Icon :icon="item.isDir?'svgfont-wenjianjia-1':'svgfont-file_golang'" :size="16"></Icon> {{ item.path }}</div>
                              <a-tooltip content="删除该项数据">
                              <Icon icon="icon-close-circle-fill" class="iconbtnc" :size="22" color="rgb(var(--red-6))" @click="delDrir(item.path,'go')"></Icon>
                              </a-tooltip>
                            </div>
                             <a-empty class="file-dir-empty" @click="openSelectGoFile('go')" v-if="formData.goFiles.length==0" description="还没选择后端文件哦！点击下面按钮选择"/>
                          </div>
                        </div>
                           <div class="go-code-btn flex">
                            <div class="go-btn-wrap" @click="openSelectGoFile('go')">选择后端代码</div>
                          </div>
                      </div>
                    </div>
                    <div class="borderbox" style="margin-top: 10px;">
                       <div class="borderItem" >
                        <div class="formGoDir">
                          <div class="go-code-list">
                            <div class="file-dir-list flex flex-middle" v-for="(item,index) in formData.vueFiles">
                              <div class="file-dir-text flex_1"><Icon :icon="item.isDir?'svgfont-wenjianjia-1':'svgfont-File-format-vue'" :size="16"></Icon> {{ item.path }}</div>
                              <a-tooltip content="删除该项数据">
                              <Icon icon="icon-close-circle-fill" class="iconbtnc" :size="22" color="rgb(var(--red-6))" @click="delDrir(item.path,'vue')"></Icon>
                              </a-tooltip>
                            </div>
                             <a-empty class="file-dir-empty" @click="openSelectGoFile('vue')" v-if="formData.vueFiles.length==0" description="还没选择前端文件哦！点击下面按钮选择"/>
                          </div>
                        </div>
                           <div class="go-code-btn flex">
                            <div class="go-btn-wrap" @click="openSelectGoFile('vue')">选择前端代码</div>
                          </div>
                      </div>
                  </div>
                  </a-col>
                  <a-col :span="24" v-if="tabwarehouse=='public'">
                      <a-alert title="打包说明">
                        1.后端文件打包只需打包自己编写过的代码，如：api、internal\controller控制器、internal\logic实现层代码，其他使用命令生成代码不用打包，如：路由、dao、service等这些安装是自动生成。<br>
                        2.前端也是只需打包自己新开发功能即可，建议在开发时把插件对应的功能放在一个文件目录内，如接口api、附件资源等。
                      </a-alert>
                    </a-col>
                </a-row>
              </a-form>
            </div>
          </a-tab-pane>


          <a-tab-pane :key="2" title="上传代码包到代码仓（筹备中）" disabled>
            <div class="formbox" v-if="currentindex==2">
              <a-form ref="formRef" :model="formData" auto-label-width >
                <a-row :gutter="16">
                    <a-col :span="12">
                      <a-form-item field="name" label="包目录名" :rules="{required:true,message:'请填写包目录名'}" >
                          <a-input v-model="formData.name" placeholder="请填包目录名（以字母开头、数字命名）" @input="filterName" allow-clear/>
                      </a-form-item>
                      <a-form-item field="title" label="名称" :rules="[{required:true,message:'请填写包名称'}]" >
                          <a-input v-model="formData.title" placeholder="请填包名称（标题）" allow-clear/>
                      </a-form-item>
                      <a-row :gutter="16">
                      <a-col :span="14"> 
                        <a-form-item field="version" label="版本号" :rules="[{required:true,message:'请填写版本号'}]" >
                            <a-input v-model="formData.version" placeholder="请填版本号" allow-clear/>
                        </a-form-item>
                      </a-col>
                      <a-col :span="10"> 
                        <a-form-item field="price" label="价格" :label-col-style="{flex:'auto'}">
                            <a-input-number v-model="formData.price" :min="0" placeholder="请填价格" allow-clear/>
                        </a-form-item>
                      </a-col>
                      </a-row>
                      <a-form-item field="cid" label="选择分类" validate-trigger="input" :rules="[{required:true,message:'请选择分类'}]">
                        <a-select v-model="formData.cid" :options="cateList" allow-search :field-names="{value: 'id', label: 'name'}" placeholder="请选择分类" allow-clear/>
                      </a-form-item>
                    <a-form-item field="des" label="描述" style="margin-bottom: 10px;">
                        <a-textarea v-model="formData.des" placeholder="请填说明" :auto-size="{minRows:3,maxRows:3}" allow-clear/>
                    </a-form-item>
                    </a-col>
                    <a-col :span="12">
                      <div class="borderbox" style="margin-top: 10px;min-height:220px;">
                        <span class="title">选择代码上传</span>
                        <div class="borderItem upfilebox" style="margin-top: 10px;">
                            <a-upload draggable action="/" @change="onChangeUpload" ref="uploadRef" :show-file-list="formData.goframe_file?false:true" :limit="1" accept=".zip" :custom-request="customRequest" :auto-upload="false"/>
                            <div class="showfile" v-if="formData.goframe_file">
                                <div class="icon"><Icon icon="svgfont-yasuowenjian" class="tbicon" :size="14" ></Icon></div>
                                <div class="name">{{ upfileName }}</div>
                                <div class="url"><a :href="formData.goframe_file" target="_blank">附件加载地址</a></div>
                            </div>
                        </div>
                      </div>
                      <div class="dirtig">
                        复制打包代码位置：
                        <a-link @click="copyText(codepack)" style=" word-break: break-all;">{{ codepack }}</a-link>
                      </div>
                    </a-col>
                    <a-col :span="24" v-if="tabwarehouse=='public'">
                      <a-alert title="上传公共仓注意事项">
                        1.上传代码仓后到个人中心（<a href="https://www.suxinwl.com/" target="_blank">Suxin快速开发社区</a>）的代码仓插件管理编辑插件的介绍详情和上传封面图。<br>
                        2.如果有开发文档再到点到查看插件详情的右上角编辑开发文档。
                      </a-alert>
                    </a-col>
                  </a-row>
                </a-form>
            </div>
          </a-tab-pane>
        </a-tabs>
        <!--表单-->
        <GetDirForm @register="registerGoDirFormModal" @ok="handleOk"/>
    </div>
  </BasicModal>
  <!--登录-->
  <Login ref="loginRef" @ok="loginOk"></Login>
</template>
<script lang="ts">
  import { defineComponent, ref, computed} from 'vue';
  import { BasicModal, useModalInner,useModal } from '/@/components/Modal';
  import useLoading from '@/hooks/loading';
  //api
  import { packCode,userUploadFile } from '@/api/developer/packinstall';
  import { getTables,TableItem } from '@/api/common';
  import { upPackToService,getCodeCate,getMenutree,menuTreeToJson } from '@/api/developer/codestore';
  import { FormInstance,Message,TreeNodeData} from '@arco-design/web-vue';
  import { cloneDeep } from 'lodash';
  import { CodeEditor, MODE } from '/@/components/CodeEditor';
  import type { RequestOption} from '@arco-design/web-vue/es/upload/interfaces';
  import {Icon} from '@/components/Icon';
  import { useClipboard } from '@vueuse/core';
  import Login from "./Login.vue"; 
  import GetDirForm from './GetDirForm.vue';
  export default defineComponent({
    name: 'AddBook',
    components: { BasicModal,CodeEditor,Icon,Login,GetDirForm },
    emits: ['success'],
    setup(_, { emit }) {
      const [registerGoDirFormModal, { openModal:openGoDirFormModal }] = useModal();
      const uploadRef = ref();
      const loginRef=ref()
      const MenutreeData = ref<TreeNodeData[]>([]);
      const tablelist = ref<TableItem[]>([]);
      const applist = ref<string[]>([]);
      const utilstool = ref<string[]>([]);
      const loadingMenu = ref(true);
      const currentindex = ref(1);
      const bmenuboxbar = ref("selectemenu");
      const upfileName=ref("");
      const upFileList=ref<any[]>([]);
      const { copy } = useClipboard();
      //表单内容
      const formData = ref<any>({
        name:"",
        title:"",
        version:"1.0.0",
        price:10,
        cid: 1,
        des:"",
        installcover:false,
        isModTidy:false,
        commandLines:"",
        //后端
        goFiles:[],
        //前端端
        vueFiles:[],
        //数据库
        packtables:[],
        menujson:[],//后端菜单json
        menutree:[],//后端菜单选中
        goframe_file:"",
      });
      const domian=ref("")
      const code_token=ref("")
      const cateList = ref<TreeNodeData[]>([]);
      const tabwarehouse=ref("")
      const codepack=ref("")
      const [registerModal, { setModalProps, closeModal }] = useModalInner(async (data) => {
          setModalProps({ confirmLoading: false })
            domian.value=data.record.domian
            code_token.value=data.record.code_token
            tabwarehouse.value=data.record.tabwarehouse
            //获取数据表
            tablelist.value = await getTables({});
            const MemuData= await getMenutree({});
            if(MemuData){
              MenutreeData.value= MemuData;
            }
            loadingMenu.value=false
            cateList.value = []
            currentindex.value = 1
      });
      const getTitle = computed(() => '本地代码打包');
      const okText = computed(() => ( currentindex.value==1?"打包代码":"上传代码到仓库"));
     //点击安装
     const { loading, setLoading } = useLoading();
     const formRef = ref<FormInstance>();
     const handleSubmit =async () => {
      if (currentindex.value !== 1) { Message.info("在线服务筹备中，将迁移至 Suxin 官网"); return; }
      try {
          const res = await formRef.value?.validate();
          if (!res) {
              if(currentindex.value==1){
                setLoading(true);
                Message.loading({content:"打包中...",id:"PackCode",duration:0})
                var savedata=cloneDeep(formData.value)
                if(savedata.packtables){
                  savedata=Object.assign({},savedata,{packtables:savedata.packtables.join()})
                }
                await packCode(savedata);
                Message.success({content:"打包成功",id:"PackCode",duration:2000})
                closeModal()
                emit('success');
                setLoading(false);
              }else{//提交到代码仓
                if(tabwarehouse.value=="public"&&!code_token.value){//登录窗口
                  Message.warning({content:"上传代码到公共仓，请先登录",id:"PackCode",duration:2000})     
                  loginRef.value.showLogin(domian.value)
                  return;
                }
                if(formData.value.goframe_file){
                  upCodeToServer()
                }else{
                  if(upFileList.value.length==0){
                    Message.warning({content:"请先选择插件代码再上传操作",id:"PackCode",duration:2000})     
                    return;
                  }
                  uploadRef.value.submit();
                }
              }
          }else{
             Message.warning({content:"请填写必填选项",id:"PackCode",duration:2000})     
          }
        } catch (error) {
          setLoading(false);
          Message.loading({content:currentindex.value==1?"打包中...":"提交代码包到仓库中...",id:"PackCode",duration:1})
        }
    }
      const modeValue = ref<MODE>(MODE.JSON);
      //菜单选择变化时同时修改菜单json
      const handleMarkeMenuBusiness=async()=>{
        const res= await menuTreeToJson({menu:formData.value.menutree});
        if(res){
          formData.value.menujson=res
        }else{
          formData.value.menujson=[]
        }
      }
      //上传附件
      const customRequest = (options: RequestOption) => {
          Message.loading({content:"上传文件中...",id:"PackCode",duration:0})
          const controller = new AbortController();
            (async function requestWrap() {
              const {
                onProgress,
                onError,
                onSuccess,
                fileItem,
              } = options;
              onProgress(20);
              const onUploadProgress = (event: ProgressEvent) => {
                let percent;
                if (event.total > 0) {
                  percent = (event.loaded / event.total) * 100;
                }
                onProgress(parseInt(String(percent), 10), event);
              };
              try {
                //开始手动上传
                const filename=fileItem?.name||""
                const resdata = await userUploadFile({ name: 'file', file: fileItem.file as Blob, filename,data:{cid:0,domainurl:domian.value,code_token:code_token.value}},onUploadProgress);
                //更新返回文件地址
                if(resdata.code==0){
                  upfileName.value=filename
                  formData.value.goframe_file=resdata.data.url
                  upCodeToServer()
                  Message.success({content:"上传文件成功",id:"PackCode",duration:2000})
                }else{
                  uploadRef.value.abort()
                  Message.warning({content:resdata.message,id:"PackCode",duration:2000})   
                }
              } catch (error) {
                onError(error);
                Message.loading({content:"上传文件中...",id:"PackCode",duration:1})
              }
            })();
            return {
              abort() {
                controller.abort();
              },
            };
      };
     //上传代码到代码仓服务器（先上传附件）
     const upCodeToServer=async()=>{
          if(!formData.value.goframe_file){
                Message.warning({content:"没有找到代码包",id:"PackCode",duration:2000})         
            }else{
              try {
                setLoading(true);
                Message.loading({content:"提交代码包到仓库中...",id:"PackCode",duration:0})
                var savedata=cloneDeep(formData.value)
                if(savedata.packtables){
                  savedata=Object.assign({},savedata,{packtables:savedata.packtables.join(),code_token:code_token.value})
                }
                await upPackToService(savedata,domian.value);
                Message.success({content:"提交代码包到仓库成功",id:"PackCode",duration:2000})
                formData.value.goframe_file="";
                upFileList.value=[];
                formRef.value?.resetFields()
                closeModal()
                emit('success');
                setLoading(false);
              } catch (error) {
                 setLoading(false);
                Message.loading({content:"提交代码包到仓库中...",id:"PackCode",duration:1})
              }
            }
      }
      //复制文本
      const copyText=async(text:any)=>{
        await copy(text);
        Message.success("复制成功");
      }
      //过滤报包名汉字
      const filterName=async(strValue:any)=>{
        var reg = /[\u4e00-\u9fa5]/g;   
        formData.value.name=strValue.replace(reg, "");   
      }
      //登录成功
      const loginOk=(val:any)=>{
        code_token.value=val
      }
      //包名填好后自动选择与包名相关数据表
      const handlePackNameChange=(val:any)=>{
        //1.选择数据库表
        if(formData.value.packtables.length==0){
          const gettablelist= tablelist.value.filter((item:any) => {
              return item.name.indexOf(val) > -1;
          });
          gettablelist.forEach((item:any)=>{
            formData.value.packtables.push(item.name)
          })
        }
      }
      //选择后端文件弹框
      const openSelectGoFile=(type:string)=>{
          openGoDirFormModal(true, {
            type: type,
            record:type=="go"?cloneDeep(formData.value.goFiles):cloneDeep(formData.value.vueFiles)
          });
      }
      //选择go目录
      const handleOk=(val:any,dirType:string)=>{
        if(dirType=="go"){
           formData.value.goFiles=val
        }else{
           formData.value.vueFiles=val
        }
      }
      //删除选择的go、vue目录数据
      const delDrir=(path:any,type:string)=>{
        if(type=="go"){
          formData.value.goFiles=formData.value.goFiles.filter(((item:any)=>item.path!=path))
        }else{
          formData.value.vueFiles=formData.value.vueFiles.filter(((item:any)=>item.path!=path))
        }
      }
      //上传的文件状态发生改变时触发
      const onChangeUpload=(fileList:any,fileItem:any)=>{
        upFileList.value=fileList
      }
      return { 
        registerModal, 
        getTitle, 
        loading,
        currentindex,
        formData,
        tablelist,applist,utilstool,
        handleSubmit,
        modeValue,
        bmenuboxbar,
        MenutreeData,
        handleMarkeMenuBusiness,
        loadingMenu,
        customRequest,
        upfileName,
        cateList,
        codepack,
        copyText,
        okText,
        formRef,
        filterName,
        loginOk,loginRef,
        tabwarehouse,
        handlePackNameChange,
        openSelectGoFile,
        registerGoDirFormModal,
        handleOk,delDrir,uploadRef,
        onChangeUpload,
      };
    },
  });
</script>
<style lang="less" scoped>
 .tabs-wrap{
  padding-top: 3px;
 }
  :deep(.arco-tabs-content){
    padding-top: 0px;
  }
  .formbox{
    padding: 15px;
  }
  :deep(.arco-tabs-type-card-gutter > .arco-tabs-content){
    border: 0 !important;
  }
  .go-code-btn{
    justify-content: center;
    .go-btn-wrap{
      text-align: center;
      border: 1px dashed var(--color-border-3);
      border-radius: 4px;
      height: 32px;
      line-height: 32px;
      width: 100%;
      margin-right: 13px;
      margin-bottom: -5px;
      margin-top: 5px;
      box-sizing: border-box;
      user-select: none;
      cursor: pointer;
      &:hover{
        opacity: 0.8;
      }
    }
  }
  .borderbox{
    width: 100%;
    border: var(--color-neutral-4) solid 1px;
    padding: 10px;
    border-radius: 5px;
    position: relative;
    .btn-title{
      position: absolute;
      top: 0%;
      left: 50%;
      transform: translate(-50%,-50%);
    }
    .borderItem{
      width: 100%;
      .tabheard{
        font-weight: 700;
        font-size: var(--size-4);
        margin-bottom: 8px;
      }
    }
  //选择后端目录
  &-scroll{
    padding-right: 5px;
  }
  .formGoDir{
    height: 180px;
    overflow-y: auto;
    padding-right: 5px;
    .file-dir-empty{
      margin-top: 30px;
    }
    .file-dir-list{
     border: 1px var(--color-neutral-2) solid;
     border-radius: 3px;
     padding: 5px 5px;
     margin-bottom: 5px;
     .file-dir-text{
      overflow: hidden;
      white-space: nowrap;
     }
     .iconbtnc{
      transform: scale(0);
        transition: .3s cubic-bezier(.25, .8, .5, 1);
     }
     &:hover{
        .iconbtnc{
         transform: scale(1);
        }
        .file-dir-text{
          overflow-x: auto;
        }
     }
    }
  }
  }
  .savebtn{
    text-align: center;
    margin-top: 15px;
  }
  //菜单
  .menubox{
    width: 100%;
    .parambox{
        width: 100%;
        border: #e5e7eb 1px solid;
        height: 130px;
        overflow: auto;
      }
  }
 :deep(.arco-tree-select-popup){
  text-align: center;
 }
 .upfilebox{
   :deep(.arco-upload-drag){
    height: 148px;
   }
 }
 .showfile{
  display: flex;
  margin-top: 5px;
  background-color: rgb(var(--arcoblue-1));
  padding: 10px;
  border-radius: 3px;
  .icon{
      padding-right: 10px;  
  }
  .name{
    flex: 1;
  }
 }
</style>

