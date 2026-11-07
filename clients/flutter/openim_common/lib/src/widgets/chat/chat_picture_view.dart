import 'dart:async';
import 'dart:io';

import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:app_im/app_im.dart';
import 'package:flutter_screenutil/flutter_screenutil.dart';
import 'package:openim_common/openim_common.dart';
import 'package:path_provider/path_provider.dart';

class ChatPictureView extends StatefulWidget {
  const ChatPictureView({
    Key? key,
    required this.message,
    required this.isISend,
    this.sendProgressStream,
  }) : super(key: key);
  final Stream<MsgStreamEv<int>>? sendProgressStream;
  final bool isISend;
  final Message message;

  @override
  State<ChatPictureView> createState() => _ChatPictureViewState();
}

class _ChatPictureViewState extends State<ChatPictureView> {
  late double _trulyWidth;
  late double _trulyHeight;
  List<Object?> _pictureFields = const [];
  int _resolution = 0;
  bool _uploadCompleteSeen = false;
  StreamSubscription? _progress;
  StreamSubscription? _vaultInvalidated;
  Message get _message => widget.message;
  Widget? _child;

  @override
  void initState() {
    super.initState();
    _listenProgress();
    _vaultInvalidated = NativeLocalVault.invalidated.listen((_) {
      if (mounted) setState(() => _refreshPicture(force: true));
    });
    _refreshPicture();
  }

  @override
  void didUpdateWidget(covariant ChatPictureView oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (!identical(oldWidget.sendProgressStream, widget.sendProgressStream)) {
      _listenProgress();
    }
    _refreshPicture();
  }

  void _listenProgress() {
    _progress?.cancel();
    _progress = widget.sendProgressStream?.listen((event) {
      if (!mounted || !widget.isISend || event.id != _message.clientMsgID)
        return;
      if (event.value < 100) {
        _uploadCompleteSeen = false;
      } else if (!_uploadCompleteSeen) {
        _uploadCompleteSeen = true;
        setState(() => _refreshPicture(force: true));
      }
    });
  }

  void _refreshPicture({bool force = false}) {
    final picture = _message.pictureElem;
    final resourceGeneration = AppIm.enterprise.resourceGeneration;
    // The SDK updates the Message in place on send completion. oldWidget may
    // contain that same object, so compare captured scalar values instead.
    final fields = <Object?>[
      _message.clientMsgID,
      _message.status,
      widget.isISend,
      picture?.sourcePath,
      picture?.sourcePicture?.url,
      picture?.bigPicture?.url,
      picture?.snapshotPicture?.url,
      picture?.sourcePicture?.width,
      picture?.sourcePicture?.height,
      picture?.bigPicture?.width,
      picture?.bigPicture?.height,
      picture?.snapshotPicture?.width,
      picture?.snapshotPicture?.height,
      widget.sendProgressStream,
      resourceGeneration,
    ];
    if (!force && listEquals(fields, _pictureFields)) return;
    if (_pictureFields.isEmpty ||
        fields[0] != _pictureFields[0] ||
        fields[1] == MessageStatus.sending && fields[1] != _pictureFields[1]) {
      _uploadCompleteSeen = false;
    }
    _pictureFields = fields;
    final request = ++_resolution;
    final vaultGeneration = NativeLocalVault.generation;
    _child = null;
    final maximumWidth =
        pictureWidth.isFinite && pictureWidth > 0 ? pictureWidth : 120.0;
    double dimension(List<num?> choices) {
      for (final value in choices) {
        if (value != null && value.toDouble().isFinite && value > 0)
          return value.toDouble();
      }
      return maximumWidth;
    }

    final width = dimension([
      picture?.sourcePicture?.width,
      picture?.bigPicture?.width,
      picture?.snapshotPicture?.width
    ]);
    final height = dimension([
      picture?.sourcePicture?.height,
      picture?.bigPicture?.height,
      picture?.snapshotPicture?.height
    ]);
    _trulyWidth = width < maximumWidth ? width : maximumWidth;
    _trulyHeight = _trulyWidth * height / width;
    final screenWidth = 1.sw, screenHeight = 1.sh;
    final limit = screenWidth.isFinite &&
            screenWidth > 0 &&
            screenHeight.isFinite &&
            screenHeight > 0
        ? 2 * maximumWidth * screenHeight / screenWidth
        : 2 * maximumWidth;
    if (!_trulyHeight.isFinite || _trulyHeight <= 0 || _trulyHeight > limit)
      _trulyHeight = _trulyWidth;
    if (!NativeLocalVault.unlocked) return;
    final snap = picture?.snapshotPicture?.url;
    final snapshot = snap?.contains('.gif') == true
        ? snap
        : snap?.adjustThumbnailAbsoluteString(540);
    final original = IMUtils.isNotNullEmptyStr(picture?.bigPicture?.url)
        ? picture!.bigPicture!.url
        : picture?.sourcePicture?.url;
    _createChildView(request, vaultGeneration, resourceGeneration,
        widget.isISend ? picture?.sourcePath : null, snapshot, original);
  }

  bool _isCurrent(int request, int vaultGeneration, int resourceGeneration) =>
      mounted &&
      request == _resolution &&
      NativeLocalVault.unlocked &&
      vaultGeneration == NativeLocalVault.generation &&
      resourceGeneration == AppIm.enterprise.resourceGeneration;

  Future<void> _createChildView(
      int request,
      int vaultGeneration,
      int resourceGeneration,
      String? path,
      String? snapshot,
      String? original) async {
    var local = false;
    if (IMUtils.isNotNullEmptyStr(path)) {
      try {
        if (Platform.isIOS && path!.contains('/Library/Caches/')) {
          final directory = await getApplicationCacheDirectory();
          path = directory.path + path.split('/Library/Caches').last;
        }
        // These are encrypted vault identities, not external-storage files.
        // Native verifies the actor/lease; no save, import or plaintext export.
        final info = await NativeLocalVault.sourceInfo(path!);
        local = info['size'] is num && (info['size'] as num) > 0;
      } catch (_) {
        local = false;
      }
    }
    if (!_isCurrent(request, vaultGeneration, resourceGeneration)) return;
    final child = local
        ? _buildPathPicture(path: path!)
        : IMUtils.isNotNullEmptyStr(snapshot)
            ? _buildUrlPicture(url: snapshot!)
            : IMUtils.isNotNullEmptyStr(original)
                ? _buildUrlPicture(url: original!)
                : null;
    setState(() => _child = child);
  }

  @override
  void dispose() {
    _resolution++;
    _progress?.cancel();
    _vaultInvalidated?.cancel();
    super.dispose();
  }

  Widget _buildUrlPicture({required String url}) => ImageUtil.networkImage(
        url: url,
        height: _trulyHeight,
        width: _trulyWidth,
        fit: BoxFit.fitWidth,
      );

  Widget _buildPathPicture({required String path}) => Stack(
        children: [
          ImageUtil.fileImage(
            file: File(path),
            height: _trulyHeight,
            width: _trulyWidth,
            fit: BoxFit.fitWidth,
          ),
          ChatProgressView(
            height: _trulyHeight,
            width: _trulyWidth,
            id: _message.clientMsgID!,
            stream: widget.sendProgressStream,
            isISend: widget.isISend,
            type: ProgressType.picture,
          ),
        ],
      );

  @override
  Widget build(BuildContext context) {
    final child = ClipRRect(
      borderRadius: borderRadius(widget.isISend),
      child: SizedBox(width: _trulyWidth, height: _trulyHeight, child: _child),
    );
    return child;
  }
}
