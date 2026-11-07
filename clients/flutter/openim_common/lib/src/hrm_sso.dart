import 'dart:io';
import 'dart:async';
import 'package:dio/dio.dart';
import 'package:openim_common/openim_common.dart';

class HrmSessionError implements Exception {
  final int status;
  final String message;
  HrmSessionError(this.status, this.message);
  @override String toString() => message;
}
class HrmPasswordChallenge extends HrmSessionError {
  final String changeTicket;
  final int expiresAt;
  HrmPasswordChallenge(this.changeTicket,this.expiresAt):super(428,'请先修改本人密码后重新登录');
}
class HrmSSO {
  static String get policyClient=>Platform.isIOS?'ios':'android';
  static int _generation=0;
  static void invalidate()=>_generation++;
  static Future<LoginCertificate>? _renewal;
  static Future<Map<String,dynamic>> request(String operation, [Map<String,dynamic>? body]) async {
    if (!['login','refresh','logout','exchange','captcha','directory','person','password/change','password/status','password/cancel'].contains(operation)) throw StateError('Unsupported account operation');
    try {
      final response = body == null
          ? await dio.get<Map<String,dynamic>>('${Config.serverBaseUrl}/sso/$operation',options:Options(validateStatus:(status)=>status!=null&&status>=200&&status<500))
          : await dio.post<Map<String,dynamic>>('${Config.serverBaseUrl}/sso/$operation',data:body,options:Options(validateStatus:(status)=>status!=null&&status>=200&&status<500));
      final result = response.data;
      if(response.statusCode==428 && result?['code']==428) {
        final data=result?['data'];
        if(data is Map && data['reasonCode']=='PASSWORD_CHANGE_REQUIRED' && data['requiresPasswordChange']==true &&
          data['changeTicket'] is String && (data['changeTicket'] as String).isNotEmpty && (data['changeTicket'] as String).length<=2048 &&
          data['expiresAt'] is num && !['imToken','chatToken','sessionToken'].any(data.containsKey)) {
          final expiry=(data['expiresAt'] as num).toInt(),now=DateTime.now().millisecondsSinceEpoch~/1000;
          if(expiry>now && expiry<=now+301)throw HrmPasswordChallenge(data['changeTicket'] as String,expiry);
        }
        throw HrmSessionError(503,'密码变更挑战无效，请重新登录');
      }
      if (response.statusCode!=200 || result == null || result['code'] != 0) throw HrmSessionError(response.statusCode ?? 503, '统一账号服务暂不可用');
      return Map<String,dynamic>.from(result['data'] is Map ? result['data'] : {});
    } on DioException catch(error) {
      throw HrmSessionError(error.response?.statusCode ?? 503, '统一账号服务暂不可用，请检查连接或重试');
    }
  }
  static LoginCertificate validate(Map<String,dynamic> data) {
    final result = LoginCertificate.fromJson(data);
    if ([result.userID,result.imToken,result.chatToken,result.sessionToken].any((x)=>x.isEmpty) || result.expiresAt <= DateTime.now().millisecondsSinceEpoch ~/ 1000) {
      throw HrmSessionError(503,'统一账号服务返回无效会话');
    }
    return result;
  }
  static Future<LoginCertificate> login(String account, String password, {String? captcha,String? captchaId}) async =>
    validate(await request('login', {'account':account,'password':IMUtils.generateMD5(password),'platform':Platform.isIOS ? 'ios' : 'android','policyClient':policyClient,'captcha':captcha,'captchaId':captchaId}));
  static Future<LoginCertificate> exchange(String ticket) async =>
    validate(await request('exchange',{'ticket':ticket,'platform':Platform.isIOS ? 'ios' : 'android','policyClient':policyClient}));
  static Future<LoginCertificate> refresh({bool force=false}) async {
    final current=DataSp.getLoginCertificate();
    if(current == null || current.sessionToken.isEmpty) throw HrmSessionError(401,'请使用统一账号重新登录');
    if(!force && current.expiresAt > DateTime.now().millisecondsSinceEpoch ~/1000 + 600) return current;
    return _renewal ??= _refresh(current).whenComplete(()=>_renewal=null);
  }
  static Future<LoginCertificate> _refresh(LoginCertificate current) async {
    final generation=_generation;
    final result=validate(await request('refresh',{'sessionToken':current.sessionToken,'policyClient':policyClient}));
    if(DataSp.getLoginCertificate()?.sessionToken != current.sessionToken || generation!=_generation) throw HrmSessionError(409,'账号会话已改变');
    await DataSp.putLoginCertificate(result);
    return result;
  }
  static Future<void> logout() async {
    invalidate();
    final current=DataSp.getLoginCertificate();
    if(current != null && current.sessionToken.isNotEmpty) await request('logout',{'sessionToken':current.sessionToken});
  }
  static Future<void> changePassword(String newPassword,{String? oldPassword,String? changeTicket}) async {
    final body=<String,dynamic>{'newPassword':IMUtils.generateMD5(newPassword)};
    if(changeTicket!=null){body['changeTicket']=changeTicket;}
    else {
      final current=DataSp.getLoginCertificate();
      if(current==null||oldPassword==null||oldPassword.isEmpty)throw HrmSessionError(401,'请重新登录');
      body['sessionToken']=current.sessionToken;body['oldPassword']=IMUtils.generateMD5(oldPassword);
    }
    final result=await request('password/change',body);
    if(result['changed']!=true||result['requiresLogin']!=true)throw HrmSessionError(503,'密码变更未确认，请重试');
    invalidate();
  }
}
