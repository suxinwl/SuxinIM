import 'dart:async';

import 'package:app_im/app_im.dart';
import 'package:flutter/material.dart';
import 'package:flutter_screenutil/flutter_screenutil.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:get/get.dart';
import 'package:openim_common/openim_common.dart';

import 'package:openim/pages/chat/group_setup/group_read_list/group_read_list_logic.dart';
import 'package:openim/pages/chat/group_setup/group_read_list/group_read_list_view.dart';

GroupMembersInfo _member(String id) => GroupMembersInfo.fromJson({
      'groupID': 'fixture-group',
      'userID': id,
      'nickname': id,
    });

GroupReadListLogic _logic(GroupReadMemberLoader loader,
        {Stream<GroupMessageReceipt>? receipts}) =>
    GroupReadListLogic(
      memberLoader: loader,
      receiptStream: receipts ?? const Stream.empty(),
      initialConversationID: 'sg_fixture-group',
      initialClientMsgID: 'fixture-message',
    );

GroupMessageReceipt _receipt() => GroupMessageReceipt.fromJson({
      'conversationID': 'sg_fixture-group',
      'groupMessageReadInfo': [
        {
          'clientMsgID': 'fixture-message',
          'hasReadCount': 1,
          'unreadCount': 1,
        },
      ],
    });

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  test(
      'initial failure is explicit, preserves error and retries the real loader',
      () async {
    final failure = StateError('synthetic network failure');
    var failRead = true;
    final filters = <int>[];
    final logic = _logic((cid, mid, {required filter, required count}) async {
      expect(cid, 'sg_fixture-group');
      expect(mid, 'fixture-message');
      filters.add(filter);
      if (filter == 0 && failRead) throw failure;
      return [_member(filter == 0 ? 'reader' : 'unread-member')];
    });
    logic.onInit();
    await Future.wait([
      logic.queryHasReadMembersList(),
      logic.queryUnreadMemberList(),
    ]);
    expect(logic.hasReadState.value, GroupReadLoadState.failed);
    expect(logic.hasReadError.value, same(failure));
    expect(logic.hasReadCount, isNull);
    expect(logic.unreadState.value, GroupReadLoadState.loaded);
    expect(logic.unreadCount, 1);
    failRead = false;
    expect(await logic.queryHasReadMembersList(), true);
    expect(logic.hasReadError.value, isNull);
    expect(logic.hasReadMemberList.single.userID, 'reader');
    expect(logic.hasReadCount, 1);
    expect(filters, [0, 1, 0]);
    logic.onClose();
  });

  test(
      'late success and error after close cannot publish or touch refresh state',
      () async {
    final read = Completer<List<GroupMembersInfo>>();
    final unread = Completer<List<GroupMembersInfo>>();
    var loads = 0;
    final logic = _logic((cid, mid, {required filter, required count}) {
      loads++;
      return filter == 0 ? read.future : unread.future;
    });
    logic.onInit();
    final reading = logic.queryHasReadMembersList();
    final unreading = logic.queryUnreadMemberList();
    logic.onClose();
    read.complete([_member('late-old-reader')]);
    unread.completeError(StateError('late-old-error'));
    expect(await reading, false);
    expect(await unreading, false);
    expect(logic.hasReadMemberList, isEmpty);
    expect(logic.unreadMemberList, isEmpty);
    expect(logic.hasReadState.value, GroupReadLoadState.loading);
    expect(logic.unreadState.value, GroupReadLoadState.loading);
    expect(logic.hasReadError.value, isNull);
    expect(logic.unreadError.value, isNull);
    expect(await logic.queryHasReadMembersList(), false);
    expect(loads, 2);
  });

  test(
      'tabs share only their own in-flight load and known zero differs from error',
      () async {
    final read = Completer<List<GroupMembersInfo>>();
    final unread = Completer<List<GroupMembersInfo>>();
    final counts = <int, int>{};
    final logic = _logic((cid, mid, {required filter, required count}) {
      counts[filter] = (counts[filter] ?? 0) + 1;
      if (filter == 0 && counts[filter] == 1) return read.future;
      if (filter == 1) return unread.future;
      return Future.value([]);
    });
    logic.onInit();
    final reading = logic.queryHasReadMembersList();
    final unreading = logic.queryUnreadMemberList();
    expect(identical(reading, logic.queryHasReadMembersList()), true);
    expect(identical(unreading, logic.queryUnreadMemberList()), true);
    read.completeError(StateError('read tab failed'));
    expect(await reading, false);
    expect(logic.unreadState.value, GroupReadLoadState.loading);
    unread.complete([_member('still-independent')]);
    expect(await unreading, true);
    expect(logic.hasReadCount, isNull);
    expect(logic.unreadCount, 1);
    expect(await logic.queryHasReadMembersList(), true);
    expect(logic.hasReadCount, 0);
    expect(logic.unreadCount, 1);
    expect(counts, {0: 2, 1: 1});
    logic.onClose();
  });

  test(
      'pending receipts coalesce one fresh load per tab without parallel queries',
      () async {
    final receipts = StreamController<GroupMessageReceipt>();
    final held = {
      0: List.generate(2, (_) => Completer<List<GroupMembersInfo>>()),
      1: List.generate(2, (_) => Completer<List<GroupMembersInfo>>()),
    };
    final counts = <int, int>{0: 0, 1: 0};
    final active = <int, int>{0: 0, 1: 0};
    final maxActive = <int, int>{0: 0, 1: 0};
    final logic = _logic((cid, mid, {required filter, required count}) {
      final call = counts[filter]!;
      counts[filter] = call + 1;
      active[filter] = active[filter]! + 1;
      if (active[filter]! > maxActive[filter]!) {
        maxActive[filter] = active[filter]!;
      }
      return held[filter]![call].future.then((members) {
        active[filter] = active[filter]! - 1;
        return members;
      });
    }, receipts: receipts.stream);
    try {
      logic.onInit();
      final oldRead = logic.queryHasReadMembersList();
      final oldUnread = logic.queryUnreadMemberList();
      for (var i = 0; i < 3; i++) {
        receipts.add(_receipt());
      }
      await Future<void>.delayed(Duration.zero);
      expect(counts, {0: 1, 1: 1});
      expect(identical(oldRead, logic.queryHasReadMembersList()), true);
      expect(identical(oldUnread, logic.queryUnreadMemberList()), true);
      held[0]![0].complete([_member('old-reader')]);
      held[1]![0].complete([_member('old-unread')]);
      await Future.wait([oldRead, oldUnread]);
      await Future<void>.delayed(Duration.zero);
      expect(counts, {0: 2, 1: 2},
          reason: 'A matching receipt must refresh a pending old snapshot.');
      final freshRead = logic.queryHasReadMembersList();
      final freshUnread = logic.queryUnreadMemberList();
      held[0]![1].complete([_member('fresh-reader')]);
      held[1]![1].complete([_member('fresh-unread')]);
      expect(await freshRead, true);
      expect(await freshUnread, true);
      expect(logic.hasReadMemberList.single.userID, 'fresh-reader');
      expect(logic.unreadMemberList.single.userID, 'fresh-unread');
      await Future<void>.delayed(Duration.zero);
      expect(counts, {0: 2, 1: 2});
      expect(maxActive, {0: 1, 1: 1});
    } finally {
      logic.onClose();
      for (final loads in held.values) {
        for (final load in loads) {
          if (!load.isCompleted) load.complete([]);
        }
      }
      await receipts.close();
    }
  });

  test('closing a dirty page prevents follow-up loads and late publication',
      () async {
    final receipts = StreamController<GroupMessageReceipt>();
    final read = Completer<List<GroupMembersInfo>>();
    final unread = Completer<List<GroupMembersInfo>>();
    final counts = <int, int>{0: 0, 1: 0};
    final logic = _logic((cid, mid, {required filter, required count}) {
      counts[filter] = counts[filter]! + 1;
      return filter == 0 ? read.future : unread.future;
    }, receipts: receipts.stream);
    logic.onInit();
    final reading = logic.queryHasReadMembersList();
    final unreading = logic.queryUnreadMemberList();
    receipts.add(_receipt());
    receipts.add(_receipt());
    await Future<void>.delayed(Duration.zero);
    logic.onClose();
    read.complete([_member('closed-reader')]);
    unread.complete([_member('closed-unread')]);
    expect(await reading, false);
    expect(await unreading, false);
    await Future<void>.delayed(Duration.zero);
    expect(counts, {0: 1, 1: 1});
    expect(logic.hasReadMemberList, isEmpty);
    expect(logic.unreadMemberList, isEmpty);
    await receipts.close();
  });

  testWidgets(
      'actual page shows unknown, failure and retry without empty success',
      (tester) async {
    Get.testMode = true;
    var allowUnread = false;
    var allowRead = false;
    final logic = _logic((cid, mid, {required filter, required count}) async {
      if ((filter == 1 && !allowUnread) || (filter == 0 && !allowRead)) {
        throw StateError('synthetic reader request refused');
      }
      return [];
    });
    Get.put(logic);
    await tester.pumpWidget(ScreenUtilInit(
      designSize: const Size(375, 812),
      builder: (_, __) => GetMaterialApp(
        locale: const Locale('en', 'US'),
        translations: TranslationService(),
        home: GroupReadListPage(),
      ),
    ));
    await tester.pumpAndSettle();
    expect(find.text('Unread (--)'), findsOneWidget);
    expect(find.text('Read (--)'), findsOneWidget);
    expect(
        find.byKey(const ValueKey('group-read-unread-error')), findsOneWidget);
    expect(find.text('Could not load message readers. Please retry.'),
        findsOneWidget);
    expect(find.text('No members'), findsNothing);
    allowUnread = true;
    await tester.tap(find.byKey(const ValueKey('group-read-unread-retry')));
    await tester.pumpAndSettle();
    expect(find.text('Unread (0)'), findsOneWidget);
    expect(find.text('Read (--)'), findsOneWidget);
    expect(find.text('No members'), findsOneWidget);
    allowRead = true;
    await tester.tap(find.text('Read (--)'));
    await tester.pumpAndSettle();
    expect(logic.index.value, 1);
    expect(find.text('Read (0)'), findsOneWidget);
    expect(find.text('No members'), findsOneWidget);
    expect(tester.takeException(), isNull);
    await tester.pumpWidget(const SizedBox());
    await Get.delete<GroupReadListLogic>();
    Get.reset();
  });
}
