import 'dart:async';
import 'dart:convert';
import 'dart:typed_data';
import 'package:app_im/app_im.dart';
import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';

class GrantAdapter implements HttpClientAdapter {
 Future<ResponseBody> Function(RequestOptions) handle;
 GrantAdapter(this.handle);
 @override Future<ResponseBody> fetch(RequestOptions o,Stream<Uint8List>? s,Future<void>? c)=>handle(o);
 @override void close({bool force=false}){}
}
ResponseBody grant(String marker,{String origin='https://im.szputy.com:9885',String? proof}) => ResponseBody.fromString(jsonEncode({'errCode':0,'data':{'url':'$origin/attachments/$marker/openim38/a','expiresAt':DateTime.now().millisecondsSinceEpoch/1000+600,'resourceProof':proof??List.filled(64,'a').join(),'resourceProofExpiresAt':DateTime.now().millisecondsSinceEpoch/1000+60}}),200,headers:{Headers.contentTypeHeader:[Headers.jsonContentType]});
EnterpriseManager configured(GrantAdapter adapter)=>EnterpriseManager(dio:Dio()..httpClientAdapter=adapter)..configure(baseUrl:()=>'https://im.szputy.com:9885/enterprise/v1',imToken:()=>'synthetic-im',chatToken:()=>'synthetic-chat',platform:()=>2,userID:()=>'employee');
void main(){
 test('original signed attachment is authorized unchanged with both credentials',() async {
  var calls=0;final source='https://192.168.10.11:9885/openim38/a%20b?X-Amz-Signature=a%2Fb';
  final manager=configured(GrantAdapter((request)async{calls++;expect(request.data['url'],source);expect(request.headers['token'],'synthetic-im');expect(request.headers['chatToken'],'synthetic-chat');return grant('current');}));
  final results=await Future.wait([manager.authorizeResource(source),manager.authorizeResource(source)]);
  expect(calls,1);expect(results[0],results[1]);manager.dispose();
 });
 test('external avatar remains unchanged without requesting a grant',() async {
  final manager=configured(GrantAdapter((_)async=>throw StateError('must not request')));
  expect(await manager.authorizeResource('https://avatar.example/person.png'),'https://avatar.example/person.png');manager.dispose();
 });
 test('new session never reuses a pending previous-session grant',() async {
  final complete=Completer<ResponseBody>();var calls=0;
  final adapter=GrantAdapter((_)async{calls++;if(calls==1) return await complete.future;return grant('new');});
  final manager=configured(adapter);
  final pending=manager.authorizeResource('https://im.szputy.com:9885/api/object/a');
  await Future<void>.delayed(Duration.zero);manager.dispose();
  complete.complete(grant('old'));await expectLater(pending,throwsStateError);
  expect(await manager.authorizeResource('https://im.szputy.com:9885/api/object/a'),contains('/attachments/new/'));expect(calls,2);manager.dispose();
 });
 test('grant must remain on selected HTTPS origin',() async {
  final manager=configured(GrantAdapter((_)async=>grant('evil',origin:'https://evil.example')));
  await expectLater(manager.authorizeResource('https://im.szputy.com:9885/api/object/a'),throwsStateError);manager.dispose();
 });
 test('preview and save require independent purpose grants and proof is never a URL parameter',()async{
  final purposes=<String>[];final manager=configured(GrantAdapter((request)async{purposes.add(request.data['purpose'] as String);return grant('current-${purposes.length}');}));
  final preview=await manager.authorizeResource('https://im.szputy.com:9885/api/object/a');
  final saved=await manager.authorizeResource('https://im.szputy.com:9885/api/object/a',purpose:'save');
  expect(purposes,['preview','save']);expect(manager.resourceHeaders(preview),{'X-Puty-Resource-Proof':List.filled(64,'a').join()});expect(Uri.parse(saved).queryParameters.containsKey('resourceProof'),isFalse);
  manager.dispose();expect(()=>manager.resourceHeaders(preview),throwsStateError);
 });
 test('image retries one fresh current proof after403 and never retries forever',()async{
  var authorizations=0,gets=0;late EnterpriseManager manager;
  manager=configured(GrantAdapter((request)async{
   if(request.method=='POST'){authorizations++;return grant('read-$authorizations',proof:List.filled(64,authorizations==1?'a':'b').join());}
   gets++;expect(request.headers['X-Puty-Resource-Proof'],List.filled(64,gets==1?'a':'b').join());expect(request.headers.containsKey('token'),isFalse);
   if(gets==1)return ResponseBody.fromString('rejected',403);
   return ResponseBody.fromBytes([1,2,3],200,headers:{Headers.contentLengthHeader:['3']});
  }));
  expect(await manager.readResourceBytes('https://im.szputy.com:9885/api/object/a'),[1,2,3]);expect(authorizations,2);expect(gets,2);manager.dispose();
  var failures=0;final denied=configured(GrantAdapter((request)async{if(request.method=='POST')return grant('denied');failures++;return ResponseBody.fromString('rejected',403);}));
  await expectLater(denied.readResourceBytes('https://im.szputy.com:9885/api/object/a'),throwsA(isA<DioException>()));expect(failures,2);denied.dispose();
 });
 test('a missing current resource proof is never accepted',()async{
  final manager=configured(GrantAdapter((_)async=>ResponseBody.fromString(jsonEncode({'errCode':0,'data':{'url':'https://im.szputy.com:9885/attachments/old/openim38/a','expiresAt':DateTime.now().millisecondsSinceEpoch/1000+600}}),200,headers:{Headers.contentTypeHeader:[Headers.jsonContentType]})));
  await expectLater(manager.authorizeResource('https://im.szputy.com:9885/api/object/a'),throwsStateError);manager.dispose();
 });
 test('public brand image maps trusted HRM and IM origins without grants',() async {
  var calls=0;final manager=configured(GrantAdapter((_)async{calls++;throw StateError('must not authorize brand');}));
  for(final origin in ['https://192.168.10.11:9885','https://im.szputy.com:9885','https://192.168.10.11:9880','https://hr.szputy.com:9880']){
   final source='$origin/resource/static/brand/default-avatar.png?v=puty2';
   expect(manager.needsResourceGrant(source),isFalse);expect(await manager.authorizeResource(source),'https://im.szputy.com:9885/resource/static/brand/default-avatar.png?v=puty2');
   final profile={'faceURL':source};await manager.authorizeResources(profile);expect(profile['faceURL'],'https://im.szputy.com:9885/resource/static/brand/default-avatar.png?v=puty2');
  }
  for(final prefix in ['', 'suxin/', 'hr/', 'im/'])for(final name in ['logo.png','favicon.ico','favicon-16x16.png','favicon-32x32.png','apple-touch-icon.png','android-chrome-192x192.png','android-chrome-512x512.png']){
   final suffix='/resource/static/brand/$prefix$name?v=puty2',source='https://hr.szputy.com:9880$suffix';expect(manager.needsResourceGrant(source),isFalse);expect(await manager.authorizeResource(source),'https://im.szputy.com:9885$suffix');
  }expect(calls,0);manager.dispose();
 });
 test('brand exception does not expose private paths or malformed brand URLs',(){
  final manager=configured(GrantAdapter((_)async=>throw StateError('must not request')));
  for(final suffix in ['/resource/uploads/a.png','/attachments/opaque/openim38/a.png','/resource/static/brand/nested/a.png','/resource/static/brand/../uploads/a.png','/resource/static/brand/a%2fb.png','/resource/static/brand/a%252fb.png','/resource/static/brand/a.png?redirect=../../uploads/a','/resource/static/brand/a.png?v=1&url=x','/resource/static/brand/a.png#fragment']){
   final url='https://im.szputy.com:9885$suffix';expect(manager.resolvePublicBrandResource(url),url);expect(manager.needsResourceGrant(url),isTrue);
  }
  for(final url in ['https://external.example/resource/static/brand/a.png','http://im.szputy.com:9885/resource/static/brand/a.png','https://u:p@im.szputy.com:9885/resource/static/brand/a.png','packages/local/avatar.png'])expect(manager.resolvePublicBrandResource(url),url);
  manager.dispose();
 });

}
