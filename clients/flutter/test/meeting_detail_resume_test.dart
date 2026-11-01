// Actual112 controller/page/Enterprise transport regressions.
// MeetingClient connection ownership is covered separately at its real layer.
import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';
import 'package:app_im/app_im.dart';
import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_screenutil/flutter_screenutil.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:get/get.dart';
import 'package:openim_common/openim_common.dart';
import 'package:openim_meeting/src/pages/meeting_detail/meeting_detail_logic.dart';
import 'package:openim_meeting/src/pages/meeting_detail/meeting_detail_view.dart';

Map<String, dynamic> _info({int end = 7200}) => {
      'roomID': 'synthetic-room-A',
      'meetingName': 'Synthetic meeting',
      'hostUserID': 'synthetic-user',
      'startTime': 1,
      'endTime': end,
    };
Map<String, dynamic> _state(String state, {int end = 7200}) => {
      'state': state,
      'ended': state == 'ended',
      'meetingInfo': _info(end: end),
    };

class _Transport implements HttpClientAdapter {
  final requests = <RequestOptions>[];
  final held = <Completer<ResponseBody>>[];
  List<Map<String, dynamic>> events = [];
  bool resyncRequired = false;
  ResponseBody body(Map<String, dynamic> data,
          {int status = 200, int code = 0}) =>
      ResponseBody.fromString(
          jsonEncode({'errCode': code, 'data': data}), status,
          headers: {
            Headers.contentTypeHeader: [Headers.jsonContentType]
          });
  @override
  Future<ResponseBody> fetch(RequestOptions options, Stream<Uint8List>? stream,
      Future<void>? cancel) async {
    requests.add(options);
    if (options.path.endsWith('/rtc/events')) {
      return body({
        'events': events,
        'cursor': events.isEmpty ? 0 : events.last['id'],
        'resyncRequired': resyncRequired
      });
    }
    if (options.path.endsWith('/get_signal_invitation_info_start_app')) {
      // This actor-only "ended" value must NEVER set detail room state.
      return body({'invitation': null, 'ended': true});
    }
    if (options.path.endsWith('/signal_get_meetings'))
      return body({'meetingInfoList': []});
    expectSync(options.path.endsWith('/get_signal_invitation_info'), isTrue);
    final next = Completer<ResponseBody>();
    held.add(next);
    return next.future;
  }

  void complete(int index, Map<String, dynamic> state,
          {int status = 200, int code = 0}) =>
      held[index].complete(body(state, status: status, code: code));
  @override
  void close({bool force = false}) {}
}

class _Fixture {
  _Fixture() {
    final dio = Dio()..httpClientAdapter = transport;
    manager = EnterpriseManager(dio: dio)
      ..configure(
        baseUrl: () => 'https://unit.invalid/enterprise/v1',
        userID: () => user,
        imToken: () => im,
        chatToken: () => chat,
        platform: () => 2,
        ready: () => ready,
      );
    logic = MeetingDetailLogic(
        enterprise: manager,
        joinMeeting: (_, session, current) async {
          expectSync(session.imToken, 'fake-im-A');
          expectSync(session.chatToken, 'fake-chat-A');
          expectSync(current(), isTrue);
          joins++;
          return true;
        });
  }
  final transport = _Transport();
  late final EnterpriseManager manager;
  late final MeetingDetailLogic logic;
  String user = 'synthetic-user', im = 'fake-im-A', chat = 'fake-chat-A';
  bool ready = true;
  int joins = 0;
}

Future<void> _flush(WidgetTester tester) async {
  // Do not pumpAndSettle while a real loading progress animation is visible.
  for (var i = 0; i < 6; i++) {
    await tester.pump(const Duration(milliseconds: 10));
    await tester.runAsync(() => Future<void>.delayed(const Duration(milliseconds: 1)));
  }
}

Future<void> _pull(WidgetTester tester, EnterpriseManager manager) async {
  var done = false;
  Object? failure;
  manager.pullEvents().then<void>((_) {
    done = true;
  }, onError: (Object error) {
    failure = error;
    done = true;
  });
  for (var i = 0; i < 30 && !done; i++) {
    await tester.pump(const Duration(milliseconds: 10));
  }
  expect(done, isTrue,
      reason:
          'Actual Enterprise pullEvents must settle while its fake-clock tasks are pumped');
  if (failure != null) throw failure!;
}

Future<void> _waitRequests(
    WidgetTester tester, _Fixture fixture, int count) async {
  for (var i = 0; i < 30 && fixture.transport.held.length < count; i++) {
    await tester.pump(const Duration(milliseconds: 10));
    await tester
        .runAsync(() => Future<void>.delayed(const Duration(milliseconds: 1)));
  }
  expect(fixture.transport.held, hasLength(count));
}

Future<void> _readyActive(WidgetTester tester, _Fixture fixture) async {
  var pumps = 0;
  final initialPhase = fixture.logic.phase.value;
  while (pumps < 30 &&
      (fixture.logic.phase.value != MeetingDetailPhase.active ||
          fixture.logic.loading.value)) {
    pumps++;
    await tester.pump(const Duration(milliseconds: 10));
  }
  // Let the request's published singleflight complete before a NEW action.
  await tester.pump();
  expect(fixture.logic.phase.value, MeetingDetailPhase.active);
  expect(fixture.logic.loading.value, isFalse);
  expect(fixture.logic.canEnter, isTrue);
  print('detail activeReady initial=$initialPhase boundedPumpCycles=$pumps');
}

Future<_Fixture> _mount(WidgetTester tester) async {
  final fixture = _Fixture();
  Get.testMode = true;
  tester.view.physicalSize = const Size(390, 844);
  tester.view.devicePixelRatio = 1;
  addTearDown(() async {
    await tester.pumpWidget(const SizedBox.shrink());
    Get.reset();
    tester.view.resetPhysicalSize();
    tester.view.resetDevicePixelRatio();
  });
  await tester.pumpWidget(ScreenUtilInit(
      designSize: const Size(390, 844),
      builder: (_, __) => GetMaterialApp(
            initialRoute: '/start',
            theme: ThemeData(fontFamily: 'Roboto'),
            translations: TranslationService(),
            locale: const Locale('en', 'US'),
            getPages: [
              GetPage(name: '/start', page: () => const Scaffold()),
              GetPage(name: '/away', page: () => const Scaffold()),
              GetPage(
                  name: '/detail',
                  transition: Transition.noTransition,
                  binding: BindingsBuilder(
                      () => Get.put<MeetingDetailLogic>(fixture.logic)),
                  page: () => MeetingDetailPage()),
            ],
          )));
  await tester.pumpAndSettle();
  Get.toNamed('/detail', arguments: {
    'meetingInfo': MeetingInfo.fromJson(_info()),
    'meetingCreator': 'Synthetic host',
  });
  await _flush(tester);
  expect(fixture.transport.held, hasLength(1));
  return fixture;
}

Button _enter(WidgetTester tester) =>
    tester.widget<Button>(find.byKey(const ValueKey('meeting-enter')));

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  setUpAll(() async {
    // The default square Ahem test glyphs overstate these time/status rows.
    // Load Flutter's bundled proportional Roboto, without suppressing overflow.
    final bytes = File(
            '../../tools/flutter-3.32.8/engine/src/flutter/txt/third_party/fonts/Roboto-Regular.ttf')
        .readAsBytesSync();
    final loader = FontLoader('Roboto')
      ..addFont(Future.value(ByteData.sublistView(bytes)));
    await loader.load();
  });
  testWidgets('cached old Started record becomes authoritative Ended, no Enter',
      (tester) async {
    final f = await _mount(tester);
    expect(find.byKey(const ValueKey('meeting-state-loading')), findsOneWidget);
    expect(_enter(tester).enabled, isFalse);
    f.transport.complete(0, _state('ended', end: 120));
    await _flush(tester);
    expect(f.logic.phase.value, MeetingDetailPhase.ended);
    expect(find.text(StrRes.meetingEnded), findsOneWidget);
    expect(_enter(tester).enabled, isFalse);
    expect(f.logic.meetingInfo.endTime, 120);
    await f.logic.enterMeeting();
    expect(f.joins, 0);
    expect(tester.takeException(), isNull);
  });

  testWidgets(
      'network/auth failure is visible Retry, never empty ended success',
      (tester) async {
    final f = await _mount(tester);
    f.transport.complete(0, {}, status: 401, code: 1501);
    await _flush(tester);
    expect(f.logic.phase.value, MeetingDetailPhase.failed);
    expect(find.byKey(const ValueKey('meeting-state-error')), findsOneWidget);
    expect(_enter(tester).enabled, isFalse);
    await tester.tap(find.byKey(const ValueKey('meeting-state-retry')));
    await _flush(tester);
    expect(f.transport.held, hasLength(2));
    f.transport.complete(1, _state('active', end: 2));
    await _flush(tester);
    // Current clock being far past end=2 is NOT a persisted ended transition.
    expect(f.logic.phase.value, MeetingDetailPhase.active);
    expect(_enter(tester).enabled, isTrue);
  });

  testWidgets('matching hint during held snapshot merges one serial rerun',
      (tester) async {
    final f = await _mount(tester);
    f.manager.notifyMeetingRoomChange('other-room');
    await _flush(tester);
    expect(f.transport.held, hasLength(1));
    for (var i = 0; i < 8; i++)
      f.manager.notifyMeetingRoomChange('synthetic-room-A');
    await _flush(tester);
    expect(f.transport.held, hasLength(1));
    f.transport.complete(0, _state('active'));
    await _flush(tester);
    expect(f.transport.held, hasLength(2));
    f.transport.complete(1, _state('ended'));
    await _flush(tester);
    expect(f.logic.phase.value, MeetingDetailPhase.ended);
    expect(f.transport.held, hasLength(2));
  });

  testWidgets('actual raw ordinary-leave event refreshes but does not end room',
      (tester) async {
    final f = await _mount(tester);
    f.transport.complete(0, _state('active'));
    await _flush(tester);
    await _pull(tester,
        f.manager); // Bootstrap actor-only no-invitation/empty unfinished list.
    await _flush(tester);
    expect(f.transport.held, hasLength(2));
    f.transport.complete(1, _state('active'));
    await _flush(tester);
    expect(f.logic.phase.value, MeetingDetailPhase.active);
    f.transport.events = [
      {
        'id': 1,
        'event': 'OnHangUp',
        'data': {
          'roomID': 'synthetic-room-A',
          'state': 'active',
          'userID': 'someone-left',
          'meetingInfo': _info(),
        }
      }
    ];
    await _pull(tester, f.manager);
    await _flush(tester);
    expect(f.transport.held, hasLength(3));
    f.transport.complete(2, _state('active'));
    await _flush(tester);
    expect(f.logic.phase.value, MeetingDetailPhase.active);
    expect(_enter(tester).enabled, isTrue);
    f.transport.events = [
      {
        'id': 2,
        'event': 'OnHangUp',
        'data': {
          'roomID': 'synthetic-room-A',
          'state': 'ended',
          'meetingInfo': _info(),
        }
      }
    ];
    await _pull(tester, f.manager);
    await _flush(tester);
    f.transport.complete(3, _state('ended', end: 120));
    await _flush(tester);
    expect(f.logic.phase.value, MeetingDetailPhase.ended);
    expect(f.logic.meetingInfo.endTime,
        120); // Never use the hint's old planned time.
  });

  testWidgets(
      'closed page ignores held response and never starts queued hint load',
      (tester) async {
    final f = await _mount(tester);
    f.manager.notifyMeetingRoomChange('synthetic-room-A');
    await _flush(tester);
    Get.offAllNamed('/away');
    await tester.pumpAndSettle();
    final previous = f.logic.phase.value;
    f.transport.complete(0, _state('ended'));
    await _flush(tester);
    expect(f.logic.phase.value, previous);
    expect(f.transport.held, hasLength(1));
    expect(tester.takeException(), isNull);
  });

  testWidgets('old pair response cannot write into new session or join',
      (tester) async {
    final f = await _mount(tester);
    f.manager.invalidateCallTerminations();
    f.user = 'synthetic-other';
    f.im = 'fake-im-B';
    f.chat = 'fake-chat-B';
    await _flush(tester);
    f.transport.complete(0, _state('active'));
    await _flush(tester);
    expect(f.logic.phase.value, MeetingDetailPhase.failed);
    expect(_enter(tester).enabled, isFalse);
    await f.logic.enterMeeting();
    expect(f.joins, 0);
    expect(f.transport.held, hasLength(1));
    final headers = f.transport.requests.first.headers;
    expect(headers['token'], 'fake-im-A');
    expect(headers['chatToken'], 'fake-chat-A');
  });

  testWidgets('Enter rechecks authoritative state and prevents stale join',
      (tester) async {
    final f = await _mount(tester);
    f.transport.complete(0, _state('active'));
    await _flush(tester);
    await _readyActive(tester, f);
    print(
        'detail phase beforeEnter phase=${f.logic.phase.value} loading=${f.logic.loading.value} entering=${f.logic.entering.value} canEnter=${f.logic.canEnter} error=${f.logic.errorType.value} requests=${f.transport.held.length}');
    final attempt = f.logic.enterMeeting();
    print(
        'detail phase startedEnter phase=${f.logic.phase.value} loading=${f.logic.loading.value} entering=${f.logic.entering.value} requests=${f.transport.held.length}');
    await _flush(tester);
    expect(f.transport.held, hasLength(2));
    f.transport.complete(1, _state('ended'));
    await _flush(tester);
    await attempt;
    expect(f.joins, 0);
    expect(f.logic.phase.value, MeetingDetailPhase.ended);
  });

  testWidgets(
      'foreground observer refreshes and preserves confirmed terminal state',
      (tester) async {
    final f = await _mount(tester);
    f.transport.complete(0, _state('active'));
    await _flush(tester);
    await _readyActive(tester, f);
    print(
        'detail phase beforeResume phase=${f.logic.phase.value} loading=${f.logic.loading.value} canEnter=${f.logic.canEnter} lifecycle=${tester.binding.lifecycleState} requests=${f.transport.held.length}');
    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.paused);
    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.resumed);
    await _flush(tester);
    expect(f.transport.held, hasLength(2));
    f.transport.complete(1, _state('ended', end: 120));
    await _flush(tester);
    f.manager.notifyMeetingRoomChange('synthetic-room-A');
    await _flush(tester);
    f.transport.complete(2, _state('active'));
    await _flush(tester);
    expect(f.logic.phase.value, MeetingDetailPhase.ended);
    expect(f.logic.meetingInfo.endTime, 120);
    expect(_enter(tester).enabled, isFalse);
    expect(f.logic.errorType.value, 'NonMonotonicMeetingState');
  });

  testWidgets(
      'bootstrap with no unfinished meeting queries current detail before Ended',
      (tester) async {
    final f = await _mount(tester);
    f.transport.complete(0, _state('active'));
    await _flush(tester);
    await _pull(tester, f.manager);
    await _flush(tester);
    expect(f.transport.held, hasLength(2));
    expect(f.logic.phase.value, MeetingDetailPhase.loading);
    expect(f.logic.phase.value, isNot(MeetingDetailPhase.ended));
    f.transport.complete(1, _state('ended', end: 120));
    await _flush(tester);
    expect(f.logic.phase.value, MeetingDetailPhase.ended);
    expect(f.logic.meetingInfo.endTime, 120);
    expect(_enter(tester).enabled, isFalse);
  });

  testWidgets(
      'actual resyncRequired refreshes missed terminal state only by room query',
      (tester) async {
    final f = await _mount(tester);
    f.transport.complete(0, _state('active'));
    await _flush(tester);
    await _pull(tester, f.manager);
    await _flush(tester);
    f.transport.complete(1, _state('active'));
    await _flush(tester);
    f.transport.resyncRequired = true;
    await _pull(tester, f.manager);
    await _flush(tester);
    expect(f.transport.held, hasLength(3));
    expect(f.logic.phase.value, MeetingDetailPhase.loading);
    f.transport.complete(2, _state('ended', end: 119));
    await _flush(tester);
    expect(f.logic.phase.value, MeetingDetailPhase.ended);
    expect(f.logic.meetingInfo.endTime, 119);
    expect(_enter(tester).enabled, isFalse);
    expect(f.joins, 0);
    expect(tester.takeException(), isNull);
  });
  test('envelope mismatch and missing status remain explicit failures', () {
    expect(
        () => MeetingRoomSnapshot.fromEnvelope('wrong-room', _state('ended')),
        throwsFormatException);
    expect(
        () => MeetingRoomSnapshot.fromEnvelope(
            'synthetic-room-A', {'meetingInfo': _info()}),
        throwsFormatException);
    expect(
        () => MeetingRoomSnapshot.fromEnvelope('synthetic-room-A', {
              ..._state('active'),
              'ended': true,
            }),
        throwsFormatException);
  });
}
