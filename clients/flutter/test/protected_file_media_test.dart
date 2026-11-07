import 'dart:async';
import 'dart:typed_data';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:openim_common/src/models/login_certificate.dart';
import 'package:openim_common/src/native_local_vault.dart';
import 'package:openim_common/src/widgets/protected_file_page.dart';
import 'package:openim_common/src/widgets/chat/chat_video_player_view.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  final messenger =
      TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
  final certificate = LoginCertificate.fromJson({
    'userID': 'synthetic-media',
    'imToken': 'synthetic',
    'chatToken': 'synthetic'
  });
  Future<void> handler(Future<dynamic> Function(MethodCall) callback) async {
    messenger.setMockMethodCallHandler(NativeLocalVault.channel, (call) async {
      if (call.method == 'unlock')
        return {'userID': 'synthetic-media', 'actorGeneration': 1};
      return callback(call);
    });
    await NativeLocalVault.unlock(certificate);
  }

  tearDown(() async {
    messenger.setMockMethodCallHandler(
        NativeLocalVault.channel, (_) async => true);
    await NativeLocalVault.lock();
    messenger.setMockMethodCallHandler(NativeLocalVault.channel, null);
  });
  test('saved media URI binds the exact task without requesting a full body',
      () async {
    final calls = <String>[];
    await handler((call) async {
      calls.add(call.method);
      expect(call.arguments, {'taskID': 'synthetic-task'});
      return 'puty-local://attachment/synthetic-task';
    });
    expect(await NativeLocalVault.attachmentPreviewUri('synthetic-task'),
        Uri.parse('puty-local://attachment/synthetic-task'));
    expect(calls, ['attachment/previewUri']);
  });
  for (final uri in [
    'https://example.invalid/video.mp4',
    'file:///tmp/video.mp4',
    'puty-local://source/synthetic-task',
    'puty-local://attachment/other-task',
    'puty-local://attachment/synthetic-task?proof=x',
    'puty-local://attachment/synthetic-task#x'
  ]) {
    test('rejects a different or externally openable identity: $uri', () async {
      await handler((_) async => uri);
      await expectLater(NativeLocalVault.attachmentPreviewUri('synthetic-task'),
          throwsStateError);
    });
  }
  test('cleanup rejects an already requested late saved-media URI', () async {
    final reply = Completer<String>();
    await handler((call) async =>
        call.method == 'attachment/previewUri' ? reply.future : true);
    final pending = NativeLocalVault.attachmentPreviewUri('synthetic-task');
    final rejected = expectLater(pending, throwsStateError);
    await NativeLocalVault.clearScopes(['synthetic-conversation'], 0, 100);
    reply.complete('puty-local://attachment/synthetic-task');
    await rejected;
  });
  test('native preview completion cannot survive a lock', () async {
    final reply = Completer<bool>();
    await handler(
        (call) async => call.method == 'media/preview' ? reply.future : true);
    final pending = NativeLocalVault.nativePreview(taskID: 'synthetic-task');
    final rejected = expectLater(pending, throwsStateError);
    await NativeLocalVault.lock();
    reply.complete(true);
    await rejected;
  });
  for (final source in [false, true]) {
    test(
        'late ${source ? 'source' : 'attachment'} plaintext is never returned after cleanup',
        () async {
      final reply = Completer<Uint8List>();
      await handler((call) async =>
          call.method == (source ? 'source/read' : 'attachment/read')
              ? reply.future
              : true);
      final pending = source
          ? NativeLocalVault.readSource('synthetic-source')
          : NativeLocalVault.read('synthetic-task');
      final rejected = expectLater(pending, throwsStateError);
      await NativeLocalVault.clearScopes(['synthetic-conversation'], 0, 100);
      final bytes = Uint8List.fromList([17, 34, 51]);
      reply.complete(bytes);
      await rejected;
    });
  }
  testWidgets(
      'saved MP4 streams through range player; cleanup removes it before a late URI arrives',
      (tester) async {
    final reply = Completer<String>(), started = Completer<void>();
    final calls = <String>[];
    await handler((call) async {
      calls.add(call.method);
      if (call.method == 'attachment/previewUri') {
        started.complete();
        return reply.future;
      }
      return true;
    });
    await tester.pumpWidget(const MaterialApp(
        home: ProtectedFilePage(
            name: 'synthetic.mp4', taskID: 'synthetic-task')));
    await tester.pump();
    await started.future;
    expect(find.byType(ChatVideoPlayerView), findsOneWidget);
    expect(calls, contains('attachment/previewUri'));
    expect(calls, isNot(contains('attachment/read')));
    await NativeLocalVault.clearScopes(['synthetic-conversation'], 0, 100);
    await tester.pump();
    reply.complete('puty-local://attachment/synthetic-task');
    await tester.pump();
    expect(find.byType(ChatVideoPlayerView), findsNothing);
    expect(find.text('当前在线授权已失效，预览已锁定'), findsOneWidget);
    await tester.pumpWidget(const SizedBox());
  });
}
