import 'dart:async';
import 'dart:io';
import 'dart:typed_data';
import 'package:flutter_test/flutter_test.dart';
import 'package:openim_common/src/enterprise_lan_tls.dart';

class Clock {
  DateTime wall = DateTime.utc(2026, 11, 3);
  Duration elapsed = Duration.zero;
  Map<Object?, Object?> peer([List<int> der = const [1, 2, 3]]) => {
        'host': '192.168.10.11',
        'port': 9885,
        'rootSHA256': EnterpriseLanPeerTrust.rootSHA256,
        'leafDER': Uint8List.fromList(der),
        'notBefore':
            wall.subtract(const Duration(days: 1)).millisecondsSinceEpoch,
        'notAfter': wall.add(const Duration(days: 1)).millisecondsSinceEpoch,
        'validatedAt': wall.millisecondsSinceEpoch
      };
  EnterpriseLanPeerTrust trust(
          Future<Map<Object?, Object?>> Function() verify) =>
      EnterpriseLanPeerTrust(
          verify: verify, wallClock: () => wall, monotonicClock: () => elapsed);
  bool accepts(EnterpriseLanPeerTrust trust,
          {List<int> der = const [1, 2, 3],
          String host = '192.168.10.11',
          int port = 9885,
          DateTime? from,
          DateTime? until}) =>
      trust.accepts(
          Uint8List.fromList(der),
          host,
          port,
          from ?? wall.subtract(const Duration(days: 1)),
          until ?? wall.add(const Duration(days: 1)));
}

class Request implements HttpClientRequest {
  @override
  dynamic noSuchMethod(Invocation invocation) => super.noSuchMethod(invocation);
}

class Client implements HttpClient {
  bool Function(X509Certificate, String, int)? callback;
  int calls = 0;
  final methods = <String>[];
  Future<HttpClientRequest> Function()? reply;
  @override
  set badCertificateCallback(
      bool Function(X509Certificate, String, int)? value) {
    callback = value;
  }

  @override
  Future<HttpClientRequest> openUrl(String method, Uri url) async {
    calls++;
    methods.add(method);
    return reply == null ? Request() : await reply!();
  }

  @override
  dynamic noSuchMethod(Invocation invocation) => super.noSuchMethod(invocation);
}

void main() {
  test('exact native-validated DER is restricted to fixed LAN host and port',
      () async {
    final clock = Clock();
    final binding = clock.trust(() async => clock.peer());
    await binding.ensure();
    expect(clock.accepts(binding), isTrue);
    expect(clock.accepts(binding, der: [1, 2, 4]), isFalse);
    expect(clock.accepts(binding, der: [1]), isFalse);
    expect(clock.accepts(binding, host: 'im.szputy.com'), isFalse);
    expect(clock.accepts(binding, host: '192.168.10.12'), isFalse);
    expect(clock.accepts(binding, port: 9880), isFalse);
    expect(
        clock.accepts(binding,
            from: clock.wall.add(const Duration(seconds: 1))),
        isFalse);
    expect(clock.accepts(binding, until: clock.wall), isFalse);
  });
  test('wall clock rollback or monotonic TTL expiry cannot extend trust',
      () async {
    final clock = Clock();
    final binding = clock.trust(() async => clock.peer());
    await binding.ensure();
    final original = clock.wall;
    clock.wall = original.subtract(const Duration(milliseconds: 1));
    expect(clock.accepts(binding), isFalse);
    clock.wall = original;
    clock.elapsed = const Duration(minutes: 15);
    expect(clock.accepts(binding), isFalse);
    clock.elapsed = const Duration(microseconds: -1);
    expect(clock.accepts(binding), isFalse);
  });
  test('wall TTL and native chain expiry both fail closed', () async {
    final clock = Clock();
    final binding = clock.trust(() async => clock.peer());
    await binding.ensure();
    clock.wall = clock.wall.add(const Duration(minutes: 15));
    expect(binding.isFresh, isFalse);
    final expiring = clock.trust(() async => clock.peer()
      ..['notAfter'] =
          clock.wall.add(const Duration(seconds: 1)).millisecondsSinceEpoch);
    await expiring.ensure();
    clock.wall = clock.wall.add(const Duration(seconds: 1));
    expect(expiring.isFresh, isFalse);
  });
  test('untrusted native root, host, port, malformed dates or DER are rejected',
      () async {
    final clock = Clock();
    for (final edit in <void Function(Map<Object?, Object?>)>[
      (m) => m['rootSHA256'] = 'wrong',
      (m) => m['host'] = 'im.szputy.com',
      (m) => m['port'] = 9880,
      (m) => m['port'] = 9885.0,
      (m) => m['leafDER'] = Uint8List(0),
      (m) => m['leafDER'] = [1, 2, 3],
      (m) => m['validatedAt'] =
          clock.wall.add(const Duration(seconds: 1)).millisecondsSinceEpoch,
      (m) => m['notBefore'] =
          clock.wall.add(const Duration(days: 2)).millisecondsSinceEpoch,
      (m) => m['notAfter'] = clock.wall.millisecondsSinceEpoch
    ]) {
      final binding = clock.trust(() async {
        final data = clock.peer();
        edit(data);
        return data;
      });
      await expectLater(binding.ensure(), throwsStateError);
      expect(binding.isFresh, isFalse);
    }
  });
  test('concurrent requests perform a single native preflight', () async {
    final clock = Clock(), ready = Completer<Map<Object?, Object?>>();
    var calls = 0;
    final binding = clock.trust(() {
      calls++;
      return ready.future;
    });
    final first = binding.ensure(), second = binding.ensure();
    ready.complete(clock.peer());
    await Future.wait([first, second]);
    expect(calls, 1);
    expect(clock.accepts(binding), isTrue);
  });
  test('invalidation rejects a late native completion', () async {
    final clock = Clock(), ready = Completer<Map<Object?, Object?>>();
    final binding = clock.trust(() => ready.future);
    final pending = binding.ensure();
    binding.invalidate();
    ready.complete(clock.peer());
    await expectLater(pending, throwsStateError);
    expect(binding.isFresh, isFalse);
  });
  test('native network failure clears the previous positive binding', () async {
    final clock = Clock();
    var failing = false;
    final binding = clock.trust(() async {
      if (failing) throw const SocketException('synthetic offline');
      return clock.peer();
    });
    await binding.ensure();
    failing = true;
    await expectLater(
        binding.ensure(force: true), throwsA(isA<SocketException>()));
    expect(clock.accepts(binding), isFalse);
  });
  test(
      'same-root renewal accepts only the newly verified leaf without rebuilding',
      () async {
    final clock = Clock();
    var der = [1, 2, 3];
    final binding = clock.trust(() async => clock.peer(der));
    await binding.ensure();
    der = [4, 5, 6];
    await binding.ensure(force: true);
    expect(clock.accepts(binding), isFalse);
    expect(clock.accepts(binding, der: der), isTrue);
  });
  test('all URI HTTP methods preflight before delegating', () async {
    final clock = Clock();
    var verified = 0;
    final binding = clock.trust(() async {
          verified++;
          return clock.peer();
        }),
        inner = Client();
    final client = EnterpriseLanHttpClient(inner, binding),
        url = Uri.parse('https://192.168.10.11:9885/sso/captcha');
    await client.getUrl(url);
    await client.postUrl(url);
    await client.putUrl(url);
    await client.patchUrl(url);
    await client.deleteUrl(url);
    await client.headUrl(url);
    expect(inner.methods, ['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'HEAD']);
    expect(verified, 1);
  });
  test('WAN does no LAN preflight and never accepts enterprise callback',
      () async {
    final clock = Clock();
    var verified = 0;
    final binding = clock.trust(() async {
          verified++;
          throw StateError('must not connect LAN');
        }),
        inner = Client();
    final client = EnterpriseLanHttpClient(inner, binding);
    await client.getUrl(Uri.parse('https://im.szputy.com:9885/sso/captcha'));
    expect(verified, 0);
    expect(clock.accepts(binding, host: 'im.szputy.com'), isFalse);
  });
  test('wrong LAN origin or credentials is rejected before transport',
      () async {
    final clock = Clock();
    var verified = 0;
    final binding = clock.trust(() async {
          verified++;
          return clock.peer();
        }),
        inner = Client();
    final client = EnterpriseLanHttpClient(inner, binding);
    for (final url in [
      'http://192.168.10.11:9885/sso/login',
      'https://192.168.10.11:9880/sso/login',
      'https://u:p@192.168.10.11:9885/sso/login'
    ]) await expectLater(client.getUrl(Uri.parse(url)), throwsStateError);
    expect(() => client.get('192.168.10.11', 9885, '/'), throwsStateError);
    expect(inner.calls, 0);
    expect(verified, 0);
  });
  test(
      'certificate rotation refreshes and retries handshake once before request body',
      () async {
    final clock = Clock();
    var verified = 0;
    final binding = clock.trust(
            () async => clock.peer(++verified == 1 ? [1, 2, 3] : [4, 5, 6])),
        inner = Client();
    inner.reply = () async {
      if (inner.calls == 1)
        throw const HandshakeException('synthetic renewed peer');
      return Request();
    };
    final client = EnterpriseLanHttpClient(inner, binding);
    await client.postUrl(Uri.parse('https://192.168.10.11:9885/sso/login'));
    expect(verified, 2);
    expect(inner.calls, 2);
    expect(clock.accepts(binding, der: [4, 5, 6]), isTrue);
  });
  test('handshake retry is bounded and cannot replace strict callback',
      () async {
    final clock = Clock();
    var verified = 0;
    final binding = clock.trust(() async {
          verified++;
          return clock.peer();
        }),
        inner = Client();
    inner.reply =
        () async => throw const HandshakeException('synthetic mismatch');
    final client = EnterpriseLanHttpClient(inner, binding),
        callback = inner.callback;
    client.badCertificateCallback = (_, __, ___) => true;
    expect(identical(callback, inner.callback), isTrue);
    await expectLater(client.getUrl(Uri.parse('https://192.168.10.11:9885/')),
        throwsA(isA<HandshakeException>()));
    expect(verified, 2);
    expect(inner.calls, 2);
    expect(binding.isFresh, isFalse);
  });
  test('transport failure invalidates the native binding before a new request',
      () async {
    final clock = Clock();
    final binding = clock.trust(() async => clock.peer()), inner = Client();
    inner.reply = () async => throw const SocketException('synthetic offline');
    final client = EnterpriseLanHttpClient(inner, binding);
    await expectLater(client.getUrl(Uri.parse('https://192.168.10.11:9885/')),
        throwsA(isA<SocketException>()));
    expect(binding.isFresh, isFalse);
  });
  test('clock rollback cannot be reset by a fresh native preflight', () async {
    final clock = Clock();
    var calls = 0;
    final binding = clock.trust(() async {
      calls++;
      return clock.peer();
    });
    await binding.ensure();
    clock.wall = clock.wall.subtract(const Duration(seconds: 1));
    await expectLater(binding.ensure(force: true), throwsStateError);
    expect(calls, 1);
    expect(binding.isFresh, isFalse);
  });
}
