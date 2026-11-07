import 'dart:async';
import 'dart:io';
import 'package:flutter/material.dart';
import 'package:wechat_camera_picker/wechat_camera_picker.dart';
import 'package:video_compress/video_compress.dart';
import 'package:openim_common/openim_common.dart';

/// Intercept the original camera XFile before the upstream file-based viewer
/// or PhotoManager gallery-save code. Only our in-app vault viewers consume it.
class ProtectedCamera {
 static Future<NativeMediaAsset?> capture(BuildContext context,{bool video=true,bool onlyVideo=false,int maxDuration=60,Locale? locale})async{
  if(Platform.isIOS){final item=await NativeLocalVault.nativeCapture(kind:onlyVideo?'video':video?'both':'photo',maxDuration:maxDuration);return item==null?null:NativeMediaAsset(item);}
  if(!await NativeLocalVault.captureSupported()){IMViews.showToast('加密拍摄需要 Android 8.0 或以上；可继续从相册或文件选择发送');return null;}
  final generation=NativeLocalVault.generation,navigator=Navigator.of(context,rootNavigator:true);
  CameraPickerPageRoute<AssetEntity>? route;String? path;CameraPickerViewType? kind;
  void close(){final current=route;if(current?.isActive==true)navigator.removeRoute(current!);}
  final subscription=NativeLocalVault.invalidated.listen((_)=>close());
  try{await CameraPicker.pickFromCamera(context,locale:locale,pageRouteBuilder:(picker)=>route=CameraPickerPageRoute<AssetEntity>(builder:(_)=>picker),pickerConfig:CameraPickerConfig(enableAudio:true,enableRecording:video,onlyEnableRecording:onlyVideo,enableScaledPreview:true,resolutionPreset:ResolutionPreset.medium,maximumRecordingDuration:Duration(seconds:maxDuration),onXFileCaptured:(file,type){path=file.path;kind=type;close();return true;},onMinimumRecordDurationNotMet:()=>IMViews.showToast(StrRes.tapTooShort)));
   if(path==null)return null;if(!NativeLocalVault.unlocked||generation!=NativeLocalVault.generation){await NativeLocalVault.discard(path!);return null;}
   // CameraX returns an encrypted producer descriptor. Seal it into the same
   // native source format used by file/album results before any viewer or SDK
   // consumer outlives the camera route; the descriptor is never a file path
   // that the SDK may reopen through /proc.
   path=await NativeLocalVault.importSource(path!);
   if(!NativeLocalVault.unlocked||generation!=NativeLocalVault.generation){await NativeLocalVault.discard(path!);return null;}
   final info=await NativeLocalVault.sourceInfo(path!);final isVideo=kind==CameraPickerViewType.video;
   final metadata=isVideo?await VideoCompress.getMediaInfo(path!):null;
   return NativeMediaAsset({'path':path,'name':info['name'],'size':info['size'],'kind':isVideo?'video':'image','mimeType':isVideo?'video/mp4':'image/jpeg','duration':((metadata?.duration??0)/1000).round(),'width':metadata?.width??0,'height':metadata?.height??0});
  }finally{await subscription.cancel();}
 }
}

class ProtectedCapturePreview extends StatefulWidget{
 final NativeMediaAsset asset;const ProtectedCapturePreview(this.asset,{super.key});
 @override State<ProtectedCapturePreview> createState()=>_ProtectedCapturePreviewState();
}
class _ProtectedCapturePreviewState extends State<ProtectedCapturePreview>{
 StreamSubscription<void>? invalidated;
 @override void initState(){super.initState();invalidated=NativeLocalVault.invalidated.listen((_){if(!mounted)return;final route=ModalRoute.of(context);if(route?.isActive==true)Navigator.of(context).removeRoute(route!);});}
 @override void dispose(){invalidated?.cancel();super.dispose();}
 @override Widget build(BuildContext context)=>Scaffold(backgroundColor:Colors.black,appBar:AppBar(title:const Text('拍摄预览')),body:Column(children:[Expanded(child:widget.asset.type==AssetType.video?ChatVideoPlayerView(path:widget.asset.protectedPath):ImageUtil.fileImage(file:File(widget.asset.protectedPath),fit:BoxFit.contain)),Row(mainAxisAlignment:MainAxisAlignment.spaceEvenly,children:[TextButton(onPressed:()=>Navigator.pop(context,false),child:const Text('重拍')),FilledButton(onPressed:()=>Navigator.pop(context,true),child:const Text('使用'))]),const SizedBox(height:16)]));
}
