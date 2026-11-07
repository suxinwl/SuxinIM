package cn.rentsoft.flutter.openim.consumer;
import io.flutter.embedding.android.FlutterFragmentActivity;
import io.flutter.embedding.engine.FlutterEngine;
import io.flutter.plugin.common.MethodChannel;

public class MainActivity extends FlutterFragmentActivity {
  private final java.util.concurrent.ExecutorService enterpriseTlsExecutor=java.util.concurrent.Executors.newSingleThreadExecutor();
  @Override public void configureFlutterEngine(FlutterEngine engine) {
    super.configureFlutterEngine(engine);
    new MethodChannel(engine.getDartExecutor().getBinaryMessenger(),"puty/enterprise_lan_tls")
      .setMethodCallHandler((call,result) -> {
        if(!call.method.equals("preflightLan") || call.arguments!=null){result.notImplemented();return;}
        enterpriseTlsExecutor.execute(() -> {
          try {java.util.Map<String,Object> verified=EnterpriseLanTls.preflight(getApplicationContext());runOnUiThread(() -> result.success(verified));}
          catch(Exception failure){runOnUiThread(() -> result.error("ENTERPRISE_TLS_REJECTED",failure.getClass().getSimpleName(),null));}
        });
      });
    // Test entrypoints receive synthetic credentials at runtime, never in APK assets.
    if (BuildConfig.TRUST_DEVELOPMENT_CA) {
      new MethodChannel(engine.getDartExecutor().getBinaryMessenger(), "openim38/native_probe")
        .setMethodCallHandler((call, result) -> {
          if (call.method.equals("configuration")) result.success(getIntent().getStringExtra("openim38Probe"));
          else if (call.method.equals("enterpriseTlsProbe")) new Thread(() -> {
            try {
              Object verified=Class.forName("cn.rentsoft.flutter.openim.consumer.EnterpriseProbeVerifier").getMethod("probe",android.content.Context.class).invoke(null,getApplicationContext());
              runOnUiThread(() -> result.success(verified));
            } catch(Exception failure) {runOnUiThread(() -> result.error("NATIVE_TLS_REJECTED",failure.getCause()==null?failure.getClass().getSimpleName():failure.getCause().getClass().getSimpleName(),null));}
          }).start();
          else result.notImplemented();
        });
    }
  }
  @Override protected void onDestroy(){enterpriseTlsExecutor.shutdownNow();super.onDestroy();}
}
