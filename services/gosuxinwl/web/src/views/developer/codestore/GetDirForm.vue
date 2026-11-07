<template>
  <BasicModal v-bind="$attrs" @register="registerModal" :isPadding="false" :ok-text="`确定(${selectFile.length})`" :loading="loading" width="900px" :minHeight="modelHeight" :title="modalTitle" @ok="handleSubmit">
    <div class="addFormbox" :style="{'min-height':`${modelHeight}px`}">
      <div class="dir-header">
        <div class="dirmun" >
          <template v-if="dirmenu&&dirmenu.length>0">
             <span class="dirmun_item" v-for="(item,index) in dirmenu" @click="onDirmun(item,index)">
              <span class="dirmun_inner">{{item['name']}}</span> 
              <span class="dirmun_separator">&gt;</span>
            </span>
          </template>
        </div>
        <div class="header-search flex" >
            <a-button type="primary" @click="clearSelect" v-if="selectFile.length>0">取消选择({{ selectFile.length }})</a-button>
            <a-input-search v-model="searchword" style="width: 100%" @press-enter="onSearchDir" allow-clear @search="onSearchDir" placeholder="搜索文件名称"/>
        </div>
      </div>
      <div class="dir-wrap">
        <a-scrollbar  style="overflow: auto;" :style="{height:`${modelHeight-57}px`}">
        <div class="file_list_box">
          <template v-for="item in dirData" >
            <div class="file-item " v-if="item.path" @click="onSelectFile(item)" @dblclick="onOpenFile(item)" :class="{'file-item-active':findSelectDir(item.path)}">
              <div class="folder-file-wrap">
                <!--文件夹-->
                <div class="folder-wrap" v-if="item.isDir" title="双击打开">
                  <Icon icon="svgfont-wenjianjia-1" :forceFallback="true" :size="65" color="#ed6f6f"></Icon>
                </div>
                <div class="img-raw" v-else-if="FileIsImg(item)" >
                  <div class="img-preview">
                    <a-image :src="GetFileFullUrl(item.path)" width="65px" height="65px" fit="contain" :preview="false"/>
                  </div>
                </div>
                <!--其他文件-音频、文件、其他-->
                <div class="img-raw" v-else >
                  <div class="img-preview">
                    <Icon :icon="getFileType(item)" :size="65" ></Icon>
                  </div>
                </div>
              </div>
               <div class="title-warp">
                <div class="edit_title" v-html="item.name"></div>
              </div>
            </div>
          </template>
        </div>
        </a-scrollbar>
      </div>
    </div>
  </BasicModal>
</template>
<script lang="ts">
  import { defineComponent, ref} from 'vue';
  import { BasicModal, useModalInner } from '/@/components/Modal';
  import useLoading from '@/hooks/loading';
  import { cloneDeep } from 'lodash-es';
  import { Message } from '@arco-design/web-vue';
  import { getPackdirs } from '@/api/developer/codestore';
  import {Icon} from '@/components/Icon';
  import { GetLocalFullUrl } from "@/utils/tool";
  export default defineComponent({
    name: 'AddForm ',
    components: { BasicModal,Icon },
    emits: ['ok'],
    setup(_, { emit }) {
      const dirmenu=ref<any[]>([]);//打开的目录
      const dirData=ref<any[]>([]);//文件目录数据源
      const dirDataCopy=ref<any[]>([]);//文件目录数据源-复制
      const searchword=ref<string>("");//搜索文件
      const timer = ref<any>(null)
      const selectFile = ref<any[]>([])//保存选择的文件及文件夹
      const modelHeight= ref(480);
      const modalTitle= ref("选择后端文件");
      const dirType= ref("go");
      //初始弹框
      const [registerModal, { setModalProps, closeModal }] = useModalInner(async (data) => {
        setModalProps({ confirmLoading: false });
        selectFile.value=data.record;
        dirType.value=data.type
        modalTitle.value=data.type=="go"?"选择后端文件":"选择前端文件"
        const packdir= await getPackdirs({type:data.type});
        if(packdir){
           dirmenu.value=[{path: "",name: "根目录",isDir:"",children:packdir.children}];
           dirData.value=packdir.children
           dirDataCopy.value=cloneDeep(dirData.value)
        }
      });
     //点击确认
     const { loading, setLoading } = useLoading();
     const handleSubmit = async () => {
      try {
          closeModal()
          emit('ok',selectFile.value,dirType.value);
          setLoading(false);
        } catch (error) {
          setLoading(false);
          Message.loading({content:"提交中",id:"upStatus",duration:1})
        }
      };
      //跳转目录
      const onDirmun=(item:any,index:number)=>{
          searchword.value=""
          dirmenu.value = dirmenu.value.filter((_, key) => key<=index); 
          dirData.value=item.children
          dirDataCopy.value=cloneDeep(dirData.value)
      }
      //搜索文件
      const onSearchDir=(str:string)=>{
         dirData.value=dirDataCopy.value.filter((item)=>item.name.indexOf(str)>-1)
      }
      //打开文件夹(双击事件)
      const onOpenFile=(item:any)=>{
        if (timer.value) clearTimeout(timer.value)
          if(item.isDir){//是文件夹
            dirmenu.value = dirmenu.value.concat([{path: item.path,name: item.name,isDir: item.isDir,children:item.children}])
            dirData.value=item.children
            dirDataCopy.value=cloneDeep(dirData.value)
            searchword.value=""
        }
      }
      //选择文件及文件夹
      const onSelectFile=(item:any)=>{
        if (timer.value) clearTimeout(timer.value)
         if(item.isDir){//是文件夹
            timer.value = setTimeout(() =>{
              if(selectFile.value.find((fdata)=>fdata.path==item.path)){
                selectFile.value=selectFile.value.filter((data)=>data.path!=item.path)
              }else{
                selectFile.value.push({path:item.path,isDir:item.isDir})
              }
            }, 300)
        }else{
            if(selectFile.value.find((fdata)=>fdata.path==item.path)){
              selectFile.value=selectFile.value.filter((data)=>data.path!=item.path)
            }else{
              selectFile.value.push({path:item.path,isDir:item.isDir})
            }
        }
      }
      //取消选择
      const clearSelect=()=>{
         selectFile.value=[]
      }
     //判断文件是否已经选择
     const findSelectDir=(path:any)=>{
        if(selectFile.value.find((item)=>item.path==path)){
          return true;
        }else{
          return false;
        }
      }
      //文件类型
      interface FileExtendNameIconMap {
        [key: string]: string
      }
      const FileIcon: FileExtendNameIconMap = {
        mp3: 'svgfont-yinpin',
        mp4: 'svgfont-shipin1',
        dir: 'svgfont-wenjianjia-1',
        ppt: 'file-ppt',
        doc: 'svgfont-wendang-docx_doc',
        docx: 'svgfont-wps',
        xls: 'svgfont-XLS',
        xlsx: 'svgfont-XLS',
        txt: 'svgfont-wenbenwendang-txt',
        rar: 'svgfont-yasuowenjian',
        zip: 'svgfont-zipyasuo',
        html: 'svgfont-HTMLtubiao',
        css: 'svgfont-css',
        js: 'svgfont-js',
        ts: 'svgfont-typescript',
        go: 'svgfont-file_golang',
        vue: 'svgfont-File-format-vue',
        other: 'svgfont-qitawenjian' // 未知文件
    }
      /** WPS、OffOfficeTypesice文件类型 */
      const OfficeTypes = ['txt','ppt', 'pptx', 'doc', 'docx', 'xls', 'xlsx','zip','rar','html','css','js','ts','vue']
      /** 音频类型 */
       const AudioTypes = ['mp3','ram','wav']
      /** 图片类型 */
      const ImageTypes = ['png','jpg','jpeg','gif']
      const getFileType =(data:any) => {
        const extension = GetFileExtension(data)
        if (!Object.keys(FileIcon).includes(extension)) {
        if(AudioTypes.includes(extension)){
          return FileIcon['mp3']
          }else if(OfficeTypes.includes(extension)){
            return FileIcon['othetxtr']
          }
          return FileIcon['other']
        }
        return FileIcon[extension]
      }
      //判断是图片
      const FileIsImg =(data:any) => {
        const extension = GetFileExtension(data)
        if (ImageTypes.includes(extension)) {
          return true
        }
        return false
      }
     /** 获取文件后缀 */
     const GetFileExtension=(data:any)=>{
        const name = data.name.toLowerCase()
        if(data.type?.toString()=="1"){
          return "dir"
        }else if(name){
          const filearr=name.split(".")
          if(filearr.length>=2){
            return filearr[filearr.length-1]
          }
        }
        return "other"
      }
      const GetFileFullUrl=(url:string)=>{
         if(dirType.value=="go"){
          return GetLocalFullUrl(url)
         }
         return url;
      }
      return { 
        registerModal, 
        handleSubmit,
        loading,
        modelHeight,
        dirmenu,
        onDirmun,
        searchword,onSearchDir,
        dirData,onOpenFile,onSelectFile,getFileType,
        FileIsImg,GetFileFullUrl,
        selectFile,clearSelect,
        findSelectDir,modalTitle,
      };
    },
  });
</script>
<style lang="less" scoped>
  .highlight{
    color: rgb(var(--red-6));
  }
  .addFormbox{
    width: 100%;
    padding: 10px 15px;
  }
  .dir-header{
    display: flex;
    padding-bottom: 5px;
    align-items: center;
    .header-search{
      width: 320px;
    }
  }
 //我的-目录
  .dirmun{
    height: 22px;
    line-height: 22px;
    overflow: hidden;
    flex: 1;
    .dirmun_item{
      display: inline-flex;
      align-items: center;
      &:last-child .dirmun_inner {
          color: rgba(0,0,0,.38);
          cursor: default;
      }
      &:last-child .dirmun_separator {
          display: none;
      }
      .dirmun_inner{
        color: #557ce1;
        cursor: pointer;
        font-size: 14px;
        font-weight: 500;
      }
      .dirmun_separator{
        padding: 0 4px;
        font-size: 14px;
        color: rgba(0,0,0,.38);
      }
    }
  }
  //文件、目录
  .file_list_box{
    padding: 0;
    margin: 0;
    overflow: hidden;
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(100px, 1fr)); /* 自动填充列 */
    row-gap: 10px; /* 设置行间距为 20px */
    column-gap: 8px; /* 设置列间距为 15px */
    .file-item{
      width: 100%;
      list-style: none;
      padding: 2px 3px 1px 3px;
      border: 1px solid var(--color-bg-3);
      &:hover{
        background-color: rgb(var(--arcoblue-1));
      }
      &.file-item-active{
        background-color: rgb(var(--arcoblue-1));
        border: 1px solid var(--color-neutral-3);
        border-radius: 3px;
      }
      .folder-file-wrap{
        .folder-wrap{
          display: flex;
          justify-content: center;
          align-items: center;
          cursor: pointer;
        }
        .img-raw{
          position: relative;
          display: flex;
          justify-content: center;
          align-items: center;
          .img-preview{
            box-sizing: border-box;
            text-align: center;
            font-size: 0;
            line-height: 0;
            width: 100%;
            user-select: none;
            .img_item{
              display: inline-block;
              width: auto;
              height: auto;
              max-width: 100%;
              max-height: 100%;
              vertical-align: middle;
              padding-bottom: 5px;
            }
          }
        }
      }
      .title-warp{
        .edit_title{
          text-align: center;
          font-size: 12px;
           word-wrap:break-word; 
           word-break:break-all; 
          // :deep(.arco-input-wrapper){
          //   background-color:transparent;
          //   .arco-input.arco-input-size-mini{
          //     text-align: center;
          //   }
          // }
        }
      }
    }
  }
</style>

