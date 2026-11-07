import 'dart:async';
import 'package:flutter/material.dart';
import 'package:flutter_download_manager/flutter_download_manager.dart' show DownloadStatus;
import 'package:app_im/app_im.dart';
import 'package:get/get.dart';
import 'package:openim_common/openim_common.dart';

class ProtectedDownloadTask {
 final status=ValueNotifier<DownloadStatus>(DownloadStatus.queued);
 final progress=ValueNotifier<double>(0);
 Map<String,dynamic>? saved;
 void dispose(){status.dispose();progress.dispose();}
}

/// Explicit saves only. Native verifies save policy, performs real Range resume
/// and writes original-name ciphertext. No public/plaintext destination exists.
class DownloadController extends GetxController {
 final downloadTaskList=<String?,ProtectedDownloadTask>{}.obs;
 StreamSubscription<void>? _locked;
 @override void onInit(){super.onInit();_locked=NativeLocalVault.invalidated.listen((_){for(final task in downloadTaskList.values){task.saved=null;task.status.value=DownloadStatus.failed;}downloadTaskList.clear();});}
 @override void onClose(){_locked?.cancel();for(final task in downloadTaskList.values){task.dispose();}super.onClose();}
 ProtectedDownloadTask? getTask(String url)=>downloadTaskList[url];
 bool isExistTask(String url)=>downloadTaskList.containsKey(url);
 bool isExistMessageTask(Message message)=>message.isFileType&&message.fileElem?.sourceUrl!=null&&isExistTask(message.fileElem!.sourceUrl!);
 ValueNotifier<DownloadStatus> getStatus(Message message)=>downloadTaskList[message.fileElem!.sourceUrl!]!.status;
 ValueNotifier<double> getProgress(Message message)=>downloadTaskList[message.fileElem!.sourceUrl!]!.progress;
 Future<void> save(String url,String sourceName,{int? size,String? conversationID,String? messageID})async{
  var task=downloadTaskList[url];if(task?.status.value==DownloadStatus.downloading){IMViews.showToast('正在加密保存附件');return;}
  task??=ProtectedDownloadTask();downloadTaskList[url]=task;task.status.value=DownloadStatus.downloading;
  try{task.saved=await NativeLocalVault.save(url,sourceName,size:size,conversationID:conversationID,messageID:messageID);task.progress.value=1;task.status.value=DownloadStatus.completed;IMViews.showToast('已加密保存：${task.saved!['name']}');}
  catch(_){task.status.value=DownloadStatus.failed;IMViews.showToast('加密保存未完成；可重试续传或检查下载权限');}
 }
 Future<void> addDownloadForMessage(Message message,{String? path})async{
  String? url;int? size;
  if(message.isFileType){url=message.fileElem?.sourceUrl;size=message.fileElem?.fileSize;}
  else if(message.isPictureType){url=message.pictureElem?.sourcePicture?.url;size=message.pictureElem?.sourcePicture?.size;}
  else if(message.isVideoType){url=message.videoElem?.videoUrl;size=message.videoElem?.videoSize;}
  else if(message.contentType==MessageType.voice){url=message.soundElem?.sourceUrl;size=message.soundElem?.dataSize;}
  if(url==null||url.isEmpty)return;
  final peer=message.sessionType==ConversationType.single?(message.sendID==AppIm.iMManager.userID?message.recvID:message.sendID):message.groupID;
  if(peer==null)return;
  final conversation=await AppIm.iMManager.conversationManager.getConversationIDBySessionType(sourceID:peer,sessionType:message.sessionType!);
  await save(url,IMUtils.messageSourceName(message,url),size:size,messageID:message.clientMsgID,conversationID:conversation is String?conversation:null);
 }
}
