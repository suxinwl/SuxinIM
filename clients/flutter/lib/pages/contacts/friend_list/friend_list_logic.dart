import 'dart:async';

import 'package:azlistview/azlistview.dart';
import 'package:app_im/app_im.dart';
import 'package:get/get.dart';
import 'package:openim/routes/app_navigator.dart';
import 'package:openim_common/openim_common.dart';

import '../../../core/controller/im_controller.dart';

class FriendListLogic extends GetxController {
  final imLoic = Get.find<IMController>();
  final friendList = <ISUserInfo>[].obs;
  final userIDList = <String>[];
  late StreamSubscription delSub;
  late StreamSubscription addSub;
  late StreamSubscription infoChangedSub;

  @override
  void onInit() {
    delSub = imLoic.friendDelSubject.listen(_delFriend);
    addSub = imLoic.friendAddSubject.listen(_addFriend);
    infoChangedSub = imLoic.friendInfoChangedSubject.listen(_friendInfoChanged);
    imLoic.onBlacklistAdd = _delFriend;
    imLoic.onBlacklistDeleted = _addFriend;
    super.onInit();
  }

  @override
  void onReady() {
    _getFriendList();
    super.onReady();
  }

  @override
  void onClose() {
    delSub.cancel();
    addSub.cancel();
    infoChangedSub.cancel();
    super.onClose();
  }

  _getFriendList() async {
    final friends = await AppIm.iMManager.friendshipManager.getFriendList(filterBlack:true);
    userIDList.clear();
    userIDList.addAll(friends.map((e)=>e.userID!).whereType<String>());
    final users = friends.map((e)=>ISUserInfo.fromJson(e.toJson())).toList();
    onUserIDList(userIDList);
    friendList.assignAll(IMUtils.convertToAZList(users).cast<ISUserInfo>());
  }

  void onUserIDList(List<String> userIDList) {}

  _addFriend(dynamic user) {
    if (user is FriendInfo || user is BlacklistInfo) {
      _addUser(user.toJson());
    }
  }

  _delFriend(dynamic user) {
    if (user is FriendInfo || user is BlacklistInfo) {
      friendList.removeWhere((e) => e.userID == user.userID);
    }
  }

  _friendInfoChanged(FriendInfo user) {
    friendList.removeWhere((e) => e.userID == user.userID);
    _addUser(user.toJson());
  }

  void _addUser(Map<String, dynamic> json) {
    final info = ISUserInfo.fromJson(json);
    friendList.add(IMUtils.setAzPinyinAndTag(info) as ISUserInfo);

    // A-Z sort.
    SuspensionUtil.sortListBySuspensionTag(friendList);

    // show sus tag.
    SuspensionUtil.setShowSuspensionStatus(friendList);
    // IMUtil.convertToAZList(friendList);

    // friendList.refresh();
  }

  void viewFriendInfo(ISUserInfo info) => AppNavigator.startUserProfilePane(
        userID: info.userID!,
      );

  void searchFriend() => AppNavigator.startSearchFriend();
}
