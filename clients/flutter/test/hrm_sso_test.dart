import 'dart:convert';
import 'dart:typed_data';
import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:openim_common/openim_common.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'native_settings_mock.dart';
class SSOAdapter implements HttpClientAdapter {
 Future<ResponseBody> Function(RequestOptions) handle;
 SSOAdapter(this.handle);
 @override Future<ResponseBody> fetch(RequestOptions options,Stream<Uint8List>? stream,Future<void>? cancel)=>handle(options);
 @override void close({bool force=false}){}
}
Map<String,dynamic> session()=>{'userID':'employee','imToken':'im-secret','chatToken':'chat-secret','sessionToken':'session-secret','expiresAt':DateTime.now().millisecondsSinceEpoch~/1000+14400,'profile':{'nickname':'员工'}};
ResponseBody reply(Map<String,dynamic> data,{int status=200})=>ResponseBody.fromString(jsonEncode({'code':status==200?0:status,'data':data}),status,headers:{Headers.contentTypeHeader:['application/json']});
void main(){
 TestWidgetsFlutterBinding.ensureInitialized();
 setUp(() async {installNativeSettingsMock();SharedPreferences.setMockInitialValues({});await DataSp.init();HrmSSO.invalidate();});
 test('unified login sends hashed password and selected complete endpoint',() async {
  RequestOptions? captured;
  dio=Dio()..httpClientAdapter=SSOAdapter((options)async{captured=options;return reply(session());});
  final result=await HrmSSO.login('1001','raw-password',captcha:'1234',captchaId:'challenge');
  expect(captured!.uri.toString(),'https://192.168.10.11:9885/sso/login');
  expect(captured!.data['password'],IMUtils.generateMD5('raw-password'));
  expect(captured!.data['account'],'1001');expect(captured!.data['captchaId'],'challenge');expect(result.userID,'employee');
  expect(captured!.data['policyClient'],'android');
 });
 test('expired and incomplete credentials are rejected',(){
  expect(()=>HrmSSO.validate({...session(),'expiresAt':1}),throwsA(isA<HrmSessionError>()));
  expect(()=>HrmSSO.validate({...session(),'sessionToken':''}),throwsA(isA<HrmSessionError>()));
 });
 test('credential diagnostics omit tokens',(){
  final text=LoginCertificate.fromJson(session()).toString();expect(text,contains('employee'));expect(text, isNot(contains('secret')));
 });
 test('revocation fails closed',() async {
  await DataSp.putLoginCertificate(LoginCertificate.fromJson(session()));
  dio=Dio()..httpClientAdapter=SSOAdapter((_)async=>reply({},status:401));
  await expectLater(HrmSSO.refresh(force:true),throwsA(isA<HrmSessionError>().having((e)=>e.status,'status',401)));
 });
 test('WAN switch selects every API endpoint together',() async {
  await DataSp.putServerConfig({'baseUrl':Config.wanOrigin,'apiUrl':'http://untrusted.example'});
  expect(Config.serverBaseUrl,Config.wanOrigin);
  expect(Config.imApiUrl,'${Config.wanOrigin}/api');expect(Config.appAuthUrl,'${Config.wanOrigin}/chat');expect(Config.imWsUrl,'wss://im.szputy.com:9885/ws');
 });
 test('forced password challenge never becomes a formal login certificate',()async{
  final expiry=DateTime.now().millisecondsSinceEpoch~/1000+299;
  dio=Dio()..httpClientAdapter=SSOAdapter((_)async=>reply({'reasonCode':'PASSWORD_CHANGE_REQUIRED','requiresPasswordChange':true,'changeTicket':'synthetic-ticket','expiresAt':expiry},status:428));
  await expectLater(HrmSSO.login('synthetic','synthetic-password'),throwsA(isA<HrmPasswordChallenge>().having((error)=>error.toString(),'safe diagnostic',isNot(contains('synthetic-ticket')))));
  expect(DataSp.getLoginCertificate(),isNull);
 });
 test('forced challenge rejects tokens and invalid expiry',()async{
  dio=Dio()..httpClientAdapter=SSOAdapter((_)async=>reply({'reasonCode':'PASSWORD_CHANGE_REQUIRED','requiresPasswordChange':true,'changeTicket':'synthetic-ticket','expiresAt':DateTime.now().millisecondsSinceEpoch~/1000+299,'imToken':'must-not-appear'},status:428));
  await expectLater(HrmSSO.login('synthetic','password'),throwsA(isA<HrmSessionError>().having((error)=>error.status,'status',503)));
 });
 test('self password change sends only MD5 hashes and existing opaque session',()async{
  await DataSp.putLoginCertificate(LoginCertificate.fromJson(session()));RequestOptions? sent;
  dio=Dio()..httpClientAdapter=SSOAdapter((options)async{sent=options;return reply({'changed':true,'requiresLogin':true});});
  await HrmSSO.changePassword('new-synthetic',oldPassword:'old-synthetic');
  expect(sent!.data,{'sessionToken':'session-secret','oldPassword':IMUtils.generateMD5('old-synthetic'),'newPassword':IMUtils.generateMD5('new-synthetic')});
 });
 test('force change sends no old password or session token',()async{
  RequestOptions? sent;dio=Dio()..httpClientAdapter=SSOAdapter((options)async{sent=options;return reply({'changed':true,'requiresLogin':true});});
  await HrmSSO.changePassword('new-synthetic',changeTicket:'synthetic-ticket');expect(sent!.data,{'changeTicket':'synthetic-ticket','newPassword':IMUtils.generateMD5('new-synthetic')});
 });
 test('non-success HTTP cannot smuggle a code-zero login',()async{
  dio=Dio()..httpClientAdapter=SSOAdapter((_)async=>ResponseBody.fromString(jsonEncode({'code':0,'data':session()}),403,headers:{Headers.contentTypeHeader:['application/json']}));
  await expectLater(HrmSSO.login('synthetic','password'),throwsA(isA<HrmSessionError>().having((error)=>error.status,'status',403)));
 });
}
