import 'dart:convert';
import 'dart:io';

import 'package:app_im/app_im.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';

const _id = 'synthetic-forward-quote';
Message _quote() => Message.fromJson({
      'contentType': MessageType.quote,
      'clientMsgID': _id,
      'sendID': 'synthetic-sender',
      'status': 2,
      'exMap': {'snapshot': 'retained'},
      'quoteElem': {
        'text': 'Reply preserved',
        'quoteMessage': {
          'clientMsgID': 'synthetic-source-from-other-conversation',
          'sendID': 'synthetic-original-sender',
          'seq': 37,
          'status': 2,
          'contentType': MessageType.video,
          'videoElem': {
            'videoUrl': 'https://unit.invalid/video.mp4',
            'snapshotUrl': 'https://unit.invalid/video.jpg',
            'duration': 7,
          },
        },
      },
    });

class _LocalHttp extends HttpOverrides {}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  const channel = MethodChannel('flutter_openim_sdk');
  final messenger =
      TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
  tearDown(() => messenger.setMockMethodCallHandler(channel, null));

  test('actual adapter routes five Quote destinations through official NotOss with snapshot and options intact', () async {
    final calls = <Map<String, dynamic>>[];
    final methods = <String>[];
    messenger.setMockMethodCallHandler(channel, (call) async {
      methods.add(call.method);
      final args = Map<String, dynamic>.from(call.arguments);
      calls.add(jsonDecode(jsonEncode(args)));
      // The pinned cleaner can mutate nested maps. The caller's source must
      // remain reusable for each destination and must never lose its snapshot.
      (args['message']['exMap'] as Map)['sdk-cleaned'] = true;
      return jsonEncode({...Map<String, dynamic>.from(args['message']), 'status': 2});
    });
    final adapter = PublicMessageAdapter();
    final message = _quote();
    final original = jsonDecode(jsonEncode(message.toJson()));
    final push = OfflinePushInfo.fromJson({'title': 'Synthetic', 'desc': 'Quote preserved', 'ex': 'private-preview'});
    for (var i = 0; i < 5; i++) {
      final result = await adapter.sendMessage(message: message, offlinePushInfo: push,
          userID: 'synthetic-target-$i', isOnlineOnly: i.isOdd, operationID: 'synthetic-op-$i');
      expect(result.contentType, MessageType.quote);
      expect(result.quoteElem!.quoteMessage!.clientMsgID, 'synthetic-source-from-other-conversation');
    }
    expect(methods, List.filled(5, 'sendMessageNotOss'));
    for (var i = 0; i < calls.length; i++) {
      expect(calls[i]['userID'], 'synthetic-target-$i');
      expect(calls[i]['groupID'], '');
      expect(calls[i]['isOnlineOnly'], i.isOdd);
      expect(calls[i]['operationID'], 'synthetic-op-$i');
      expect(calls[i]['offlinePushInfo']['title'], 'Synthetic');
      expect(Message.fromJson(Map<String, dynamic>.from(calls[i]['message'])).quoteElem!.toJson(), original['quoteElem']);
    }
    expect(message.toJson(), original);
  });

  test('ordinary text and upload media retain original sendMessage route', () async {
    final methods = <String>[];
    final types = <int>[];
    messenger.setMockMethodCallHandler(channel, (call) async {
      methods.add(call.method);
      final message = Map<String, dynamic>.from(call.arguments['message']);
      types.add(message['contentType']);
      return jsonEncode({...message, 'status': 2});
    });
    final adapter = PublicMessageAdapter();
    final input = [MessageType.text, MessageType.picture, MessageType.voice, MessageType.video, MessageType.file];
    for (final type in input) {
      await adapter.sendMessage(message: Message(contentType: type, clientMsgID: 'synthetic-$type'),
          offlinePushInfo: OfflinePushInfo(), userID: 'synthetic-target');
    }
    expect(methods, List.filled(input.length, 'sendMessage'));
    expect(types, input);
  });

  test('group Quote still hydrates receipt summary after actual SDK channel success', () => HttpOverrides.runWithHttpOverrides(() async {
    final server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
    final received = <Map<String, dynamic>>[];
    server.listen((request) async {
      final data = jsonDecode(await utf8.decoder.bind(request).join()) as Map;
      received.add(Map<String, dynamic>.from(data));
      request.response.headers.contentType = ContentType.json;
      request.response.write(jsonEncode({'errCode': 0, 'data': {'num': {_id: {'readNum': 1, 'unreadNum': 2}}}}));
      await request.response.close();
    });
    AppIm.enterprise.configure(baseUrl: () => 'http://127.0.0.1:${server.port}/enterprise/v1',
        imToken: () => 'synthetic-im', chatToken: () => 'synthetic-chat', platform: () => 2);
    final calls = <String>[];
    messenger.setMockMethodCallHandler(channel, (call) async {
      calls.add(call.method);
      if (call.method == 'getOneConversation') return jsonEncode({'conversationID': 'sg_synthetic-group', 'conversationType': 3, 'unreadCount': 0});
      return jsonEncode({...Map<String, dynamic>.from(call.arguments['message']),
        'groupID': 'synthetic-group', 'sessionType': 3, 'seq': 38, 'status': 2});
    });
    try {
      final result = await PublicMessageAdapter().sendMessage(message: _quote(),
          offlinePushInfo: OfflinePushInfo(), groupID: 'synthetic-group', operationID: 'synthetic-group-op');
      expect(calls, ['sendMessageNotOss', 'getOneConversation']);
      expect(received.single['clientMsgIDs'], [_id]);
      expect(result.attachedInfoElem!.groupHasReadInfo!.hasReadCount, 1);
      expect(result.attachedInfoElem!.groupHasReadInfo!.unreadCount, 2);
      expect(AppIm.receiptKnown.contains(_id), isTrue);
    } finally {
      AppIm.enterprise.dispose();
      AppIm.receiptKnown.remove(_id);
      await server.close(force: true);
    }
  }, _LocalHttp()));

  test('Quote send rejection propagates without retrying unsafe normal route', () async {
    final calls = <String>[];
    messenger.setMockMethodCallHandler(channel, (call) async {
      calls.add(call.method);
      throw PlatformException(code: 'synthetic-denied', message: 'Synthetic rejection');
    });
    await expectLater(PublicMessageAdapter().sendMessage(message: _quote(),
        offlinePushInfo: OfflinePushInfo(), userID: 'synthetic-target'),
        throwsA(isA<PlatformException>().having((error) => error.code, 'code', 'synthetic-denied')));
    expect(calls, ['sendMessageNotOss']);
  });
}
