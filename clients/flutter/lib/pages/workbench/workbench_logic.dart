import 'package:get/get.dart';
import 'package:openim_common/openim_common.dart';
import 'package:url_launcher/url_launcher.dart';
class WorkbenchLogic extends GetxController {
 final list=<UniMPInfo>[].obs;
 final loading=false.obs;
 final error=''.obs;
 @override void onReady(){super.onReady();refreshList();}
 Future<void> refreshList() async {
  loading.value=true;error.value='';
  try{list.assignAll(await Apis.queryUniMPList());}
  catch(_){error.value='发现菜单暂不可用，请重试';}
  finally{loading.value=false;}
 }
 Future<void> open(UniMPInfo item) async {
  final uri=Uri.tryParse(item.url ?? '');
  if(uri==null || uri.scheme!='https' || uri.userInfo.isNotEmpty){IMViews.showToast('链接不可用');return;}
  if(!await launchUrl(uri,mode:LaunchMode.externalApplication)) IMViews.showToast('链接暂时无法打开');
 }
}
