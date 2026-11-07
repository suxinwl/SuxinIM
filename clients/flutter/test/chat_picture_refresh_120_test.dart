import 'dart:async';
import 'dart:io';
import 'dart:typed_data';
import 'dart:ui' as ui;

import 'package:app_im/app_im.dart';
import 'package:extended_image/extended_image.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_screenutil/flutter_screenutil.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:openim_common/openim_common.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  final messenger =
      TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
  final calls = <MethodCall>[];
  final available = <String>{};
  final delayedInfo = <String, Completer<void>>{};
  late Uint8List png;
  var permissionCalls = 0;

  Message picture({String? path, String? url, int status = 1}) => Message(
        clientMsgID: 'synthetic-picture',
        status: status,
        contentType: MessageType.picture,
        exMap: <String, dynamic>{},
        pictureElem: PictureElem(
          sourcePath: path,
          sourcePicture: PictureInfo(width: 40, height: 30),
          bigPicture: PictureInfo(url: url),
        ),
      );

  Widget view(Message message, {Stream<MsgStreamEv<int>>? progress}) =>
      ScreenUtilInit(
        designSize: const Size(390, 844),
        builder: (_, __) => MaterialApp(
          home: Scaffold(
            body: ChatPictureView(
              key: const ValueKey('same-picture-cell'),
              message: message,
              isISend: true,
              sendProgressStream: progress,
            ),
          ),
        ),
      );

  Future<void> drain(WidgetTester tester) async {
    for (var i = 0; i < 6; i++) {
      await tester.pump();
      await tester.runAsync(
          () => Future<void>.delayed(const Duration(milliseconds: 5)));
    }
    await tester.pump();
  }

  bool frameVisible(WidgetTester tester) => tester
      .widgetList<ExtendedRawImage>(find.byType(ExtendedRawImage))
      .any((image) => image.image != null && image.image!.width == 1);

  List<String> paths(String method) => calls
      .where((call) => call.method == method)
      .map((call) => (call.arguments as Map)['path'] as String)
      .toList();

  setUp(() async {
    calls.clear();
    available.clear();
    delayedInfo.clear();
    permissionCalls = 0;
    final recorder = ui.PictureRecorder();
    ui.Canvas(recorder).drawColor(const ui.Color(0xff0088ff), ui.BlendMode.src);
    final drawing = recorder.endRecording();
    final image = await drawing.toImage(1, 1);
    final bytes = await image.toByteData(format: ui.ImageByteFormat.png);
    png = Uint8List.fromList(
        bytes!.buffer.asUint8List(bytes.offsetInBytes, bytes.lengthInBytes));
    image.dispose();
    drawing.dispose();
    messenger.setMockMethodCallHandler(NativeLocalVault.channel, (call) async {
      calls.add(call);
      if (call.method == 'unlock')
        return {'userID': 'synthetic-picture-owner', 'actorGeneration': 1};
      if (call.method == 'lock') return true;
      final path = (call.arguments as Map)['path'] as String;
      if (call.method == 'source/info') {
        if (delayedInfo[path] != null) await delayedInfo[path]!.future;
        if (!available.contains(path))
          throw PlatformException(code: 'SYNTHETIC_SOURCE_UNAVAILABLE');
        return {'name': 'capture.png', 'size': png.length};
      }
      if (call.method == 'source/read') {
        if (!available.contains(path))
          throw PlatformException(code: 'SYNTHETIC_SOURCE_UNAVAILABLE');
        return Uint8List.fromList(png);
      }
      throw StateError('Unexpected native action ${call.method}');
    });
    // Android 13+ storage permission can stay denied for app-private media.
    messenger.setMockMethodCallHandler(
        const MethodChannel('flutter.baseflow.com/permissions/methods'),
        (call) async {
      permissionCalls++;
      return 0;
    });
    AppIm.enterprise.configure(
        baseUrl: () => 'https://im.szputy.com:9885/enterprise/v1',
        imToken: () => 'synthetic-im',
        chatToken: () => 'synthetic-chat',
        userID: () => 'synthetic-picture-owner',
        platform: () => 2);
    await NativeLocalVault.unlock(LoginCertificate.fromJson({
      'userID': 'synthetic-picture-owner',
      'imToken': 'synthetic-im',
      'chatToken': 'synthetic-chat'
    }));
  });

  tearDown(() async {
    for (final gate in delayedInfo.values) {
      if (!gate.isCompleted) gate.complete();
    }
    await NativeLocalVault.lock();
    messenger.setMockMethodCallHandler(NativeLocalVault.channel, null);
    messenger.setMockMethodCallHandler(
        const MethodChannel('flutter.baseflow.com/permissions/methods'), null);
    AppIm.enterprise.dispose();
  });

  testWidgets(
      'protected capture renders without a filesystem file or storage permission',
      (tester) async {
    const path = '/synthetic/sqlcipher/capture.png';
    expect(File(path).existsSync(), isFalse);
    available.add(path);
    await tester.pumpWidget(view(picture(path: path)));
    await drain(tester);
    expect(paths('source/info'), [path]);
    expect(paths('source/read'), [path]);
    expect(frameVisible(tester), isTrue);
    expect(permissionCalls, 0);
    expect(
        calls.any((c) =>
            c.method.startsWith('attachment/') || c.method == 'source/import'),
        isFalse);
  });

  testWidgets(
      'same mutable message refreshes its new source without recreating the cell',
      (tester) async {
    available.addAll(['/synthetic/first.png', '/synthetic/second.png']);
    final message = picture(path: '/synthetic/first.png');
    await tester.pumpWidget(view(message));
    await drain(tester);
    final state = tester.state(find.byType(ChatPictureView));
    message.pictureElem!.sourcePath = '/synthetic/second.png';
    message.status = 2;
    await tester.pumpWidget(view(message));
    await drain(tester);
    expect(
        identical(tester.state(find.byType(ChatPictureView)), state), isTrue);
    expect(paths('source/read'),
        ['/synthetic/first.png', '/synthetic/second.png']);
    expect(frameVisible(tester), isTrue);
  });

  testWidgets(
      'late uploaded URL appears in the mounted cell after in-place update',
      (tester) async {
    final message = picture();
    await tester.pumpWidget(view(message));
    await drain(tester);
    expect(find.byType(FutureBuilder<String>), findsNothing);
    final state = tester.state(find.byType(ChatPictureView));
    // The native reader substitutes for remote transport in this render test;
    // resource proof and HTTP authorization have independent transport tests.
    available.add('/synthetic/uploaded.png');
    message.pictureElem!.bigPicture!.url = '/synthetic/uploaded.png';
    message.status = 2;
    await tester.pumpWidget(view(message));
    await drain(tester);
    expect(
        identical(tester.state(find.byType(ChatPictureView)), state), isTrue);
    expect(find.byType(FutureBuilder<String>), findsOneWidget);
    expect(paths('source/read'), ['/synthetic/uploaded.png']);
    expect(frameVisible(tester), isTrue);
  });

  testWidgets(
      'status completion retries the same previously unavailable protected source',
      (tester) async {
    const path = '/synthetic/pending.png';
    final message = picture(path: path);
    await tester.pumpWidget(view(message));
    await drain(tester);
    expect(frameVisible(tester), isFalse);
    available.add(path);
    message.status = 2;
    await tester.pumpWidget(view(message));
    await drain(tester);
    expect(paths('source/info'), [path, path]);
    expect(frameVisible(tester), isTrue);
  });

  testWidgets(
      'upload completion retries same fields once and unrelated rebuild does not reread',
      (tester) async {
    const path = '/synthetic/pending-progress.png';
    final message = picture(path: path);
    final progress = StreamController<MsgStreamEv<int>>.broadcast();
    addTearDown(progress.close);
    await tester.pumpWidget(view(message, progress: progress.stream));
    await drain(tester);
    available.add(path);
    progress.add(MsgStreamEv(id: message.clientMsgID!, value: 100));
    await drain(tester);
    expect(frameVisible(tester), isTrue);
    final infoCount = paths('source/info').length;
    final readCount = paths('source/read').length;
    progress.add(MsgStreamEv(id: message.clientMsgID!, value: 100));
    await tester.pumpWidget(view(message, progress: progress.stream));
    await drain(tester);
    expect(paths('source/info').length, infoCount);
    expect(paths('source/read').length, readCount);
    // A genuine new upload attempt on the same task can complete again.
    progress.add(MsgStreamEv(id: message.clientMsgID!, value: 10));
    progress.add(MsgStreamEv(id: message.clientMsgID!, value: 100));
    await drain(tester);
    expect(paths('source/info').length, infoCount + 1);
    expect(paths('source/read').length, readCount);
  });

  testWidgets('superseded metadata response cannot paint the old source',
      (tester) async {
    const first = '/synthetic/late.png';
    const second = '/synthetic/current.png';
    available.addAll([first, second]);
    final gate = delayedInfo[first] = Completer<void>();
    final message = picture(path: first);
    await tester.pumpWidget(view(message));
    await drain(tester);
    message.pictureElem!.sourcePath = second;
    await tester.pumpWidget(view(message));
    await drain(tester);
    gate.complete();
    await drain(tester);
    expect(paths('source/read'), [second]);
    expect(frameVisible(tester), isTrue);
    expect(tester.takeException(), isNull);
  });

  testWidgets(
      'lock removes a rendered frame and prevents pending metadata from repainting',
      (tester) async {
    const path = '/synthetic/locked.png';
    available.add(path);
    final message = picture(path: path);
    await tester.pumpWidget(view(message));
    await drain(tester);
    expect(frameVisible(tester), isTrue);
    final gate = delayedInfo[path] = Completer<void>();
    message.status = 2;
    await tester.pumpWidget(view(message));
    await drain(tester);
    await NativeLocalVault.lock();
    await drain(tester);
    gate.complete();
    await drain(tester);
    expect(frameVisible(tester), isFalse);
    expect(paths('source/read'), [path]);
    expect(tester.takeException(), isNull);
  });

  testWidgets('disposed cell ignores a pending metadata response',
      (tester) async {
    const path = '/synthetic/disposed.png';
    available.add(path);
    final gate = delayedInfo[path] = Completer<void>();
    await tester.pumpWidget(view(picture(path: path)));
    await drain(tester);
    await tester.pumpWidget(const SizedBox());
    gate.complete();
    await drain(tester);
    expect(paths('source/read'), isEmpty);
    expect(tester.takeException(), isNull);
  });

  testWidgets(
      'zero dimensions stay finite and arriving dimensions recompute in place',
      (tester) async {
    const path = '/synthetic/zero-dimensions.png';
    available.add(path);
    final message = picture(path: path);
    message.pictureElem!.sourcePicture!
      ..width = 0
      ..height = 0;
    await tester.pumpWidget(view(message));
    await drain(tester);
    final before = tester.getSize(find.byType(ChatPictureView));
    expect(before.width.isFinite && before.width > 0, isTrue);
    expect(before.height.isFinite && before.height > 0, isTrue);
    expect(frameVisible(tester), isTrue);
    message.pictureElem!.sourcePicture!
      ..width = 40
      ..height = 20;
    await tester.pumpWidget(view(message));
    await drain(tester);
    expect(tester.getSize(find.byType(ChatPictureView)), const Size(40, 20));
    expect(tester.takeException(), isNull);
  });

  testWidgets(
      'resource session change rejects metadata from the previous viewer',
      (tester) async {
    const path = '/synthetic/old-resource-session.png';
    available.add(path);
    final gate = delayedInfo[path] = Completer<void>();
    await tester.pumpWidget(view(picture(path: path)));
    await drain(tester);
    AppIm.enterprise.configure(
        baseUrl: () => 'https://im.szputy.com:9885/enterprise/v1',
        imToken: () => 'synthetic-new-im',
        chatToken: () => 'synthetic-new-chat',
        userID: () => 'synthetic-new-viewer',
        platform: () => 2);
    gate.complete();
    await drain(tester);
    expect(paths('source/read'), isEmpty);
    expect(frameVisible(tester), isFalse);
    expect(tester.takeException(), isNull);
  });
}
