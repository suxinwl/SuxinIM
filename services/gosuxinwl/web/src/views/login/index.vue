<template>
  <div class="layoutNew-box" :class="{phoneStyle:isPhone}"
       :style="isPhone?`background:#ffffff;`:`background-image: `+(theme === 'dark'?'':'url('+loginBigBg+');')">
    <div class="header-nav pc_height flex flex-middle flex-between">
      <div class="icon-left ">
         <img style="height: 37px;" :src="brandLogo" />
         <span class="app-title">{{ AppTitle }}</span>
      </div>
      <div class="arco-right">
        <a-select v-model="currentLocale" :style="{width:'100%'}" :bordered="false" @change="(value) => changeLocale(String(value))">
          <a-option v-for="item in locales" :value="item.value">{{ item.label }}</a-option>
        </a-select>
      </div>
    </div>
    <div style="height: 11px;"></div>
    <!--内容-->
    <div class="login-container flex-all-center">
      <!--左边介绍·可选择1或者2方式-->
      <div class="left-banner" v-if="!isPhone">
        <div class="hotspot-img">
          <!--1.自定义文字-->
          <div class="custom-notes">
            <div class="notes-header">
              <div class="notes-title" v-html="loginTitle">
                 
              </div>
              <div class="notes-desc">
                <template v-for="text in loginDesc">
                  <span class="desc-line">/</span>{{ text }}
                </template>
              </div>
            </div>
            <div class="image-wrap">
                <img style="width:108px;height:108px;object-fit:contain" :src="brandLogo" alt="GoSuxin">
            </div>
          </div>
          <!--2.整张图-->
          <!-- <img src="https://res.volccdn.com/obj/volc-console-fe/vconsole-static/auth.ydl_banner.97198265.png"> -->
        </div>
      </div>
      <!--右边登录表单-->
      <div class="right-form">
        <div class="login-card">
          <div class="login-title">{{isEmailLogin?$t('login.form.tabemail'):$t('login.form.welcome')+`${AppTitle}`}}</div>
          <!--表单-->
          <EmailLogin v-show="isEmailLogin" />
          <a-tabs v-show="!isEmailLogin" class="login-right-form">
            <a-tab-pane key="1" :title="$t('login.form.tabacount')">
              <AccountLogin />
            </a-tab-pane>
            <a-tab-pane key="2" :title="$t('login.form.tabmobile')">
              <PhoneLogin />
            </a-tab-pane>
          </a-tabs>
          <!--协议-->
          <div class="protocol-text">
            {{ $t('login.form.login.agree') }} {{ AppTitle }}
            <a href="https://www.suxinwl.com/" target="_blank">{{ $t('login.form.login.service') }}</a>
            {{ $t('login.form.login.and') }}
            <a href="https://www.suxinwl.com/" target="_blank">{{ $t('login.form.login.policy') }}</a>
          </div>
          <!--其他登录方式-->
          <div class="login-ouath text-conent">
            <a-divider orientation="center">{{ $t('login.form.other') }}</a-divider>
            <div class="optoin-list">
              <div v-if="isEmailLogin" class="mode item" @click="toggleLoginMode"><icon-user />{{ $t('login.form.tabacountmobile') }}</div>
              <div v-else class="mode item" @click="toggleLoginMode"><icon-email />{{ $t('login.form.tabemail') }}</div>
               <a class="item icon-circle flex-all-center" title="使用 飞书 账号登录" @click="onOauth('lark')">
               <icon-bytedance-color />
              </a>
               <a class="item icon-circle flex-all-center" title="使用 飞书 账号登录" @click="onOauth('lark')">
                <Icon icon="icon-lark-color" :size="24" />
              </a>
              <a class="item icon-circle flex-all-center" title="使用 微信 账号登录" @click="onOauth('wechat')">
                <Icon icon="icon-wechat" :size="24" color="#28c445" />
              </a>
            </div>
          </div>
          <!--没有账号跳转注册-->
          <div class="no-account-register flex-all-center">
            {{ $t('login.form.no-acount') }}<a href="https://www.suxinwl.com/"><span style="font-weight: 500;">{{ $t('login.form.register.now') }}</span></a> 
          </div>

        </div>
      </div>
    </div>
    <!--底部-->
    <div class="footer-container flex flex-middle flex-center" v-if="!isPhone">
      <div class="beian-box">
        <div class="text-copyright">
          <span>{{ Address }} <span v-if="Team">&amp;</span> <a :href="TeamSite" target="_blank">{{ Team }} </a> </span>
          <span class="copyright">ⓒ 2018-{{nowyear}} <a :href="CompanySite" target="_blank">{{Company}}</a> {{ $t("footer.copyright") }}</span>
        </div>

      </div>

    </div>

  </div>
</template>

<script setup lang="ts">
const brandLogo = `${import.meta.env.BASE_URL}logo.png?v=puty3`;

import { ref,computed,onMounted,onUnmounted } from 'vue';
import AccountLogin from './components/account/index.vue'
import PhoneLogin from './components/phone/index.vue'
import EmailLogin from './components/email/index.vue'
import { useDark } from '@vueuse/core';
import { useAppStore } from '@/store';
import { Icon } from '@/components/Icon';
import { Message } from '@arco-design/web-vue'
import { LOCALE_OPTIONS } from '@/locale';
import useLocale from '@/hooks/locale';
import loginBigBg from './image/login_big_bg.jpeg'; // 确保路径正确
const appStore = useAppStore();
const theme = computed(() => {
  return appStore.theme;
});
useDark({
  selector: 'body',
  attribute: 'arco-theme',
  valueDark: 'dark',
  valueLight: 'light',
  storageKey: 'arco-theme',
  onChanged(dark: boolean) {
    appStore.toggleTheme(dark);
  },
});
const { changeLocale, currentLocale } = useLocale();
const locales = [...LOCALE_OPTIONS];
//获取网站配置-应用名称
const Address = window?.globalConfig.Address
const TeamSite = window?.globalConfig.TeamSite
const Team = window?.globalConfig.Team
const nowyear = new Date().getFullYear()
const CompanySite = window?.globalConfig.CompanySite
const Company = window?.globalConfig.Company
const isEmailLogin = ref(false)
// 切换登录模式
const toggleLoginMode = () => {
  isEmailLogin.value = !isEmailLogin.value
}
//获取网站配置-应用名称
const AppTitle = computed(() => {
  switch (currentLocale.value) {
    case 'zh-CN':
      return window?.globalConfig.AppTitle_zhCN;
    case 'zh-TW':
      return window?.globalConfig.AppTitle_zhTW;
    case 'en-US':
      return window?.globalConfig.AppTitle_enUS;
    default:
      return window?.globalConfig.AppTitle_enUS;
  }
});
//获取网站配置-简介标题
const loginTitle = computed(() => {
  switch (currentLocale.value) {
    case 'zh-CN':
      return window?.globalConfig.loginTitle_zhCN;
    case 'zh-TW':
      return window?.globalConfig.loginTitle_zhTW;
    case 'en-US':
      return window?.globalConfig.loginTitle_enUS;
    default:
      return window?.globalConfig.loginTitle_enUS;
  }
});
//获取网站配置-简介说明
const loginDesc = computed(() => {
  switch (currentLocale.value) {
    case 'zh-CN':
      return window?.globalConfig.loginDesc_zhCN;
    case 'zh-TW':
      return window?.globalConfig.loginDesc_zhTW;
    case 'en-US':
      return window?.globalConfig.loginDesc_enUS;
    default:
      return window?.globalConfig.loginDesc_enUS;
  }
});
//判断移动端、pc端
const isMobile=()=> {
  let flag = navigator.userAgent.match(/(phone|pad|pod|iPhone|iPod|ios|iPad|Android|Mobile|BlackBerry|IEMobile|MQQBrowser|JUC|Fennec|wOSBrowser|BrowserNG|WebOS|Symbian|Windows Phone)/i)
  if (flag) {
      let content = `width=device-width, user-scalable=no, initial-scale=0.8, maximum-scale=0.8, minimum-scale=1.0`
      let meta = document.querySelector('meta[name=viewport]')
      if(!meta) {
        meta = document.createElement('meta')
        meta.setAttribute('name', 'viewport')
        document.head.appendChild(meta)
      } 
      meta.setAttribute('content', content)
    return true;
  } else {
    return false;
  }
}
const isPhone = ref(isMobile()) 
onMounted(() => {
    isPhone.value =isMobile()  
})
onUnmounted(() => {
  if (isPhone.value) {
      let content = `width=device-width,initial-scale=0.3,minimum-scale=0.3,maximum-scale=1,viewport-fit=cover`
      let meta = document.querySelector('meta[name=viewport]')
      if(meta) {
         meta.setAttribute('content', content)
      } 
  } 
})
// 第三方登录授权
const onOauth = async (source: string) => {
  Message.warning({ content: '请根据需求接第三方登录授权', id: "socialAuth" })
  // const { data } = await socialAuth(source)
  // window.location.href = data.authorizeUrl
}
</script>
<style lang="less" scoped>
:deep(.arco-tabs-nav-tab-list){
  margin-left: -16px;
}
:deep(.arco-tabs-tab-title){
  font-size: 16px;
}
:deep(.arco-tabs-tab:hover){
  color: rgb(var(--primary-6));
}
:deep(.arco-tabs-nav::before){
  height: 0px;
}
:deep(.arco-input-wrapper){
  background-color: transparent;
  border: 1px solid var(--color-neutral-3);
}
:deep(.arco-tabs-nav-type-line .arco-tabs-tab:hover .arco-tabs-tab-title::before){
  background: transparent;
}
:deep(.sub-title){
  background-clip: text;
  background-image: linear-gradient(123deg, #299dff 13.15%, #986afe 88.72%);
  color: transparent;
  display: inline-block;
}
.layoutNew-box {
  background-repeat: no-repeat;
  background-size: cover;
  min-height: 100vh;
  .header-nav {
      padding-left: 32px;
      .icon-left{
        img{
          vertical-align: text-bottom;
        }
        .app-title{
          position: relative;
          margin-left:5px;
          letter-spacing: 2px;
          padding-bottom: 2px;
          font-size: 28px;
          font-weight: 900;
          vertical-align: text-bottom;
          color: var(--color-neutral-10);
        }
      }
      .icon-left,h1{
        padding: 0;
        margin: 0;
      }
      .arco-right{
        margin-right: 41px;
        margin-top: 8px;
        min-width: 106px;
      }
  }
  .pc_height{
    padding-top: 26px;
    height: 63px;
  }
  //内容
  .login-container{
    box-sizing: border-box;
    height: calc(100vh - 125px);
    margin: 0 auto;
    max-width: 1200px;
    min-height: 650px;
    padding: 0 40px;
    //左边介绍
    .left-banner{
      align-items: center;
      display: flex;
      flex: 1 1;
      height: 100%;
      position: relative;
      .hotspot-img{
        max-width: 100%;
        min-height: 650px;
        .custom-notes{
          .notes-header{
            .notes-title{
              font-size: 39px;
              font-weight: 900;
              letter-spacing: 2px;
              color: var(--color-neutral-10);
              
            }
            .notes-desc{
              font-size: 20px;
              padding-top: 20px;
              font-weight: 600;
              color: var(--color-neutral-8);
              .desc-line{
                color: #605dff;
                padding-left: 10px;
                padding-right: 3px;
                font-weight: 600;
                &:first-child{
                  padding-left: 0px;
                }
              }
            }
          }
          .image-wrap{
            padding-top: 60px;
            text-align: center;
            width: 100%;
          }
        }
        img {
            max-width: 540px;
            object-fit: contain;
            width: 100%;
        }
      }
    }
    //右边提交登录表单
    .right-form{
      .login-card{
          background: var(--color-bg-2);
          border-radius: 20px;
          box-shadow: 0 5px 15px rgba(0, 0, 0, .05);
          box-sizing: border-box;
          min-height: 632px;
          position: relative;
          width: 476px;
          display: flex;
          flex-direction: column;
          margin-bottom: 30px;
          padding: 48px 43px 32px;
          .login-title{
            color: var(--color-neutral-10);
            font-size: 24px;
            font-weight: 500;
            letter-spacing: .003em;
            line-height: 32px;
            white-space:nowrap;
            overflow: hidden;
          }
          //表单tab
          .login-right-form{
            margin-top: 20px;
          }
          //协议
          .protocol-text{
            color: var(--color-neutral-8);
            font-size: 12px;
            font-weight: 400;
            line-height: 20px;
            margin-bottom: 4px;
          }
          //其他登录方式
          .text-conent{
            color: var(--color-neutral-6);
            display: flex;
            font-size: 14px;
            justify-content: space-between;
          }
          .login-ouath{
            margin-top: auto;
            padding: 0 5px;
            flex-wrap: wrap;
            .optoin-list{
              align-items: center;
              display: flex;
              justify-content: center;
              width: 100%;
              .item {
                margin-right: 15px;
              }
              .icon-circle{
                border: 1px solid #eaedf1;
                border-radius: 32px;
                box-sizing: border-box;
                height: 32px;
                width: 32px;
              }
              .mode {
                color: var(--color-text-2);
                font-size: 12px;
                font-weight: 400;
                line-height: 20px;
                padding: 6px 10px;
                align-items: center;
                border: 1px solid var(--color-border-3);
                border-radius: 32px;
                box-sizing: border-box;
                display: flex;
                height: 32px;
                justify-content: center;
                cursor: pointer;
                user-select: none;
                .icon {
                  width: 21px;
                  height: 20px;
                }
              }
              .mode svg {
                font-size: 16px;
                margin-right: 10px;
              }
              .mode:hover,.icon-circle:hover {
                background: rgba(var(--primary-6), 0.05);
                border: 1px solid rgb(var(--primary-3));
                color: rgb(var(--arcoblue-6));
              }
            }
          }
          //提示注册
          .no-account-register{
            color: var(--color-neutral-8);
            font-size: 14px;
            font-weight: 400;
            line-height: 22px;
            margin-top: auto;
          }
      }
    }
  }
  //底部
  .footer-container{
    box-sizing: border-box;
    .beian-box{
      padding-bottom: 10px;
      font-size: 13px;
      .text-copyright{
        color: var(--color-neutral-8);
        font-weight: 400;
        letter-spacing: .2px;
        line-height: 20px;
        text-align: center;
        .copyright{
          padding-left: 3px;
        }
      }
      .text-below{
        .below-left{
          margin-right: 20px;
          .beian-img {
              float: left;
              height: 12px;
              margin-right: 4px;
              width: 12px;
          }
          span{
            color: var(--color-neutral-8);
            padding: 0px 3px;
          }
          a {
            color: var(--color-neutral-8);
              font-weight: 400;
              letter-spacing: .2px;
              line-height: 20px;
          }
        }
        .below-right{
            color: var(--color-neutral-8);
            font-weight: 400;
            letter-spacing: .2px;
            line-height: 20px;
            text-align: center;
        }
      }
    }
  }
}
a {
  color: rgb(var(--arcoblue-6));
  cursor: pointer !important;
  text-decoration: none;
}
@media screen and (max-width: 800px) {
  .left-banner{
     visibility:hidden;
     width: 0px;
  }
  .footer-container{
    display: none;
  }
  .layoutNew-box{
    background: #fff !important;
  }
  .layoutNew-box .login-container .left-banner{
    flex: unset;
  }
  .layoutNew-box{
    height: 100vh;
    overflow: hidden;
  }
  .layoutNew-box .login-container{
    height: 100%;
    padding:0px;
    .right-form{
      height: 100%;
      .login-card{
        height: 100%;
        width: 100%;
        box-shadow:none !important;
        padding: 48px 20px 0px 20px;
      }
    }
    
  }
}
:deep(.arco-divider-text){
    white-space:nowrap;
}
//手机端样式
.phoneStyle{
  height: 100vh;
  width: 100vw;
  box-sizing: border-box;
  padding: 0px;
  margin: 0px;
  display: flex;
  flex-flow: column;
  justify-content: space-between;
  position: relative;
   .header-nav{
    position: absolute;
    padding-left: 10px;
    z-index: 999;
    overflow: hidden;
    width: 100vw;
   }
   .pc_height {
      padding-top: 16px;
      height: unset;
    }
   .arco-right{
    margin-right:0px !important;
    min-width: auto !important;
    margin-top:unset !important;
   }
   .icon-left{
    white-space: nowrap; 
    text-overflow: ellipsis;
    overflow: hidden;
   }
   .icon-left img{
    height: 20px !important;
   }
   .app-title{
    font-size: 16px !important;
    white-space:nowrap;
    overflow: hidden;
   }
  .login-container{
    position: fixed;
    top:0;
    padding:0px;
    width: 100vw;
    height: 100vh !important;
    .right-form{
      width: 100%;
      .login-card{
        width: 100%;
        box-sizing: border-box;
        box-shadow:none !important;
        padding: 70px 20px 20px 20px;
        .login-title{
            color: var(--color-neutral-10);
            font-size: 20px;
            font-weight: 500;
            letter-spacing: .003em;
            line-height: 32px;
            white-space:nowrap;
            overflow: hidden;
        }
      }
    }
    .no-account-register{
      padding-bottom: 20px;
    }
    
  }
}
</style>
