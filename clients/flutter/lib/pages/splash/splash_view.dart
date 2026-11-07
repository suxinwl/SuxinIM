import 'package:flutter/material.dart';
import 'package:flutter_screenutil/flutter_screenutil.dart';
import 'package:get/get.dart';
import 'package:openim_common/openim_common.dart';

import 'splash_logic.dart';

class SplashPage extends StatelessWidget {
  final logic = Get.find<SplashLogic>();

  SplashPage({super.key});

  @override
  Widget build(BuildContext context) {
    return Scaffold(
        backgroundColor: Colors.white,
        body: Container(
          decoration: BoxDecoration(
            gradient: LinearGradient(
              colors: [Styles.c_0089FF_opacity10, Styles.c_FFFFFF_opacity0],
              begin: Alignment.topCenter,
              end: Alignment.bottomCenter,
            ),
          ),
          child: Stack(
            alignment: Alignment.center,
            children: [
              Obx(() => logic.recovering.value
                  ? Center(
                      child: Padding(
                          padding: const EdgeInsets.all(24),
                          child:
                              Column(mainAxisSize: MainAxisSize.min, children: [
                            const Icon(Icons.lock_outline, size: 40),
                            const SizedBox(height: 16),
                            Text(
                                logic.recoveryError.value.isEmpty
                                    ? '正在联网恢复会话…'
                                    : logic.recoveryError.value,
                                textAlign: TextAlign.center),
                            const SizedBox(height: 16),
                            if (logic.busy.value)
                              const CircularProgressIndicator()
                            else ...[
                              ElevatedButton(
                                  onPressed: logic.retry,
                                  child: const Text('重试恢复')),
                              TextButton(
                                  onPressed: logic.chooseAnotherAccount,
                                  child: const Text('使用其他账号')),
                            ],
                          ])))
                  : const SizedBox.shrink()),
              Positioned(
                bottom: 130.h,
                child: ImageRes.splashLogo.toImage
                  ..width = 55.61.w
                  ..height = 78.91.h,
              ),
            ],
          ),
        ));
  }
}
