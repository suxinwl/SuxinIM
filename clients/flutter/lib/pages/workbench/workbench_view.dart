import 'package:flutter/material.dart';
import 'package:get/get.dart';
import 'package:openim_common/openim_common.dart';
import 'workbench_logic.dart';
class WorkbenchPage extends StatelessWidget {
 final logic=Get.find<WorkbenchLogic>();
 WorkbenchPage({super.key});
 @override Widget build(BuildContext context)=>Scaffold(
  appBar:AppBar(title:const Text('发现'),actions:[IconButton(onPressed:logic.refreshList,icon:const Icon(Icons.refresh))]),
  body:Obx((){
   if(logic.loading.value)return const Center(child:CircularProgressIndicator());
   if(logic.error.value.isNotEmpty)return Center(child:TextButton(onPressed:logic.refreshList,child:Text(logic.error.value)));
   if(logic.list.isEmpty)return const Center(child:Text('尚未配置发现菜单'));
   return RefreshIndicator(onRefresh:logic.refreshList,child:ListView.separated(
    itemCount:logic.list.length,separatorBuilder:(_,__)=>const Divider(height:1),
    itemBuilder:(_,index){final item=logic.list[index];return ListTile(
     leading:item.icon?.isNotEmpty==true?ImageUtil.networkImage(url:item.icon!,width:40,height:40):const Icon(Icons.apps),
     title:Text(item.name??''),trailing:const Icon(Icons.open_in_new),onTap:()=>logic.open(item));},
   ));
  }),
 );
}
