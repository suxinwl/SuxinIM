import 'dart:async';
import 'dart:convert';
import 'dart:typed_data';
import 'package:flutter/material.dart';
import 'package:app_im/app_im.dart';
import 'package:openim_common/openim_common.dart';
import 'package:get/get.dart';
import 'chat/chat_video_player_view.dart';

/// In-app memory preview. Opening a file does not perform an explicit save or
/// export plaintext to another application or a system downloads directory.
class ProtectedFilePage extends StatefulWidget {
  final String name;
  final String? source, taskID;
  final Message? message;
  const ProtectedFilePage(
      {super.key, required this.name, this.source, this.taskID, this.message});
  @override
  State<ProtectedFilePage> createState() => _ProtectedFileState();
}

class _ProtectedFileState extends State<ProtectedFilePage> {
  static const mediaExtensions = {
    'mp4',
    'm4v',
    'mov',
    'webm',
    '3gp',
    'mkv',
    'mp3',
    'm4a',
    'aac',
    'wav',
    'ogg',
    'opus'
  };
  String get extension => widget.name.split('.').last.toLowerCase();
  Uint8List? bytes;
  bool busy = true;
  String? error;
  int generation = 0;
  StreamSubscription<void>? locked;
  @override
  void initState() {
    super.initState();
    locked = NativeLocalVault.invalidated.listen((_) {
      generation++;
      bytes?.fillRange(0, bytes!.length, 0);
      if (mounted)
        setState(() {
          bytes = null;
          busy = false;
          error = '当前在线授权已失效，预览已锁定';
        });
    });
    load();
  }

  Future<void> load() async {
    final attempt = ++generation;
    try {
      if (!NativeLocalVault.unlocked) throw StateError('Locked');
      if (mediaExtensions.contains(extension)) {
        setState(() => busy = false);
        return;
      }
      final data = widget.taskID != null
          ? await NativeLocalVault.read(widget.taskID!)
          : await AppIm.enterprise.readResourceBytes(widget.source!);
      if (!mounted || attempt != generation || !NativeLocalVault.unlocked) {
        data.fillRange(0, data.length, 0);
        return;
      }
      setState(() {
        bytes = data;
        busy = false;
      });
    } catch (_) {
      if (mounted && attempt == generation)
        setState(() {
          busy = false;
          error = '附件预览不可用，请检查当前权限并重试';
        });
    }
  }

  @override
  void dispose() {
    generation++;
    locked?.cancel();
    bytes?.fillRange(0, bytes!.length, 0);
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final data = bytes;
    Widget body;
    if (busy) {
      body = const Center(child: CircularProgressIndicator());
    } else if (error != null) {
      body = Center(child: Text(error!));
    } else if (mediaExtensions.contains(extension)) {
      body = ColoredBox(
          color: Colors.black,
          child:
              ChatVideoPlayerView(taskID: widget.taskID, url: widget.source));
    } else if (data != null &&
        ['png', 'jpg', 'jpeg', 'gif', 'webp', 'bmp'].contains(extension)) {
      body = InteractiveViewer(child: Image.memory(data, cacheWidth: 2048));
    } else if (data != null &&
        ['txt', 'csv', 'json', 'xml', 'log', 'md'].contains(extension)) {
      body = SingleChildScrollView(
          padding: const EdgeInsets.all(16),
          child: SelectableText(utf8.decode(
              data.sublist(0, data.length.clamp(0, 2 << 20)),
              allowMalformed: true)));
    } else {
      body =
          const Center(child: Text('此格式暂不支持内置预览。附件可按当前下载权限加密保存，客户端不会生成明文副本。'));
    }
    return Scaffold(
        appBar: AppBar(title: Text(widget.name), actions: [
          if (widget.message != null)
            IconButton(
                tooltip: '加密保存',
                icon: const Icon(Icons.download),
                onPressed: busy
                    ? null
                    : () async {
                        await Get.find<DownloadController>()
                            .addDownloadForMessage(widget.message!);
                      })
        ]),
        body: body);
  }
}
