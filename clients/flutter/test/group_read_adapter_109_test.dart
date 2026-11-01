import 'dart:convert';
import 'dart:io';

import 'package:app_im/app_im.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';

const _cid = 'sg_group-read-fixture';
const _gid = 'group-read-fixture';
const _messageID = 'group-read-message';

Map<String, dynamic> _message({String id = _messageID, int seq = 2}) => {
      'clientMsgID': id,
      'groupID': _gid,
      'sendID': 'fixture-sender',
      'sessionType': 3,
      'seq': seq,
      'status': 2,
      'contentType': 101,
      'textElem': {'content': 'group receipt fixture'},
    };

Map<String, dynamic> _findResult(List<Map<String, dynamic>> messages) => {
      'totalCount': messages.length,
      // Exact fixed SDK findMessageList wire model, not searchLocalMessages.
      'findResultItems': [
        {
          'conversationID': _cid,
          'conversationType': 3,
          'messageCount': messages.length,
          'messageList': messages,
        }
      ],
    };

// Only the synthetic local HTTP boundary is real. No production network/TLS
// configuration is changed and no real account or credential is used.
class _LocalHttp extends HttpOverrides {}

class _ReceiptFixture {
  final HttpServer server;
  final requests = <Map<String, dynamic>>[];
  final errors = <Object>[];

  _ReceiptFixture(this.server);

  static Future<_ReceiptFixture> start() async {
    final fixture =
        _ReceiptFixture(await HttpServer.bind(InternetAddress.loopbackIPv4, 0));
    fixture.server.listen(fixture._handle);
    return fixture;
  }

  String get baseUrl => 'http://127.0.0.1:${server.port}/enterprise/v1';

  Future<void> _handle(HttpRequest request) async {
    try {
      final body = Map<String, dynamic>.from(
          jsonDecode(await utf8.decoder.bind(request).join()) as Map);
      requests.add({'path': request.uri.path, 'body': body});
      Map<String, dynamic> data;
      switch (request.uri.path.split('/').last) {
        case 'mark_group_message_read':
          data = {
            'mark': {_messageID: 1}
          };
          break;
        case 'get_group_message_has_read':
          data = {
            'count': 2,
            'reads': [
              {'userID': 'fixture-current-member'},
              {'userID': 'fixture-former-member'},
            ],
          };
          break;
        case 'get_group_message_read_num':
          // Actual receipts.go accepts both spellings. A strict fake here
          // would incorrectly manufacture a failure for the working hydrate.
          final ids = body['clientMsgIDs'] ?? body['clientMsgIDList'];
          if (ids is! List || ids.isEmpty) {
            throw StateError('Receipt request requires message IDs');
          }
          data = {
            'num': {
              for (final id in ids) id as String: {'readNum': 1, 'unreadNum': 2}
            }
          };
          break;
        default:
          throw StateError('Unexpected synthetic receipt endpoint');
      }
      request.response.headers.contentType = ContentType.json;
      request.response.write(jsonEncode({'errCode': 0, 'data': data}));
      await request.response.close();
    } catch (error) {
      errors.add(error);
      request.response.statusCode = HttpStatus.internalServerError;
      await request.response.close();
    }
  }

  Future<void> close() => server.close(force: true);
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  const channel = MethodChannel('flutter_openim_sdk');
  final messenger =
      TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
  late _ReceiptFixture fixture;
  final calls = <MethodCall>[];

  setUp(() async {
    fixture = await _ReceiptFixture.start();
    calls.clear();
    AppIm.receiptKnown.clear();
    AppIm.enterprise.configure(
      baseUrl: () => fixture.baseUrl,
      imToken: () => 'synthetic-im',
      chatToken: () => 'synthetic-chat',
      platform: () => 2,
    );
    messenger.setMockMethodCallHandler(channel, (call) async {
      calls.add(call);
      if (call.method == 'findMessageList') {
        expect(call.arguments['searchParams'], [
          {
            'conversationID': _cid,
            'clientMsgIDList': [_messageID],
          }
        ]);
        return jsonEncode(_findResult([_message()]));
      }
      if (call.method == 'getGroupMembersInfo') {
        expect(call.arguments['groupID'], _gid);
        return jsonEncode([
          {
            'userID': 'fixture-current-member',
            'groupID': _gid,
            'nickname': 'Current group member',
          }
        ]);
      }
      if (call.method == 'getUsersInfo') {
        return jsonEncode([
          {
            'userID': 'fixture-former-member',
            'nickname': 'Former group member',
          }
        ]);
      }
      throw StateError('Unexpected fixed SDK method ${call.method}');
    });
  });

  tearDown(() async {
    messenger.setMockMethodCallHandler(channel, null);
    AppIm.enterprise.dispose();
    AppIm.receiptKnown.clear();
    await fixture.close();
    expect(fixture.errors, isEmpty);
  });

  Future<T> localHttp<T>(Future<T> Function() action) =>
      HttpOverrides.runWithHttpOverrides(action, _LocalHttp());

  test('fixed SDK preserves separate find and search result models', () {
    final result = SearchResult.fromJson(_findResult([_message()]));
    expect(result.searchResultItems, isNull);
    expect(result.findResultItems!.single.messageList!.single.seq, 2);
    expect(result.findResultItems!.single.messageList!.single.clientMsgID,
        _messageID);
  });

  test(
      'group read mark uses real fixed SDK find results and actual message seq',
      () async {
    await localHttp(() => AppIm.iMManager.messageManager
        .sendGroupMessageReadReceipt(_cid, [_messageID]));
    final request = fixture.requests.single;
    expect(request['path'], '/enterprise/v1/msg/mark_group_message_read');
    expect(request['body'], {
      'conversationID': _cid,
      'groupID': _gid,
      'clientMsgs': {_messageID: 'fixture-sender'},
      'seqs': {_messageID: 2},
    });
  });

  test('reader details resolve fixed find results and former member profiles',
      () async {
    final readers = await localHttp(() => AppIm.iMManager.messageManager
        .getGroupMessageReaderList(_cid, _messageID));
    expect(readers.map((m) => m.userID),
        ['fixture-current-member', 'fixture-former-member']);
    expect(readers.map((m) => m.nickname),
        ['Current group member', 'Former group member']);
    final body = fixture.requests.single['body'] as Map;
    expect(body['seq'], 2);
    expect(body['groupID'], _gid);
    expect(body['clientMsgID'], _messageID);
    expect(calls.map((c) => c.method),
        ['findMessageList', 'getGroupMembersInfo', 'getUsersInfo']);
  });

  test(
      'existing hydrate IDs contract remains compatible and fills known counts',
      () async {
    final message = Message.fromJson(_message());
    await localHttp(
        () => PublicMessageAdapter.hydrate([message], conversationID: _cid));
    expect(message.attachedInfoElem!.groupHasReadInfo!.hasReadCount, 1);
    expect(message.attachedInfoElem!.groupHasReadInfo!.unreadCount, 2);
    expect(AppIm.receiptKnown, contains(_messageID));
    expect(fixture.requests.single['body']['clientMsgIDs'], [_messageID]);
  });

  test(
      'genuinely absent local find message is rejected before enterprise access',
      () async {
    messenger.setMockMethodCallHandler(channel, (call) async {
      expect(call.method, 'findMessageList');
      return jsonEncode(_findResult([]));
    });
    await expectLater(
        AppIm.iMManager.messageManager
            .getGroupMessageReaderList(_cid, _messageID),
        throwsStateError);
    expect(fixture.requests, isEmpty);
  });
}
