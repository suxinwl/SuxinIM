import 'package:flutter/material.dart';
import 'package:app_im/app_im.dart';
import 'package:get/get.dart';
import 'package:openim/pages/contacts/group_profile_panel/group_profile_panel_logic.dart';
import 'package:openim/routes/app_navigator.dart';
import 'package:openim_common/openim_common.dart';
import 'package:pull_to_refresh/pull_to_refresh.dart';
import 'package:sprintf/sprintf.dart';

enum SearchType {
  user,
  group,
}

class AddContactsBySearchLogic extends GetxController {
  final refreshCtrl = RefreshController();
  final searchCtrl = TextEditingController();
  final focusNode = FocusNode();
  final userInfoList = <DirectoryPerson>[].obs;
  final searchError = ''.obs;
  final searching = false.obs;
  final searched = false.obs;
  int _searchGeneration = 0;
  final groupInfoList = <GroupInfo>[].obs;
  late SearchType searchType;
  int pageNo = 0;

  @override
  void onClose() {
    _searchGeneration++;
    refreshCtrl.dispose();
    searchCtrl.dispose();
    focusNode.dispose();
    super.onClose();
  }

  @override
  void onInit() {
    searchType = Get.arguments['searchType'] ?? SearchType.user;
    searchCtrl.addListener(() {
      _searchGeneration++;
      userInfoList.clear();
      searchError.value = '';
      searched.value = false;
      searching.value = false;
      if (searchKey.isEmpty) {
        focusNode.requestFocus();
        userInfoList.clear();
        groupInfoList.clear();
      }
    });
    super.onInit();
  }

  bool get isSearchUser => searchType == SearchType.user;

  String get searchKey => searchCtrl.text.trim();

  bool get isNotFoundUser => searched.value && !searching.value && searchError.value.isEmpty && userInfoList.isEmpty && searchKey.isNotEmpty;

  bool get isNotFoundGroup => groupInfoList.isEmpty && searchKey.isNotEmpty;

  void search() {
    if (searchKey.isEmpty) return;
    if (isSearchUser) {
      searchUser();
    } else {
      searchGroup();
    }
  }

  void searchUser() => _loadUsers();

  void loadMoreUser() => _loadUsers(append: true);

  Future<void> _loadUsers({bool append = false}) async {
    if (searching.value || searchKey.isEmpty) return;
    final attempt = ++_searchGeneration, query = searchKey;
    final nextPage = append ? pageNo + 1 : 1;
    searching.value = true; searchError.value = ''; searched.value = true;
    if (!append) userInfoList.clear();
    try {
      final result = await HrmDirectory.instance.list(keyword: query, page: nextPage, pageSize: 20);
      if (isClosed || attempt != _searchGeneration) return;
      if (append) { userInfoList.addAll(result.items); } else { userInfoList.assignAll(result.items); }
      pageNo = nextPage;
      refreshCtrl.refreshCompleted();
      if (userInfoList.length >= result.total) { refreshCtrl.loadNoData(); } else { refreshCtrl.loadComplete(); }
    } catch (_) {
      if (isClosed || attempt != _searchGeneration) return;
      searchError.value = '员工搜索暂不可用，请重试';
      refreshCtrl.loadFailed();
    } finally {
      if (!isClosed && attempt == _searchGeneration) searching.value = false;
    }
  }

  void searchGroup() async {
    var list = await AppIm.iMManager.groupManager.getGroupsInfo(
      groupIDList: [searchKey],
    );
    groupInfoList.assignAll(list);
  }

  String getMatchContent(UserFullInfo userInfo) {
    // String searchPrefix = "%s";
    // if (keyword == userInfo.userID) {
    //   searchPrefix = StrRes.searchIDIs;
    // } else if (keyword == userInfo.phoneNumber) {
    //   searchPrefix = StrRes.searchPhoneIs;
    // } else if (keyword == userInfo.email) {
    //   searchPrefix = StrRes.searchEmailIs;
    // } else if (keyword == userInfo.nickname) {
    //   searchPrefix = StrRes.searchNicknameIs;
    // }
    return sprintf(StrRes.searchNicknameIs, [userInfo.nickname]);
  }

  String? getShowName(dynamic info) {
    if (info is DirectoryPerson) {
      return info.nickname;
    } else if (info is GroupInfo) {
      return info.groupName;
    }
    return null;
  }

  void viewInfo(dynamic info) {
    if (info is DirectoryPerson) {
      AppNavigator.startUserProfilePane(
        userID: info.userID,
        nickname: info.nickname,
        faceURL: info.faceURL,
      );
    } else if (info is GroupInfo) {
      AppNavigator.startGroupProfilePanel(
        groupID: info.groupID,
        joinGroupMethod: JoinGroupMethod.search,
      );
    }
  }

  String getShowTitle(info) {
    if (!isSearchUser) {
      return sprintf(StrRes.searchGroupNicknameIs, [getShowName(info)]);
    }

    final person = info as DirectoryPerson;
    return '${person.nickname}  ·  工号：${person.employeeNo.isEmpty ? '—' : person.employeeNo}${person.department.isEmpty ? '' : '  ·  ${person.department}'}';
  }
}
