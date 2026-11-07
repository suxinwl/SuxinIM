import 'package:get/get.dart';
import 'package:openim_common/openim_common.dart';
import '../../../core/controller/im_controller.dart';
import '../../../core/controller/app_controller.dart';
class ServerConfigLogic extends GetxController {
 final isIP=(!Config.serverBaseUrl.contains('im.szputy.com')).obs;
 void switchServer(bool lan)=>isIP.value=lan;
 Future<void> confirm() async {
  final origin=isIP.value ? Config.lanOrigin : Config.wanOrigin;
  await DataSp.removeLoginCertificate();
  await DataSp.putServerConfig({'baseUrl':origin});
  await Get.find<IMController>().reconfigureServer();
  await Get.find<AppController>().queryPublicBrand();
  IMViews.showToast('连接已切换，请使用统一账号登录');
  Get.back();
 }
}
