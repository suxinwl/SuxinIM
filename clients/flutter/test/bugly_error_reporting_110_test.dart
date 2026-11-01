// Tests the actual vendored FlutterBugly public API, not a duplicate zone algorithm.
// Real Flutter foundation and mock native MethodChannel; no app or device import.
import 'dart:async';
import 'dart:convert';

import 'package:flutter/foundation.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:flutter_bugly/flutter_bugly.dart';

const channel = MethodChannel('crazecoder/flutter_bugly');

class _ObservedError {
  _ObservedError(this.error, this.stack);
  final Object error;
  final StackTrace stack;
}

Future<List<_ObservedError>> _uncaught(
  Object error,
  StackTrace stack, {
  FlutterExceptionHandler? observer,
  String? filter,
}) async {
  final received = <_ObservedError>[];
  final first = Completer<void>();
  Zone.current
      .fork(
        specification: ZoneSpecification(
          handleUncaughtError:
              (self, parent, origin, actualError, actualStack) {
                received.add(_ObservedError(actualError, actualStack));
                if (!first.isCompleted) first.complete();
              },
        ),
      )
      .run(() {
        FlutterBugly.postCatchedException(
          () {
            // Intentionally uncaught, but owned by the test's actual parent zone.
            Future<void>.error(error, stack);
          },
          onException: observer,
          filterRegExp: filter,
          debugUpload: true,
        );
      });
  await first.future.timeout(const Duration(seconds: 2));
  await Future<void>.delayed(Duration.zero);
  await Future<void>.delayed(Duration.zero);
  return received;
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  late FlutterExceptionHandler? originalOnError;
  late FlutterExceptionHandler originalPresentError;
  late DebugPrintCallback originalDebugPrint;
  final localFramework = <FlutterErrorDetails>[];
  final reportingFailures = <FlutterErrorDetails>[];
  final calls = <MethodCall>[];

  setUp(() {
    originalOnError = FlutterError.onError;
    originalPresentError = FlutterError.presentError;
    originalDebugPrint = debugPrint;
    FlutterError.resetErrorCount();
    localFramework.clear();
    reportingFailures.clear();
    calls.clear();
    FlutterError.onError = localFramework.add;
    FlutterError.presentError = reportingFailures.add;
    FlutterBugly.debugPlatformOverride = 'android';
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(channel, (call) async {
          calls.add(call);
          if (call.method == 'initBugly') {
            return jsonEncode({'isSuccess': true, 'appId': 'synthetic-id'});
          }
          return null;
        });
  });

  tearDown(() {
    FlutterBugly.dispose();
    FlutterBugly.debugPlatformOverride = null;
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(channel, null);
    FlutterError.onError = originalOnError;
    FlutterError.presentError = originalPresentError;
    debugPrint = originalDebugPrint;
  });

  test(
    'no ID keeps original framework details and makes no native request',
    () async {
      final error = StateError('synthetic-framework');
      final stack = StackTrace.fromString('synthetic-framework-stack');
      final details = FlutterErrorDetails(exception: error, stack: stack);
      FlutterBugly.postCatchedException(
        () => FlutterError.reportError(details),
        debugUpload: true,
      );
      await FlutterBugly.uploadException(
        message: 'direct-no-id',
        detail: 'stack',
      );
      await Future<void>.delayed(Duration.zero);
      expect(localFramework, [same(details)]);
      expect(localFramework.single.exception, same(error));
      expect(localFramework.single.stack, same(stack));
      expect(calls, isEmpty);
      expect(reportingFailures, isEmpty);
    },
  );

  test(
    'uncaught async error and stack reach captured parent exactly once',
    () async {
      final error = StateError('synthetic-async');
      final stack = StackTrace.fromString('synthetic-async-stack');
      final received = await _uncaught(error, stack);
      expect(received, hasLength(1));
      expect(received.single.error, same(error));
      expect(received.single.stack, same(stack));
      expect(calls, isEmpty);
      expect(reportingFailures, isEmpty);
    },
  );

  test(
    'actual default framework logger retains complete error and stack',
    () async {
      final lines = <String>[];
      debugPrint = (message, {wrapWidth}) {
        if (message != null) lines.add(message);
      };
      FlutterError.onError = (details) =>
          FlutterError.dumpErrorToConsole(details, forceReport: true);
      FlutterBugly.postCatchedException(
        () => FlutterError.reportError(
          FlutterErrorDetails(
            exception: StateError('synthetic-visible-framework-error'),
            stack: StackTrace.fromString(
              '#0 syntheticVisibleFrame (file:///synthetic/proposal.dart:7:1)',
            ),
          ),
        ),
        debugUpload: true,
      );
      await Future<void>.delayed(Duration.zero);
      expect(lines.join('\n'), contains('synthetic-visible-framework-error'));
      expect(lines.join('\n'), contains('syntheticVisibleFrame'));
      expect(calls, isEmpty);
    },
  );

  test(
    'missing ID and failed native initialization never enable uploads',
    () async {
      expect((await FlutterBugly.init(androidAppId: '')).isSuccess, false);
      expect(calls, isEmpty);
      TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
          .setMockMethodCallHandler(channel, (call) async {
            calls.add(call);
            return jsonEncode({'isSuccess': false});
          });
      expect(
        (await FlutterBugly.init(androidAppId: 'synthetic-id')).isSuccess,
        false,
      );
      await _uncaught(StateError('after-failed-init'), StackTrace.current);
      expect(calls.map((e) => e.method), ['initBugly']);
      TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
          .setMockMethodCallHandler(channel, (call) async {
            calls.add(call);
            throw PlatformException(code: 'SYNTHETIC_INIT_FAILURE');
          });
      await expectLater(
        FlutterBugly.init(androidAppId: 'synthetic-id'),
        throwsA(
          isA<PlatformException>().having(
            (e) => e.code,
            'code',
            'SYNTHETIC_INIT_FAILURE',
          ),
        ),
      );
      await _uncaught(StateError('after-rejected-init'), StackTrace.current);
      expect(calls.map((e) => e.method), ['initBugly', 'initBugly']);
    },
  );

  test('pending init cannot upload until successful completion', () async {
    final pending = Completer<String>();
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(channel, (call) async {
          calls.add(call);
          if (call.method == 'initBugly') return pending.future;
          return null;
        });
    final initialization = FlutterBugly.init(androidAppId: 'synthetic-id');
    await _uncaught(StateError('before-init-success'), StackTrace.current);
    expect(calls.map((e) => e.method), ['initBugly']);
    pending.complete(jsonEncode({'isSuccess': true, 'appId': 'synthetic-id'}));
    expect((await initialization).isSuccess, true);
    await _uncaught(StateError('after-init-success'), StackTrace.current);
    expect(calls.map((e) => e.method), ['initBugly', 'postCatchedException']);
  });

  test(
    'upload failure is observed once without replacing original async error',
    () async {
      await FlutterBugly.init(androidAppId: 'synthetic-id');
      TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
          .setMockMethodCallHandler(channel, (call) async {
            calls.add(call);
            throw PlatformException(code: 'SYNTHETIC_UPLOAD_FAILURE');
          });
      final original = StateError('original-must-survive');
      final stack = StackTrace.fromString('original-stack-must-survive');
      final received = await _uncaught(original, stack);
      expect(received, hasLength(1));
      expect(received.single.error, same(original));
      expect(received.single.stack, same(stack));
      expect(
        calls.where((e) => e.method == 'postCatchedException'),
        hasLength(1),
      );
      expect(reportingFailures, hasLength(1));
      expect(
        reportingFailures.single.exception,
        isA<PlatformException>().having(
          (e) => e.code,
          'code',
          'SYNTHETIC_UPLOAD_FAILURE',
        ),
      );
    },
  );

  test(
    'observer failure remains separate and original still forwards once',
    () async {
      final original = StateError('original-error');
      final secondary = StateError('observer-error');
      final received = await _uncaught(
        original,
        StackTrace.current,
        observer: (_) => throw secondary,
      );
      expect(received, hasLength(1));
      expect(received.single.error, same(original));
      expect(reportingFailures.single.exception, same(secondary));
      expect(calls, isEmpty);
    },
  );

  test(
    'async observer failure is observed without reentering reporting',
    () async {
      final original = StateError('original-async-observer');
      final secondary = StateError('async-observer-error');
      final received = await _uncaught(
        original,
        StackTrace.current,
        observer: (_) async {
          await Future<void>.value();
          throw secondary;
        },
      );
      expect(received, hasLength(1));
      expect(received.single.error, same(original));
      expect(reportingFailures.single.exception, same(secondary));
      expect(calls, isEmpty);
    },
  );

  test(
    'upload filters never suppress the original local framework diagnostics',
    () async {
      await FlutterBugly.init(androidAppId: 'synthetic-id');
      final details = FlutterErrorDetails(
        exception: StateError('synthetic-filter-match'),
        stack: StackTrace.current,
      );
      FlutterBugly.postCatchedException(
        () => FlutterError.reportError(details),
        filterRegExp: 'synthetic-filter-match',
        debugUpload: true,
      );
      await Future<void>.delayed(Duration.zero);
      expect(localFramework.single, same(details));
      expect(calls.map((e) => e.method), ['initBugly']);
    },
  );

  test(
    'late older initialization cannot enable a newer failed generation',
    () async {
      final oldResult = Completer<String>();
      var initializations = 0;
      TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
          .setMockMethodCallHandler(channel, (call) async {
            calls.add(call);
            if (call.method == 'initBugly') {
              initializations++;
              if (initializations == 1) return oldResult.future;
              return jsonEncode({'isSuccess': false});
            }
            return null;
          });
      final older = FlutterBugly.init(androidAppId: 'synthetic-old');
      await Future<void>.delayed(Duration.zero);
      expect(
        (await FlutterBugly.init(androidAppId: 'synthetic-new')).isSuccess,
        false,
      );
      oldResult.complete(jsonEncode({'isSuccess': true}));
      await older;
      await _uncaught(StateError('must-not-upload'), StackTrace.current);
      expect(calls.map((e) => e.method), ['initBugly', 'initBugly']);
    },
  );

  test('dispose respects another handler owner and disables uploads', () async {
    await FlutterBugly.init(androidAppId: 'synthetic-id');
    FlutterBugly.postCatchedException(() {}, debugUpload: true);
    void anotherOwner(FlutterErrorDetails details) {}
    FlutterError.onError = anotherOwner;
    FlutterBugly.dispose();
    expect(FlutterError.onError, same(anotherOwner));
    await FlutterBugly.uploadException(
      message: 'after-dispose',
      detail: 'stack',
    );
    expect(calls.map((e) => e.method), ['initBugly']);
  });
}
