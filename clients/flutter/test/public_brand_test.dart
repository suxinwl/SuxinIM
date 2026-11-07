import 'dart:convert';
import 'dart:typed_data';
import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:openim_common/openim_common.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'native_settings_mock.dart';

class BrandAdapter implements HttpClientAdapter {
  final Future<ResponseBody> Function(RequestOptions) handle;
  BrandAdapter(this.handle);
  @override
  Future<ResponseBody> fetch(RequestOptions options, Stream<Uint8List>? stream, Future<void>? cancel) => handle(options);
  @override
  void close({bool force = false}) {}
}
void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  setUp(() async { installNativeSettingsMock(); SharedPreferences.setMockInitialValues({}); await DataSp.init(); });
  test('runtime name uses generated fallback while retaining technical package identity', () {
    expect(AppBrand.displayName, 'PUTY内部通讯'); expect(AppBrand.technicalName, 'PUTY_IM');
    expect(AppBrand.nameFrom({'appName': 'Configured Brand'}), 'Configured Brand');
    expect(AppBrand.nameFrom({'appName': ' '}), AppBrand.displayName);
  });
  test('anonymous pre-login brand consumes only display fields without any session credential', () async {
    RequestOptions? captured;
    dio = Dio()..httpClientAdapter = BrandAdapter((options) async {
      captured = options;
      return ResponseBody.fromString(jsonEncode({'errCode': 0, 'data': {'config': {'appName': 'Runtime Brand', 'webTitle': 'Portal', 'administrativeSetting': 'ignored'}}}), 200, headers: {Headers.contentTypeHeader: ['application/json']});
    });
    expect(await Apis.getPublicBrand(), {'appName': 'Runtime Brand', 'webTitle': 'Portal'});
    expect(captured!.uri.toString(), 'https://192.168.10.11:9885/chat/client_config/get');
    expect(captured!.headers.containsKey('token'), false); expect(captured!.headers.containsKey('chatToken'), false);
    expect(captured!.followRedirects, false); expect(captured!.data, isEmpty);
  });
  test('invalid remote brand values are ignored', () async {
    dio = Dio()..httpClientAdapter = BrandAdapter((_) async => ResponseBody.fromString(jsonEncode({'errCode': 0, 'data': {'config': {'appName': 'Name\nBreak', 'webTitle': 12}}}), 200, headers: {Headers.contentTypeHeader: ['application/json']}));
    expect(await Apis.getPublicBrand(), isEmpty);
  });
}
