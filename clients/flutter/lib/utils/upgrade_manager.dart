import 'release_policy.dart';
import 'package:url_launcher/url_launcher.dart';
import 'dart:io';

import 'package:app_installer/app_installer.dart';
import 'package:flutter/material.dart';
import 'package:flutter_local_notifications/flutter_local_notifications.dart';
import 'package:get/get.dart';
import 'package:openim_common/openim_common.dart';
import 'package:package_info_plus/package_info_plus.dart';
import 'package:permission_handler/permission_handler.dart';
import 'package:rxdart/rxdart.dart';

import '../widgets/upgrade_view.dart';

class UpgradeManger {
  PackageInfo? packageInfo;
  UpgradeInfoV2? upgradeInfoV2;
  bool isShowUpgradeDialog = false;
  String? _deferredVersion;
  bool _downloading = false;
  final forcedUpdateGate = ValueNotifier<Widget?>(null);
  final subject = PublishSubject<double>();
  void closeSubject() {
    subject.close();
    forcedUpdateGate.dispose();
  }

  void ignoreUpdate() {
    if (upgradeInfoV2?.needForceUpdate == true) return;
    DataSp.putIgnoreVersion(
        '${upgradeInfoV2!.buildVersion}+${upgradeInfoV2!.buildVersionNo}');
    Get.back();
  }

  void laterUpdate() {
    if (upgradeInfoV2?.needForceUpdate == true) return;
    _deferredVersion =
        '${upgradeInfoV2!.buildVersion}+${upgradeInfoV2!.buildVersionNo}';
    Get.back();
  }

  Future<void> getAppInfo() async {
    packageInfo ??= await PackageInfo.fromPlatform();
  }

  Future<void> nowUpdate() async {
    if (_downloading || upgradeInfoV2 == null) return;
    final info = upgradeInfoV2!;
    final uri = Uri.tryParse(info.downloadURL ?? '');
    if (uri == null || uri.scheme != 'https') {
      IMViews.showToast('更新地址必须使用 HTTPS');
      return;
    }
    if (Platform.isIOS) {
      await launchUrl(uri, mode: LaunchMode.externalApplication);
      return;
    }
    _downloading = true;
    String? path;
    try {
      if (info.sha256 == null ||
          !RegExp(r'^[a-fA-F0-9]{64}$').hasMatch(info.sha256!) ||
          (info.size ?? 0) <= 0) {
        throw StateError('安装包校验元数据不完整');
      }
      path = await IMUtils.createTempFile(
          dir: 'apk',
          name: 'openim-${info.buildVersion}-${info.buildVersionNo}.apk');
      await HttpUtil.download(uri.toString(), cachePath: path,
          onProgress: (count, total) {
        if (total > 0 && !subject.isClosed) subject.add(count / total);
      });
      final file = File(path);
      await ReleasePolicy.verifyPackage(file, info.size!, info.sha256!);
      if (!await Permission.requestInstallPackages.isGranted)
        await Permission.requestInstallPackages.request();
      await AppInstaller.installApk(path);
    } catch (error) {
      if (path != null && await File(path).exists()) await File(path).delete();
      IMViews.showToast('更新失败，可重新点击更新：$error');
    } finally {
      _downloading = false;
    }
  }

  Future<void> checkUpdate() => _check(manual: true);
  Future<void> autoCheckVersionUpgrade() => _check();
  Future<void> _check({bool manual = false}) async {
    if (isShowUpgradeDialog) return;
    try {
      await getAppInfo();
      upgradeInfoV2 = await Apis.checkUpgradeV2();
      if (!canUpdate) {
        if (manual) IMViews.showToast('已是最新版本');
        return;
      }
      final info = upgradeInfoV2!;
      final key = '${info.buildVersion}+${info.buildVersionNo}';
      if (!manual &&
          info.needForceUpdate != true &&
          (DataSp.getIgnoreVersion() == key || _deferredVersion == key)) return;
      isShowUpgradeDialog = true;
      final view = UpgradeViewV2(
          upgradeInfo: info,
          packageInfo: packageInfo!,
          onLater: laterUpdate,
          onIgnore: ignoreUpdate,
          onNow: nowUpdate,
          subject: subject);
      if (info.needForceUpdate == true) {
        forcedUpdateGate.value = view;
        return;
      }
      await Get.dialog(view,
          barrierDismissible: info.needForceUpdate != true,
          routeSettings: const RouteSettings(name: 'upgrade_dialog'));
    } catch (error) {
      if (manual) IMViews.showToast('暂时无法检查更新，请稍后重试');
    } finally {
      isShowUpgradeDialog = forcedUpdateGate.value != null;
    }
  }

  bool get canUpdate {
    final info = upgradeInfoV2;
    if (info?.buildHaveNewVersion != true || info?.buildVersion == null)
      return false;
    return ReleasePolicy.isNewer(
        info!.buildVersion!,
        int.tryParse(info.buildVersionNo ?? '') ?? 0,
        packageInfo!.version,
        int.tryParse(packageInfo!.buildNumber) ?? 0);
  }
}

class NotificationService {
  // Handle displaying of notifications.
  static final NotificationService _notificationService =
      NotificationService._internal();
  final FlutterLocalNotificationsPlugin _flutterLocalNotificationsPlugin =
      FlutterLocalNotificationsPlugin();
  final AndroidInitializationSettings _androidInitializationSettings =
      const AndroidInitializationSettings('@mipmap/ic_launcher');

  factory NotificationService() {
    return _notificationService;
  }

  NotificationService._internal() {
    init();
  }

  void init() async {
    final InitializationSettings initializationSettings =
        InitializationSettings(
      android: _androidInitializationSettings,
    );
    await _flutterLocalNotificationsPlugin.initialize(initializationSettings);
  }

  void createNotification(int count, int i, int id, String status) {
    //show the notifications.
    var androidPlatformChannelSpecifics = AndroidNotificationDetails(
        'progress channel', 'progress channel',
        channelDescription: 'progress channel description',
        channelShowBadge: false,
        importance: Importance.max,
        priority: Priority.high,
        onlyAlertOnce: true,
        showProgress: true,
        maxProgress: count,
        progress: i);
    var platformChannelSpecifics =
        NotificationDetails(android: androidPlatformChannelSpecifics);
    _flutterLocalNotificationsPlugin
        .show(id, status, '$i%', platformChannelSpecifics, payload: 'item x');
  }
}
