import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';
import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:openim_common/openim_common.dart';
class InstallerAdapter implements HttpClientAdapter {
 final requests=<RequestOptions>[];
 @override Future<ResponseBody> fetch(RequestOptions options,Stream<Uint8List>? stream,Future<void>? cancel) async {
  requests.add(options);final bytes=utf8.encode('synthetic signed-package transport fixture');
  return ResponseBody.fromBytes(bytes,200,headers:{Headers.contentLengthHeader:['${bytes.length}']});
 }
 @override void close({bool force=false}){}
}
void main(){
 test('public installer download works without session or attachment authorization',() async {
  final temporary=await Directory.systemTemp.createTemp('puty-installer-transport-');addTearDown(()=>temporary.delete(recursive:true));
  final adapter=InstallerAdapter(),client=Dio()..httpClientAdapter=adapter;
  final target='${temporary.path}/candidate.apk';
  await HttpUtil.downloadPublicInstaller('https://192.168.10.11:9885/downloads/IM-3.8.15-android-arm64.apk',cachePath:target,client:client);
  expect(await File(target).readAsString(),'synthetic signed-package transport fixture');
  final request=adapter.requests.single;expect(request.uri.path,'/downloads/IM-3.8.15-android-arm64.apk');
  expect(request.followRedirects,isFalse);for(final header in ['token','chatToken','Authorization','Cookie'])expect(request.headers.keys.map((k)=>k.toLowerCase()),isNot(contains(header.toLowerCase())));
 });
 test('public installer transport cannot bypass private attachment paths or TLS',() async {
  final adapter=InstallerAdapter(),client=Dio()..httpClientAdapter=adapter;
  for(final url in ['http://192.168.10.11:9885/downloads/a.apk','https://evil.example/downloads/a.apk','https://user:secret@im.szputy.com:9885/downloads/a.apk','https://im.szputy.com:9885/api/object/a.apk','https://im.szputy.com:9885/attachments/grant/a.apk','https://hr.szputy.com:9880/downloads/a.apk','https://im.szputy.com:9885/downloads/nested/a.apk','https://im.szputy.com:9885/downloads/a.txt']) {
   await expectLater(HttpUtil.downloadPublicInstaller(url,cachePath:'unused',client:client),throwsStateError);
  }
  expect(adapter.requests,isEmpty);
 });
}
