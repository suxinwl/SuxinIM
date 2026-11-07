import 'dart:async';
import 'package:flutter/material.dart';
import 'package:openim_common/openim_common.dart';

class PasswordChangePage extends StatefulWidget {
  final String? changeTicket;
  const PasswordChangePage({super.key,this.changeTicket});
  @override State<PasswordChangePage> createState()=>_PasswordChangeState();
}
class _PasswordChangeState extends State<PasswordChangePage> {
  final oldPassword=TextEditingController(),newPassword=TextEditingController(),confirm=TextEditingController();
  bool busy=false,valid=true,completed=false;
  String? error;
  Timer? expiry;
  @override void initState(){super.initState();if(widget.changeTicket!=null){valid=false;unawaited(checkChallenge());}}
  Future<void> checkChallenge() async {
    try {
      final response=await HrmSSO.request('password/status',{'changeTicket':widget.changeTicket});
      final until=(response['expiresAt'] as num?)?.toInt()??0;
      final remaining=until-DateTime.now().millisecondsSinceEpoch~/1000;
      if(response['requiresPasswordChange']!=true||remaining<=0)throw StateError('Expired challenge');
      if(mounted)setState(()=>valid=true);
      expiry=Timer(Duration(seconds:remaining),(){if(mounted)setState((){valid=false;error='改密凭证已过期，请返回重新登录';});});
    }catch(_){if(mounted)setState((){valid=false;error='改密凭证已失效，请返回重新登录';});}
  }
  @override void dispose(){expiry?.cancel();oldPassword.clear();newPassword.clear();confirm.clear();oldPassword.dispose();newPassword.dispose();confirm.dispose();if(!completed&&widget.changeTicket!=null)unawaited(HrmSSO.request('password/cancel',{'changeTicket':widget.changeTicket}).catchError((Object _)=><String,dynamic>{}));super.dispose();}
  Future<void> submit() async {
    if(busy||!valid)return;
    if(newPassword.text.length<8||newPassword.text!=confirm.text||(widget.changeTicket==null&&oldPassword.text.isEmpty)){setState(()=>error='请输入原密码，并确认两次新密码一致（至少8位）');return;}
    setState((){busy=true;error=null;});
    try {
      await HrmSSO.changePassword(newPassword.text,oldPassword:oldPassword.text,changeTicket:widget.changeTicket);
      completed=true;await NativeLocalVault.lock();
      if(mounted)Navigator.of(context).pop(true);
    }catch(failure){if(mounted)setState(()=>error=failure is HrmSessionError?failure.message:'密码变更未完成，请检查连接后重试');}
    finally{if(mounted)setState(()=>busy=false);}
  }
  @override Widget build(BuildContext context)=>Scaffold(appBar:AppBar(title:Text(widget.changeTicket==null?'修改本人密码':'首次登录请修改密码')),body:ListView(padding:const EdgeInsets.all(24),children:[
    const Text('修改后所有终端的旧会话都会失效，请使用新密码重新登录。'),
    if(widget.changeTicket==null)TextField(controller:oldPassword,obscureText:true,enableSuggestions:false,autocorrect:false,decoration:const InputDecoration(labelText:'原密码')),
    TextField(controller:newPassword,obscureText:true,enableSuggestions:false,autocorrect:false,decoration:const InputDecoration(labelText:'新密码（至少8位）')),
    TextField(controller:confirm,obscureText:true,enableSuggestions:false,autocorrect:false,decoration:const InputDecoration(labelText:'确认新密码')),
    if(error!=null)Padding(padding:const EdgeInsets.symmetric(vertical:12),child:Text(error!,style:const TextStyle(color:Colors.red))),
    const SizedBox(height:24),FilledButton(onPressed:busy||!valid?null:submit,child:Text(busy?'正在修改…':'确认修改并重新登录')),
  ]));
}
