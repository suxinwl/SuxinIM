package io.openim.flutter_openim_sdk;

import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.timeout;
import static org.mockito.Mockito.verify;

import io.flutter.plugin.common.MethodCall;
import io.flutter.plugin.common.MethodChannel;
import io.openim.flutter_openim_sdk.manager.IMManager;
import org.junit.Test;

/**
 * This demonstrates a simple unit test of the Java portion of this plugin's
 * implementation.
 *
 * Once you have built the plugin's example app, you can run these tests from
 * the command
 * line by running `./gradlew testDebugUnitTest` in the `example/android/`
 * directory, or
 * you can run them directly from IDEs that support JUnit such as Android
 * Studio.
 */

public class FlutterOpenimSdkPluginTest {
  @Test
  public void onMethodCall_getPlatformVersion_returnsExpectedValue() {
    FlutterOpenimSdkPlugin plugin = new FlutterOpenimSdkPlugin();

    final MethodCall call = new MethodCall("getPlatformVersion", null);
    MethodChannel.Result mockResult = mock(MethodChannel.Result.class);
    plugin.onMethodCall(call, mockResult);

    verify(mockResult).success("Android " + android.os.Build.VERSION.RELEASE);
  }

  @Test
  public void initSDK_whenAlreadyInitialized_returnsWithoutCallingNativeSDK() {
    FlutterOpenimSdkPlugin.isInitialized = true;
    MethodChannel.Result result = mock(MethodChannel.Result.class);

    try {
      new IMManager().initSDK(new MethodCall("initSDK", null), result);

      verify(result, timeout(1000)).success(true);
    } finally {
      FlutterOpenimSdkPlugin.isInitialized = false;
    }
  }
}
