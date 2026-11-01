import 'dart:convert';
import 'dart:typed_data';
import 'package:app_im/app_im.dart';
import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter/services.dart';

class FakeTransport implements HttpClientAdapter {
  final List<RequestOptions> requests = [];
  Map<String, dynamic> Function(RequestOptions) respond;
  FakeTransport(this.respond);
  @override
  Future<ResponseBody> fetch(RequestOptions options, Stream<Uint8List>? stream,
      Future<void>? cancel) async {
    requests.add(options);
    return ResponseBody.fromString(jsonEncode(respond(options)), 200, headers: {
      Headers.contentTypeHeader: [Headers.jsonContentType]
    });
  }

  @override
  void close({bool force = false}) {}
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  test('legacy input updates call the pinned conversation input API', () async {
    const channel = MethodChannel('flutter_openim_sdk');
    final calls = <MethodCall>[];
    final manager = AppIm.iMManager;
    final oldToken = manager.token;
    var oldUser = '';
    try {
      oldUser = manager.userID;
    } catch (_) {
      // A fresh official SDK has not assigned its late userID before login.
    }
    final oldLogin = manager.isLogined;
    manager.token = 'synthetic-input-token';
    manager.userID = 'synthetic-self';
    manager.isLogined = true;
    final messenger =
        TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
    messenger.setMockMethodCallHandler(channel, (call) async {
      calls.add(call);
      if (call.method == 'getOneConversation') {
        expect(call.arguments['sourceID'], 'synthetic-peer');
        return jsonEncode({
          'conversationID': 'si_synthetic-peer_synthetic-self',
          'conversationType': 1,
          'unreadCount': 0
        });
      }
      expect(call.method, 'changeInputStates');
      return null;
    });
    try {
      await manager.messageManager
          .typingStatusUpdate(userID: 'synthetic-peer', msgTip: 'yes');
      await manager.messageManager
          .typingStatusUpdate(userID: 'synthetic-peer', msgTip: 'no');
      final input =
          calls.where((x) => x.method == 'changeInputStates').toList();
      expect(input.map((x) => x.arguments['focus']), [true, false]);
      expect(
          input.every((x) =>
              x.arguments['conversationID'] ==
              'si_synthetic-peer_synthetic-self'),
          true);
      expect(calls.any((x) => x.method == 'typingStatusUpdate'), false);
    } finally {
      messenger.setMockMethodCallHandler(channel, null);
      manager.token = oldToken;
      manager.userID = oldUser;
      manager.isLogined = oldLogin;
    }
  });
  test('captured pair verification never reads a replacement login token',
      () async {
    final adapter = FakeTransport((request) => {
          'errCode': 0,
          'data': {'invitation': null}
        });
    final manager = EnterpriseManager(dio: Dio()..httpClientAdapter = adapter)
      ..configure(
          baseUrl: () => 'https://example.test/enterprise/v1',
          imToken: () => 'new-global-im',
          chatToken: () => 'new-global-chat',
          platform: () => 9);
    await manager.verifySession(
        imToken: 'captured-im', chatToken: 'captured-chat', platformID: 2);
    expect(adapter.requests.single.headers['token'], 'captured-im');
    expect(adapter.requests.single.headers['chatToken'], 'captured-chat');
    expect(adapter.requests.single.headers['platformID'], 2);
    manager.dispose();
  });
  test('input lookup from an older login never publishes the new session state',
      () async {
    const channel = MethodChannel('flutter_openim_sdk');
    final manager = AppIm.iMManager;
    manager.userID = 'synthetic-self';
    manager.token = 'input-old-token';
    manager.isLogined = true;
    var changes = 0;
    final messenger =
        TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
    messenger.setMockMethodCallHandler(channel, (call) async {
      if (call.method == 'getOneConversation') {
        manager.token = 'input-new-token';
        return jsonEncode({
          'conversationID': 'synthetic-cid',
          'conversationType': 1,
          'unreadCount': 0
        });
      }
      if (call.method == 'changeInputStates') changes++;
      return null;
    });
    try {
      await manager.messageManager
          .typingStatusUpdate(userID: 'synthetic-peer', msgTip: 'yes');
      expect(changes, 0);
      expect(manager.token, 'input-new-token');
    } finally {
      messenger.setMockMethodCallHandler(channel, null);
      manager.token = null;
      manager.isLogined = false;
    }
  });
  test(
      'quote factory rejects an unsent source before crossing the native boundary',
      () async {
    const channel = MethodChannel('flutter_openim_sdk');
    var calls = 0;
    final messenger =
        TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
    messenger.setMockMethodCallHandler(channel, (call) async {
      calls++;
      expect(call.method, 'createQuoteMessage');
      expect(call.arguments['quoteMessage']['sendID'], 'sender');
      return jsonEncode({'clientMsgID': 'quote-result', 'status': 2});
    });
    try {
      final adapter = PublicMessageAdapter();
      await expectLater(
          adapter.createQuoteMessage(text: 'reply', quoteMsg: Message()),
          throwsArgumentError);
      expect(calls, 0);
      final source = Message()
        ..clientMsgID = 'sent'
        ..sendID = 'sender'
        ..status = 2;
      final reply =
          await adapter.createQuoteMessage(text: 'reply', quoteMsg: source);
      expect(reply.clientMsgID, 'quote-result');
      expect(calls, 1);
    } finally {
      messenger.setMockMethodCallHandler(channel, null);
    }
  });
  test(
      'enterprise requests use both current tokens and preserve original signal data',
      () async {
    final adapter = FakeTransport((_) => {
          'errCode': 0,
          'data': {
            'signalResp': {
              'invite': {
                'roomID': 'r1',
                'token': 'room-token',
                'liveURL': 'wss://example.test/livekit'
              }
            }
          }
        });
    final dio = Dio()..httpClientAdapter = adapter;
    final manager = EnterpriseManager(dio: dio)
      ..configure(
          baseUrl: () => 'https://example.test/enterprise/v1',
          imToken: () => 'im-token',
          chatToken: () => 'chat-token',
          platform: () => 2);
    final result = await manager.signalingInvite(
        info: SignalingInfo(
            invitation: InvitationInfo(
                inviterUserID: 'u1',
                inviteeUserIDList: ['u2'],
                mediaType: 'video')));
    expect(result.roomID, 'r1');
    final request = adapter.requests.single;
    expect(
        request.uri.path, '/enterprise/v1/signaling/signal_message_assemble');
    expect(request.headers['token'], 'im-token');
    expect(request.headers['chatToken'], 'chat-token');
    expect(request.headers['platformID'], 2);
    expect(
        request.data['signalReq']['invite']['invitation']['inviteeUserIDList'],
        ['u2']);
    manager.dispose();
  });
  test(
      'first connect skips historical event replay and restores only an active invitation',
      () async {
    final adapter = FakeTransport((request) => request.method == 'GET'
        ? {
            'errCode': 0,
            'data': {'events': [], 'cursor': 42, 'resyncRequired': false}
          }
        : {
            'errCode': 0,
            'data': {
              'invitation': {
                'roomID': 'active',
                'inviterUserID': 'u1',
                'inviteeUserIDList': ['u2']
              }
            }
          });
    final manager = EnterpriseManager(dio: Dio()..httpClientAdapter = adapter)
      ..configure(
          baseUrl: () => 'https://example.test/enterprise/v1',
          imToken: () => 'im',
          chatToken: () => 'chat',
          platform: () => 2);
    final seen = <String?>[];
    await manager.setSignalingListener(OnSignalingListener(
        onReceiveNewInvitation: (info) => seen.add(info.invitation?.roomID)));
    expect(adapter.requests.first.queryParameters['after'], -1);
    expect(seen, ['active']);
    manager.dispose();
  });
  test('business errors fail instead of yielding fake success', () async {
    final manager = EnterpriseManager(
        dio: Dio()
          ..httpClientAdapter = FakeTransport(
              (_) => {'errCode': 401, 'errMsg': 'revoked', 'data': {}}))
      ..configure(
          baseUrl: () => 'https://example.test/enterprise/v1',
          imToken: () => 'old',
          chatToken: () => 'old',
          platform: () => 2);
    await expectLater(manager.request('/rtc/records'), throwsStateError);
    manager.dispose();
  });

  test('a host mute for another participant never mutes the local microphone',
      () async {
    var pulls = 0;
    final adapter = FakeTransport((request) {
      if (request.method == 'POST')
        return {
          'errCode': 0,
          'data': {'invitation': null}
        };
      pulls++;
      return {
        'errCode': 0,
        'data': {
          'cursor': pulls == 1 ? 10 : 12,
          'events': pulls == 1
              ? []
              : [
                  {
                    'id': 11,
                    'event': 'OnStreamChanged',
                    'data': {
                      'roomID': 'r',
                      'userID': 'other',
                      'streamType': 'audio',
                      'mute': true
                    }
                  },
                  {
                    'id': 12,
                    'event': 'OnStreamChanged',
                    'data': {
                      'roomID': 'r',
                      'userID': 'self',
                      'muteAll': true,
                      'mute': true
                    }
                  }
                ]
        }
      };
    });
    final manager = EnterpriseManager(dio: Dio()..httpClientAdapter = adapter)
      ..configure(
          baseUrl: () => 'https://example.test/enterprise/v1',
          imToken: () => 'im',
          chatToken: () => 'chat',
          platform: () => 2,
          userID: () => 'self');
    final muted = <String?>[];
    await manager.setSignalingListener(OnSignalingListener(
        onMeetingStreamChanged: (e) => muted.add(e.streamType)));
    await manager.pullEvents();
    expect(muted, ['audio', 'video']);
    manager.dispose();
  });
  test(
      'accepting on PC closes the mobile ringing session through the other-device event',
      () async {
    var pulls = 0;
    final adapter = FakeTransport((request) {
      if (request.method == 'POST')
        return {
          'errCode': 0,
          'data': {'invitation': null}
        };
      pulls++;
      return {
        'errCode': 0,
        'data': {
          'cursor': pulls == 1 ? 10 : 11,
          'events': pulls == 1
              ? []
              : [
                  {
                    'id': 11,
                    'event': 'OnInviteeAccepted',
                    'data': {
                      'opUserID': 'self',
                      'opPlatformID': 5,
                      'userID': 'self',
                      'invitation': {'roomID': 'r'}
                    }
                  }
                ]
        }
      };
    });
    final manager = EnterpriseManager(dio: Dio()..httpClientAdapter = adapter)
      ..configure(
          baseUrl: () => 'https://example.test/enterprise/v1',
          imToken: () => 'im',
          chatToken: () => 'chat',
          platform: () => 2,
          userID: () => 'self');
    var other = 0, normal = 0;
    await manager.setSignalingListener(OnSignalingListener(
        onInviteeAccepted: (_) => normal++,
        onInviteeAcceptedByOtherDevice: (_) => other++));
    await manager.pullEvents();
    await manager.pullEvents();
    expect(other, 1);
    expect(normal, 0);
    manager.dispose();
  });
}
