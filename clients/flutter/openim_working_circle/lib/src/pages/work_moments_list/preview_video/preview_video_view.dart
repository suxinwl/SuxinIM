import 'package:flutter/material.dart';
import 'package:openim_common/openim_common.dart';
class PreviewVideoPage extends StatelessWidget{
 final String url;final String? coverUrl,heroTag;
 const PreviewVideoPage({super.key,required this.url,this.coverUrl,this.heroTag});
 @override Widget build(BuildContext context)=>Material(color:Colors.black,child:ChatVideoPlayerView(url:url,coverUrl:coverUrl,heroTag:heroTag));
}
