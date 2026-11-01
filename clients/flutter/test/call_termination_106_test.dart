// Actual termination coordinator and actual EnterpriseManager request wiring.
// Dio adapter/request callbacks are network boundaries; no native/UI/network IO.
import 'dart:async';
import 'dart:convert';
import 'dart:typed_data';

import 'package:app_im/app_im.dart';
import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';

class _Http implements HttpClientAdapter {
  _Http(this.respond);
  final FutureOr<ResponseBody> Function(RequestOptions options) respond;
  final requests = <RequestOptions>[];
  @override
  Future<ResponseBody> fetch(RequestOptions options, Stream<Uint8List>? stream,
      Future<void>? cancel) async {
    requests.add(options);
    return await respond(options);
  }

  @override
  void close({bool force = false}) {}
}

ResponseBody _ok([Map<String, dynamic> data = const {}]) =>
    ResponseBody.fromString(jsonEncode({'errCode': 0, 'data': data}), 200,
        headers: {
          Headers.contentTypeHeader: [Headers.jsonContentType]
        });

DioException _timeout() => DioException(
    requestOptions: RequestOptions(path: 'synthetic-no-network'),
    type: DioExceptionType.receiveTimeout,
    message: 'Synthetic response was lost');

Future<void> _until(bool Function() condition) async {
  final until = DateTime.now().add(const Duration(seconds: 3));
  while (!condition()) {
    if (DateTime.now().isAfter(until)) fail('Call fixture never became ready');
    await Future<void>.delayed(Duration.zero);
  }
}

SignalingInfo _info(String room) => SignalingInfo(
    userID: 'untrusted-input-user',
    invitation: InvitationInfo(
        roomID: room,
        groupID: 'synthetic-group',
        inviteeUserIDList: ['synthetic-peer'],
        sessionType: 2));

class _Manager {
  _Manager(_Http adapter) {
    value = EnterpriseManager(dio: Dio()..httpClientAdapter = adapter)
      ..configure(
          baseUrl: () => base,
          userID: () => user,
          imToken: () => im,
          chatToken: () => chat,
          platform: () => platform,
          ready: () => ready);
  }
  late final EnterpriseManager value;
  String base = 'https://example.test/enterprise/v1';
  String user = 'synthetic-caller';
  String im = 'synthetic-old-im';
  String chat = 'synthetic-old-chat';
  int platform = 2;
  bool ready = true;
}

const _session = CallTerminationSession('https://example.test/enterprise/v1',
    'synthetic-caller', 'synthetic-im', 'synthetic-chat', 2, 1);

void main() {
  test(
      'actual Manager same action singleflight captures immutable info and pair',
      () async {
    final response = Completer<ResponseBody>();
    final http = _Http((_) => response.future);
    final manager = _Manager(http);
    addTearDown(manager.value.dispose);
    final info = _info('same-action');
    final first = manager.value.terminateCall(method: 'hungUp', info: info);
    final repeated = manager.value.terminateCall(method: 'hungUp', info: info);
    expect(identical(first, repeated), isTrue);
    await _until(() => http.requests.length == 1);
    info.invitation!.roomID = 'mutated-after-dispatch';
    info.invitation!.inviteeUserIDList!.add('late-mutated-peer');
    info.userID = 'later-input';
    final request = http.requests.single;
    final captured = request.data['signalReq']['hungUp'] as Map;
    expect(captured['userID'], 'synthetic-caller');
    expect(captured['invitation']['roomID'], 'same-action');
    expect(captured['invitation']['inviteeUserIDList'], ['synthetic-peer']);
    expect(request.headers['token'], 'synthetic-old-im');
    expect(request.headers['chatToken'], 'synthetic-old-chat');
    expect(request.headers['platformID'], 2);
    response.complete(_ok());
    expect((await first).status, CallTerminationStatus.confirmed);
    await repeated;
  });

  test('actual Manager pending cancel cannot swallow later hungUp action',
      () async {
    final heldCancel = Completer<ResponseBody>();
    final http = _Http((request) async {
      final action = (request.data['signalReq'] as Map).keys.single;
      if (action == 'cancel') return await heldCancel.future;
      return _ok();
    });
    final manager = _Manager(http);
    addTearDown(manager.value.dispose);
    final info = _info('action-change');
    final cancel = manager.value.terminateCall(method: 'cancel', info: info);
    await _until(() => http.requests.length == 1);
    final hangup = manager.value.terminateCall(method: 'hungUp', info: info);
    try {
      expect(identical(cancel, hangup), isFalse);
      await _until(() => http.requests.length == 2);
      expect((await hangup).status, CallTerminationStatus.confirmed);
      expect(http.requests.map((r) => (r.data['signalReq'] as Map).keys.single),
          ['cancel', 'hungUp']);
    } finally {
      heldCancel.complete(_ok());
      await cancel;
    }
  });

  test('actual Manager invalidation cancels old request without new-pair retry',
      () async {
    final held = Completer<ResponseBody>();
    final http = _Http((request) async {
      if (httpAction(request) == 'hungUp') return await held.future;
      return _ok();
    });
    final manager = _Manager(http);
    addTearDown(manager.value.dispose);
    final operation =
        manager.value.terminateCall(method: 'hungUp', info: _info('old-room'));
    await _until(() => http.requests.length == 1);
    manager.value.invalidateCallTerminations();
    manager.user = 'synthetic-replacement-user';
    manager.im = 'synthetic-new-im';
    manager.chat = 'synthetic-new-chat';
    manager.base = 'https://replacement.test/enterprise/v1';
    final result = await operation.timeout(const Duration(seconds: 3));
    expect(result.status, CallTerminationStatus.cancelled);
    expect(await manager.value.retryPendingCallTerminations(), isEmpty);
    held.complete(_ok());
    await Future<void>.delayed(Duration.zero);
    expect(http.requests.length, 1);
    expect(http.requests.single.headers['token'], 'synthetic-old-im');
    expect(http.requests.single.uri.host, 'example.test');
  });

  for (final code in [401, 403]) {
    test('actual Manager HTTP$code stops query and foreground retry', () async {
      final http = _Http((_) => ResponseBody.fromString('{}', code, headers: {
            Headers.contentTypeHeader: [Headers.jsonContentType]
          }));
      final manager = _Manager(http);
      addTearDown(manager.value.dispose);
      final result = await manager.value
          .terminateCall(method: 'hungUp', info: _info('auth-$code'));
      expect(result.status, CallTerminationStatus.failed);
      expect(http.requests.length, 1);
      await manager.value.retryPendingCallTerminations();
      expect(http.requests.length, 1);
    });
  }

  for (final state in <Map<String, dynamic>>[
    {'ended': true},
    {
      'ended': false,
      'state': 'active',
      'participantState': {'synthetic-caller': 'left'}
    }
  ]) {
    test(
        'actual coordinator confirms timed-out POST only from authoritative $state',
        () async {
      final paths = <String>[];
      final coordinator = CallTerminationCoordinator(
          retryDelays: const [],
          current: (_) => true,
          request: (session, path, body, cancel) async {
            paths.add(path);
            if (path.endsWith('signal_message_assemble')) throw _timeout();
            return state;
          });
      final result = await coordinator.terminate(
          _session, 'hungUp', _info('query-confirmed').toJson());
      expect(result.status, CallTerminationStatus.confirmed);
      expect(result.attempts, 1);
      expect(paths.length, 2);
      coordinator.invalidate();
    });
  }

  test('activeInvitation=false alone never confirms a failed termination',
      () async {
    final coordinator = CallTerminationCoordinator(
        retryDelays: const [],
        current: (_) => true,
        request: (session, path, body, cancel) async {
          if (path.endsWith('signal_message_assemble')) throw _timeout();
          return {
            'ended': false,
            'activeInvitationForActor': false,
            'participantState': {'synthetic-caller': 'joined'}
          };
        });
    final result = await coordinator.terminate(
        _session, 'hungUp', _info('not-confirmed').toJson());
    expect(result.status, CallTerminationStatus.failed);
    coordinator.invalidate();
  });

  test(
      'current snapshot guard throw settles actual coordinator instead of leaking Future',
      () async {
    final coordinator = CallTerminationCoordinator(
        retryDelays: const [],
        current: (_) => throw StateError('synthetic guard failure'),
        request: (session, path, body, cancel) async {
          fail('Guard rejection must not make network request');
        });
    final result = await coordinator
        .terminate(_session, 'hungUp', _info('bad-guard').toJson())
        .timeout(const Duration(seconds: 2));
    expect(result.status, CallTerminationStatus.failed);
    coordinator.invalidate();
  });

  test(
      'ordinary failed task retains captured pair for explicit foreground retry',
      () async {
    var recovered = false;
    final sessions = <CallTerminationSession>[];
    final coordinator = CallTerminationCoordinator(
        retryDelays: const [],
        current: (_) => true,
        request: (session, path, body, cancel) async {
          sessions.add(session);
          if (recovered) return {};
          if (path.endsWith('signal_message_assemble')) throw _timeout();
          return {'ended': false};
        });
    final first = await coordinator.terminate(
        _session, 'cancel', _info('resume').toJson());
    expect(first.status, CallTerminationStatus.failed);
    recovered = true;
    final resumed = await coordinator.retryPending();
    expect(resumed.single.status, CallTerminationStatus.confirmed);
    expect(sessions.every((session) => identical(session, _session)), isTrue);
    coordinator.invalidate();
  });
}

String? httpAction(RequestOptions request) =>
    (request.data['signalReq'] as Map?)?.keys.single as String?;
