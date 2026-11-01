import component from './zh-CN/component';
import globalHeader from './zh-CN/globalHeader';
import menu from './zh-CN/menu';
import pages from './zh-CN/pages';
import pwa from './zh-CN/pwa';
import settingDrawer from './zh-CN/settingDrawer';
import settings from './zh-CN/settings';

export default {
  and: '和',
  login: '登录',
  'login.welcome': '欢迎使用OpenIM',
  'login.account': '账户',
  'login.account.required': '请输入账户!',
  'login.secrect': '密码',
  'login.secrect.required': '请输入密码!',
  'login.agreement': '我已阅读并同意',
  'login.agreement.service': '服务协议',
  'login.agreement.privacy': '隐私政策',
  'navBar.lang': '语言',
  'layout.user.link.help': '帮助',
  'layout.user.link.privacy': '隐私',
  'layout.user.link.terms': '条款',
  'app.copyright.produced': 'OpenIM后台管理系统',
  'app.preview.down.block': '下载此页面到本地项目',
  'app.welcome.link.fetch-blocks': '获取全部区块',
  'app.welcome.link.block-list': '基于 block 开发，快速构建标准页面',
  ...pages,
  ...globalHeader,
  ...menu,
  ...settingDrawer,
  ...settings,
  ...pwa,
  ...component,
};
