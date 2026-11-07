import 'dart:io';
import 'enterprise_lan_tls.dart';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_bugly/flutter_bugly.dart';
import 'package:app_im/app_im.dart';
import 'package:hive_flutter/hive_flutter.dart';
import 'package:openim_common/openim_common.dart';
import 'package:path_provider/path_provider.dart';

class Config {
  //初始化全局信息
  static Future init(Function() runApp) async {
    WidgetsFlutterBinding.ensureInitialized();
    NativeLocalVault.install();
    final ca = await rootBundle.load('packages/openim_common/assets/certificates/puty_enterprise_root_ca.crt');
    SecurityContext.defaultContext.setTrustedCertificatesBytes(ca.buffer.asUint8List());
    EnterpriseLanTls.instance.install(()=>serverBaseUrl);
    try {
      final path = (await getApplicationDocumentsDirectory()).path;
      cachePath = '$path/';
      await DataSp.init();
      EnterpriseLanTls.instance.profileReady();
      await Hive.initFlutter(path);
      await CacheController.prepareLegacyCaches();
      // await SpeechToTextUtil.instance.initSpeech();
      HttpUtil.init();
    } catch (_) {
      // Continuing with uninitialized encrypted settings would silently fall
      // back to plaintext/default storage. Startup must fail closed instead.
      rethrow;
    }

    runApp();

    // 设置屏幕方向
    SystemChrome.setPreferredOrientations([
      DeviceOrientation.portraitUp,
      DeviceOrientation.portraitDown,
    ]);

    // 状态栏透明（Android）
    var brightness = Platform.isAndroid ? Brightness.dark : Brightness.light;
    SystemChrome.setSystemUIOverlayStyle(SystemUiOverlayStyle(
      statusBarColor: Colors.transparent,
      statusBarBrightness: brightness,
      statusBarIconBrightness: brightness,
    ));

    const androidCrashID = String.fromEnvironment('OPENIM_BUGLY_ANDROID_ID');
    const iosCrashID = String.fromEnvironment('OPENIM_BUGLY_IOS_ID');
    if ((Platform.isAndroid && androidCrashID.isNotEmpty) ||
        (Platform.isIOS && iosCrashID.isNotEmpty)) {
      FlutterBugly.init(androidAppId: androidCrashID, iOSAppId: iosCrashID);
    }
  }

  static late String cachePath;
  static const uiW = 375.0;
  static const uiH = 812.0;

  /// 默认公司配置
  static const String deptName = "OpenIM";
  static const String deptID = '0';

  /// 全局字体size
  static const double textScaleFactor = 1.0;

  // Server administrator secrets are never embedded in the client.

  ///务必更换为自己app的key
  static const mapKey = String.fromEnvironment('OPENIM_MAP_KEY');

  /// 离线消息默认类型
  static OfflinePushInfo offlinePushInfo = OfflinePushInfo(
    title: StrRes.offlineMessage,
    desc: "",
    iOSBadgeCount: true,
    iOSPushSound: '+1',
  );

  /// 二维码：scheme
  static const friendScheme = "io.openim.app/addFriend/";
  static const groupScheme = "io.openim.app/joinGroup/";

  /// ip
  /// web.rentsoft.cn
  /// 203.56.175.233
  static const baseUrl = String.fromEnvironment('OPENIM_BASE_URL',
      defaultValue: 'https://192.168.10.11:9885');
  static const lanOrigin = 'https://192.168.10.11:9885';
  static const wanOrigin = 'https://im.szputy.com:9885';
  static String get serverBaseUrl {
    final saved = DataSp.getServerConfig();
    final candidate = saved?['baseUrl'] ?? saved?['authUrl'];
    if (candidate is String) {
      final uri = Uri.tryParse(candidate);
      if (uri != null && [lanOrigin,wanOrigin].contains(uri.origin)) {
        return uri.origin;
      }
    }
    return Platform.isIOS ? wanOrigin : baseUrl;
  }

  static String get enterpriseUrl => '$serverBaseUrl/enterprise/v1';


  /// 服务器IP
  static String get serverIp {
    String? ip;
    var server = DataSp.getServerConfig();
    if (null != server) {
      ip = server['serverIP'];
      Logger.print('缓存serverIP: $ip');
    }
    return ip ?? Uri.parse(baseUrl).authority;
  }

  /// 商业版管理后台
  /// $apiScheme://$host/complete_admin/
  /// $apiScheme://$host:10009
  /// 端口：10009
  // static String get chatTokenUrl {
  //   String? url;
  //   var server = DataSp.getServerConfig();
  //   if (null != server) {
  //     url = server['chatTokenUrl'];
  //     Logger.print('缓存chatTokenUrl: $url');
  //   }
  //   return url ??
  //       (_isIP ? "http://$_host:10009" : "https://$_host/complete_admin");
  // }

  /// 登录注册手机验 证服务器地址
  /// $apiScheme://$host/chat/
  /// $apiScheme://$host:50008
  /// 端口：50008
  static String get appAuthUrl => '$serverBaseUrl/chat';

  /// IM sdk api地址
  /// $apiScheme://$host/api/
  /// $apiScheme://$host:50002
  /// 端口：50002
  static String get imApiUrl => '$serverBaseUrl/api';

  /// IM ws 地址
  /// $socketScheme://$host/ws
  /// $socketScheme://$host:50001
  /// 端口：50001
  static String get imWsUrl => "${serverBaseUrl.replaceFirst('https://', 'wss://')}/ws";

  /// 图片存储
  static String get objectStorage {
    String? storage;
    var server = DataSp.getServerConfig();
    if (null != server) {
      storage = server['objectStorage'];
      Logger.print('缓存objectStorage: $storage');
    }
    return storage ?? 'minio';
  }
}
