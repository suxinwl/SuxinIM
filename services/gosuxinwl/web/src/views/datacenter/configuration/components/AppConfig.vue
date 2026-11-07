<template>
   <a-card class="general-card contentcard" v-if="formData">
        <template #title> 应用(系统)主要配置(对应manifest/config/app.yaml或系统数据表) 是整个项目共用配置</template>
        <a-form ref="formRef" :model="formData" auto-label-width>
          <a-list class="list-layout" :gridProps="{ gutter: 0, span: 12 }" :bordered="false" :loading="loading">
            <a-list-item  >
              <a-form-item
              label="登录启用人机验证"
              field="loginCaptcha"
              tooltip="登录是否启用人机验证，启用后登录时需要输入正确的数字验证码才可继续登录"
              >
                 <a-switch type="round" v-model="formData.loginCaptcha" :checked-value="true" :unchecked-value="false">
                    <template #checked>{{ $t('cell.open') }}</template>
                    <template #unchecked>{{ $t('cell.close') }}</template>
                  </a-switch>
              </a-form-item>
              <a-form-item
                label="前端源代码目录位置"
                field="vueobjroot"
                extra="配置代码生成时-前端代码根目录位置(开发必改)"
                >
                 <a-input v-model="formData.vueobjroot" placeholder="填写附件请求路径前缀" allow-clear/>
                </a-form-item>
            </a-list-item>
            <!--右边-->
            <a-list-item >
              <a-form-item
              label="是否允许多端登录"
              field="MultiLogin"
              >
                <a-radio-group v-model="formData.MultiLogin">
                  <a-radio :value="false">单端</a-radio>
                  <a-radio :value="true">多端</a-radio>
                </a-radio-group>
              </a-form-item>
              <!-- <a-form-item
              label="启用请求对称加密"
              field="validityApi"
              tooltip="是否启用接口请求对称加密，启用后请求后端接口需要检验请求头的对称密文"
              >
                 <a-switch type="round" v-model="formData.validityApi" :checked-value="true" :unchecked-value="false">
                    <template #checked>{{ $t('cell.open') }}</template>
                    <template #unchecked>{{ $t('cell.close') }}</template>
                  </a-switch>
              </a-form-item> -->
            </a-list-item>
          </a-list>
          <a-list class="list-layout" :bordered="false">
            <a-list-item style=" border: none;margin-left: 7px;">
               <div class="frombtn">
                    <a-button type="primary" html-type="submit" style="width: 120px;" @click="submitAttachmentConfig">保存</a-button>
                </div>
            </a-list-item>
          </a-list>
        </a-form>
    </a-card>
  </template>
  
  <script lang="ts" setup>
    import {  ref, onMounted } from 'vue';
    //api
    import { saveConfig,getConfig} from '@/api/datacenter/appconfig';
    import { Message } from '@arco-design/web-vue';
    import { cloneDeep } from 'lodash-es';
    import useLoading from '@/hooks/loading';
    const { loading, setLoading } = useLoading();
    //数据配置
    const formData=ref({
      vueobjroot:"",
      loginCaptcha:false,
      MultiLogin:false,
    })
    //保存邮箱
    const submitAttachmentConfig=async()=>{
      try{
        Message.loading({content:"保存中",id:"updata",duration:0})
        await saveConfig(cloneDeep(formData.value));
        Message.success({content:"保存成功",id:"updata",duration:2000})
      } catch (error) {
        Message.loading({content:"保存中",id:"updata",duration:1})
      }
    }
    //组件挂载完成后执行的函数
    onMounted(()=>{
      InitData()
    })
    //加载数据
    const InitData=async()=>{
      setLoading(true);
      try {
        const emaildata = await getConfig({});
        formData.value=Object.assign({},formData.value,emaildata)
      } catch (err) {
      } finally {
        setLoading(false);
      }
    }
  </script>
  
  <style scoped lang="less">
  .contentcard{
        overflow: hidden;
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
    .frombtn{
      width: 100%;
      text-align: center;
    }
  </style>
  