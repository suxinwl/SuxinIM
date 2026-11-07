<template>
   <a-card class="general-card contentcard" v-if="formData">
        <template #title> 
          文件上传配置
         <span class="tig" >配置文件位置：manifest/config/upload.yaml</span>  
        </template>
        <a-form ref="formRef" :model="formData" auto-label-width :disabled="busy">
          <a-row :gutter="80">
            <a-col :span="24">
              <a-form-item
                label="可上传文件类型"
                field="AllowedExt"
                tooltip="允许上传的文件类型，使用英文逗号(,)隔开，文件类型后缀前面带点，如：.jpg,.jpeg,.png,.pdf,.ico"
              >
                <a-input v-model="formData.AllowedExt" placeholder="填写允许上传文件类型" allow-clear style="max-width: 560px;"/>
              </a-form-item>
            </a-col>
            <a-col :span="24">
              <a-form-item
                label="传输文件最大值"
                field="MaxBodySize"
                tooltip="限制上传文件的大小，超过大小的文件会被禁止提交"
              >
                <a-input-number v-model="formData.MaxBodySize" placeholder="填写允许上传文件的大小" allow-clear append="MB" style="max-width: 560px;"/>
              </a-form-item>
            </a-col>
            <a-col :span="24">
              <a-form-item
                label="文件存储方式"
                field="Type"
                tooltip="文件存储方式，支持本地、腾讯云、阿里云、七牛云及123云盘"
              >
                 <a-radio-group type="button" v-model="formData.Type" @change="handleChangeType">
                  <a-radio value="local">本地</a-radio>
                  <a-radio value="tencentcos">腾讯云</a-radio>
                  <a-radio value="alioss">阿里云</a-radio>
                  <a-radio value="qiniuoss">七牛云</a-radio>
                  <a-radio value="pan123">123 云盘</a-radio>
                  <a-tooltip content="如果业务需要你可继续扩展" background-color="#ff9a2e">
                    <a-radio value="huaweiobs" disabled>华为云</a-radio>
                  </a-tooltip>
                </a-radio-group>
              </a-form-item>
            </a-col>
            <a-col :span="24" v-if="formData.Type!='pan123'">
              <a-form-item
                label="文件访问路径"
                field="BaseUrl"
                tooltip="访问上传服务器的文件，访问的路径。"
              >
                <a-input v-model="formData.BaseUrl" placeholder="填写文件访问路径" allow-clear style="max-width: 560px;"/>
              </a-form-item>
            </a-col>

            <a-col :span="24" v-if="formData.Type!='local' && formData.Type!='pan123'">
              <a-form-item
                label="自定义访问域名"
                field="Endpoint"
                tooltip="自定义服务请求的访问域名"
              >
                <a-input v-model="formData.Endpoint" placeholder="填写自定义服务访问域名" allow-clear style="max-width: 560px;"/>
              </a-form-item>
            </a-col>
            <a-col :span="24" v-if="formData.Type!='local' && formData.Type!='pan123'">
              <a-form-item
                label="密钥Key或秘钥id"
                field="KeyId"
                tooltip="密钥Key或秘钥id对应AccessKey或者是appID编号"
              >
                <a-input v-model="formData.KeyId" placeholder="填写密钥Key或秘钥id" allow-clear style="max-width: 560px;"/>
              </a-form-item>
            </a-col>
            <a-col :span="24" v-if="formData.Type!='local' && formData.Type!='pan123'">
              <a-form-item
                label="密钥SecretKey"
                field="Secret"
                tooltip="对应云存储给的SecretKey,是AccessKey ID的密码"
              >
                <a-input-password v-model="formData.Secret" placeholder="填写密钥SecretKey" allow-clear style="max-width: 560px;"/>
              </a-form-item>
            </a-col>
            <a-col :span="24" v-if="formData.Type=='alioss'||formData.Type=='qiniuoss'||formData.Type=='tencentcos'">
              <a-form-item
                label="空间名称"
                field="BucketName"
                tooltip="对象存储OSS中用于存储文件（Object）的基础容器,如阿里OSS的Bucket名称"
              >
                <a-input v-model="formData.BucketName" placeholder="填写空间名称" allow-clear style="max-width: 560px;"/>
              </a-form-item>
            </a-col>
            <a-col :span="24" v-if="formData.Type=='tencentcos'">
              <a-form-item
                label="所属地域"
                field="Region"
                tooltip="指存储桶的所属地域，选择与您最近的一个地区。例如，您在深圳，地域可以选择广州，即 ap-guangzhou"
              >
                <a-input v-model="formData.Region" placeholder="填写存储所属地域" allow-clear style="max-width: 560px;"/>
              </a-form-item>
            </a-col>
              <a-col :span="24" v-if="formData.Type!='pan123'">
              <a-form-item
                label="上传文件路径"
                field="DirPath"
                tooltip="上传到服务器目录路径，如本地：/resource/uploads/、腾讯云:/suxin、七牛云：image、阿里云：resource/uploads，注意：阿里云和七牛云路径前后不能带/否则报错"
              >
                <a-input v-model="formData.DirPath" placeholder="填写上传附件路径" allow-clear style="max-width: 560px;"/>
              </a-form-item>
            </a-col>
            <a-col :span="24" v-if="formData.Type=='qiniuoss'">
              <a-form-item
                label="是否使用https"
                field="UseHTTPS"
                tooltip="是否使用https域名进行资源管理"
              >
                <a-switch type="round" v-model="formData.UseHTTPS" :checked-value="true" :unchecked-value="false">
                  <template #checked>{{ $t('cell.open') }}</template>
                  <template #unchecked>{{ $t('cell.close') }}</template>
                </a-switch>
              </a-form-item>
            </a-col>
            <a-col :span="24" v-if="formData.Type=='qiniuoss'">
              <a-form-item
                label="存储区域"
                field="Zone"
                tooltip="文件存储的区域"
              >
               <a-select v-model="formData.Zone" placeholder="选择存储区域" style="max-width: 560px;">
                  <a-option :value="1">华东机房</a-option>
                  <a-option :value="2">华东-浙江(2区)</a-option>
                  <a-option :value="3">华北-河北</a-option>
                  <a-option :value="4">华南-广东</a-option>
                  <a-option :value="5">北美机房</a-option>
                  <a-option :value="6">新加坡机房</a-option>
                </a-select>
              </a-form-item>
            </a-col>

            <a-col v-if="formData.Type==='pan123'" :span="24">
              <a-alert style="margin-bottom:20px;max-width:720px">切换仅影响新上传。目标目录需在123控制台开启直链。已有附件仍使用原账号，不能直接更换账号。</a-alert>
              <a-form-item label="开发者 clientID" field="clientID"><a-input v-model="formData.clientID" autocomplete="off" style="max-width:560px" /></a-form-item>
              <a-form-item label="开发者 clientSecret" field="clientSecret"><a-input-password v-model="formData.clientSecret" autocomplete="new-password" :placeholder="hasClientSecret ? '已配置，留空保留' : '填写开发者密钥'" style="max-width:560px" /></a-form-item>
              <a-form-item label="目标目录 ID" field="parentId"><a-input-number v-model="formData.parentId" :min="0" :precision="0" style="max-width:560px" /><template #help>0 表示根目录；文件按日期保存在目标目录下。</template></a-form-item>
              <a-form-item label="URL 鉴权"><a-switch v-model="formData.urlAuth" /><template #help>与123控制台的 URL 鉴权开关保持一致；这里不会修改云盘控制台设置。</template></a-form-item>
              <a-form-item v-if="formData.urlAuth" label="CDN 鉴权密钥" field="cdnKey"><a-input-password v-model="formData.cdnKey" autocomplete="new-password" :placeholder="hasCDNKey ? '已配置，留空保留' : '填写与123控制台一致的鉴权密钥'" style="max-width:560px" /></a-form-item>
              <a-form-item label="账号 UID"><span>{{ testedUID || savedUID || '测试或保存时自动查询' }}</span></a-form-item>
              <a-form-item><a-space direction="vertical" fill>
                <a-button v-if="canTest" :loading="testing" :disabled="busy" @click="runTest">测试连接与上传</a-button>
                <div class="pan-help">测试会上传小图片和 PDF，校验下载后回收测试文件。附件保持公开访问，URL 鉴权仅用于 CDN 链接签名。</div>
                <a-alert v-for="(step,index) in testSteps" :key="index" :type="step.ok ? 'success' : 'error'">{{ step.name }}：{{ step.message }}</a-alert>
              </a-space></a-form-item>
            </a-col>
            <a-col :span="24">
                <a-form-item style="max-width: 560px;">
                  <div class="frombtn">
                      <a-button type="primary" html-type="button" :loading="saving" :disabled="busy" style="width: 120px;" @click="submitAttachmentConfig">保存</a-button>
                  </div>
                </a-form-item>
            </a-col>
          </a-row>
        </a-form>
    </a-card>
  </template>
  
  <script lang="ts" setup>
    import { computed, ref, onMounted, watch } from 'vue';
    import { useRoute } from 'vue-router';
    //api
    import { saveConfig,getConfig,testConnection} from '@/api/datacenter/uploadconfig';
    import { Message } from '@arco-design/web-vue';
    import { cloneDeep } from 'lodash-es';
    //数据配置
    const formData=ref({
      Type:"local",
      MaxBodySize:600,
      AllowedExt:".jpg,.jpeg,.png,.pdf",
      BaseUrl:"",
      DirPath:"/resource/uploads/",
      Endpoint:"",
      KeyId:"",
      Secret:"",
      BucketName:"",
      DestBucketName:"",
      Region:"",
      UseHTTPS:false,
      Zone:4,
      clientID:"", clientSecret:"", parentId:0, urlAuth:true, cdnKey:"",

    })
    const saving=ref(false), testing=ref(false);
    const busy=computed(()=>saving.value || testing.value);
    const hasClientSecret=ref(false),hasCDNKey=ref(false),savedUID=ref(0),testedUID=ref(0);
    const testSteps=ref<{name:string;ok:boolean;message:string}[]>([]);
    const route=useRoute();
    const canTest=computed(()=>((route.meta.btnroles || []) as string[]).some(x=>x==='*'||x==='pan123Test'));
    watch(()=>[formData.value.clientID,formData.value.clientSecret,formData.value.parentId,formData.value.urlAuth,formData.value.cdnKey],()=>{testSteps.value=[];testedUID.value=0;});
    const runTest=async()=>{
      if(busy.value)return;
      testing.value=true;testSteps.value=[];
      try {const result=await testConnection({clientID:formData.value.clientID,clientSecret:formData.value.clientSecret,parentId:formData.value.parentId,urlAuth:formData.value.urlAuth,cdnKey:formData.value.cdnKey});testSteps.value=result.steps;testedUID.value=result.uid;if(result.success)Message.success('上传与直链测试通过');else Message.error('连接测试未通过，请查看结果');}
      finally{testing.value=false;}
    };
    // 保存上传配置
    const submitAttachmentConfig=async()=>{
      if(busy.value)return;
      saving.value=true;
      try{
        Message.loading({content:"保存中",id:"updata",duration:0})
        await saveConfig(cloneDeep(formData.value));
        await InitData()
        Message.success({content:"保存成功",id:"updata",duration:2000})
      } catch (error) {
        Message.clear()
      } finally {saving.value=false;}
    }
    //组件挂载完成后执行的函数
    onMounted(()=>{
      InitData()
    })
    //加载数据
    var baseData:any
    const InitData=async()=>{
      const emaildata = await getConfig({});
      baseData=emaildata
      hasClientSecret.value=Boolean(emaildata.pan123?.hasClientSecret);hasCDNKey.value=Boolean(emaildata.pan123?.hasCDNKey);savedUID.value=emaildata.pan123?.uid || 0;
      formData.value.clientSecret='';formData.value.cdnKey='';
      formData.value=Object.assign({},formData.value,emaildata)
      handleChangeType(formData.value.Type)
    }
    //切换上传方式
    const handleChangeType=(value:any)=>{
      testSteps.value=[];testedUID.value=0;
      if(value=="pan123"){
        const p=baseData.pan123 || {};
        Object.assign(formData.value,{clientID:p.clientID || '',clientSecret:'',parentId:p.parentId || 0,urlAuth:p.urlAuth ?? true,cdnKey:''});
      }else if(value=="local"){
        formData.value=Object.assign({},formData.value,{BaseUrl:baseData.local.LBaseUrl,DirPath:baseData.local.LDirPath})
      }else if(value=="tencentcos"){
        formData.value=Object.assign({},formData.value,{BaseUrl:baseData.tencentcos.TBaseUrl,Endpoint:baseData.tencentcos.TEndpoint,
          KeyId:baseData.tencentcos.TKeyId,Secret:baseData.tencentcos.TSecret,BucketName:baseData.tencentcos.TBucketName,
          Region:baseData.tencentcos.TRegion,DirPath:baseData.tencentcos.TDirPath})
      }else if(value=="alioss"){
        formData.value=Object.assign({},formData.value,{BaseUrl:baseData.alioss.ABaseUrl,Endpoint:baseData.alioss.AEndpoint,
          KeyId:baseData.alioss.AKeyId,Secret:baseData.alioss.ASecret,BucketName:baseData.alioss.ABucketName,
          DirPath:baseData.alioss.ADirPath})
      }else if(value=="qiniuoss"){
        formData.value=Object.assign({},formData.value,{BaseUrl:baseData.qiniuoss.QBaseUrl,Endpoint:baseData.qiniuoss.QEndpoint,
          KeyId:baseData.qiniuoss.QKeyId,Secret:baseData.qiniuoss.QSecret,BucketName:baseData.qiniuoss.QBucketName,
        DestBucketName:baseData.qiniuoss.QDestBucketName,UseHTTPS:baseData.qiniuoss.QUseHTTPS,Zone:baseData.qiniuoss.QZone,
        DirPath:baseData.qiniuoss.QDirPath})
      }
    }
  </script>
  
  <style scoped lang="less">
  .pan-help{max-width:650px;color:var(--color-text-3);line-height:1.7;}
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
    .tig{
      font-size: 12px;
      color: var(--color-neutral-4);
      padding-left: 5px;
    }
  </style>
