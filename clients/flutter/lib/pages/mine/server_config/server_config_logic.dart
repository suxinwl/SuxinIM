import 'package:common_utils/common_utils.dart';
import 'package:flutter/material.dart';
import 'package:get/get.dart';
import 'package:openim_common/openim_common.dart';

class ServerConfigLogic extends GetxController {
  var checked = true.obs;
  var index = 0.obs;
  var ipCtrl = TextEditingController();
  var authCtrl = TextEditingController();
  var imApiCtrl = TextEditingController();
  var imWsCtrl = TextEditingController();
  var isIP = true.obs;

  void switchServer(bool isIp) {
    isIP.value = isIp;
    ipCtrl.clear();
    final hintText = isIP.value ? 'ip' : 'Domain';
    _setEndpoints(hintText);
  }

  String get _authority => ipCtrl.text
      .trim()
      .replaceFirst(RegExp(r'^https?://'), '')
      .replaceAll(RegExp(r'/+$'), '');
  void _setEndpoints(String authority) {
    authCtrl.text = 'https://$authority/chat';
    imApiCtrl.text = 'https://$authority/api';
    imWsCtrl.text = 'wss://$authority/ws';
  }

  @override
  void onInit() {
    ipCtrl.text = Config.serverIp;
    authCtrl.text = Config.appAuthUrl;
    imApiCtrl.text = Config.imApiUrl;
    imWsCtrl.text = Config.imWsUrl;

    isIP.value = RegexUtil.isIP(Uri.parse(Config.serverBaseUrl).host);

    ipCtrl.addListener(() {
      _setEndpoints(_authority);
    });
    super.onInit();
  }

  @override
  void onClose() {
    authCtrl.dispose();
    imApiCtrl.dispose();
    imWsCtrl.dispose();
    ipCtrl.dispose();
    super.onClose();
  }

  void toggleRadio() {
    checked.value = !checked.value;
  }

  void confirm() async {
    if (ipCtrl.text.isEmpty) {
      IMViews.showToast('Enter Server Address!');
      return;
    }
    final base = Uri.tryParse('https://$_authority');
    final auth = Uri.tryParse(authCtrl.text.trim());
    final api = Uri.tryParse(imApiCtrl.text.trim());
    final ws = Uri.tryParse(imWsCtrl.text.trim());
    if (base == null ||
        base.host.isEmpty ||
        (base.path.isNotEmpty && base.path != '/') ||
        auth?.scheme != 'https' ||
        auth!.host.isEmpty ||
        api?.scheme != 'https' ||
        api!.host.isEmpty ||
        ws?.scheme != 'wss' ||
        ws!.host.isEmpty) {
      IMViews.showToast('请填写域名/IP及端口，并使用 HTTPS / WSS 地址');
      return;
    }
    await DataSp.putServerConfig({
      'serverIP': _authority,
      'baseUrl': base.origin,
      'authUrl': authCtrl.text.trim().replaceAll(RegExp(r'/+$'), ''),
      'apiUrl': imApiCtrl.text.trim().replaceAll(RegExp(r'/+$'), ''),
      'wsUrl': imWsCtrl.text.trim(),
    });
    IMViews.showToast(StrRes.serverSettingTips);
  }

  void toggleTab(i) {
    index.value = i;
  }
}
