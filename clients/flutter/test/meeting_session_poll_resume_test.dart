import 'dart:async';
import 'dart:convert';
import 'dart:typed_data';

import 'package:app_im/app_im.dart';
import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';

ResponseBody _body(Map<String, dynamic> data) => ResponseBody.fromString(
    jsonEncode({'errCode': 0, 'data': data}), 200,
    headers: {Headers.contentTypeHeader: [Headers.jsonContentType]});

class _Transport implements HttpClientAdapter {
  final entered = Completer<void>();
  final held = Completer<ResponseBody>();
  bool holdMeetings = false;
  int eventPulls = 0;
  @override
  Future<ResponseBody> fetch(RequestOptions options, Stream<Uint8List>? stream,
      Future<void>? cancelFuture) async {
    if (options.path.endsWith('/rtc/events')) {
      eventPulls++;
      return _body({
        'cursor': eventPulls == 1 ? 10 : 11,
        'events': eventPulls == 1
            ? []
            : [
                {
                  'id': 11,
                  'event': 'OnReceiveNewInvitation',
                  'data': {
                    'invitation': {'roomID': 'synthetic-room'}
                  }
                }
              ],
      });
    }
    if (options.path.endsWith('/get_signal_invitation_info_start_app')) {
      return _body({});
    }
    if (options.path.endsWith('/signal_get_meetings')) {
      if (!holdMeetings) return _body({'meetingInfoList': []});
    } else if (!options.path.endsWith('/get_signal_invitation_info')) {
      throw StateError('Unexpected controlled transport route');
    }
    entered.complete();
    return held.future;
  }

  @override
  void close({bool force = false}) {}
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  for (final holdMeetings in [true, false]) {
    test(
        holdMeetings
            ? 'old bootstrap meeting list cannot publish into replacement session'
            : 'old verified invitation cannot dispatch after session changes',
        () async {
      final transport = _Transport()..holdMeetings = holdMeetings;
      var identity = 'synthetic-A';
      final manager = EnterpriseManager(
          dio: Dio()..httpClientAdapter = transport)
        ..configure(
            baseUrl: () => 'https://unit.invalid/enterprise/v1',
            userID: () => identity,
            imToken: () => 'fake-im-$identity',
            chatToken: () => 'fake-chat-$identity',
            platform: () => 2);
      final seen = <String>[];
      final subscription = manager.meetingInfoChanges.stream
          .listen((info) => seen.add(info.roomID!));
      manager.listener = OnSignalingListener(
          onReceiveNewInvitation: (_) => seen.add('invitation'));
      if (!holdMeetings) await manager.pullEvents();
      final pending = manager.pullEvents();
      await transport.entered.future;
      manager.invalidateCallTerminations();
      identity = 'synthetic-B';
      transport.held.complete(_body(holdMeetings
          ? {
              'meetingInfoList': [
                {'roomID': 'synthetic-old', 'startTime': 1, 'endTime': 2}
              ]
            }
          : {'state': 'active', 'ended': false}));
      await pending;
      await Future<void>.delayed(Duration.zero);
      expect(seen, isEmpty);
      manager.dispose();
      await subscription.cancel();
    });
  }
}
