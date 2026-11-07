import 'dart:async';

import 'package:flutter/material.dart';
import 'package:openim_common/openim_common.dart';
import '../../utils/voice_record.dart';

class ChatRecordVoiceView extends StatefulWidget {
  const ChatRecordVoiceView({
    Key? key,
    required this.builder,
    this.maxRecordSec = 60,
    this.onInterrupt,
    this.onCompleted,
  }) : super(key: key);
  final int maxRecordSec;
  final Function()? onInterrupt;
  final Function(int sec, String path)? onCompleted;
  final Widget Function(BuildContext context, int sec) builder;

  @override
  State<ChatRecordVoiceView> createState() => _ChatRecordVoiceViewState();
}

class _ChatRecordVoiceViewState extends State<ChatRecordVoiceView>{
 late final VoiceRecord _record;int _duration=0;bool _completed=false;
 void complete(int seconds,String path){if(_completed){NativeLocalVault.discard(path);return;}_completed=true;widget.onCompleted?.call(seconds,path);}
 @override void initState(){super.initState();_record=VoiceRecord(maxRecordSec:widget.maxRecordSec,onFinished:complete,onInterrupt:(sec,path){complete(sec,path);widget.onInterrupt?.call();},onDuration:(seconds){if(mounted)setState(()=>_duration=seconds);});unawaited(_record.start());}
 @override void dispose(){unawaited(_record.stop().whenComplete(_record.dispose));super.dispose();}
 @override Widget build(BuildContext context)=>widget.builder(context,_duration);
}

// class ChatRecordVoiceView extends StatelessWidget {
//   const ChatRecordVoiceView({
//     Key? key,
//   }) : super(key: key);
//
//
//   @override
//   Widget build(BuildContext context) {
//     return Material(
//       color: Colors.transparent,
//       child: Center(
//         child: Container(
//           width: 138.w,
//           height: 124.h,
//           decoration: BoxDecoration(
//             color: Styles.c_0C1C33_opacity60,
//             borderRadius: BorderRadius.circular(6.r),
//           ),
//           child: Column(),
//         ),
//       ),
//     );
//   }
// }

class _ArrowClipper extends CustomClipper<Path> {
  @override
  Path getClip(Size size) {
    Path path = Path();
    path.moveTo(0, -2);
    path.lineTo(size.width, -2);
    path.lineTo(size.width / 2, size.height * 2 / 4);

    // path.moveTo(0, size.height);
    // path.lineTo(size.width / 2, size.height / 2);
    // path.lineTo(size.width, size.height);
    return path;
  }

  @override
  bool shouldReclip(CustomClipper<Path> oldClipper) {
    return true;
  }
}
