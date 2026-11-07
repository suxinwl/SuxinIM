// Actual OpenIMLive -> singleton EnterpriseManager -> loopback HTTP. The
// signaling observer is controlled; no production account or media is used.
import 'dart:convert';
import 'dart:io';

import 'package:app_im/app_im.dart';
import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:openim_live/src/live_controller.dart';

class _Live with OpenIMLive {
  final errors = <Object>[];
  final stacks = <StackTrace>[];
  bool observerThrows = false;
  @override
  void onError(error, stack) {
    errors.add(error as Object);
    stacks.add(stack as StackTrace);
    if (observerThrows) throw StateError('SYNTHETIC_PRIVATE_OBSERVER_DETAIL');
  }
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  late HttpServer server;
  HttpOverrides? previousHttp;
  final requests = <Map<String, dynamic>>[];
  final errorsFromFixture = <Object>[];
  var status = 200, errCode = 0;
  var invalidCertificate = false;
  late _Live live;

  SignalingInfo info() => SignalingInfo(
        userID: 'synthetic-caller',
        invitation: InvitationInfo(
          roomID: 'synthetic-room',
          inviterUserID: 'synthetic-caller',
          inviteeUserIDList: ['synthetic-peer'],
          mediaType: 'audio',
          sessionType: 1,
        ),
      );
  Future<SignalingCertificate> call(String method) {
    if (method == 'invite') return live.onDialSingle(info());
    if (method == 'inviteInGroup') return live.onDialGroup(info());
    return live.onTapPickup(info());
  }

  String codeFor(String method) => method == 'invite'
      ? 'CALL_SIGNAL_INVITE_FAILED'
      : method == 'inviteInGroup'
          ? 'CALL_SIGNAL_GROUP_INVITE_FAILED'
          : 'CALL_SIGNAL_ACCEPT_FAILED';

  setUpAll(() async {
    // Flutter's default test HTTP client returns 400 without doing any I/O.
    // Only this explicit ephemeral loopback fixture is contacted instead.
    previousHttp = HttpOverrides.current;
    HttpOverrides.global = null;
    server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
    server.listen((request) async {
      try {
        expect(request.uri.path,
            '/enterprise/v1/signaling/signal_message_assemble');
        final body = jsonDecode(await utf8.decoder.bind(request).join())
            as Map<String, dynamic>;
        requests.add(body);
        final method = (body['signalReq'] as Map).keys.single as String;
        final certificate = <String, dynamic>{
          'roomID': 'synthetic-room',
          'token': 'SYNTHETIC_PRIVATE_TOKEN',
          'liveURL': 'wss://example.invalid/livekit',
          if (invalidCertificate) 'busyLineUserIDList': 27,
        };
        request.response.statusCode = status;
        request.response.headers.contentType = ContentType.json;
        request.response.write(jsonEncode({
          'errCode': errCode,
          'errMsg': 'SYNTHETIC_PRIVATE_SERVER_DETAIL',
          'errDlt': 'SYNTHETIC_PRIVATE_TOKEN',
          'data': {
            'signalResp': {method: certificate}
          },
        }));
        await request.response.close();
      } catch (error) {
        errorsFromFixture.add(error);
        await request.response.close();
      }
    });
    AppIm.enterprise.configure(
        baseUrl: () => 'http://127.0.0.1:${server.port}/enterprise/v1',
        imToken: () => 'SYNTHETIC_PRIVATE_IM_TOKEN',
        chatToken: () => 'SYNTHETIC_PRIVATE_CHAT_TOKEN',
        userID: () => 'synthetic-caller',
        platform: () => 2);
  });
  tearDownAll(() async {
    AppIm.enterprise.dispose();
    await server.close(force: true);
    HttpOverrides.global = previousHttp;
  });
  setUp(() {
    requests.clear();
    errorsFromFixture.clear();
    status = 200;
    errCode = 0;
    invalidCertificate = false;
    live = _Live();
  });
  tearDown(() async {
    live.onCloseLive();
    await Future<void>.delayed(Duration.zero);
    expect(errorsFromFixture, isEmpty);
  });

  for (final method in ['invite', 'inviteInGroup', 'accept']) {
    test('real $method preserves a successful typed certificate', () async {
      final result = await call(method);
      expect(result, isA<SignalingCertificate>());
      expect(result.roomID, 'synthetic-room');
      expect(result.token, 'SYNTHETIC_PRIVATE_TOKEN');
      expect(result.liveURL, 'wss://example.invalid/livekit');
      expect(live.errors, isEmpty);
      expect((requests.single['signalReq'] as Map).keys, [method]);
    });
    for (final failure in ['application', 'http', 'certificate']) {
      test(
          'real $method $failure error keeps its cause and shows only a fixed safe code',
          () async {
        if (failure == 'application') errCode = 403;
        if (failure == 'http') status = 403;
        if (failure == 'certificate') invalidCertificate = true;
        await expectLater(call(method), throwsA(isA<CallSignalingFailure>()));
        expect(live.errors, hasLength(1));
        final error = live.errors.single as CallSignalingFailure;
        expect(error.code, codeFor(method));
        expect(error.toString(), codeFor(method));
        expect(error.toString(), isNot(contains('SYNTHETIC_PRIVATE')));
        expect(error.toString(), isNot(contains('127.0.0.1')));
        expect(error.toString(), isNot(contains('catchError')));
        expect(identical(error.originalStack, live.stacks.single), isTrue);
        if (failure == 'application') expect(error.cause, isA<StateError>());
        if (failure == 'http') expect(error.cause, isA<DioException>());
        if (failure == 'certificate') {
          expect(error.cause, isA<NoSuchMethodError>());
        }
      });
    }
  }
  test('failing observer cannot replace the original signaling failure',
      () async {
    errCode = 403;
    live.observerThrows = true;
    await expectLater(call('accept'), throwsA(isA<CallSignalingFailure>()));
    final error = live.errors.single as CallSignalingFailure;
    expect(error.code, 'CALL_SIGNAL_ACCEPT_FAILED');
    expect(error.cause, isA<StateError>());
    expect(error.cause.toString(), contains('SYNTHETIC_PRIVATE_SERVER_DETAIL'));
    expect(error.toString(), isNot(contains('OBSERVER')));
  });
}
