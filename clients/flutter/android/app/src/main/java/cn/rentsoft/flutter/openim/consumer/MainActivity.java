package cn.rentsoft.flutter.openim.consumer;
import io.flutter.embedding.android.FlutterFragmentActivity;
import io.flutter.embedding.engine.FlutterEngine;
import io.flutter.plugin.common.MethodChannel;

public class MainActivity extends FlutterFragmentActivity {
  @Override public void configureFlutterEngine(FlutterEngine engine) {
    super.configureFlutterEngine(engine);
    // Test entrypoints receive synthetic credentials at runtime, never in APK assets.
    if (BuildConfig.TRUST_DEVELOPMENT_CA) {
      new MethodChannel(engine.getDartExecutor().getBinaryMessenger(), "openim38/native_probe")
        .setMethodCallHandler((call, result) -> {
          if (call.method.equals("configuration")) result.success(getIntent().getStringExtra("openim38Probe"));
          else result.notImplemented();
        });
    }
  }
}
