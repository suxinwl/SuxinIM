// Real toolbar, ChatLogic picker/send methods, PhotoManager permission check,
// native-vault generation and SDK message creation. Only native transports and
// picker selection results are synthetic; no device, account login or upload.
import 'dart:async';
import 'dart:convert';
import 'dart:typed_data';
import 'package:file_picker/file_picker.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_easyloading/flutter_easyloading.dart';
import 'package:flutter_image_compress/flutter_image_compress.dart';
import 'package:flutter_screenutil/flutter_screenutil.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:get/get.dart';
import 'package:openim/core/controller/app_controller.dart';
import 'package:openim/core/controller/im_controller.dart';
import 'package:openim/core/session_epoch.dart';
import 'package:openim/pages/chat/chat_logic.dart';
import 'package:openim/pages/conversation/conversation_logic.dart';
import 'package:openim_common/openim_common.dart';
import 'package:wechat_assets_picker/wechat_assets_picker.dart';

class _Inert extends Fake {
  final onStart = InternalFinalCallback<void>(callback: () {});
  final onDelete = InternalFinalCallback<void>(callback: () {});
}

class _IM extends _Inert implements IMController {
  @override
  final session = SessionEpoch();
}

class _App extends _Inert implements AppController {}

class _Conversation extends _Inert implements ConversationLogic {}

class _Cache extends _Inert implements CacheController {}

class _Download extends _Inert implements DownloadController {}

class _Texts extends Translations {
  @override
  Map<String, Map<String, String>> get keys => {
        'zh_CN': {
          'toolboxAlbum': '相册',
          'toolboxCamera': '拍摄',
          'toolboxCall': '通话',
          'toolboxFile': '文件',
          'toolboxCard': '名片',
          'toolboxLocation': '位置'
        }
      };
}

class _Chat extends ChatLogic {
  bool closed = false;
  @override
  bool get isClosed => closed;
  @override
  void scrollBottom() {}
}

class _PickerChat extends _Chat {
  final selected = <String>[];
  Future<void> Function()? sendBarrier;
  @override
  Future<void> sendPicture({required String path}) async {
    selected.add(path);
    await sendBarrier?.call();
  }

  @override
  Future<void> sendFile(
      {required String filePath, required String fileName}) async {
    selected.add(filePath);
    await sendBarrier?.call();
  }
}

class _Files extends FilePicker {
  int calls = 0;
  Future<FilePickerResult?> Function()? next;
  @override
  Future<FilePickerResult?> pickFiles(
      {String? dialogTitle,
      String? initialDirectory,
      FileType type = FileType.any,
      List<String>? allowedExtensions,
      Function(FilePickerStatus)? onFileLoading,
      bool allowCompression = false,
      int compressionQuality = 0,
      bool allowMultiple = false,
      bool withData = false,
      bool withReadStream = false,
      bool lockParentWindow = false,
      bool readSequential = false}) async {
    calls++;
    expect(type, FileType.any);
    expect(withData, isFalse);
    return await next?.call();
  }
}

class _Album extends AssetPickerDelegate {
  int calls = 0;
  List<AssetEntity>? result;
  @override
  Future<List<AssetEntity>?> pickAssets(BuildContext context,
      {Key? key,
      AssetPickerConfig pickerConfig = const AssetPickerConfig(),
      PermissionRequestOption? permissionRequestOption,
      bool useRootNavigator = true,
      AssetPickerPageRouteBuilder<List<AssetEntity>>? pageRouteBuilder}) async {
    calls++;
    // Execute the actual package permissionCheck (authorized OR limited).
    await permissionCheck(
        requestOption: permissionRequestOption ??
            PermissionRequestOption(
                androidPermission: AndroidPermission(
                    type: pickerConfig.requestType, mediaLocation: false)));
    return result;
  }
}

class _Compression extends UnsupportedFlutterImageCompress {
  Completer<Uint8List>? barrier;
  final entered = Completer<void>();
  @override
  Future<Uint8List> compressWithList(Uint8List image,
      {int minWidth = 1920,
      int minHeight = 1080,
      int quality = 95,
      int rotate = 0,
      int inSampleSize = 1,
      bool autoCorrectionAngle = true,
      CompressFormat format = CompressFormat.jpeg,
      bool keepExif = false}) async {
    if (!entered.isCompleted) entered.complete();
    return barrier == null
        ? Uint8List.fromList([1, 2, 3])
        : await barrier!.future;
  }
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  final messenger =
      TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
  late _IM im;
  late _Files files;
  late _Album album;
  late _Compression compression;
  late List<String> nativeCalls, sdkCalls;
  late PermissionState photos;
  late int legacyPermissions;
  Future<dynamic> Function(MethodCall)? sdkReply;
  LoginCertificate certificate([String account = 'synthetic-picker']) =>
      LoginCertificate.fromJson({
        'userID': account,
        'imToken': 'synthetic-im',
        'chatToken': 'synthetic-chat'
      });
  Future<void> activate() async {
    final a = im.session
        .begin('synthetic-picker', 'synthetic-im', 'synthetic-chat', 2);
    im.session.activate(a);
    await NativeLocalVault.unlock(certificate());
  }

  Future<void> mount(WidgetTester t, {Widget? child}) async {
    t.view.devicePixelRatio = 1;
    t.view.physicalSize = const Size(375, 812);
    addTearDown(t.view.resetDevicePixelRatio);
    addTearDown(t.view.resetPhysicalSize);
    await t.pumpWidget(ScreenUtilInit(
        designSize: const Size(375, 812),
        builder: (_, __) => GetMaterialApp(
            translations: _Texts(),
            locale: const Locale('zh', 'CN'),
            builder: EasyLoading.init(),
            home: Scaffold(body: child ?? const SizedBox()))));
    await t.pump();
  }

  setUp(() async {
    Get.testMode = true;
    nativeCalls = [];
    sdkCalls = [];
    legacyPermissions = 0;
    photos = PermissionState.authorized;
    sdkReply = null;
    messenger.setMockMethodCallHandler(
        const MethodChannel('com.ryanheise.audio_session'), (_) async => null);
    messenger.setMockMethodCallHandler(
        const MethodChannel('flutter.baseflow.com/permissions/methods'),
        (c) async {
      legacyPermissions++;
      return <int, int>{15: 0, 9: 0};
    });
    messenger.setMockMethodCallHandler(
        const MethodChannel('com.fluttercandies/photo_manager'), (c) async {
      expect(c.method, 'requestPermissionExtend');
      return photos.index;
    });
    messenger.setMockMethodCallHandler(NativeLocalVault.channel, (c) async {
      nativeCalls.add(c.method);
      if (c.method == 'unlock')
        return {'userID': (c.arguments as Map)['userID'], 'actorGeneration': 1};
      if (c.method == 'source/import') {
        final path = (c.arguments as Map)['path'] as String;
        return path.startsWith('/vault/')
            ? path
            : '/vault/outgoing/${path.split('/').last}';
      }
      if (c.method == 'source/info')
        return {'name': 'synthetic.png', 'size': 3};
      if (c.method == 'source/read') return Uint8List.fromList([1, 2, 3]);
      if (c.method == 'source/fromBytes')
        return '/vault/outgoing/compressed.png';
      if (c.method == 'source/discard' || c.method == 'lock') return true;
      throw StateError('Unexpected native operation');
    });
    messenger.setMockMethodCallHandler(
        const MethodChannel('flutter_openim_sdk'), (c) async {
      sdkCalls.add(c.method);
      if (sdkReply != null) return await sdkReply!(c);
      throw StateError('No SDK action permitted');
    });
    im = _IM();
    Get.put<IMController>(im);
    Get.put<AppController>(_App());
    Get.put<ConversationLogic>(_Conversation());
    Get.put<CacheController>(_Cache());
    Get.put<DownloadController>(_Download());
    files = _Files();
    FilePicker.platform = files;
    album = _Album();
    AssetPicker.setPickerDelegate(album);
    compression = _Compression();
    FlutterImageCompressPlatform.instance = compression;
    await activate();
  });
  tearDown(() async {
    await EasyLoading.dismiss(animation: false);
    await NativeLocalVault.lock();
    AssetPicker.setPickerDelegate(const AssetPickerDelegate());
    Get.reset();
  });
  testWidgets(
      'toolbar album and file callbacks are not gated by denied legacy storage permissions',
      (t) async {
    int albums = 0, documents = 0;
    await mount(t,
        child: ChatToolBox(
            onTapAlbum: () => albums++, onTapFile: () => documents++));
    await t.tap(find.text(StrRes.toolboxAlbum));
    await t.tap(find.text(StrRes.toolboxFile));
    await t.pump();
    expect(albums, 1);
    expect(documents, 1);
    // Image and label share exactly one handler; no nested duplicate callback.
    await t.tapAt(t.getTopLeft(
            find.byKey(ValueKey('chat-toolbox-${StrRes.toolboxAlbum}'))) +
        const Offset(35, 25));
    await t.tapAt(t.getTopLeft(
            find.byKey(ValueKey('chat-toolbox-${StrRes.toolboxFile}'))) +
        const Offset(35, 25));
    expect(albums, 2);
    expect(documents, 2);
    expect(legacyPermissions, 0);
    await mount(t,
        child: ChatToolBox(
            attachmentBusy: true,
            onTapAlbum: () => albums++,
            onTapFile: () => documents++));
    await t.tap(find.byKey(ValueKey('chat-toolbox-${StrRes.toolboxAlbum}')));
    await t.tap(find.byKey(ValueKey('chat-toolbox-${StrRes.toolboxFile}')));
    expect(albums, 2);
    expect(documents, 2);
  });
  testWidgets(
      'limited photo access sends only selected encrypted sources and awaits processing',
      (t) async {
    await mount(t);
    photos = PermissionState.limited;
    album.result = [
      NativeMediaAsset(
          {'path': '/selected/photo.png', 'kind': 'image', 'name': 'photo.png'})
    ];
    final chat = _PickerChat();
    final barrier = Completer<void>();
    chat.sendBarrier = () => barrier.future;
    final pending = chat.onTapAlbum();
    await t.pump();
    expect(chat.selected, ['/vault/outgoing/photo.png']);
    expect(chat.pickingAttachment.value, isTrue);
    await chat.onTapAlbum();
    await chat.onTapFile();
    expect(album.calls, 1);
    expect(files.calls, 0);
    barrier.complete();
    await pending;
    expect(chat.pickingAttachment.value, isFalse);
    expect(nativeCalls, contains('source/import'));
    expect(
        nativeCalls.where(
            (x) => x.startsWith('attachment/') || x.startsWith('archive/')),
        isEmpty);
  });
  testWidgets(
      'denied photos show an error and allow a later authorized retry; cancellation is harmless',
      (t) async {
    await mount(t);
    final chat = _PickerChat();
    photos = PermissionState.denied;
    await chat.onTapAlbum();
    await t.pump();
    expect(chat.selected, isEmpty);
    expect(chat.pickingAttachment.value, isFalse);
    expect(find.text('相册未能打开或读取，请检查照片权限后重试'), findsOneWidget);
    await EasyLoading.dismiss(animation: false);
    photos = PermissionState.authorized;
    album.result = null;
    await chat.onTapAlbum();
    expect(album.calls, 2);
    expect(chat.selected, isEmpty);
    await chat.onTapFile();
    expect(files.calls, 1);
    expect(chat.pickingAttachment.value, isFalse);
  });
  testWidgets(
      'file error releases busy state and retry securely imports every selection',
      (t) async {
    await mount(t);
    final chat = _PickerChat();
    files.next =
        () async => throw PlatformException(code: 'SYNTHETIC_PICKER_FAILURE');
    await chat.onTapFile();
    await t.pump();
    expect(chat.pickingAttachment.value, isFalse);
    expect(find.text('文件选择或读取未完成，请重试'), findsOneWidget);
    await EasyLoading.dismiss(animation: false);
    files.next = () async => FilePickerResult([
          PlatformFile(name: '职员名单.xlsx', size: 3, path: '/selected/职员名单.xlsx'),
          PlatformFile(name: 'photo.png', size: 3, path: '/selected/photo.png')
        ]);
    await chat.onTapFile();
    expect(chat.selected,
        ['/vault/outgoing/职员名单.xlsx', '/vault/outgoing/photo.png']);
    expect(files.calls, 2);
    expect(legacyPermissions, 0);
  });
  for (final invalidation in ['logout', 'account-change', 'page-close']) {
    Future<void> invalidate(_Chat chat) async {
      if (invalidation == 'logout') {
        im.session.invalidate();
        await NativeLocalVault.lock();
      }
      if (invalidation == 'account-change')
        await NativeLocalVault.unlock(certificate('synthetic-other'));
      if (invalidation == 'page-close') chat.closed = true;
    }

    testWidgets('file message creation cannot send late after $invalidation',
        (t) async {
      await mount(t);
      final chat = _Chat();
      final reply = Completer<String>(), entered = Completer<void>();
      sdkReply = (c) async {
        entered.complete();
        return reply.future;
      };
      final pending = chat.sendFile(
          filePath: '/selected/report.xlsx', fileName: 'report.xlsx');
      for (var i = 0; i < 10 && !entered.isCompleted; i++) {
        await t.pump(const Duration(milliseconds: 10));
      }
      expect(entered.isCompleted, isTrue,
          reason:
              'native=$nativeCalls sdk=$sdkCalls active=${im.session.active} vault=${NativeLocalVault.unlocked}');
      await invalidate(chat);
      reply.complete(
          jsonEncode({'clientMsgID': 'synthetic-message', 'contentType': 105}));
      await pending;
      expect(sdkCalls, ['createFileMessageFromFullPath']);
      expect(chat.messageList, isEmpty);
    });
    testWidgets(
        'image compression cannot create or send late after $invalidation',
        (t) async {
      await mount(t);
      final chat = _Chat();
      compression.barrier = Completer<Uint8List>();
      final pending = chat.sendPicture(path: '/selected/photo.png');
      for (var i = 0; i < 10 && !compression.entered.isCompleted; i++) {
        await t.pump(const Duration(milliseconds: 10));
      }
      expect(compression.entered.isCompleted, isTrue,
          reason: 'native=$nativeCalls sdk=$sdkCalls');
      await invalidate(chat);
      compression.barrier!.complete(Uint8List.fromList([4, 5, 6]));
      await pending;
      expect(sdkCalls, isEmpty);
      expect(chat.messageList, isEmpty);
    });
  }
}
