import 'package:flutter/material.dart';
import 'package:flutter_easyloading/flutter_easyloading.dart';
import 'package:flutter_screenutil/flutter_screenutil.dart';
import 'package:get/get.dart';
import 'package:openim_common/openim_common.dart';

import '../core/controller/app_controller.dart';
import 'persistent_update_gate.dart';

class AppView extends StatelessWidget {
  const AppView({Key? key, required this.builder}) : super(key: key);
  final Widget Function(Locale? locale, TransitionBuilder builder) builder;

  @override
  Widget build(BuildContext context) {
    return GetBuilder<AppController>(
      init: AppController(),
      builder: (ctrl) => FocusDetector(
        onForegroundGained: () => ctrl.runningBackground(false),
        onForegroundLost: () => ctrl.runningBackground(true),
        child: ScreenUtilInit(
          designSize: const Size(Config.uiW, Config.uiH),
          minTextAdapt: true,
          splitScreenMode: true,
          fontSizeResolver: (fontSize, _) => fontSize.toDouble(),
          builder: (_, child) => builder(ctrl.getLocale(), _builder(ctrl)),
        ),
      ),
    );
  }

  static TransitionBuilder _builder(AppController ctrl) => EasyLoading.init(
        builder: (context, widget) {
          return MediaQuery(
            data: MediaQuery.of(context).copyWith(
              textScaleFactor: Config.textScaleFactor,
            ),
            child: PersistentUpdateGate(
                gate: ctrl.forcedUpdateGate, child: widget!),
          );
        },
      );
}
