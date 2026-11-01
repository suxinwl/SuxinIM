package cn.rentsoft.flutter.openim.consumer;

import android.app.Application;
import android.content.Context;
import android.system.Os;
import java.io.File;
import java.io.FileOutputStream;
import java.io.InputStream;

/** Configure development roots before providers, FlutterEngine or gomobile can load. */
public final class OpenImApplication extends Application {
  @Override protected void attachBaseContext(Context base) {
    super.attachBaseContext(base);
    if (!BuildConfig.TRUST_DEVELOPMENT_CA) return;
    try {
      File roots = new File(getFilesDir(), "openim38-development-trust");
      if (!roots.isDirectory() && !roots.mkdirs()) throw new IllegalStateException("Cannot create trust directory");
      File ca = new File(roots, "openim38-development-ca.crt");
      int resource = getResources().getIdentifier("openim38_dev_ca", "raw", getPackageName());
      try (InputStream in = getResources().openRawResource(resource);
           FileOutputStream out = new FileOutputStream(ca)) {
        byte[] buffer = new byte[4096];
        int length;
        while ((length = in.read(buffer)) != -1) out.write(buffer, 0, length);
      }
      Os.setenv("SSL_CERT_FILE", ca.getAbsolutePath(), true);
      Os.setenv("SSL_CERT_DIR", roots.getAbsolutePath(), true);
    } catch (Exception error) {
      throw new IllegalStateException("Cannot configure development trust anchor", error);
    }
  }
}
