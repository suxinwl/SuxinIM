import 'package:flutter/material.dart';
import 'package:app_im/app_im.dart';
import 'package:flutter_screenutil/flutter_screenutil.dart';
import 'package:get/get.dart';
import 'package:openim/routes/app_navigator.dart';
import 'package:openim_common/openim_common.dart';
import 'package:pull_to_refresh/pull_to_refresh.dart';

import '../conversation/conversation_logic.dart';

class GlobalSearchLogic extends CommonSearchLogic {
  final conversationLogic = Get.find<ConversationLogic>();
  final textMessageRefreshCtrl = RefreshController();
  final fileMessageRefreshCtrl = RefreshController();
  final contactsList = <UserInfo>[].obs;
  final employees = <DirectoryPerson>[].obs;
  final employeesLoading = false.obs;
  final employeeError = ''.obs;
  final employeeTotal = 0.obs;
  int _employeePage = 1, _searchGeneration = 0;
  final groupList = <GroupInfo>[].obs;
  final textSearchResultItems = <SearchResultItems>[].obs;

  // final fileSearchResultItems = <SearchResultItems>[].obs;
  final fileMessageList = <Message>[].obs;
  final index = 0.obs;
  final tabs = [
    StrRes.globalSearchAll,
    StrRes.globalSearchContacts,
    StrRes.globalSearchGroup,
    StrRes.globalSearchChatHistory,
    StrRes.globalSearchChatFile,
  ];

  int textMessagePageIndex = 1;
  int fileMessagePageIndex = 1;
  int count = 20;

  switchTab(int index) {
    this.index.value = index;
  }

  @override
  void clearList() {
    _searchGeneration++;
    employees.clear(); employeesLoading.value = false; employeeError.value = ''; employeeTotal.value = 0;
    _clearLocalLists();
  }
  void _clearLocalLists() {
    contactsList.clear();
    groupList.clear();
    textSearchResultItems.clear();
    fileMessageList.clear();
  }

  bool get isSearchNotResult =>
      searchKey.isNotEmpty && !employeesLoading.value && employeeError.value.isEmpty && employees.isEmpty && contactsList.isEmpty && groupList.isEmpty && textSearchResultItems.isEmpty && fileMessageList.isEmpty;

  Future<void> _searchEmployees(String keyword, int generation, {bool append = false}) async {
    employeesLoading.value = true; employeeError.value = '';
    if (!append) employees.clear();
    final page = append ? _employeePage + 1 : 1;
    try {
      final result = await HrmDirectory.instance.list(keyword: keyword, page: page, pageSize: 20);
      if (isClosed || generation != _searchGeneration || keyword != searchKey) return;
      if (append) { employees.addAll(result.items); } else { employees.assignAll(result.items); }
      employeeTotal.value = result.total; _employeePage = page;
    } catch (_) {
      if (!isClosed && generation == _searchGeneration) employeeError.value = '企业同事搜索暂不可用，请重试';
    } finally {
      if (!isClosed && generation == _searchGeneration) employeesLoading.value = false;
    }
  }
  void loadMoreEmployees() { if (!employeesLoading.value) _searchEmployees(searchKey, _searchGeneration, append: true); }
  void retryEmployees() => _searchEmployees(searchKey, _searchGeneration);
  void viewDirectoryPerson(DirectoryPerson person) => AppNavigator.startUserProfilePane(userID: person.userID, nickname: person.nickname, faceURL: person.faceURL);

  search() async {
    if (searchKey.isEmpty) { clearList(); return; }
    final generation = ++_searchGeneration;
    _clearLocalLists();
    _searchEmployees(searchKey, generation);
    final result = await LoadingView.singleton.wrap(
        asyncFunction: () => Future.wait([
              searchFriend(),
              // searchDeptMember(),
              searchGroup(),
              searchTextMessage(
                pageIndex: textMessagePageIndex = 1,
                count: count,
              ),
              searchFileMessage(
                pageIndex: fileMessagePageIndex = 1,
                count: count,
              ),
            ]));
    if (isClosed || generation != _searchGeneration) return;
    final friendList = (result[0] as List<FriendInfo>).map((e) => UserInfo(userID: e.userID, nickname: e.nickname, faceURL: e.faceURL));
    // final deptMemberList = result[1] as List<DeptMemberInfo>;
    final gList = result[1] as List<GroupInfo>;
    final textMessageResult = (result[2] as SearchResult).searchResultItems;
    final fileMessageResult = (result[3] as SearchResult).searchResultItems;

    _clearLocalLists();

    contactsList
        // ..assignAll(deptMemberList)
        .addAll(friendList);
    groupList.assignAll(gList);
    textSearchResultItems.assignAll(textMessageResult ?? []);
    fileMessageList.clear();
    if (null != fileMessageResult && fileMessageResult.isNotEmpty) {
      for (var element in fileMessageResult) {
        fileMessageList.addAll(element.messageList!);
      }
    }
    if ((textMessageResult ?? []).length < count) {
      textMessageRefreshCtrl.loadNoData();
    } else {
      textMessageRefreshCtrl.loadComplete();
    }
    if ((fileMessageResult ?? []).length < count) {
      fileMessageRefreshCtrl.loadNoData();
    } else {
      fileMessageRefreshCtrl.loadComplete();
    }
  }

  void loadTextMessage() async {
    final result = await searchTextMessage(pageIndex: ++textMessagePageIndex, count: count);
    final textMessageResult = result.searchResultItems;
    textSearchResultItems.addAll(textMessageResult ?? []);
    if ((textMessageResult ?? []).length < count) {
      textMessageRefreshCtrl.loadNoData();
    } else {
      textMessageRefreshCtrl.loadComplete();
    }
  }

  void loadFileMessage() async {
    final result = await searchFileMessage(pageIndex: ++fileMessagePageIndex, count: count);
    final fileMessageResult = result.searchResultItems;
    if (null != fileMessageResult && fileMessageResult.isNotEmpty) {
      for (var element in fileMessageResult) {
        fileMessageList.addAll(element.messageList!);
      }
    }
    if ((fileMessageResult ?? []).length < count) {
      fileMessageRefreshCtrl.loadNoData();
    } else {
      fileMessageRefreshCtrl.loadComplete();
    }
  }

  /// 最多显示2条
  List<T> subList<T>(List<T> list) => list.sublist(0, list.length > 2 ? 2 : list.length).toList();

  String calContent(Message message) => IMUtils.calContent(
        content: IMUtils.parseMsg(message, replaceIdToNickname: true),
        key: searchKey,
        style: Styles.ts_8E9AB0_14sp,
        usedWidth: 80.w + 26.w,
      );

  void viewUserProfile(UserInfo info) => AppNavigator.startUserProfilePane(
        userID: info.userID!,
        nickname: info.nickname,
        faceURL: info.faceURL,
      );

  void viewFile(Message message) => IMUtils.previewFile(message);

  void viewGroup(GroupInfo groupInfo) {
    conversationLogic.toChat(
      groupID: groupInfo.groupID,
      nickname: groupInfo.groupName,
      faceURL: groupInfo.faceURL,
      sessionType: groupInfo.sessionType,
    );
  }

  void viewMessage(SearchResultItems item) {
    if (item.messageCount! > 1) {
      AppNavigator.startExpandChatHistory(
        searchResultItems: item,
        defaultSearchKey: searchKey,
      );
    } else {
      AppNavigator.startPreviewChatHistory(
        conversationInfo: ConversationInfo(
          conversationID: item.conversationID!,
          showName: item.showName,
          faceURL: item.faceURL,
        ),
        message: item.messageList!.first,
      );
    }
  }
}

abstract class CommonSearchLogic extends GetxController {
  final searchCtrl = TextEditingController();
  final focusNode = FocusNode();

  void clearList();

  @override
  void onInit() {
    searchCtrl.addListener(_clearInput);
    super.onInit();
  }

  @override
  void onClose() {
    focusNode.dispose();
    searchCtrl.dispose();
    super.onClose();
  }

  _clearInput() {
    if (searchKey.isEmpty) {
      clearList();
    }
  }

  String get searchKey => searchCtrl.text.trim();

  Future<List<FriendInfo>> searchFriend() =>
      Apis.searchFriendInfo(searchCtrl.text.trim()).then((list) => list.map((e) => FriendInfo.fromJson(e.toJson())).toList());

  // Future<List<DeptMemberInfo>> searchDeptMember() =>
  //     OApis.searchDeptMember(keyword: searchKey)
  //         .then((value) => value.departmentMemberList ?? []);

  Future<List<GroupInfo>> searchGroup() =>
      AppIm.iMManager.groupManager.searchGroups(keywordList: [searchCtrl.text.trim()], isSearchGroupName: true, isSearchGroupID: true);

  Future<SearchResult> searchTextMessage({
    int pageIndex = 1,
    int count = 20,
  }) =>
      AppIm.iMManager.messageManager.searchLocalMessages(
        keywordList: [searchKey],
        messageTypeList: [MessageType.text, MessageType.atText],
        pageIndex: pageIndex,
        count: count,
      );

  Future<SearchResult> searchFileMessage({
    int pageIndex = 1,
    int count = 20,
  }) =>
      AppIm.iMManager.messageManager.searchLocalMessages(
        keywordList: [searchKey],
        messageTypeList: [MessageType.file],
        pageIndex: pageIndex,
        count: count,
      );

  String? parseID(e) {
    if (e is ConversationInfo) {
      return e.isSingleChat ? e.userID : e.groupID;
    } else if (e is GroupInfo) {
      return e.groupID;
    } else if (e is UserInfo) {
      return e.userID;
    } else if (e is FriendInfo) {
      return e.userID;
    } else {
      return null;
    }
  }

  String? parseNickname(e) {
    if (e is ConversationInfo) {
      return e.showName;
    } else if (e is GroupInfo) {
      return e.groupName;
    } else if (e is UserInfo) {
      return e.nickname;
    } else if (e is FriendInfo) {
      return e.nickname;
    } else {
      return null;
    }
  }

  String? parseFaceURL(e) {
    if (e is ConversationInfo) {
      return e.faceURL;
    } else if (e is GroupInfo) {
      return e.faceURL;
    } else if (e is UserInfo) {
      return e.faceURL;
    } else if (e is FriendInfo) {
      return e.faceURL;
    } else {
      return null;
    }
  }
}
