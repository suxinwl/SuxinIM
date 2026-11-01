// Isolated development test entrypoint. No credentials are embedded in this file.
import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'dart:ui' as ui;
import 'package:app_im/app_im.dart';
import 'package:crypto/crypto.dart';
import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:path_provider/path_provider.dart';
import 'package:video_compress/video_compress.dart';
import 'package:video_player/video_player.dart';

void main() async {
  WidgetsFlutterBinding.ensureInitialized();
  runApp(const MaterialApp(
      home: Scaffold(
          body: Center(child: Text('OpenIM38 native SDK verification')))));
  final report = <String, dynamic>{
    'sdk': AppIm.sdkVersion,
    'steps': <String>[]
  };
  void step(String value) {
    (report['steps'] as List).add(value);
    print('OPENIM38_PROBE $value');
  }

  try {
    if (!const bool.fromEnvironment('OPENIM_DEVELOPMENT'))
      throw StateError('development flavor required');
    final ca = await rootBundle
        .load('packages/openim_common/assets/certificates/openim38_dev_ca.crt');
    SecurityContext.defaultContext
        .setTrustedCertificatesBytes(ca.buffer.asUint8List());
    final raw = await const MethodChannel('openim38/native_probe')
        .invokeMethod<String>('configuration');
    if (raw == null)
      throw StateError('runtime synthetic configuration missing');
    final c = jsonDecode(raw) as Map<String, dynamic>;
    final base = c['baseURL'] as String,
        a = c['sender'] as Map,
        b = c['receiver'] as Map;
    final dio = Dio(BaseOptions(
        connectTimeout: const Duration(seconds: 30),
        receiveTimeout: const Duration(seconds: 60)));
    Future<Map> login(Map user) async {
      final response = await dio.post('$base/chat/account/login',
          data: {
            'email': user['email'],
            'password':
                md5.convert(utf8.encode(c['password'] as String)).toString(),
            'platform': 2
          },
          options: Options(headers: {
            'operationID':
                'android-native-probe-${DateTime.now().millisecondsSinceEpoch}'
          }));
      final body = response.data as Map;
      if (body['errCode'] != 0)
        throw StateError('Chat login failed (${body['errCode']})');
      return body['data'] as Map;
    }

    final certA = await login(a), certB = await login(b);
    step('chat_https_login_pass');
    final im = AppIm.iMManager;
    final dir = await getApplicationSupportDirectory();
    final nativeDir = Directory('${dir.path}/native-probe');
    await nativeDir.create(recursive: true);
    var connected = Completer<void>();
    var checkingRevoked = false;
    final revokedConnection = Completer<void>();
    void revokedDenied() {
      if (checkingRevoked && !revokedConnection.isCompleted)
        revokedConnection.complete();
    }

    await im.initSDK(
        platformID: 2,
        apiAddr: '$base/api',
        wsAddr: '${base.replaceFirst('https://', 'wss://')}/ws',
        dataDir: nativeDir.path,
        logLevel: 1,
        isLogStandardOutput: false,
        listener: OnConnectListener(
            onConnectSuccess: () {
              if (checkingRevoked && !revokedConnection.isCompleted) {
                revokedConnection.completeError(
                    StateError('revoked token opened a WebSocket'));
                return;
              }
              if (!connected.isCompleted) connected.complete();
            },
            onConnectFailed: (code, msg) {
              if (checkingRevoked) {
                revokedDenied();
                return;
              }
              if (!connected.isCompleted)
                connected.completeError(
                    StateError('native connect failed ($code): $msg'));
            },
            onKickedOffline: revokedDenied,
            onUserTokenInvalid: revokedDenied,
            onUserTokenExpired: revokedDenied));
    await im.userManager.setUserListener(OnUserListener());
    await im.groupManager.setGroupListener(OnGroupListener());
    await im.friendshipManager.setFriendshipListener(OnFriendshipListener());
    await im.messageManager.setAdvancedMsgListener(OnAdvancedMsgListener());
    var synced = Completer<void>();
    await im.conversationManager.setConversationListener(
        OnConversationListener(onSyncServerFinish: (_) {
      if (!synced.isCompleted) synced.complete();
    }, onSyncServerFailed: (_) {
      if (!synced.isCompleted)
        synced.completeError(StateError('native sync failed'));
    }));
    await im
        .login(userID: certA['userID'], token: certA['imToken'])
        .timeout(const Duration(seconds: 90));
    await connected.future.timeout(const Duration(seconds: 60));
    await synced.future.timeout(const Duration(seconds: 90));
    step('native_go_tls_ws_login_and_sync_pass');
    final marker = 'openim38-native-${DateTime.now().millisecondsSinceEpoch}';
    final text = await im.messageManager.createTextMessage(text: marker);
    final sentText = await im.messageManager.sendMessage(
        message: text,
        userID: certB['userID'],
        offlinePushInfo: OfflinePushInfo());
    if (sentText.status != 2)
      throw StateError('text status ${sentText.status}');
    step('native_text_send_pass');
    final payload = utf8
        .encode('$marker\n${List.filled(8192, 'attachment-integrity').join()}');
    final file = File('${nativeDir.path}/$marker.txt');
    await file.writeAsBytes(payload);
    final fileMsg = await im.messageManager.createFileMessageFromFullPath(
        filePath: file.path, fileName: '$marker.txt');
    final sentFile = await im.messageManager
        .sendMessage(
            message: fileMsg,
            userID: certB['userID'],
            offlinePushInfo: OfflinePushInfo())
        .timeout(const Duration(seconds: 120));
    if (sentFile.status != 2 || (sentFile.fileElem?.sourceUrl?.isEmpty ?? true))
      throw StateError('file upload not completed');
    step('native_s3_upload_tls_pass');
    final sentMessages = <Message>[sentText, sentFile];
    Future<Message> send(Message message) async {
      final sent = await im.messageManager.sendMessage(
          message: message,
          userID: certB['userID'],
          offlinePushInfo: OfflinePushInfo());
      if (sent.status != 2) throw StateError('message status ${sent.status}');
      sentMessages.add(sent);
      return sent;
    }

    await send(await im.messageManager.createLocationMessage(
        latitude: 22.54,
        longitude: 114.06,
        description:
            jsonEncode({'name': 'Synthetic location', 'addr': marker})));
    await send(await im.messageManager.createCardMessage(
        userID: certA['userID'], nickname: 'Synthetic Android sender'));
    await send(await im.messageManager
        .createQuoteMessage(text: '$marker quote', quoteMsg: sentText));
    await send(await im.messageManager.createMergerMessage(
        messageList: [sentText, sentFile],
        title: 'Synthetic merged messages',
        summaryList: [marker, 'attachment']));
    await send(await im.messageManager.createForwardMessage(message: sentText));
    final customPayload = jsonEncode({
      'customType': 901,
      'data': {'type': 'video', 'state': 'hangup', 'duration': 3}
    });
    final custom = await send(await im.messageManager.createCustomMessage(
        data: customPayload,
        extension: '',
        description: 'Synthetic call record'));
    step('native_location_card_quote_merge_forward_send_pass');
    final mediaExpected = <String, String>{};
    if (c['fixtureDir'] is String) {
      final fixture = c['fixtureDir'] as String;
      final png = File('$fixture/synthetic.png');
      final image = await send(await im.messageManager
          .createImageMessageFromFullPath(imagePath: png.path));
      mediaExpected[image.clientMsgID!] =
          sha256.convert(await png.readAsBytes()).toString();
      final wav = File('$fixture/synthetic.wav');
      final sound = await send(await im.messageManager
          .createSoundMessageFromFullPath(soundPath: wav.path, duration: 1));
      mediaExpected[sound.clientMsgID!] =
          sha256.convert(await wav.readAsBytes()).toString();
      final media = <Map<String, dynamic>>[];
      report['inputVideoOrientation'] = <String, dynamic>{};
      for (final name in ['synthetic.mp4', 'synthetic-rotate90.mp4']) {
        final original = File('$fixture/$name');
        final before = await VideoCompress.getMediaInfo(original.path);
        (report['inputVideoOrientation'] as Map)[name] = before.orientation;
        if (name == 'synthetic-rotate90.mp4' &&
            (before.orientation ?? 0) % 180 != 90)
          throw StateError(
              'rotation fixture must contain an actual 90 degree display matrix');
        final compressed = await VideoCompress.compressVideo(original.path,
            quality: VideoQuality.MediumQuality,
            includeAudio: true,
            deleteOrigin: false);
        if (compressed == null ||
            compressed.isCancel == true ||
            compressed.path == null)
          throw StateError('native compression cancelled unexpectedly');
        final output = File(compressed.path!);
        final thumbnail =
            await VideoCompress.getFileThumbnail(output.path, quality: 80);
        if (!await thumbnail.exists() || await thumbnail.length() == 0)
          throw StateError('thumbnail missing');
        final thumbnailCodec =
            await ui.instantiateImageCodec(await thumbnail.readAsBytes());
        final thumbnailFrame = await thumbnailCodec.getNextFrame();
        final thumbnailWidth = thumbnailFrame.image.width;
        final thumbnailHeight = thumbnailFrame.image.height;
        thumbnailFrame.image.dispose();
        thumbnailCodec.dispose();
        if (name == 'synthetic-rotate90.mp4' &&
            thumbnailWidth >= thumbnailHeight) {
          throw StateError('Rotated video thumbnail lost portrait orientation');
        }
        final player = VideoPlayerController.file(output);
        await player.initialize().timeout(const Duration(seconds: 30));
        if (player.value.duration.inMilliseconds < 1500 ||
            player.value.size.isEmpty)
          throw StateError('compressed video metadata invalid');
        if (name == 'synthetic-rotate90.mp4' &&
            player.value.size.width >= player.value.size.height &&
            player.value.rotationCorrection % 180 != 90)
          throw StateError('90 degree video lost its portrait orientation');
        await player.play();
        await Future<void>.delayed(const Duration(milliseconds: 500));
        if (player.value.hasError)
          throw StateError('compressed video decode failed');
        media.add({
          'fixture': name,
          'inputOrientation': before.orientation,
          'outputOrientation': compressed.orientation,
          'width': player.value.size.width,
          'height': player.value.size.height,
          'rotationCorrection': player.value.rotationCorrection,
          'durationMs': player.value.duration.inMilliseconds,
          'thumbnailBytes': await thumbnail.length(),
          'thumbnailPath': thumbnail.path,
          'outputPath': output.path,
          'thumbnailWidth': thumbnailWidth,
          'thumbnailHeight': thumbnailHeight,
          'originalRetained': await original.exists()
        });
        await player.dispose();
        final video = await send(await im.messageManager
            .createVideoMessageFromFullPath(
                videoPath: output.path,
                videoType: 'mp4',
                duration: ((compressed.duration ?? 0) / 1000).round(),
                snapshotPath: thumbnail.path));
        mediaExpected[video.clientMsgID!] =
            sha256.convert(await output.readAsBytes()).toString();
      }
      report['media'] = media;
      step(
          'native_image_sound_video_rotation_compression_thumbnail_decode_send_pass');
      final cancelInput = File('$fixture/synthetic-cancel.mp4');
      if (await cancelInput.exists()) {
        final pending = VideoCompress.compressVideo(cancelInput.path,
            quality: VideoQuality.LowQuality,
            deleteOrigin: false,
            includeAudio: true);
        await Future<void>.delayed(const Duration(milliseconds: 80));
        await VideoCompress.cancelCompression();
        final cancelled = await pending.timeout(const Duration(seconds: 30));
        if (cancelled != null && cancelled.isCancel != true)
          throw StateError('compression completed before cancellation');
        if (!await cancelInput.exists())
          throw StateError('cancel deleted original');
        step('native_compression_cancel_and_original_retained_pass');
      }
    }
    if (c['crossReceiver'] is Map) {
      final target = c['crossReceiver'] as Map;
      final cross = <Map<String, dynamic>>[];
      for (final sent in sentMessages.toList()) {
        final forwarded =
            await im.messageManager.createForwardMessage(message: sent);
        final result = await im.messageManager.sendMessage(
            message: forwarded,
            userID: target['userID'],
            offlinePushInfo: OfflinePushInfo());
        if (result.status != 2) throw StateError('cross-platform send failed');
        cross.add({
          'clientMsgID': result.clientMsgID,
          'contentType': result.contentType,
          if (mediaExpected[sent.clientMsgID] != null)
            'sha256': mediaExpected[sent.clientMsgID],
          if (sent.clientMsgID == sentFile.clientMsgID)
            'sha256': sha256.convert(payload).toString(),
        });
      }
      report['crossPlatformMessages'] = cross;
      step('native_to_web_all_types_sent_pass');
    }
    await im.logout();
    connected = Completer<void>();
    synced = Completer<void>();
    await im
        .login(userID: certB['userID'], token: certB['imToken'])
        .timeout(const Duration(seconds: 90));
    await connected.future.timeout(const Duration(seconds: 60));
    await synced.future.timeout(const Duration(seconds: 90));
    final conv = await im.conversationManager
        .getOneConversation(sourceID: certA['userID'], sessionType: 1);
    final history = await im.messageManager.getAdvancedHistoryMessageList(
        conversationID: conv.conversationID, count: 100);
    if (!(history.messageList ?? [])
        .any((m) => m.clientMsgID == sentText.clientMsgID))
      throw StateError('receiver text history missing');
    final received = (history.messageList ?? [])
        .where((m) => m.clientMsgID == sentFile.clientMsgID)
        .first;
    final download = await dio.get<List<int>>(received.fileElem!.sourceUrl!,
        options: Options(responseType: ResponseType.bytes));
    if (sha256.convert(download.data!).toString() !=
        sha256.convert(payload).toString())
      throw StateError('attachment hash mismatch');
    step('receiver_history_and_attachment_sha256_pass');
    final historyMessages = history.messageList ?? [];
    if (historyMessages
            .firstWhere((m) => m.clientMsgID == custom.clientMsgID)
            .customElem
            ?.data !=
        customPayload) {
      throw StateError('Custom subtype payload was not preserved');
    }
    report['customMessageSubtype'] = {
      'contentType': 110,
      'customType': 901,
      'payloadPreserved': true
    };
    for (final sent in sentMessages) {
      final match =
          historyMessages.where((m) => m.clientMsgID == sent.clientMsgID);
      if (match.isEmpty || match.first.contentType != sent.contentType)
        throw StateError(
            'receiver typed history missing (${sent.contentType})');
      final expected = mediaExpected[sent.clientMsgID];
      if (expected != null) {
        final m = match.first;
        final url = m.pictureElem?.sourcePicture?.url ??
            m.soundElem?.sourceUrl ??
            m.videoElem?.videoUrl;
        if (url == null || !url.startsWith('https://'))
          throw StateError('media HTTPS URL missing');
        final bytes = await dio.get<List<int>>(url,
            options: Options(responseType: ResponseType.bytes));
        if (sha256.convert(bytes.data!).toString() != expected)
          throw StateError(
              'media download checksum mismatch (${m.contentType})');
      }
    }
    report['receivedContentTypes'] = historyMessages
        .where((m) => sentMessages.any((s) => s.clientMsgID == m.clientMsgID))
        .map((m) => m.contentType)
        .toList();
    step('receiver_all_message_types_and_media_sha256_pass');
    final conversations = await im.conversationManager
        .getConversationListSplit(offset: 0, count: 100);
    final oa = conversations.where(
        (v) => v.conversationType == 4 && v.userID == 'upgrade38_oa_fixture');
    if (oa.isEmpty || oa.first.latestMsg?.contentType != 1400)
      throw StateError('OA session4/content1400 missing');
    report['oa'] = {
      'conversationType': oa.first.conversationType,
      'contentType': oa.first.latestMsg?.contentType
    };
    step('native_oa_session4_content1400_parse_pass');
    await im.friendshipManager.setFriendRemark(
        userID: certA['userID'], remark: 'native-remark-check');
    await Future<void>.delayed(const Duration(seconds: 1));
    final namedFriend = await im.friendshipManager
        .getFriendsInfo(userIDList: [certA['userID']]);
    await im.friendshipManager
        .setFriendRemark(userID: certA['userID'], remark: '');
    await Future<void>.delayed(const Duration(seconds: 1));
    final clearedFriend = await im.friendshipManager
        .getFriendsInfo(userIDList: [certA['userID']]);
    final remarkPassed = namedFriend.single.remark == 'native-remark-check' &&
        (clearedFriend.single.remark ?? '').isEmpty;
    report['sameInstanceFriendRemarkSetAndClearPassed'] = remarkPassed;
    step(remarkPassed
        ? 'native_same_instance_friend_remark_set_clear_pass'
        : 'native_same_instance_friend_remark_clear_failed');
    final globalOptions = <int?>[];
    try {
      for (final option in [0, 2, 0]) {
        await im.userManager.setSelfInfo(globalRecvMsgOpt: option);
        await Future<void>.delayed(const Duration(milliseconds: 500));
        globalOptions
            .add((await im.userManager.getSelfUserInfo()).globalRecvMsgOpt);
      }
    } finally {
      await im.userManager.setSelfInfo(globalRecvMsgOpt: 0);
    }
    report['globalDoNotDisturbReadback'] = globalOptions;
    if (globalOptions.join(',') != '0,2,0') {
      throw StateError('Global do-not-disturb did not read back 0,2,0');
    }
    step('native_global_do_not_disturb_0_2_0_readback_pass');
    final previousBlack = (await im.friendshipManager.getBlacklist()).any(
        (entry) =>
            entry.userID == certA['userID'] ||
            entry.blockUserID == certA['userID']);
    try {
      await im.friendshipManager.addBlacklist(userID: certA['userID']);
      final hidden = await im.friendshipManager
          .getFriendsInfo(userIDList: [certA['userID']], filterBlack: true);
      final retained = await im.friendshipManager
          .getFriendsInfo(userIDList: [certA['userID']], filterBlack: false);
      if (hidden.isNotEmpty || retained.length != 1) {
        throw StateError(
            'Native friend blacklist filtering did not preserve requested semantics');
      }
      report['friendBlacklistFilter'] = {
        'filterTrueCount': hidden.length,
        'filterFalseCount': retained.length,
        'restoresPreviousState': true
      };
    } finally {
      if (!previousBlack)
        await im.friendshipManager.removeBlacklist(userID: certA['userID']);
    }
    step('native_friend_blacklist_filter_and_restore_pass');
    final logoutHeaders = Options(headers: {
      'token': certB['imToken'],
      'chatToken': certB['chatToken'],
      'platformID': '2',
      'operationID': marker
    });
    final revoke = await dio.post('$base/enterprise/v1/session/logout',
        data: {}, options: logoutHeaders);
    if (revoke.data['errCode'] != 0 || revoke.data['data']['revoked'] != true)
      throw StateError('session logout did not revoke');
    // Successful revocation can kick the socket before the HTTP reply arrives.
    try {
      await im.logout();
    } catch (_) {}
    final repeated = await dio.post('$base/enterprise/v1/session/logout',
        data: {}, options: logoutHeaders);
    if (repeated.data['errCode'] != 0 ||
        repeated.data['data']['revoked'] != true)
      throw StateError('session logout repeat not idempotent');
    var enterpriseDenied = false;
    try {
      final old = await dio.get('$base/enterprise/v1/rtc/events?after=-1',
          options: logoutHeaders);
      enterpriseDenied = old.data['errCode'] != 0;
    } on DioException catch (e) {
      enterpriseDenied = e.response?.statusCode == 401;
    }
    if (!enterpriseDenied)
      throw StateError('revoked enterprise pair still accepted');
    final apiOptions =
        Options(headers: {'token': certB['imToken'], 'operationID': marker});
    var apiDenied = false;
    try {
      final apiCheck = await dio.post('$base/api/user/get_users_info',
          data: {
            'userIDs': [certB['userID']]
          },
          options: apiOptions);
      apiDenied = apiCheck.data['errCode'] != 0;
      report['revokedIMAPIErrorCode'] = apiCheck.data['errCode'];
    } on DioException catch (error) {
      apiDenied = [401, 403].contains(error.response?.statusCode);
      report['revokedIMAPIHTTPStatus'] = error.response?.statusCode;
      if (!apiDenied) rethrow;
    }
    if (!apiDenied) throw StateError('revoked native API token still accepted');
    AppIm.iMManager.isLogined = false;
    AppIm.iMManager.token = null;
    await Future<void>.delayed(const Duration(seconds: 1));
    checkingRevoked = true;
    try {
      await im
          .login(
              userID: certB['userID'],
              token: certB['imToken'],
              checkLoginStatus: false)
          .timeout(const Duration(seconds: 30));
    } catch (_) {
      revokedDenied();
    }
    // Cached offline self-info can make login() return before network validation.
    // The security property is API rejection and inability to open the socket.
    await revokedConnection.future.timeout(const Duration(seconds: 30));
    step(
        'native_explicit_logout_idempotent_enterprise_and_sdk_old_token_denied_pass');
    if (!remarkPassed)
      throw StateError(
          'Native friend remark clear did not converge in the same SDK instance');
    report['passed'] = true;
  } catch (error) {
    report['passed'] = false;
    report['errorType'] = error.runtimeType.toString();
    report['error'] = error
        .toString()
        .replaceAll(RegExp(r'eyJ[A-Za-z0-9_.-]+'), '[redacted]');
  }
  final path = (await getApplicationSupportDirectory()).path;
  await File('$path/native-probe-report.json')
      .writeAsString(jsonEncode(report));
  print('OPENIM38_PROBE_RESULT ${jsonEncode(report)}');
}
