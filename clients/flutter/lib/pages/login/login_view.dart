import 'dart:convert';
import 'package:flutter/material.dart';
import 'package:get/get.dart';
import 'login_logic.dart';

class LoginPage extends StatelessWidget {
  final logic = Get.find<LoginLogic>();
  LoginPage({super.key});
  @override
  Widget build(BuildContext context) => Scaffold(
        body: SafeArea(
            child: SingleChildScrollView(
                padding: const EdgeInsets.all(32),
                child: Obx(() => Column(children: [
                      const SizedBox(height: 40),
                      Image.asset(
                          'packages/openim_common/assets/images/ic_login_logo.png',
                          width: 96,
                          height: 96),
                      const SizedBox(height: 20),
                      Text(logic.displayName,
                          style: const TextStyle(
                              fontSize: 24, fontWeight: FontWeight.bold)),
                      const SizedBox(height: 32),
                      TextField(
                          controller: logic.phoneCtrl,
                          enabled: !logic.busy,
                          autofillHints: const [AutofillHints.username],
                          decoration: InputDecoration(
                              labelText: '统一账号 / 工号',
                              suffixIcon: IconButton(
                                  key: const Key('clear-unified-account'),
                                  onPressed:
                                      logic.busy ? null : logic.phoneCtrl.clear,
                                  icon: const Icon(Icons.clear)))),
                      const SizedBox(height: 16),
                      TextField(
                          controller: logic.pwdCtrl,
                          enabled: !logic.busy,
                          obscureText: logic.obscureText.value,
                          autofillHints: const [AutofillHints.password],
                          decoration: InputDecoration(
                              labelText: '密码',
                              suffixIcon: IconButton(
                                  onPressed: logic.busy
                                      ? null
                                      : () => logic.obscureText.toggle(),
                                  icon: const Icon(Icons.visibility)))),
                      if (logic.captchaRequired.value) ...[
                        const SizedBox(height: 16),
                        Row(children: [
                          Expanded(
                              child: TextField(
                                  controller: logic.verificationCodeCtrl,
                                  enabled: !logic.busy,
                                  decoration:
                                      const InputDecoration(labelText: '验证码'))),
                          GestureDetector(
                              onTap: logic.busy ? null : logic.loadCaptcha,
                              child: logic.captchaImage.value.isEmpty
                                  ? const Text('刷新验证码')
                                  : Image.memory(
                                      base64Decode(logic.captchaImage.value
                                          .split(',')
                                          .last),
                                      width: 100,
                                      height: 40))
                        ])
                      ],
                      const SizedBox(height: 24),
                      Wrap(
                          alignment: WrapAlignment.center,
                          spacing: 8,
                          children: [
                            SizedBox(
                                width: 144,
                                child: RadioListTile<bool>(
                                    key: const Key('login-network-internal'),
                                    contentPadding: EdgeInsets.zero,
                                    dense: true,
                                    value: true,
                                    groupValue:
                                        logic.networkConfigurationKnown.value
                                            ? logic.internalNetwork.value
                                            : null,
                                    onChanged: logic.busy
                                        ? null
                                        : (value) =>
                                            logic.switchNetwork(value!),
                                    title: const Text('内部通讯'))),
                            SizedBox(
                                width: 144,
                                child: RadioListTile<bool>(
                                    key: const Key('login-network-public'),
                                    contentPadding: EdgeInsets.zero,
                                    dense: true,
                                    value: false,
                                    groupValue:
                                        logic.networkConfigurationKnown.value
                                            ? logic.internalNetwork.value
                                            : null,
                                    onChanged: logic.busy
                                        ? null
                                        : (value) =>
                                            logic.switchNetwork(value!),
                                    title: const Text('公网通讯'))),
                          ]),
                      if (logic.switchingNetwork.value)
                        const Padding(
                            padding: EdgeInsets.only(bottom: 12),
                            child: Row(
                                mainAxisAlignment: MainAxisAlignment.center,
                                children: [
                                  SizedBox(
                                      width: 16,
                                      height: 16,
                                      child: CircularProgressIndicator(
                                          strokeWidth: 2)),
                                  SizedBox(width: 8),
                                  Text('正在切换通讯连接…')
                                ])),
                      if (logic.networkError.value.isNotEmpty) ...[
                        Text(logic.networkError.value,
                            style: TextStyle(
                                color: Theme.of(context).colorScheme.error)),
                        TextButton(
                            onPressed: logic.busy
                                ? null
                                : () => logic
                                    .switchNetwork(logic.internalNetwork.value),
                            child: const Text('重试连接')),
                      ],
                      SizedBox(
                          width: double.infinity,
                          child: FilledButton(
                              onPressed: logic.canLogin ? logic.login : null,
                              child: const Text('登录'))),
                      const Text('账号、密码和个人资料由人事系统统一管理。'),
                      const SizedBox(height: 24),
                      Text(logic.versionInfo.value),
                    ])))),
      );
}
