import 'dart:async';

import 'package:app_im/app_im.dart';
import 'package:flutter/foundation.dart';
import 'package:get/get.dart';
import 'package:pull_to_refresh/pull_to_refresh.dart';

import '../../../../core/controller/im_controller.dart';

enum GroupReadLoadState { initial, loading, loaded, failed }

typedef GroupReadMemberLoader = Future<List<GroupMembersInfo>> Function(
  String conversationID,
  String clientMsgID, {
  required int filter,
  required int count,
});

class GroupReadListLogic extends GetxController {
  GroupReadListLogic({
    GroupReadMemberLoader? memberLoader,
    Stream<GroupMessageReceipt>? receiptStream,
    String? initialConversationID,
    String? initialClientMsgID,
  })  : _memberLoader = memberLoader ?? _loadMembers,
        _receiptStream = receiptStream,
        _initialConversationID = initialConversationID,
        _initialClientMsgID = initialClientMsgID;

  final GroupReadMemberLoader _memberLoader;
  final Stream<GroupMessageReceipt>? _receiptStream;
  final String? _initialConversationID;
  final String? _initialClientMsgID;
  StreamSubscription<GroupMessageReceipt>? _receiptSubscription;
  Future<bool>? _hasReadPending;
  Future<bool>? _unreadPending;
  bool _hasReadDirty = false;
  bool _unreadDirty = false;
  bool _closed = false;

  late String conversationID;
  late String clientMsgID;
  final hasReadMemberList = <GroupMembersInfo>[].obs;
  final unreadMemberList = <GroupMembersInfo>[].obs;
  final hasReadState = GroupReadLoadState.initial.obs;
  final unreadState = GroupReadLoadState.initial.obs;
  final hasReadError = Rxn<Object>();
  final unreadError = Rxn<Object>();
  final hasReadRefreshController = RefreshController();
  final unreadRefreshController = RefreshController();
  final index = 0.obs;
  final count = 100000;

  static Future<List<GroupMembersInfo>> _loadMembers(
    String conversationID,
    String clientMsgID, {
    required int filter,
    required int count,
  }) =>
      AppIm.iMManager.messageManager.getGroupMessageReaderList(
          conversationID, clientMsgID,
          filter: filter, count: count);

  @override
  void onInit() {
    conversationID = _initialConversationID ?? Get.arguments['conversationID'];
    clientMsgID = _initialClientMsgID ?? Get.arguments['clientMsgID'];
    unawaited(queryHasReadMembersList());
    unawaited(queryUnreadMemberList());
    final receipts =
        _receiptStream ?? Get.find<IMController>().recvGroupReadReceiptSubject;
    _receiptSubscription = receipts.listen((receipt) {
      if (_closed || receipt.conversationID != conversationID) return;
      final message = receipt.groupMessageReadInfo.firstWhereOrNull(
        (element) => element.clientMsgID == clientMsgID,
      );
      if (message != null) {
        unawaited(_query(hasRead: true, refreshAfterPending: true));
        unawaited(_query(hasRead: false, refreshAfterPending: true));
      }
    });
    super.onInit();
  }

  @override
  void onClose() {
    _closed = true;
    _hasReadDirty = false;
    _unreadDirty = false;
    final subscription = _receiptSubscription;
    _receiptSubscription = null;
    if (subscription != null) {
      unawaited(subscription.cancel().catchError((Object error) {
        debugPrint('Group read listener cleanup failed: ${error.runtimeType}');
      }));
    }
    hasReadRefreshController.dispose();
    unreadRefreshController.dispose();
    super.onClose();
  }

  Future<bool> queryHasReadMembersList() => _query(hasRead: true);
  Future<bool> queryUnreadMemberList() => _query(hasRead: false);

  Future<bool> _query(
      {required bool hasRead, bool refreshAfterPending = false}) {
    if (_closed) return Future.value(false);
    final pending = hasRead ? _hasReadPending : _unreadPending;
    if (pending != null) {
      // A receipt may be newer than the in-flight query's snapshot. Keep one
      // follow-up per tab; ordinary repeated clicks only share the same load.
      if (refreshAfterPending) {
        if (hasRead) {
          _hasReadDirty = true;
        } else {
          _unreadDirty = true;
        }
      }
      return pending;
    }
    final completion = Completer<bool>();
    if (hasRead) {
      _hasReadPending = completion.future;
    } else {
      _unreadPending = completion.future;
    }
    final state = hasRead ? hasReadState : unreadState;
    final errorState = hasRead ? hasReadError : unreadError;
    state.value = GroupReadLoadState.loading;
    errorState.value = null;
    Future<bool>.sync(() async {
      try {
        final list = await _memberLoader(conversationID, clientMsgID,
            filter: hasRead ? 0 : 1, count: count);
        if (_closed) return false;
        (hasRead ? hasReadMemberList : unreadMemberList).assignAll(list);
        state.value = GroupReadLoadState.loaded;
        (hasRead ? hasReadRefreshController : unreadRefreshController)
            .loadNoData();
        return true;
      } catch (error) {
        // Initial load, tabs and receipt callbacks all observe failures here.
        // Preserve the actual error and an explicit failed state, not empty success.
        if (!_closed) {
          errorState.value = error;
          state.value = GroupReadLoadState.failed;
          (hasRead ? hasReadRefreshController : unreadRefreshController)
              .loadFailed();
        }
        return false;
      } finally {
        // Clear before completing the caller so an immediate retry is a new load.
        var rerun = false;
        if (hasRead) {
          if (identical(_hasReadPending, completion.future)) {
            _hasReadPending = null;
            rerun = _hasReadDirty;
            _hasReadDirty = false;
          }
        } else {
          if (identical(_unreadPending, completion.future)) {
            _unreadPending = null;
            rerun = _unreadDirty;
            _unreadDirty = false;
          }
        }
        if (rerun && !_closed) unawaited(_query(hasRead: hasRead));
      }
    }).then(completion.complete, onError: (Object error, StackTrace stack) {
      // A state/refresh notification failure is also observed at this boundary.
      if (!_closed) {
        errorState.value = error;
        state.value = GroupReadLoadState.failed;
      }
      completion.complete(false);
    });
    return completion.future;
  }

  void switchTab(int selected) {
    index.value = selected;
    if (selected == 0) {
      unawaited(queryUnreadMemberList());
    } else {
      unawaited(queryHasReadMembersList());
    }
  }

  int? get unreadCount => unreadState.value == GroupReadLoadState.loaded
      ? unreadMemberList.length
      : null;
  int? get hasReadCount => hasReadState.value == GroupReadLoadState.loaded
      ? hasReadMemberList.length
      : null;
}
