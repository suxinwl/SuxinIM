import 'dart:async';
import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:file_picker/file_picker.dart';
import 'package:openim_common/src/models/login_certificate.dart';
import 'package:openim_common/src/native_local_vault.dart';
import 'package:openim_common/src/widgets/local_archive_access.dart';
import 'package:openim_common/src/widgets/local_archive_page.dart';

class SyntheticEncryptedPicker extends FilePicker {
  int calls = 0;
  @override
  Future<FilePickerResult?> pickFiles(
      {String? dialogTitle,
      String? initialDirectory,
      FileType type = FileType.any,
      List<String>? allowedExtensions,
      Function(FilePickerStatus)? onFileLoading,
      bool allowCompression = false,
      int compressionQuality = 0,
      bool allowMultiple = false,
      bool withData = false,
      bool withReadStream = false,
      bool lockParentWindow = false,
      bool readSequential = false}) async {
    expect(type, FileType.any);
    expect(allowMultiple, isFalse);
    expect(withData, isFalse);
    calls++;
    return FilePickerResult([
      PlatformFile(path: 'puty-media:synthetic-source', name: '合成.txt', size: 3)
    ]);
  }
}

class FakeArchiveAccess implements LocalArchiveAccess {
  bool online = true;
  final locks = StreamController<void>.broadcast();
  final changes = StreamController<void>.broadcast();
  final List<List<Object>> calls = [];
  Completer<List<Map<String, dynamic>>>? messagePending;
  Completer<ArchiveAttachmentPage>? attachmentPending;
  Completer<ArchiveAttachment?>? importPending;
  Completer<ArchiveCleanupPreview>? previewPending;
  ArchiveAttachment? imported = const ArchiveAttachment(
      taskID: 'local-import-task',
      name: '已有合成文件.pdf',
      month: '202611',
      size: 123,
      conversationID: 'chat-a',
      messageID: '',
      localImportID: 'opaque-local-import',
      firstImportedAt: 1793721600000);
  bool failPreview = false;
  List<Map<String, dynamic>> messageValues = [];
  String? picked = 'opaque-selection';
  bool saveResult = true;
  bool failExport = false;
  bool failCommit = false;
  @override
  bool get unlocked => online;
  @override
  Stream<void> get locked => locks.stream;
  @override
  Stream<void> get invalidated => changes.stream;
  void loseAccess() {
    online = false;
    locks.add(null);
  }

  @override
  Future<List<ArchiveConversation>> conversations(int offset) async =>
      offset == 0
          ? const [
              ArchiveConversation('chat-a', '合成联系人 A'),
              ArchiveConversation('chat-b', '合成联系人 B')
            ]
          : [];
  @override
  Future<List<Map<String, dynamic>>> messages(String id,
      {required int before, String beforeID = ''}) async {
    calls.add(['messages', id, before, beforeID]);
    if (messagePending != null) return await messagePending!.future;
    return messageValues;
  }

  @override
  Future<ArchiveAttachmentPage> attachments(
      {String? conversationID, String? cursor}) async {
    calls.add(['attachments', conversationID ?? '', cursor ?? '']);
    if (attachmentPending != null) return await attachmentPending!.future;
    return const ArchiveAttachmentPage([
      ArchiveAttachment(
          taskID: 'opaque-task',
          name: '原文件名.txt',
          month: '202611',
          size: 1234,
          conversationID: 'chat-a',
          messageID: 'm1'),
    ], null);
  }

  @override
  Future<void> clear(String id, int from, int to) async {
    calls.add(['clear', id, from, to]);
    changes.add(null);
  }

  @override
  Future<ArchiveCleanupPreview> previewClear(
      String id, int from, int to) async {
    calls.add(['preview', id, from, to]);
    if (failPreview) throw StateError('synthetic private failure');
    if (previewPending != null) return await previewPending!.future;
    return const ArchiveCleanupPreview(2, 1, 3, 1);
  }

  @override
  Future<ArchiveCleanupPreview> previewClearScopes(
      List<String> ids, int from, int toExclusive) async {
    calls.add(['previewScopes', List<String>.from(ids), from, toExclusive]);
    if (failPreview) throw StateError('synthetic private failure');
    if (previewPending != null) return await previewPending!.future;
    return const ArchiveCleanupPreview(2, 1, 3, 1, 4321);
  }

  @override
  Future<void> clearScopes(List<String> ids, int from, int toExclusive) async {
    calls.add(['clearScopes', List<String>.from(ids), from, toExclusive]);
    changes.add(null);
  }

  @override
  Future<ArchiveAttachment?> importAttachment(String id) async {
    calls.add(['import', id]);
    if (importPending != null) return await importPending!.future;
    return imported;
  }

  @override
  Future<String> exportBackup(String phrase) async {
    calls.add(['export', phrase]);
    if (failExport) {
      throw StateError('synthetic secret/path must never be rendered');
    }
    return 'private-encrypted-zip';
  }

  @override
  Future<bool> saveBackup(String path) async {
    calls.add(['save', path]);
    return saveResult;
  }

  @override
  Future<String?> pickRestoreBackup() async {
    calls.add(['pick']);
    return picked;
  }

  @override
  Future<String> prepareRestore(String selected, String phrase) async {
    calls.add(['prepare', selected, phrase]);
    return 'opaque-restore-id';
  }

  @override
  Future<void> cancelRestore(String id) async {
    calls.add(['cancel', id]);
  }

  @override
  Future<void> commitRestore(String id) async {
    calls.add(['commit', id]);
    if (failCommit) throw StateError('synthetic failure');
    loseAccess();
  }
}

Future<void> openPage(WidgetTester tester, FakeArchiveAccess access,
    {Future<void> Function()? restored,
    String? conversationID = 'chat-a'}) async {
  await tester.pumpWidget(MaterialApp(
      home: LocalArchivePage(
          conversationID: conversationID,
          access: access,
          onRestored: restored)));
  await settle(tester);
}

Future<void> tab(WidgetTester tester, String text) async {
  await tester.tap(find.text(text));
  await settle(tester);
}

Future<void> inputPhrase(WidgetTester tester, {bool creating = false}) async {
  await tester.enterText(find.byKey(const ValueKey('backup-passphrase')),
      'synthetic-backup-passphrase');
  if (creating) {
    await tester.enterText(find.byKey(const ValueKey('backup-confirm')),
        'synthetic-backup-passphrase');
  }
  await tester.tap(find.text(creating ? '生成加密备份' : '校验备份'));
  await settle(tester);
}

Future<void> settle(WidgetTester tester) async {
  // Dialog operations deliberately keep the progress indicator alive.
  for (var i = 0; i < 5; i++) {
    await tester.pump(const Duration(milliseconds: 150));
  }
}

Finder button(String text) => find.ancestor(
    of: find.text(text),
    matching: find.byWidgetPredicate((widget) => widget is ButtonStyleButton));

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  test(
      'native adapter preserves exact cursor and opaque backup method contract without key material',
      () async {
    final calls = <MethodCall>[];
    var quiesced = 0;
    NativeLocalVault.quiesceSDKForRestore = () async {
      quiesced++;
    };
    final messenger =
        TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
    messenger.setMockMethodCallHandler(NativeLocalVault.channel, (call) async {
      calls.add(call);
      switch (call.method) {
        case 'unlock':
          return {'userID': 'test_ui_archive', 'actorGeneration': 1};
        case 'archive/page':
          return jsonEncode({
            'items': [
              {'client_msg_id': 'x', 'send_time': 1000, 'content': 'synthetic'}
            ]
          });
        case 'attachment/list':
          return jsonEncode({
            'items': [
              {
                'taskID': 'opaque-task',
                'name': '合成.txt',
                'size': 3,
                'month': '202611'
              }
            ],
            'nextCursor': 'opaque-page'
          });
        case 'archive/previewClear':
          return jsonEncode({
            ...Map<String, dynamic>.from(call.arguments as Map),
            'messageCount': 2,
            'localImportCount': 1,
            'attachmentReferences': 3,
            'filesToDelete': 1
          });
        case 'archive/previewClearScopes':
          return jsonEncode({
            ...Map<String, dynamic>.from(call.arguments as Map),
            'messageCount': 2,
            'localImportCount': 1,
            'attachmentReferences': 3,
            'filesToDelete': 1,
            'estimatedReleaseBytes': 4321
          });
        case 'archive/clearScopes':
          return true;
        case 'attachment/import':
          return jsonEncode({
            'taskID': 'import-task',
            'name': '合成.txt',
            'size': 3,
            'month': '202611',
            'conversationID': 'chat-a',
            'messageID': '',
            'localImportID': 'opaque-import',
            'firstImportedAt': 1793721600000
          });
        case 'source/discard':
          return null;
        case 'backup/export':
          return 'opaque-cipher-zip';
        case 'backup/save':
          return true;
        case 'backup/pick':
          return 'opaque-picker';
        case 'backup/prepare':
          return 'opaque-restore';
        case 'backup/cancel':
          return null;
        case 'backup/commit':
          expect(quiesced, 1);
          return true;
        case 'lock':
          return true;
      }
      throw StateError('Unexpected method');
    });
    addTearDown(() async {
      await NativeLocalVault.lock();
      NativeLocalVault.quiesceSDKForRestore = null;
      messenger.setMockMethodCallHandler(NativeLocalVault.channel, null);
    });
    await NativeLocalVault.unlock(LoginCertificate.fromJson({
      'userID': 'test_ui_archive',
      'imToken': 'synthetic-im',
      'chatToken': 'synthetic-chat'
    }));
    const access = NativeArchiveAccess();
    expect(
        (await access.messages('chat-a', before: 1000, beforeID: 'anchor'))
            .single['client_msg_id'],
        'x');
    expect(calls.last.arguments, {
      'conversationID': 'chat-a',
      'before': 1000,
      'beforeID': 'anchor',
      'limit': 50
    });
    final page =
        await access.attachments(conversationID: 'chat-a', cursor: 'cursor');
    expect(calls.last.arguments,
        {'conversationID': 'chat-a', 'cursor': 'cursor', 'limit': 50});
    expect(page.items.single.name, '合成.txt');
    expect(page.nextCursor, 'opaque-page');
    final preview = await access.previewClear('chat-a', 100, 200);
    expect(calls.last.arguments,
        {'conversationID': 'chat-a', 'from': 100, 'to': 200});
    expect(preview.localImportCount, 1);
    final union = await access.previewClearScopes(['chat-a', 'chat-b'], 0, 200);
    expect(calls.last.arguments, {
      'conversationIDs': ['chat-a', 'chat-b'],
      'from': 0,
      'toExclusive': 200
    });
    expect(union.estimatedReleaseBytes, 4321);
    await access.clearScopes(['chat-a', 'chat-b'], 0, 200);
    expect(calls.last.method, 'archive/clearScopes');
    final picker = SyntheticEncryptedPicker();
    FilePicker.platform = picker;
    final imported = await access.importAttachment('chat-a');
    expect(picker.calls, 1);
    expect(imported!.localImportID, 'opaque-import');
    expect(
        calls
            .singleWhere((call) => call.method == 'attachment/import')
            .arguments,
        {'path': 'puty-media:synthetic-source', 'conversationID': 'chat-a'});
    expect(calls.last.method, 'source/discard');
    expect(calls.last.arguments, {'path': 'puty-media:synthetic-source'});
    final path = await access.exportBackup('synthetic-independent-phrase');
    expect(await access.saveBackup(path), isTrue);
    expect(calls.last.arguments, {'path': 'opaque-cipher-zip'});
    final picked = await access.pickRestoreBackup();
    final restore =
        await access.prepareRestore(picked!, 'synthetic-independent-phrase');
    await access.cancelRestore(restore);
    expect(calls.last.arguments, {'restoreID': 'opaque-restore'});
    await access.commitRestore(restore);
    expect(quiesced, 1);
    expect(access.unlocked, isFalse);
    expect(calls.last.method, 'lock');
    for (final call in calls) {
      if (call.arguments is Map) {
        expect(
            (call.arguments as Map)
                .keys
                .any((key) => key.toString().toLowerCase().contains('master')),
            isFalse);
      }
    }
  });

  test('native restore cannot commit when SDK stop gate is missing or fails',
      () async {
    final methods = <String>[];
    final messenger =
        TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
    messenger.setMockMethodCallHandler(NativeLocalVault.channel, (call) async {
      methods.add(call.method);
      if (call.method == 'unlock') {
        return {'userID': 'test_ui_archive', 'actorGeneration': 1};
      }
      if (call.method == 'lock') return true;
      throw StateError('Unexpected native commit');
    });
    addTearDown(() async {
      NativeLocalVault.quiesceSDKForRestore = null;
      await NativeLocalVault.lock();
      messenger.setMockMethodCallHandler(NativeLocalVault.channel, null);
    });
    await NativeLocalVault.unlock(LoginCertificate.fromJson({
      'userID': 'test_ui_archive',
      'imToken': 'synthetic',
      'chatToken': 'synthetic'
    }));
    NativeLocalVault.quiesceSDKForRestore = null;
    await expectLater(
        NativeLocalVault.commitRestore('opaque-restore'), throwsStateError);
    expect(methods, ['unlock']);
    NativeLocalVault.quiesceSDKForRestore = () async {
      throw StateError('Synthetic SDK stop refusal');
    };
    await expectLater(
        NativeLocalVault.commitRestore('opaque-restore'), throwsStateError);
    expect(methods, ['unlock', 'lock']);
    expect(NativeLocalVault.unlocked, isFalse);
    expect(NativeLocalVault.restoring, isFalse);
  });

  test(
      'native preview echo drift and import owner drift fail closed and discard only the encrypted selection',
      () async {
    final calls = <MethodCall>[];
    final messenger =
        TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
    messenger.setMockMethodCallHandler(NativeLocalVault.channel, (call) async {
      calls.add(call);
      switch (call.method) {
        case 'unlock':
          return {'userID': 'test_ui_archive', 'actorGeneration': 1};
        case 'archive/previewClear':
          return jsonEncode({
            'conversationID': 'chat-b',
            'from': 100,
            'to': 200,
            'messageCount': 2,
            'localImportCount': 1,
            'attachmentReferences': 3,
            'filesToDelete': 1
          });
        case 'archive/previewClearScopes':
          return jsonEncode({
            'conversationIDs': ['chat-b'],
            'from': 100,
            'toExclusive': 200,
            'messageCount': 2,
            'localImportCount': 1,
            'attachmentReferences': 3,
            'filesToDelete': 1,
            'estimatedReleaseBytes': 4321
          });
        case 'attachment/import':
          return jsonEncode({
            'taskID': 'x',
            'name': 'untrusted-name',
            'size': 1,
            'conversationID': 'chat-b',
            'messageID': '',
            'localImportID': 'i',
            'firstImportedAt': 1
          });
        case 'source/discard':
          return null;
        case 'lock':
          return true;
        default:
          throw StateError('Unexpected write');
      }
    });
    addTearDown(() async {
      await NativeLocalVault.lock();
      messenger.setMockMethodCallHandler(NativeLocalVault.channel, null);
    });
    await NativeLocalVault.unlock(LoginCertificate.fromJson({
      'userID': 'test_ui_archive',
      'imToken': 'synthetic-im',
      'chatToken': 'synthetic-chat'
    }));
    const access = NativeArchiveAccess();
    await expectLater(
        access.previewClear('chat-a', 100, 200), throwsStateError);
    await expectLater(
        access.previewClearScopes(['chat-a'], 100, 200), throwsStateError);
    FilePicker.platform = SyntheticEncryptedPicker();
    await expectLater(access.importAttachment('chat-a'), throwsStateError);
    expect(calls.map((call) => call.method), [
      'unlock',
      'archive/previewClear',
      'archive/previewClearScopes',
      'attachment/import',
      'source/discard'
    ]);
    expect(calls.last.arguments, {'path': 'puty-media:synthetic-source'});
  });
  test(
      'cleanup calendar bounds use fixed UTC+08 across month/year and leap day',
      () {
    final values =
        archiveDateBounds(DateTime(2024, 2, 29), DateTime(2024, 3, 1));
    expect(DateTime.fromMillisecondsSinceEpoch(values[0], isUtc: true),
        DateTime.utc(2024, 2, 28, 16));
    expect(DateTime.fromMillisecondsSinceEpoch(values[1], isUtc: true),
        DateTime.utc(2024, 3, 1, 15, 59, 59, 999));
    expect(archiveCalendarDate(DateTime.utc(2026, 12, 31, 18)).year, 2027);
    expect(() => archiveDateBounds(DateTime(2026, 2), DateTime(2026, 1)),
        throwsArgumentError);
    expect(archiveCleanupBounds(DateTime(2024, 2, 29), DateTime(2024, 3, 1)), [
      DateTime.utc(2024, 2, 28, 16).millisecondsSinceEpoch,
      DateTime.utc(2024, 3, 1, 16).millisecondsSinceEpoch
    ]);
    expect(archiveBeforeBounds(DateTime(2027, 1, 1)),
        [0, DateTime.utc(2026, 12, 31, 16).millisecondsSinceEpoch]);
  });

  test(
      'attachment DTO rejects missing identity and invalid size without partial results',
      () {
    expect(ArchiveAttachment.parse({'taskID': 'x', 'name': 'x', 'size': -1}),
        isNull);
    expect(ArchiveAttachment.parse({'taskID': '', 'name': 'x', 'size': 1}),
        isNull);
    expect(
        () => ArchiveAttachmentPage.parse({
              'items': [
                {'taskID': 'x', 'name': 'x', 'size': 1.5}
              ]
            }),
        throwsFormatException);
    expect(() => ArchiveAttachmentPage.parse({'items': [], 'nextCursor': 1}),
        throwsFormatException);
  });

  test(
      'import and cleanup DTO require complete typed counts and import identity',
      () {
    expect(
        ArchiveAttachment.parse({
          'taskID': 'i',
          'name': 'x',
          'size': 1,
          'localImportID': 'i',
          'firstImportedAt': -1
        }),
        isNull);
    expect(
        ArchiveAttachment.parse(
            {'taskID': 'i', 'name': 'x', 'size': 1, 'firstImportedAt': 1}),
        isNull);
    for (final count in [null, -1, 1.5, '1', true]) {
      expect(
          () => ArchiveCleanupPreview.parse({
                'messageCount': count,
                'localImportCount': 0,
                'attachmentReferences': 0,
                'filesToDelete': 0
              }),
          throwsFormatException);
    }
    final valid = ArchiveCleanupPreview.parse({
      'messageCount': 2,
      'localImportCount': 1,
      'attachmentReferences': 3,
      'filesToDelete': 1
    });
    expect(valid.localImportCount, 1);
    for (final invalidBytes in [null, -1, 1.5, '1', true, 9007199254740992]) {
      expect(
          () => ArchiveCleanupPreview.parse({
                'messageCount': 2,
                'localImportCount': 1,
                'attachmentReferences': 3,
                'filesToDelete': 1,
                'estimatedReleaseBytes': invalidBytes
              }, requireReleaseBytes: true),
          throwsFormatException);
    }
  });

  testWidgets(
      'import requires a selected conversation before opening the native picker',
      (tester) async {
    final access = FakeArchiveAccess();
    addTearDown(access.locks.close);
    await openPage(tester, access, conversationID: null);
    await tab(tester, '已存附件');
    await tester.tap(find.text('导入已有文件'));
    await settle(tester);
    expect(find.text('请先选择联系人或群组，再导入已有文件。'), findsOneWidget);
    expect(access.calls.where((call) => call.first == 'import'), isEmpty);
  });

  testWidgets('import stays local scoped and preserves external originals',
      (tester) async {
    final access = FakeArchiveAccess();
    addTearDown(access.locks.close);
    await openPage(tester, access);
    await tab(tester, '已存附件');
    await tester.tap(find.text('导入已有文件'));
    await settle(tester);
    expect(access.calls, [
      ['import', 'chat-a']
    ]);
    expect(find.text('已有合成文件.pdf'), findsOneWidget);
    expect(find.textContaining('本机导入'), findsWidgets);
    expect(find.text('文件已加密导入本机档案；未发送消息，外部原件保留。'), findsOneWidget);
    expect(find.byIcon(Icons.share), findsNothing);
  });

  testWidgets('late import after lock never renders file metadata',
      (tester) async {
    final access = FakeArchiveAccess()..importPending = Completer();
    addTearDown(access.locks.close);
    await openPage(tester, access);
    await tab(tester, '已存附件');
    await tester.tap(find.text('导入已有文件'));
    await tester.pump();
    access.loseAccess();
    await tester.pump();
    access.importPending!.complete(access.imported);
    await settle(tester);
    expect(find.text('已有合成文件.pdf'), findsNothing);
    expect(find.text('合成联系人 A'), findsNothing);
  });

  testWidgets(
      'wrong-scope native import result is rejected without rendering its name',
      (tester) async {
    final access = FakeArchiveAccess()
      ..imported = const ArchiveAttachment(
          taskID: 'x',
          name: 'wrong-scope-name',
          month: '202611',
          size: 1,
          conversationID: 'chat-b',
          messageID: '',
          localImportID: 'i',
          firstImportedAt: 1);
    addTearDown(access.locks.close);
    await openPage(tester, access);
    await tab(tester, '已存附件');
    await tester.tap(find.text('导入已有文件'));
    await settle(tester);
    expect(find.text('wrong-scope-name'), findsNothing);
    expect(find.textContaining('文件导入未完成。'), findsOneWidget);
  });

  testWidgets(
      'cleanup preview failure never offers or executes a destructive fallback',
      (tester) async {
    final access = FakeArchiveAccess()..failPreview = true;
    addTearDown(access.locks.close);
    await openPage(tester, access);
    await tab(tester, '本机清理');
    await tester.tap(find.text('仅清理本机'));
    await settle(tester);
    expect(access.calls.map((call) => call.first), ['previewScopes']);
    expect(find.text('确认清理'), findsNothing);
    expect(find.textContaining('synthetic private'), findsNothing);
    expect(find.textContaining('本机清理未完成'), findsOneWidget);
  });

  testWidgets('lock during cleanup preview discards counts and never clears',
      (tester) async {
    final access = FakeArchiveAccess()..previewPending = Completer();
    addTearDown(access.locks.close);
    await openPage(tester, access);
    await tab(tester, '本机清理');
    await tester.tap(find.text('仅清理本机'));
    await tester.pump();
    access.loseAccess();
    await tester.pump();
    access.previewPending!
        .complete(const ArchiveCleanupPreview(2, 1, 3, 1, 4321));
    await settle(tester);
    expect(find.text('确认清理'), findsNothing);
    expect(access.calls.map((call) => call.first), ['previewScopes']);
  });

  testWidgets('locked archive refuses all reads and mutations', (tester) async {
    final access = FakeArchiveAccess()..online = false;
    addTearDown(access.locks.close);
    await openPage(tester, access, restored: () async {});
    expect(find.text('请联网登录后打开本机档案。'), findsOneWidget);
    await tab(tester, '加密备份');
    expect(
        tester.widget<ButtonStyleButton>(button('导出加密备份')).onPressed, isNull);
    expect(access.calls, isEmpty);
  });

  testWidgets(
      'same-millisecond history sends both cursor fields and deduplicates',
      (tester) async {
    final access = FakeArchiveAccess()
      ..messageValues = [
        {'client_msg_id': 'z', 'send_time': 1000, 'content': 'first'},
        {'client_msg_id': 'y', 'send_time': 1000, 'content': 'second'},
      ];
    addTearDown(access.locks.close);
    await openPage(tester, access);
    await tester.tap(find.text('查看历史'));
    await settle(tester);
    access.messageValues = [
      {'client_msg_id': 'y', 'send_time': 1000, 'content': 'second'},
      {'client_msg_id': 'x', 'send_time': 1000, 'content': 'third'},
    ];
    await tester.ensureVisible(find.text('更早历史'));
    await tester.tap(find.text('更早历史'));
    await settle(tester);
    expect(
        access.calls.where((call) => call.first == 'messages').last.sublist(2),
        [1000, 'y']);
    expect(find.text('second'), findsOneWidget);
    expect(find.text('third'), findsOneWidget);
  });

  testWidgets(
      'lock during archive read discards late plaintext and attachment names',
      (tester) async {
    final access = FakeArchiveAccess()..messagePending = Completer();
    addTearDown(access.locks.close);
    await openPage(tester, access);
    await tester.tap(find.text('查看历史'));
    await tester.pump();
    access.loseAccess();
    await tester.pump();
    access.messagePending!.complete([
      {
        'client_msg_id': 'private',
        'send_time': 1000,
        'content': 'late-private-body'
      }
    ]);
    await settle(tester);
    expect(find.text('late-private-body'), findsNothing);
    expect(find.byKey(const ValueKey('archive-error')), findsOneWidget);
    expect(find.text('合成联系人 A'), findsNothing);
  });

  testWidgets(
      'attachments show original names and no external open or share action',
      (tester) async {
    final access = FakeArchiveAccess();
    addTearDown(access.locks.close);
    await openPage(tester, access);
    await tab(tester, '已存附件');
    await tester.tap(find.text('读取本机附件'));
    await settle(tester);
    expect(find.text('原文件名.txt'), findsOneWidget);
    expect(access.calls.last, ['attachments', 'chat-a', '']);
    expect(find.byIcon(Icons.share), findsNothing);
    expect(find.text('外部打开'), findsNothing);
    access.loseAccess();
    await settle(tester);
    expect(find.text('原文件名.txt'), findsNothing);
  });

  testWidgets(
      'export confirms independent phrase and only passes encrypted native handle to destination',
      (tester) async {
    final access = FakeArchiveAccess();
    addTearDown(access.locks.close);
    await openPage(tester, access);
    await tab(tester, '加密备份');
    await tester.tap(find.text('导出加密备份'));
    await settle(tester);
    await tester.enterText(
        find.byKey(const ValueKey('backup-passphrase')), 'short');
    await tester.tap(find.text('生成加密备份'));
    await settle(tester);
    expect(find.text('备份口令至少 12 个字符'), findsOneWidget);
    expect(access.calls, isEmpty);
    await inputPhrase(tester, creating: true);
    expect(access.calls.map((call) => call.first), ['export', 'save']);
    expect(access.calls.last, ['save', 'private-encrypted-zip']);
    expect(find.text('private-encrypted-zip'), findsNothing);
    expect(find.text('synthetic-backup-passphrase'), findsNothing);
  });

  testWidgets('lock closes phrase dialog before a backup can start',
      (tester) async {
    final access = FakeArchiveAccess();
    addTearDown(access.locks.close);
    await openPage(tester, access);
    await tab(tester, '加密备份');
    await tester.tap(find.text('导出加密备份'));
    await settle(tester);
    await tester.enterText(find.byKey(const ValueKey('backup-passphrase')),
        'synthetic-backup-passphrase');
    access.loseAccess();
    await settle(tester);
    expect(find.byKey(const ValueKey('backup-passphrase')), findsNothing);
    expect(access.calls, isEmpty);
  });

  testWidgets(
      'validated restore cancellation cancels opaque preparation and never commits',
      (tester) async {
    final access = FakeArchiveAccess();
    addTearDown(access.locks.close);
    await openPage(tester, access, restored: () async {});
    await tab(tester, '加密备份');
    await tester.tap(find.text('选择加密备份并恢复'));
    await settle(tester);
    await inputPhrase(tester);
    expect(find.text('恢复此账户的本机档案'), findsOneWidget);
    await tester.tap(find.text('取消'));
    await settle(tester);
    expect(
        access.calls.map((call) => call.first), ['pick', 'prepare', 'cancel']);
    expect(access.calls.last, ['cancel', 'opaque-restore-id']);
  });

  testWidgets(
      'successful restore locks and requires fresh login callback without cancellation',
      (tester) async {
    final access = FakeArchiveAccess();
    addTearDown(access.locks.close);
    var freshLogin = 0;
    await openPage(tester, access, restored: () async {
      freshLogin++;
    });
    await tab(tester, '加密备份');
    await tester.tap(find.text('选择加密备份并恢复'));
    await settle(tester);
    await inputPhrase(tester);
    await tester.tap(find.text('恢复并重新登录'));
    await settle(tester);
    expect(freshLogin, 1);
    expect(access.online, isFalse);
    expect(
        access.calls.map((call) => call.first), ['pick', 'prepare', 'commit']);
    expect(find.text('opaque-restore-id'), findsNothing);
  });

  testWidgets('failed restore cancels preparation and does not route away',
      (tester) async {
    final access = FakeArchiveAccess()..failCommit = true;
    addTearDown(access.locks.close);
    var freshLogin = 0;
    await openPage(tester, access, restored: () async {
      freshLogin++;
    });
    await tab(tester, '加密备份');
    await tester.tap(find.text('选择加密备份并恢复'));
    await settle(tester);
    await inputPhrase(tester);
    await tester.tap(find.text('恢复并重新登录'));
    await settle(tester);
    expect(freshLogin, 0);
    expect(access.calls.map((call) => call.first),
        ['pick', 'prepare', 'commit', 'cancel']);
    expect(find.textContaining('恢复未完成。'), findsOneWidget);
  });

  testWidgets('chat entry cannot restore without owning logout navigation',
      (tester) async {
    final access = FakeArchiveAccess();
    addTearDown(access.locks.close);
    await openPage(tester, access);
    await tab(tester, '加密备份');
    expect(tester.widget<ButtonStyleButton>(button('选择加密备份并恢复')).onPressed,
        isNull);
    expect(find.textContaining('请从“我的'), findsOneWidget);
  });

  testWidgets('backup errors never expose exception text, paths or credentials',
      (tester) async {
    final access = FakeArchiveAccess()..failExport = true;
    addTearDown(access.locks.close);
    await openPage(tester, access);
    await tab(tester, '加密备份');
    await tester.tap(find.text('导出加密备份'));
    await settle(tester);
    await inputPhrase(tester, creating: true);
    expect(find.textContaining('加密备份未完成。'), findsOneWidget);
    expect(find.textContaining('synthetic secret'), findsNothing);
    expect(access.calls.where((call) => call.first == 'save'), isEmpty);
  });

  testWidgets(
      'successful scoped cleanup clears visible data through invalidation without logging out',
      (tester) async {
    final access = FakeArchiveAccess()
      ..messageValues = [
        {'client_msg_id': 'm', 'send_time': 1000, 'content': 'visible-body'}
      ];
    addTearDown(access.locks.close);
    addTearDown(access.changes.close);
    await openPage(tester, access);
    await tester.tap(find.text('查看历史'));
    await settle(tester);
    expect(find.text('visible-body'), findsOneWidget);
    await tab(tester, '本机清理');
    await tester.tap(find.text('仅清理本机'));
    await settle(tester);
    expect(find.textContaining('消息 2 条，本机导入 1 项，附件引用 3 项'), findsOneWidget);
    expect(access.calls.last.first, 'previewScopes');
    expect(find.textContaining('4321 字节'), findsOneWidget);
    await tester.tap(find.text('确认清理'));
    await settle(tester);
    final clear =
        access.calls.singleWhere((call) => call.first == 'clearScopes');
    final preview =
        access.calls.singleWhere((call) => call.first == 'previewScopes');
    expect(clear.sublist(1), preview.sublist(1));
    expect(clear[1], ['chat-a']);
    final from =
        DateTime.fromMillisecondsSinceEpoch(clear[2] as int, isUtc: true);
    final to =
        DateTime.fromMillisecondsSinceEpoch(clear[3] as int, isUtc: true);
    expect(from.hour, 16);
    expect(from.minute, 0);
    expect(to.hour, 16);
    expect(to.minute, 0);
    expect(access.online, isTrue);
    expect(find.text('visible-body'), findsNothing);
    expect(find.byType(LinearProgressIndicator), findsNothing);
  });

  testWidgets(
      'multi conversation cleanup uses exactly one union preview and preserves history selection',
      (tester) async {
    final access = FakeArchiveAccess();
    addTearDown(access.locks.close);
    addTearDown(access.changes.close);
    await openPage(tester, access);
    await tab(tester, '本机清理');
    await tester.tap(find.text('选择清理联系人 / 群组（已选 1）'));
    await settle(tester);
    await tester.tap(find.byKey(const ValueKey('cleanup-scope-chat-b')));
    await tester.tap(find.text('确定范围'));
    await settle(tester);
    expect(find.text('选择清理联系人 / 群组（已选 2）'), findsOneWidget);
    await tester.tap(find.text('仅清理本机'));
    await settle(tester);
    expect(access.calls.length, 1);
    expect(access.calls.single[0], 'previewScopes');
    expect(access.calls.single[1], ['chat-a', 'chat-b']);
    expect(find.textContaining('已选 2 个联系人 / 群组'), findsOneWidget);
    expect(find.textContaining('4321 字节'), findsOneWidget);
    await tester.tap(find.text('确认清理'));
    await settle(tester);
    expect(access.calls.map((call) => call.first),
        ['previewScopes', 'clearScopes']);
    expect(access.calls[0].sublist(1), access.calls[1].sublist(1));
    await tab(tester, '消息历史');
    await tester.tap(find.text('查看历史'));
    await settle(tester);
    expect(access.calls.last[0], 'messages');
    expect(access.calls.last[1], 'chat-a');
  });

  testWidgets(
      'before mode excludes Beijing midnight and does not use a last millisecond fudge',
      (tester) async {
    final access = FakeArchiveAccess();
    addTearDown(access.locks.close);
    await openPage(tester, access);
    await tab(tester, '本机清理');
    await tester.tap(find.byKey(const ValueKey('cleanup-date-mode')));
    await settle(tester);
    await tester.tap(find.text('某天之前').last);
    await settle(tester);
    await tester.tap(find.text('仅清理本机'));
    await settle(tester);
    final preview = access.calls.single;
    expect(preview[0], 'previewScopes');
    expect(preview[2], 0);
    final expected = archiveBeforeBounds(archiveCalendarDate(DateTime.now()));
    expect(preview[3], expected[1]);
    expect(find.textContaining('00:00 之前（不含该时刻）'), findsWidgets);
    await tester.tap(find.text('取消'));
    await settle(tester);
    expect(access.calls.length, 1);
  });

  testWidgets(
      'clearing selections disables cleanup and never implies all conversations',
      (tester) async {
    final access = FakeArchiveAccess();
    addTearDown(access.locks.close);
    await openPage(tester, access);
    await tab(tester, '本机清理');
    await tester.tap(find.text('选择清理联系人 / 群组（已选 1）'));
    await settle(tester);
    await tester.tap(find.byKey(const ValueKey('cleanup-scope-chat-a')));
    await tester.tap(find.text('确定范围'));
    await settle(tester);
    expect(tester.widget<ButtonStyleButton>(button('仅清理本机')).onPressed, isNull);
    expect(access.calls, isEmpty);
  });

  testWidgets(
      'invalidation dismisses a visible union preview and requires a fresh calculation',
      (tester) async {
    final access = FakeArchiveAccess();
    addTearDown(access.locks.close);
    addTearDown(access.changes.close);
    await openPage(tester, access);
    await tab(tester, '本机清理');
    await tester.tap(find.text('仅清理本机'));
    await settle(tester);
    expect(find.text('确认清理'), findsOneWidget);
    access.changes.add(null);
    await settle(tester);
    expect(find.text('确认清理'), findsNothing);
    expect(access.calls.map((call) => call.first), ['previewScopes']);
    await tester.tap(find.text('仅清理本机'));
    await settle(tester);
    expect(access.calls.map((call) => call.first),
        ['previewScopes', 'previewScopes']);
    await tester.tap(find.text('取消'));
    await settle(tester);
  });

  testWidgets('locking also dismisses the scope chooser and its contact names',
      (tester) async {
    final access = FakeArchiveAccess();
    addTearDown(access.locks.close);
    await openPage(tester, access);
    await tab(tester, '本机清理');
    await tester.tap(find.text('选择清理联系人 / 群组（已选 1）'));
    await settle(tester);
    expect(find.text('选择清理联系人 / 群组'), findsOneWidget);
    expect(find.text('合成联系人 B'), findsOneWidget);
    access.loseAccess();
    await settle(tester);
    expect(find.text('选择清理联系人 / 群组'), findsNothing);
    expect(find.text('合成联系人 B'), findsNothing);
    expect(find.text('合成联系人 A'), findsNothing);
    expect(access.calls, isEmpty);
  });
}
