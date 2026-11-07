import 'package:flutter/material.dart';
import 'package:app_im/app_im.dart';
import 'package:get/get.dart';
import 'package:openim_common/openim_common.dart';
import 'package:package_info_plus/package_info_plus.dart';

import '../../core/controller/im_controller.dart';
import '../../core/controller/app_controller.dart';
import '../../core/controller/push_controller.dart';
import '../../routes/app_navigator.dart';
import 'password_change_page.dart';

enum LoginType {
  phone,
  email,
}

extension LoginTypeExt on LoginType {
  int get rawValue {
    switch (this) {
      case LoginType.phone:
        return 0;
      case LoginType.email:
        return 1;
    }
  }

  String get name {
    switch (this) {
      case LoginType.phone:
        return StrRes.phoneNumber;
      case LoginType.email:
        return StrRes.email;
    }
  }

  String get hintText {
    switch (this) {
      case LoginType.phone:
        return StrRes.plsEnterPhoneNumber;
      case LoginType.email:
        return StrRes.plsEnterEmail;
    }
  }

  String get exclusiveName {
    switch (this) {
      case LoginType.phone:
        return StrRes.email;
      case LoginType.email:
        return StrRes.phoneNumber;
    }
  }
}

class LoginLogic extends GetxController {
  String get displayName => AppBrand.nameFrom(Get.isRegistered<AppController>()
      ? Get.find<AppController>().clientConfigMap
      : null);
  final imLogic = Get.find<IMController>();
  final pushLogic = Get.find<PushController>();
  final phoneCtrl = TextEditingController();
  final pwdCtrl = TextEditingController();
  final verificationCodeCtrl = TextEditingController();
  final obscureText = true.obs;
  final enabled = false.obs;
  final areaCode = "+86".obs;
  final isPasswordLogin = true.obs;
  final internalNetwork = (Config.serverBaseUrl == Config.lanOrigin).obs;
  final networkConfigurationKnown = true.obs;
  final switchingNetwork = false.obs;
  final loggingIn = false.obs;
  final networkError = ''.obs;
  bool get busy => switchingNetwork.value || loggingIn.value;
  bool get canLogin =>
      enabled.value &&
      !busy &&
      networkConfigurationKnown.value &&
      networkError.value.isEmpty;
  final captchaRequired = false.obs;
  final captchaImage = ''.obs;
  String captchaId = '';
  int _captchaRequest = 0;
  Future<void> loadCaptcha() async {
    final request = ++_captchaRequest;
    final origin = Config.serverBaseUrl;
    try {
      final value = await HrmSSO.request('captcha');
      if (isClosed ||
          request != _captchaRequest ||
          origin != Config.serverBaseUrl) return;
      captchaRequired.value = value['show'] == true || value['enabled'] == true;
      captchaImage.value = (value['image'] ?? value['img'] ?? '').toString();
      captchaId = (value['captchaId'] ?? value['id'] ?? '').toString();
    } catch (_) {
      if (!isClosed &&
          request == _captchaRequest &&
          origin == Config.serverBaseUrl) {
        IMViews.showToast('验证码暂不可用，请重试');
      }
    }
  }

  Future<void> switchNetwork(bool internal) async {
    if (busy ||
        (networkConfigurationKnown.value &&
            internal == internalNetwork.value &&
            networkError.value.isEmpty)) return;
    switchingNetwork.value = true;
    networkError.value = '';
    ++_captchaRequest;
    captchaId = '';
    captchaImage.value = '';
    captchaRequired.value = false;
    verificationCodeCtrl.clear();
    try {
      await imLogic.prepareLogin();
      if (isClosed) return;
      if (await DataSp.removeLoginCertificate() != true)
        throw StateError('Protected session update failed');
      final origin = internal ? Config.lanOrigin : Config.wanOrigin;
      final previousOrigin = Config.serverBaseUrl;
      try {
        if (await DataSp.putServerConfig({'baseUrl': origin}) != true)
          throw StateError('Protected connection update failed');
        networkConfigurationKnown.value = true;
      } catch (_) {
        // Protected preference writes update their RAM view before the native
        // commit. Restore the prior origin before presenting it as selected.
        try {
          if (await DataSp.putServerConfig({'baseUrl': previousOrigin}) != true)
            throw StateError('Protected connection restore failed');
          networkConfigurationKnown.value = true;
        } catch (_) {
          networkConfigurationKnown.value = false;
        }
        rethrow;
      }
      if (isClosed) return;
      internalNetwork.value = Config.serverBaseUrl == Config.lanOrigin;
      await imLogic.reconfigureServer();
      if (isClosed) return;
      await Future.wait([
        if (Get.isRegistered<AppController>())
          Get.find<AppController>().queryPublicBrand(),
        loadCaptcha(),
      ]);
    } catch (_) {
      if (!isClosed) networkError.value = '通讯连接未完成，请重试或切换其他连接';
    } finally {
      if (!isClosed) {
        internalNetwork.value = Config.serverBaseUrl == Config.lanOrigin;
        switchingNetwork.value = false;
      }
    }
  }

  final versionInfo = ''.obs;
  final loginType = LoginType.phone.obs;
  String? get email =>
      loginType.value == LoginType.email ? phoneCtrl.text.trim() : null;
  String? get phone =>
      loginType.value == LoginType.phone ? phoneCtrl.text.trim() : null;
  LoginType operateType = LoginType.phone;

  _initData() async {
    var map = DataSp.getLoginAccount();
    if (map is Map) {
      String? phoneNumber = map["phoneNumber"];
      String? areaCode = map["areaCode"];
      if (phoneNumber != null && phoneNumber.isNotEmpty) {
        phoneCtrl.text = phoneNumber;
      }
      if (areaCode != null && areaCode.isNotEmpty) {
        this.areaCode.value = areaCode;
      }
    }

    loginType.value =
        (DataSp.getLoginType()) == 0 ? LoginType.phone : LoginType.email;
  }

  @override
  void onClose() {
    phoneCtrl.dispose();
    pwdCtrl.dispose();
    verificationCodeCtrl.dispose();
    super.onClose();
  }

  @override
  void onInit() {
    _initData();
    phoneCtrl.addListener(_onChanged);
    pwdCtrl.addListener(_onChanged);
    verificationCodeCtrl.addListener(_onChanged);
    super.onInit();
  }

  @override
  void onReady() {
    super.onReady();
    getPackageInfo();
    loadCaptcha();
    if (Get.isRegistered<AppController>())
      Get.find<AppController>().queryPublicBrand();
  }

  _onChanged() {
    enabled.value = isPasswordLogin.value &&
            phoneCtrl.text.trim().isNotEmpty &&
            pwdCtrl.text.trim().isNotEmpty ||
        !isPasswordLogin.value &&
            phoneCtrl.text.trim().isNotEmpty &&
            verificationCodeCtrl.text.trim().isNotEmpty;
  }

  Future<void> login() async {
    if (busy || networkError.value.isNotEmpty) return;
    loggingIn.value = true;
    try {
      await DataSp.putLoginType(loginType.value.rawValue);
      await LoadingView.singleton.wrap(asyncFunction: () async {
        var suc = await _login();
        if (suc && !isClosed && imLogic.session.active) {
          Get.find<CacheController>().resetCache();
          AppNavigator.startMain();
        }
      });
    } catch (_) {
      if (!isClosed) IMViews.showToast('登录未完成，请检查网络或重试');
    } finally {
      if (!isClosed) loggingIn.value = false;
    }
  }

  Future<bool> _login() async {
    try {
      await imLogic.prepareLogin();
      if (isClosed) return false;
      final data = await HrmSSO.login(phoneCtrl.text.trim(), pwdCtrl.text,
          captcha: verificationCodeCtrl.text.trim(), captchaId: captchaId);
      if (isClosed) return false;
      await imLogic.prepareLogin();
      if (isClosed) return false;
      final account = {
        "areaCode": areaCode.value,
        "phoneNumber": phoneCtrl.text
      };
      await DataSp.putLoginCertificate(data);
      await DataSp.putLoginAccount(account);
      Logger.print('login : ${data.userID}');
      final attempt = await imLogic.login(data.userID, data.imToken);
      if (isClosed || !imLogic.session.established(attempt)) return false;
      Logger.print('im login success');
      pushLogic.login(data.userID);
      Logger.print('push login success');
      return true;
    } catch (e) {
      if (e is HrmPasswordChallenge && !isClosed) {
        await Get.to<bool>(
            () => PasswordChangePage(changeTicket: e.changeTicket));
        pwdCtrl.clear();
        verificationCodeCtrl.clear();
        await loadCaptcha();
        return false;
      }
      Logger.print('Login failed: ${e.runtimeType}');
      if (!isClosed) {
        IMViews.showToast(e is HrmSessionError ? e.message : '登录未完成，请检查网络或重试');
        await loadCaptcha();
      }
    }
    return false;
  }

  void togglePasswordType() {
    isPasswordLogin.value = !isPasswordLogin.value;
  }

  void toggleLoginType() {
    if (loginType.value == LoginType.phone) {
      loginType.value = LoginType.email;
    } else {
      loginType.value = LoginType.phone;
    }

    phoneCtrl.text = '';
  }

  Future<bool> getVerificationCode() async {
    if (phone?.isNotEmpty == true &&
        !IMUtils.isMobile(areaCode.value, phoneCtrl.text)) {
      IMViews.showToast(StrRes.plsEnterRightPhone);
      return false;
    }

    if (email?.isNotEmpty == true && !phoneCtrl.text.isEmail) {
      IMViews.showToast(StrRes.plsEnterRightEmail);
      return false;
    }

    return sendVerificationCode();
  }

  /// [usedFor] 1：注册，2：重置密码 3：登录
  Future<bool> sendVerificationCode() => LoadingView.singleton.wrap(
      asyncFunction: () => Apis.requestVerificationCode(
            areaCode: areaCode.value,
            phoneNumber: phone,
            email: email,
            usedFor: 3,
          ));

  void openCountryCodePicker() async {
    String? code = await IMViews.showCountryCodePicker();
    if (null != code) areaCode.value = code;
  }

  void registerNow() => AppNavigator.startRegister();

  void forgetPassword() => AppNavigator.startForgetPassword();

  void getPackageInfo() async {
    PackageInfo packageInfo = await PackageInfo.fromPlatform();
    final version = packageInfo.version;
    final appName = packageInfo.appName;
    final buildNumber = packageInfo.buildNumber;

    versionInfo.value =
        '$appName $version+$buildNumber SDK: ${AppIm.sdkVersion}';
  }
}
