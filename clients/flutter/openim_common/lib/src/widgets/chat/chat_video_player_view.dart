import 'dart:async';
import 'dart:io';

import 'package:chewie/chewie.dart';
import 'package:flutter/cupertino.dart';
import 'package:flutter/material.dart';
import 'package:get/get.dart';
import 'package:openim_common/openim_common.dart';
import 'package:video_player/video_player.dart';
import 'package:app_im/app_im.dart';

import '../custom_cupertino_controls.dart';

abstract class VideoControllerService {
  Future<VideoPlayerController> getVideo(String videoUrl);
  Future<File?> getCacheFile(String videoUrl);
}

class CachedVideoControllerService extends VideoControllerService {
  CachedVideoControllerService();

  @override
  Future<VideoPlayerController> getVideo(String videoUrl) async {
    final url=await AppIm.enterprise.authorizeResource(videoUrl);
    return VideoPlayerController.networkUrl(Uri.parse(url),httpHeaders:AppIm.enterprise.resourceHeaders(url));
  }

  @override
  Future<File?> getCacheFile(String videoUrl) async {
    // TODO: implement getCacheFile
    return null;
  }
}

class ChatVideoPlayerView extends StatefulWidget {
  const ChatVideoPlayerView({
    Key? key,
    this.path,
    this.taskID,
    this.url,
    this.coverUrl,
    this.file,
    this.heroTag,
    this.oDownload,
    this.autoPlay = true,
    this.muted = false,
  }) : super(key: key);
  final String? path;
  final String? taskID;
  final String? url;
  final File? file;
  final String? coverUrl;
  final String? heroTag;
  final bool autoPlay;
  final bool muted;
  final Function(String? url)? oDownload;

  @override
  State<ChatVideoPlayerView> createState() => _ChatVideoPlayerViewState();
}

class _ChatVideoPlayerViewState extends State<ChatVideoPlayerView> with SingleTickerProviderStateMixin {
  VideoPlayerController? _videoPlayerController;
  StreamSubscription<void>? _invalidated;int _attempt=0;String? _error;
  ChewieController? _chewieController;

  final _cachedVideoControllerService = CachedVideoControllerService();

  @override
  void initState() {
    super.initState();
    _invalidated=NativeLocalVault.invalidated.listen((_){_attempt++;_chewieController?.pause();_chewieController?.dispose();_chewieController=null;final player=_videoPlayerController;_videoPlayerController=null;unawaited(player?.dispose());if(mounted)setState(()=>_error='当前媒体预览已关闭，请重新授权打开');});
    initializePlayer();
  }

  @override
  void dispose() {
    _chewieController?.pause();
    _attempt++;_invalidated?.cancel();
    _videoPlayerController?.dispose();
    _chewieController?.dispose();
    super.dispose();
  }

  Future<void> initializePlayer() async {
    final attempt=++_attempt;VideoPlayerController? player;
    try{
      String? path=widget.file?.path??_path;
      if(path!=null){try{await NativeLocalVault.sourceInfo(path);}catch(_){path=null;}}
      if(Platform.isIOS){await NativeLocalVault.nativePreview(path:path,taskID:widget.taskID,url:path==null&&widget.taskID==null?_url:null);if(mounted&&attempt==_attempt)setState(()=>_error='媒体预览已关闭');return;}
      player=widget.taskID!=null?VideoPlayerController.networkUrl(await NativeLocalVault.attachmentPreviewUri(widget.taskID!)):path!=null?VideoPlayerController.networkUrl(await NativeLocalVault.sourcePreviewUri(path)):await _cachedVideoControllerService.getVideo(_url!);
      await player.initialize();
      if(!mounted||attempt!=_attempt||!NativeLocalVault.unlocked){await player.dispose();return;}
      _videoPlayerController=player;if(widget.muted)await player.setVolume(0);_createChewieController();setState((){});
    }catch(_){await player?.dispose();if(mounted&&attempt==_attempt)setState(()=>_error='媒体预览不可用，请重新授权后重试');}
  }

  void _createChewieController() {
    _chewieController = ChewieController(
      videoPlayerController: _videoPlayerController!,
      autoPlay: widget.autoPlay,
      looping: false,
      allowFullScreen: false,
      allowPlaybackSpeedChanging: false,
      showControlsOnInitialize: true,
      customControls: CustomCupertinoControls(backgroundColor: Colors.black.withOpacity(0.7), iconColor: Colors.white),
      // hideControlsTimer: const Duration(seconds: 1),
      optionsTranslation: OptionsTranslation(
        playbackSpeedButtonText: StrRes.playSpeed,
        cancelButtonText: StrRes.cancel,
      ),
      additionalOptions: (context) => [
        OptionItem(
          onTap: () {
            widget.oDownload?.call(widget.url);
            Get.back();
          },
          iconData: Icons.download_outlined,
          title: StrRes.download,
        ),
      ],
    );
  }

  Future<void> toggleVideo() async {
    await _videoPlayerController?.pause();
    await initializePlayer();
  }

  @override
  Widget build(BuildContext context) {
    return SafeArea(
      child: Stack(
        children: [
          if(_error!=null)Center(child:Text(_error!,style:const TextStyle(color:Colors.white)))
          else if (_chewieController != null && _chewieController!.videoPlayerController.value.isInitialized)
            Chewie(controller: _chewieController!)
          else
            _buildCoverView(context),
        ],
      ),
    );
  }

  Widget _buildCoverView(BuildContext context) => Stack(
        alignment: Alignment.center,
        children: [
          if (null != widget.coverUrl)
            Center(
              child: ImageUtil.networkImage(
                  url: widget.coverUrl!,
                  loadProgress: false,
                  height: MediaQuery.of(context).size.height,
                  width: MediaQuery.of(context).size.width,
                  fit: BoxFit.fitWidth),
            ),
          const Center(
            child: CupertinoActivityIndicator(
              color: Colors.white,
            ),
          ),
        ],
      );

  String? get _path => widget.path;

  String? get _url => widget.url;
}
