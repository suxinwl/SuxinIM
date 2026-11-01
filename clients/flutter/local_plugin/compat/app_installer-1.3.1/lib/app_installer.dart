import 'dart:async';
import 'package:flutter/foundation.dart';
import 'package:flutter/services.dart';

class AppInstaller {
  static const MethodChannel _channel = const MethodChannel('app_installer');

  ///去应用商店
  ///[androidAppId] Android package name
  ///[iOSAppId] iOS Bundle Id
  ///[macOSAppId] macOS Bundle Id
  ///[review] iOS App Store evaluation
  static Future<void> goStore(String androidAppId, String iOSAppId,
      {String? macOSAppId, bool review = false}) async {
    await _channel.invokeMethod('goStore', {
      'androidAppId': androidAppId,
      'iOSAppId': iOSAppId,
      'macOSAppId': macOSAppId,
      'review': review,
    });
  }

  ///安装 Apk
  ///[apkPath] Apk file path
  static Future<void> installApk(String apkPath) async {
    //判断 Android 平台
    if (defaultTargetPlatform == TargetPlatform.android) {
      final accepted = await _channel.invokeMethod<bool>('installApk', {'apkPath': apkPath});
      if (accepted != true) {
        throw PlatformException(code: 'INSTALLER_REJECTED', message: 'APK installer did not accept the package');
      }
    }
  }
}
