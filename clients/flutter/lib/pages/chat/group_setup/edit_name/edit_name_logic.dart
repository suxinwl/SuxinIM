import 'package:flutter/material.dart';
import 'package:app_im/app_im.dart';
import 'package:get/get.dart';
import 'package:openim/pages/chat/group_setup/group_setup_logic.dart';
import 'package:openim_common/openim_common.dart';

enum EditNameType {
  myGroupMemberNickname,
  groupNickname,
}

class EditGroupNameLogic extends GetxController {
  final groupSetupLogic = Get.find<GroupSetupLogic>();
  late TextEditingController inputCtrl;
  late EditNameType type;

  @override
  void onInit() {
    type = Get.arguments['type'];
    inputCtrl = TextEditingController(
      text: type == EditNameType.groupNickname ? groupSetupLogic.groupInfo.value.groupName : groupSetupLogic.myGroupMembersInfo.value.nickname,
    );
    super.onInit();
  }

  @override
  void onClose() {
    inputCtrl.dispose();
    super.onClose();
  }

  String? get title => type == EditNameType.myGroupMemberNickname ? StrRes.myGroupMemberNickname : StrRes.groupName;

  void save() async {
    await LoadingView.singleton.wrap(asyncFunction: () async {
      if (type == EditNameType.groupNickname) {
        await AppIm.iMManager.groupManager
            .setGroupInfo(GroupInfo(groupID: groupSetupLogic.groupInfo.value.groupID, groupName: inputCtrl.text.trim()));
      } else if (type == EditNameType.myGroupMemberNickname) {
        await AppIm.iMManager.groupManager.setGroupMemberNickname(
          groupID: groupSetupLogic.groupInfo.value.groupID,
          userID: AppIm.iMManager.userID,
          groupNickname: inputCtrl.text.trim(),
        );
      }
    });
    IMViews.showToast(StrRes.setSuccessfully);
    Get.back();
  }
}
