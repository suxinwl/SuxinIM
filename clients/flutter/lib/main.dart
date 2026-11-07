import 'dart:async';
import 'dart:ui';
import 'package:flutter/material.dart';
import 'package:openim_common/openim_common.dart';

import 'app.dart';

void main(){
  var initialized=false;
  runZonedGuarded(()async{
    WidgetsFlutterBinding.ensureInitialized();
    // Exceptions can contain SDK message bodies, identities or signed URLs.
    // Production diagnostics expose only fixed categories, never stringify them.
    FlutterError.onError=(_)=>debugPrint('PUTY_UI_ERROR');
    PlatformDispatcher.instance.onError=(_,__){debugPrint('PUTY_ASYNC_ERROR');return true;};
    ErrorWidget.builder=(_)=>const Center(child:Text('页面暂不可用，请重试',textDirection:TextDirection.ltr));
    await Config.init(()=>runApp(const ChatApp()));initialized=true;
  },(_,__){
    debugPrint('PUTY_APPLICATION_ERROR');
    if(!initialized)runApp(const MaterialApp(home:Scaffold(body:Center(child:Text('受保护数据初始化未完成，请关闭应用后重试')))));
  });
}
