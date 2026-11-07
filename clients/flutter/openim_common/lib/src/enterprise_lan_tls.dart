import 'dart:async';
import 'dart:io';
import 'dart:typed_data';
import 'package:flutter/services.dart';
import 'package:flutter/widgets.dart';

/// Binds Dart's IP-name-constraint compatibility exception to a certificate
/// already verified by the Android PKIX/TLS stack. No unvalidated certificate
/// and no public endpoint can enter this binding.
class EnterpriseLanPeerTrust {
  static const host = '192.168.10.11', port = 9885;
  static const rootSHA256 =
      '836c3d6c28d5078cc8da6ebeff8e5f9a7fb7f140554e6a7834011d72299c3d69';
  static const ttl = Duration(minutes: 15);
  final Future<Map<Object?, Object?>> Function() verify;
  final DateTime Function() wallClock;
  final Duration Function() monotonicClock;
  EnterpriseLanPeerTrust(
      {required this.verify,
      DateTime Function()? wallClock,
      Duration Function()? monotonicClock})
      : wallClock = wallClock ?? DateTime.now,
        monotonicClock = monotonicClock ?? _elapsed;
  static final _watch = Stopwatch()..start();
  static Duration _elapsed() => _watch.elapsed;
  Uint8List? _der;
  int _notBefore = 0, _notAfter = 0, _validatedAt = 0, _generation = 0;
  int _wallHighWater = 0;
  Duration _monotonicAt = Duration.zero;
  Future<void>? _flight;
  static bool isEndpoint(Uri uri) =>
      uri.scheme == 'https' &&
      uri.host == host &&
      uri.port == port &&
      uri.userInfo.isEmpty;
  bool _observeWall(int now) {
    if (now < _wallHighWater) return false;
    _wallHighWater = now;
    return true;
  }

  bool get isFresh => _isFreshAt(wallClock());
  bool _isFreshAt(DateTime instant) {
    final now = instant.millisecondsSinceEpoch,
        elapsed = monotonicClock() - _monotonicAt;
    return _der != null &&
        _observeWall(now) &&
        now >= _validatedAt &&
        now >= _notBefore &&
        now < _notAfter &&
        now - _validatedAt < ttl.inMilliseconds &&
        elapsed >= Duration.zero &&
        elapsed < ttl;
  }

  void invalidate() {
    _generation++;
    _der = null;
  }

  Future<void> ensure({bool force = false}) {
    if (!force && isFresh) return Future.value();
    return _flight ??= _refresh().whenComplete(() {
      _flight = null;
    });
  }

  Future<void> _refresh() async {
    final generation = _generation;
    try {
      if (!_observeWall(wallClock().millisecondsSinceEpoch)) {
        throw StateError('Enterprise TLS clock moved backwards');
      }
      final data = await verify().timeout(const Duration(seconds: 15));
      final der = data['leafDER'],
          notBefore = data['notBefore'],
          notAfter = data['notAfter'],
          validatedAt = data['validatedAt'];
      if (data['host'] != host ||
          data['port'] is! int ||
          data['port'] != port ||
          data['rootSHA256'] != rootSHA256 ||
          der is! Uint8List ||
          der.isEmpty ||
          der.length > 65536 ||
          notBefore is! int ||
          notAfter is! int ||
          validatedAt is! int ||
          validatedAt < notBefore ||
          validatedAt >= notAfter ||
          generation != _generation)
        throw StateError('Invalid native enterprise TLS verification');
      final now = wallClock().millisecondsSinceEpoch;
      if (!_observeWall(now) ||
          now < validatedAt ||
          now < notBefore ||
          now >= notAfter ||
          now - validatedAt >= ttl.inMilliseconds)
        throw StateError('Native enterprise TLS verification expired');
      _der = Uint8List.fromList(der);
      _notBefore = notBefore;
      _notAfter = notAfter;
      _validatedAt = validatedAt;
      _monotonicAt = monotonicClock();
    } catch (_) {
      invalidate();
      rethrow;
    }
  }

  bool accepts(Uint8List der, String requestedHost, int requestedPort,
      DateTime validFrom, DateTime validUntil) {
    final now = wallClock();
    if (requestedHost != host ||
        requestedPort != port ||
        !_isFreshAt(now) ||
        now.isBefore(validFrom) ||
        !now.isBefore(validUntil) ||
        der.length != _der!.length) return false;
    var difference = 0;
    for (var i = 0; i < der.length; i++) difference |= der[i] ^ _der![i];
    return difference == 0;
  }
}

class EnterpriseLanTls with WidgetsBindingObserver {
  EnterpriseLanTls._();
  static final instance = EnterpriseLanTls._();
  static const _channel = MethodChannel('puty/enterprise_lan_tls');
  final trust = EnterpriseLanPeerTrust(
      verify: () async =>
          await _channel.invokeMapMethod<Object?, Object?>('preflightLan') ??
          <Object?, Object?>{});
  String Function()? _profile;
  Timer? _timer;
  bool _installed = false;
  bool _profileReady = false;
  void profileReady() {_profileReady = true;}
  void install(String Function() profile) {
    if (!Platform.isAndroid || _installed) return;
    _installed = true;
    _profile = profile;
    HttpOverrides.global = _EnterpriseLanHttpOverrides(trust);
    WidgetsBinding.instance.addObserver(this);
    _timer ??= Timer.periodic(
        const Duration(minutes: 5), (_) => _refreshCurrentProfile());
  }

  void _refreshCurrentProfile() {
    if (!_profileReady) return;
    try {
      if (_profile?.call() == 'https://192.168.10.11:9885')
        unawaited(trust.ensure(force: true).catchError((Object _) {}));
    } catch (_) {
      trust.invalidate();
    }
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    if (state == AppLifecycleState.resumed) _refreshCurrentProfile();
  }
}

class _EnterpriseLanHttpOverrides extends HttpOverrides {
  final EnterpriseLanPeerTrust trust;
  _EnterpriseLanHttpOverrides(this.trust);
  @override
  HttpClient createHttpClient(SecurityContext? context) =>
      EnterpriseLanHttpClient(super.createHttpClient(context), trust);
}

/// Every HTTP method, including WebSocket.connect's HTTPS upgrade, passes the
/// asynchronous preflight before a synchronous certificate decision is needed.
class EnterpriseLanHttpClient implements HttpClient {
  final HttpClient _inner;
  final EnterpriseLanPeerTrust trust;
  EnterpriseLanHttpClient(this._inner, this.trust) {
    _inner.badCertificateCallback = (cert, host, port) => trust.accepts(
        cert.der, host, port, cert.startValidity, cert.endValidity);
  }
  @override
  Future<HttpClientRequest> openUrl(String method, Uri url) async {
    final lan = EnterpriseLanPeerTrust.isEndpoint(url);
    if (url.host == EnterpriseLanPeerTrust.host && !lan)
      throw StateError('Enterprise endpoint requires its fixed HTTPS origin');
    if (lan) await trust.ensure();
    try {
      return await _inner.openUrl(method, url);
    } on HandshakeException {
      if (!lan) rethrow;
      // A same-root renewal may replace the leaf between preflight and request.
      // Obtain a fresh complete native verification, then retry the handshake
      // once, before the HTTP request/body has been handed to its caller.
      try {
        await trust.ensure(force: true);
        return await _inner.openUrl(method, url);
      } catch (_) {
        trust.invalidate();
        rethrow;
      }
    } catch (_) {
      if (lan) trust.invalidate();
      rethrow;
    }
  }

  @override
  Future<HttpClientRequest> open(
      String method, String host, int port, String path) {
    if (host == EnterpriseLanPeerTrust.host)
      throw StateError('Enterprise endpoint requires an HTTPS URI');
    return _inner.open(method, host, port, path);
  }

  @override
  Future<HttpClientRequest> getUrl(Uri url) => openUrl('GET', url);
  @override
  Future<HttpClientRequest> postUrl(Uri url) => openUrl('POST', url);
  @override
  Future<HttpClientRequest> putUrl(Uri url) => openUrl('PUT', url);
  @override
  Future<HttpClientRequest> patchUrl(Uri url) => openUrl('PATCH', url);
  @override
  Future<HttpClientRequest> deleteUrl(Uri url) => openUrl('DELETE', url);
  @override
  Future<HttpClientRequest> headUrl(Uri url) => openUrl('HEAD', url);
  @override
  Future<HttpClientRequest> get(String host, int port, String path) =>
      open('GET', host, port, path);
  @override
  Future<HttpClientRequest> post(String host, int port, String path) =>
      open('POST', host, port, path);
  @override
  Future<HttpClientRequest> put(String host, int port, String path) =>
      open('PUT', host, port, path);
  @override
  Future<HttpClientRequest> patch(String host, int port, String path) =>
      open('PATCH', host, port, path);
  @override
  Future<HttpClientRequest> delete(String host, int port, String path) =>
      open('DELETE', host, port, path);
  @override
  Future<HttpClientRequest> head(String host, int port, String path) =>
      open('HEAD', host, port, path);
  @override
  bool get autoUncompress => _inner.autoUncompress;
  @override
  set autoUncompress(bool value) {
    _inner.autoUncompress = value;
  }

  @override
  Duration get idleTimeout => _inner.idleTimeout;
  @override
  set idleTimeout(Duration value) {
    _inner.idleTimeout = value;
  }

  @override
  Duration? get connectionTimeout => _inner.connectionTimeout;
  @override
  set connectionTimeout(Duration? value) {
    _inner.connectionTimeout = value;
  }

  @override
  int? get maxConnectionsPerHost => _inner.maxConnectionsPerHost;
  @override
  set maxConnectionsPerHost(int? value) {
    _inner.maxConnectionsPerHost = value;
  }

  @override
  String? get userAgent => _inner.userAgent;
  @override
  set userAgent(String? value) {
    _inner.userAgent = value;
  }

  @override
  set authenticate(Future<bool> Function(Uri, String, String?)? value) {
    _inner.authenticate = value;
  }

  @override
  set authenticateProxy(
      Future<bool> Function(String, int, String, String?)? value) {
    _inner.authenticateProxy = value;
  }

  @override
  set findProxy(String Function(Uri)? value) {
    _inner.findProxy = value;
  }

  @override
  set connectionFactory(
      Future<ConnectionTask<Socket>> Function(Uri, String?, int?)? value) {
    _inner.connectionFactory = value;
  }

  @override
  set keyLog(Function(String)? value) {
    _inner.keyLog = value;
  }

  @override
  set badCertificateCallback(
      bool Function(X509Certificate, String, int)? value) {
    // The enterprise compatibility policy cannot be replaced by a caller's
    // permissive callback. Public endpoints always retain default rejection.
  }
  @override
  void addCredentials(
          Uri url, String realm, HttpClientCredentials credentials) =>
      _inner.addCredentials(url, realm, credentials);
  @override
  void addProxyCredentials(String host, int port, String realm,
          HttpClientCredentials credentials) =>
      _inner.addProxyCredentials(host, port, realm, credentials);
  @override
  void close({bool force = false}) => _inner.close(force: force);
}
