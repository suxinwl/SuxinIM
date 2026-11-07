// Test-only immutable fixtures run on pinned Flutter 3.32.8 / Dart 3.8;
// the application's existing minimum SDK constraint is intentionally unchanged.
// ignore_for_file: sdk_version_since

import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'dart:ui' as ui;

import 'package:flutter/painting.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:app_im/app_im.dart';
import 'package:dio/dio.dart';
import 'package:flutter_image_compress/flutter_image_compress.dart';
// Exercise the actual transitive native plugin MethodChannel, not a fake codec.
// ignore: depend_on_referenced_packages
import 'package:flutter_image_compress_common/flutter_image_compress_common.dart';
import 'package:openim_common/src/models/login_certificate.dart';
import 'package:openim_common/src/native_local_vault.dart';
import 'package:openim_common/src/utils/protected_resource_image.dart';
import 'package:openim_common/src/utils/utils.dart';

class _ReadonlyHTTP implements HttpClientAdapter {
  _ReadonlyHTTP(this.body);
  final Stream<Uint8List> Function() body;
  @override
  Future<ResponseBody> fetch(RequestOptions options, Stream<Uint8List>? request,
      Future<void>? cancel) async {
    if (options.method == 'POST') {
      return ResponseBody.fromString(
          jsonEncode({
            'errCode': 0,
            'data': {
              'url': 'https://im.szputy.com:9885/attachments/synthetic-object',
              'resourceProof': 'a' * 64,
              'resourceProofExpiresAt':
                  DateTime.now().millisecondsSinceEpoch ~/ 1000 + 900,
              'expiresAt': DateTime.now().millisecondsSinceEpoch ~/ 1000 + 900
            }
          }),
          200,
          headers: {
            Headers.contentTypeHeader: [Headers.jsonContentType]
          });
    }
    return ResponseBody(body(), 200);
  }

  @override
  void close({bool force = false}) {}
}

// Raw MethodChannel envelope is deliberately read-only. setMockMethodCallHandler
// would encode a new mutable envelope and lose this native boundary condition.
void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  final messenger =
      TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
  const codec = StandardMethodCodec();
  Uint8List? png;
  bool rejectRead = false;
  Completer<void>? readGate, readStarted;
  ByteData? latestReadReply;
  Uint8List? stagedBytes;

  ByteData readonlyReply(Object value) {
    final encoded = codec.encodeSuccessEnvelope(value);
    final readOnly = encoded.buffer
        .asUint8List(encoded.offsetInBytes, encoded.lengthInBytes)
        .asUnmodifiableView();
    return ByteData.view(
        readOnly.buffer, readOnly.offsetInBytes, readOnly.lengthInBytes);
  }

  Future<ByteData?> handler(ByteData? message) async {
    final call = codec.decodeMethodCall(message!);
    if (call.method == 'unlock') {
      return codec.encodeSuccessEnvelope(
          {'userID': 'synthetic-readonly-image', 'actorGeneration': 1});
    }
    if (call.method == 'source/read' || call.method == 'attachment/read') {
      if (readStarted?.isCompleted == false) readStarted!.complete();
      if (readGate != null) await readGate!.future;
      if (rejectRead) {
        return codec.encodeErrorEnvelope(code: 'SYNTHETIC_AUTH_REJECTED');
      }
      return latestReadReply = readonlyReply(png!);
    }
    if (call.method == 'source/import') {
      return codec.encodeSuccessEnvelope('/synthetic/protected.png');
    }
    if (call.method == 'source/info') {
      return codec
          .encodeSuccessEnvelope({'name': 'original.png', 'size': png!.length});
    }
    if (call.method == 'source/fromBytes') {
      stagedBytes =
          Uint8List.fromList((call.arguments as Map)['bytes'] as Uint8List);
      return codec.encodeSuccessEnvelope('/synthetic/compressed.png');
    }
    return codec.encodeSuccessEnvelope(true);
  }

  setUp(() async {
    rejectRead = false;
    readGate = null;
    readStarted = null;
    latestReadReply = null;
    stagedBytes = null;
    // Actual engine generates a valid PNG, then actual engine decodes it below.
    final recorder = ui.PictureRecorder();
    ui.Canvas(recorder).drawColor(const ui.Color(0xffff0000), ui.BlendMode.src);
    final picture = recorder.endRecording();
    final image = await picture.toImage(1, 1);
    final data = await image.toByteData(format: ui.ImageByteFormat.png);
    png = Uint8List.fromList(
        data!.buffer.asUint8List(data.offsetInBytes, data.lengthInBytes));
    image.dispose();
    picture.dispose();
    messenger.setMockMessageHandler(NativeLocalVault.channel.name, handler);
    await NativeLocalVault.unlock(LoginCertificate.fromJson({
      'userID': 'synthetic-readonly-image',
      'imToken': 'synthetic',
      'chatToken': 'synthetic'
    }));
  });

  tearDown(() async {
    if (readGate?.isCompleted == false) readGate!.complete();
    await NativeLocalVault.lock();
    messenger.setMockMessageHandler(NativeLocalVault.channel.name, null);
    png?.fillRange(0, png!.length, 0);
    png = null;
  });

  test('real codec accepts readonly native image bytes, which cannot be wiped',
      () async {
    final borrowed = (await NativeLocalVault.channel.invokeMethod<Uint8List>(
        'source/read', {'path': '/synthetic/encrypted-source'}))!;
    expect(() => borrowed.fillRange(0, borrowed.length, 0),
        throwsUnsupportedError);
    final buffer = await ui.ImmutableBuffer.fromUint8List(borrowed);
    final decoded =
        await PaintingBinding.instance.instantiateImageCodecWithSize(buffer);
    try {
      final frame = await decoded.getNextFrame();
      expect(frame.image.width, 1);
      expect(frame.image.height, 1);
      frame.image.dispose();
    } finally {
      decoded.dispose();
    }
    expect(borrowed.take(4), png!.take(4));
  });

  for (final source in [false, true]) {
    test(
        '${source ? 'source' : 'attachment'} read owns mutable bytes without changing native view',
        () async {
      final owned = source
          ? await NativeLocalVault.readSource('/synthetic/encrypted-source')
          : await NativeLocalVault.read('synthetic-task');
      final borrowed = codec.decodeEnvelope(latestReadReply!) as Uint8List;
      expect(() => borrowed.fillRange(0, borrowed.length, 0),
          throwsUnsupportedError);
      expect(owned, png);
      owned.fillRange(0, owned.length, 0);
      expect(owned.every((byte) => byte == 0), isTrue);
      expect(borrowed, png);
    });
    test(
        '${source ? 'source' : 'attachment'} late readonly reply keeps generation rejection',
        () async {
      readGate = Completer<void>();
      readStarted = Completer<void>();
      final pending = source
          ? NativeLocalVault.readSource('/synthetic/encrypted-source')
          : NativeLocalVault.read('synthetic-task');
      final rejected = expectLater(pending, throwsStateError);
      await readStarted!.future;
      await NativeLocalVault.clearScopes(['synthetic-conversation'], 0, 100);
      readGate!.complete();
      await rejected;
      expect(codec.decodeEnvelope(latestReadReply!), png);
    });
  }

  test('compression plugin readonly result is cloned before staging and wiping',
      () async {
    final previous = FlutterImageCompressPlatform.instance;
    FlutterImageCompressCommon.registerWith();
    ByteData? borrowed;
    messenger.setMockMessageHandler('flutter_image_compress', (message) async {
      final call = codec.decodeMethodCall(message!);
      expect(call.method, 'compressWithList');
      return borrowed = readonlyReply(png!);
    });
    try {
      final output =
          await IMUtils.compressImageAndGetFile(File('/synthetic/input.png'));
      expect(output?.path, '/synthetic/compressed.png');
      expect(stagedBytes, png);
      expect(codec.decodeEnvelope(borrowed!), png);
    } finally {
      messenger.setMockMessageHandler('flutter_image_compress', null);
      FlutterImageCompressPlatform.instance = previous;
      stagedBytes?.fillRange(0, stagedBytes!.length, 0);
    }
  });

  for (final fail in [false, true]) {
    test(
        'HTTP readonly stream ${fail ? 'keeps original failure' : 'returns owned mutable bytes'}',
        () async {
      final borrowed = Uint8List.fromList(png!).asUnmodifiableView();
      final adapter = _ReadonlyHTTP(() async* {
        yield borrowed;
        if (fail) throw StateError('Synthetic transfer failed');
      });
      final manager = EnterpriseManager(dio: Dio()..httpClientAdapter = adapter)
        ..configure(
            baseUrl: () => 'https://im.szputy.com:9885/enterprise/v1',
            imToken: () => 'synthetic-im',
            chatToken: () => 'synthetic-chat',
            userID: () => 'synthetic-user',
            platform: () => 2);
      try {
        final pending = manager.readResourceBytes(
            'https://im.szputy.com:9885/api/object/synthetic');
        if (fail) {
          await expectLater(
              pending,
              throwsA(isA<StateError>().having((e) => e.message,
                  'original failure', 'Synthetic transfer failed')));
        } else {
          final owned = await pending;
          expect(owned, png);
          owned.fillRange(0, owned.length, 0);
        }
        expect(borrowed, png);
      } finally {
        manager.dispose();
      }
    });
  }

  test('provider publishes a real first frame from immutable native bytes',
      () async {
    final provider = ProtectedResourceImage('/synthetic/encrypted-source');
    final stream = provider.loadImage(
        provider, PaintingBinding.instance.instantiateImageCodecWithSize);
    final result = Completer<ImageInfo>();
    final listener = ImageStreamListener((image, synchronous) {
      if (!result.isCompleted) result.complete(image.clone());
    }, onError: (Object error, StackTrace? stack) {
      if (!result.isCompleted) result.completeError(error, stack);
    });
    stream.addListener(listener);
    try {
      final frame = await result.future;
      expect(frame.image.width, 1);
      expect(frame.image.height, 1);
      frame.dispose();
    } finally {
      stream.removeListener(listener);
    }
  });

  test('native read authorization failure remains a failure', () async {
    rejectRead = true;
    final provider = ProtectedResourceImage('/synthetic/encrypted-source');
    var decodeCalled = false;
    final stream = provider.loadImage(provider, (buffer,
        {ui.TargetImageSize Function(int, int)? getTargetSize}) {
      decodeCalled = true;
      return PaintingBinding.instance
          .instantiateImageCodecWithSize(buffer, getTargetSize: getTargetSize);
    });
    final failure = Completer<Object>();
    final listener = ImageStreamListener((image, synchronous) {
      if (!failure.isCompleted) {
        failure.complete(StateError('Unexpected frame'));
      }
    }, onError: (Object error, StackTrace? stack) {
      if (!failure.isCompleted) failure.complete(error);
    });
    stream.addListener(listener);
    try {
      expect(await failure.future, isA<PlatformException>());
      expect(decodeCalled, isFalse);
    } finally {
      stream.removeListener(listener);
    }
  });

  test('decoder failure is preserved rather than replaced by cleanup',
      () async {
    final provider = ProtectedResourceImage('/synthetic/encrypted-source');
    final stream = provider.loadImage(provider, (buffer,
        {ui.TargetImageSize Function(int, int)? getTargetSize}) async {
      buffer.dispose();
      throw StateError('Synthetic decoder failed');
    });
    final failure = Completer<Object>();
    final listener = ImageStreamListener((image, synchronous) {
      if (!failure.isCompleted) {
        failure.complete(StateError('Unexpected frame'));
      }
    }, onError: (Object error, StackTrace? stack) {
      if (!failure.isCompleted) failure.complete(error);
    });
    stream.addListener(listener);
    try {
      expect(
          await failure.future,
          isA<StateError>().having((e) => e.message, 'original decoder error',
              'Synthetic decoder failed'));
    } finally {
      stream.removeListener(listener);
    }
  });
}
