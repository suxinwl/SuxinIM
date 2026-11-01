// Private x86_64 AVD validation entrypoint. Never used for the ARM64 delivery.
// With no synthetic runtime intent, executes the unchanged original main UI.
import 'package:flutter/services.dart';
import 'package:flutter/widgets.dart';
import 'package:openim/main.dart' as application;
import 'native_probe.dart' as probe;

void main() async {
  WidgetsFlutterBinding.ensureInitialized();
  if (!const bool.fromEnvironment('OPENIM_DEVELOPMENT')) {
    throw StateError('Private validation entrypoint requires development mode');
  }
  final config = await const MethodChannel('openim38/native_probe')
      .invokeMethod<String>('configuration');
  if (config != null) {
    probe.main();
  } else {
    application.main();
  }
}
