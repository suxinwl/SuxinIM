import 'package:flutter/material.dart';
import 'package:get/get.dart';
import 'server_config_logic.dart';
class ServerConfigPage extends StatelessWidget {
 final logic=Get.find<ServerConfigLogic>();
 ServerConfigPage({super.key});
 @override Widget build(BuildContext context)=>Scaffold(
  appBar:AppBar(title:const Text('通讯连接'),actions:[TextButton(onPressed:logic.confirm,child:const Text('保存'))]),
  body:Obx(()=>Column(children:[
   RadioListTile<bool>(value:true,groupValue:logic.isIP.value,onChanged:(_)=>logic.switchServer(true),title:const Text('公司内网'),subtitle:const Text('https://192.168.10.11:9885')),
   RadioListTile<bool>(value:false,groupValue:logic.isIP.value,onChanged:(_)=>logic.switchServer(false),title:const Text('公网'),subtitle:const Text('https://im.szputy.com:9885')),
   const Padding(padding:EdgeInsets.all(24),child:Text('切换后使用所选连接的整套 HTTPS/WSS 服务并重新登录。'))
  ])),
 );
}
