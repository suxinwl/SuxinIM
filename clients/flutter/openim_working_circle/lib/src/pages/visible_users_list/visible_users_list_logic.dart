import 'package:app_im/app_im.dart';
import 'package:get/get.dart';
import 'package:openim_common/openim_common.dart';

class VisibleUsersListLogic extends GetxController {
  late WorkMoments workMoments;
  final listUserInfo = <UserInfo>[].obs;

  @override
  void onInit() {
    workMoments = Get.arguments['workMoments'];
    super.onInit();
  }

  @override
  void onReady() {
    _queryUserInfo();
    super.onReady();
  }

  String get title => workMoments.permission == 2 ? StrRes.whoCanWatch : StrRes.partiallyInvisible;

  void _queryUserInfo() async {
    final list = await LoadingView.singleton.wrap(
      asyncFunction: () => AppIm.iMManager.userManager.getUsersInfo(
        userIDList: workMoments.permissionUsers!.map((e) => e.userID!).toList(),
      ),
    );
    listUserInfo.addAll(list.map((e) => e.simpleUserInfo));
  }

  void viewUserInfo() {}
}
