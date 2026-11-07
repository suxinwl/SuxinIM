const domain="";//您的域名//独立部署需要填写Go服务api接口域名(部署在Go目录下可以留空)
const localhost="http://127.0.0.1:19880";//本项目本地 HTTP 后端；9880 为 HTTPS 入口
window.globalConfig = {
	Main_url:`${domain}`,//域名
	Main_url_dev:`${localhost}`,//域名
	Root_url:`${domain}/admin`,//Api服务器域名
    Root_url_dev:`${localhost}/admin`,//Api服务器域名-开发环境
	Upload_url:`${domain}/common`,//Api服务器域名
	Upload_url_dev:`${localhost}/common`,//Api服务器域名-开发环境
	AppTitle_zhCN:"GoSuxin开发框架",
    AppTitle_zhTW:"GoSuxin開發框架",
	AppTitle_enUS:"GoSuxin Dev frame",
    CompanySite:"https://www.suxinwl.com/",//公司官网
    ICP:"",//备案号
    Company:"深圳市速信网络科技有限公司",//公司名称
    Address:"中国·深圳",
    
    TeamSite:"https://www.suxinwl.com/",//技术团队官网
    Team:"Suxin技术团队",//技术团队，没有则填空""
    loginTitle_zhCN:`Go语言开发用<span class="sub-title">GoSuxin框架</span>`,
    loginTitle_zhTW:`Go語言開發用<span class="sub-title">GoSuxin框架</span>`,
    loginTitle_enUS:`<span class="sub-title">GoSuxin</span> framework is used for Go development`,
    loginDesc_zhCN:["开发效率高","基础功能完善","开发文档全面"],
    loginDesc_zhTW:["開發效率高","基礎功能完善","開發文檔全面"],
    loginDesc_enUS:["High development efficiency","Complete basic functions","Comprehensive development documentation"],
    Copyright:"深圳市速信网络科技有限公司",
    RouterHome:"home",//路由默认入口
    MaxSizeImage:5,//最大上传图片大小,单位M
	MaxSizeVideo:150,//最大上传视频大小,单位M
    DefaultAccountPassword:[],//演示账号密码值为：["admin","admin888"]，正式项目请修改为空数组-即值为：[]
};
