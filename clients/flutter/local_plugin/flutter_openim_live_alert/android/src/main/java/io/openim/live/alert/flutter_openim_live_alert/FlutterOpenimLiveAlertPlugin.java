package io.openim.live.alert.flutter_openim_live_alert;

import android.app.Activity;
import android.content.Context;
import android.os.Build;
import android.provider.Settings;

import androidx.annotation.NonNull;

import io.flutter.embedding.engine.plugins.FlutterPlugin;
import io.flutter.embedding.engine.plugins.activity.ActivityAware;
import io.flutter.embedding.engine.plugins.activity.ActivityPluginBinding;
import io.flutter.plugin.common.MethodCall;
import io.flutter.plugin.common.MethodChannel;
import io.flutter.plugin.common.MethodChannel.MethodCallHandler;
import io.flutter.plugin.common.MethodChannel.Result;
import io.openim.live.alert.flutter_openim_live_alert.services.CallService;
import io.openim.live.alert.flutter_openim_live_alert.utils.Utils;

/**
 * FlutterOpenimLiveAlertPlugin
 */
public class FlutterOpenimLiveAlertPlugin implements FlutterPlugin, MethodCallHandler, ActivityAware {
    /// The MethodChannel that will the communication between Flutter and native Android
    ///
    /// This local reference serves to register the plugin with the Flutter Engine and unregister it
    /// when the Flutter Engine is detached from the Activity
    public static MethodChannel channel;
    private Context context;
    private Activity activity;

    @Override
    public void onAttachedToEngine(@NonNull FlutterPluginBinding flutterPluginBinding) {
        context = flutterPluginBinding.getApplicationContext();
        channel = new MethodChannel(flutterPluginBinding.getBinaryMessenger(), "flutter_openim_live_alert");
        channel.setMethodCallHandler(this);
    }

    @Override
    public void onMethodCall(@NonNull MethodCall call, @NonNull Result result) {
      try {
        Context callContext = activity != null ? activity : context;
        if (callContext == null) {
            result.error("engine_detached", "Call alert is unavailable", null);
            return;
        }
        switch (call.method) {
            case "showLiveAlert":
                if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.M && !Settings.canDrawOverlays(callContext)) {
                    result.error("overlay_permission_required", "Display-over-other-apps permission is required", null);
                    return;
                }
                String title = call.argument("title");
                String rejectText = call.argument("rejectText");
                String acceptText = call.argument("acceptText");
                CallService.startService(callContext, title, rejectText, acceptText);
                break;
            case "closeLiveAlert":
                CallService.stopService(callContext);
                break;
            case "closeLiveAlertAndMoveTaskToFront": {
                CallService.stopService(callContext);
                if (activity == null) {
                    result.error("activity_unavailable", "Open the app to accept this call", null);
                    return;
                }
                String packageName = call.argument("packageName");
                String activityName = call.argument("activityName");
                if (null == packageName) {
                    packageName = activity.getPackageName();
                }
                if (null == activityName) {
                    activityName = activity.getClass().getName();
                }
                Utils.moveTaskToFront(activity, packageName, activityName);
                break;
            }
            default:
                result.notImplemented();
                return;
        }
        result.success(null);
      } catch (RuntimeException error) {
        result.error("call_alert_unavailable", error.getMessage(), null);
      }
    }

    @Override
    public void onDetachedFromEngine(@NonNull FlutterPluginBinding binding) {
        if (context != null) CallService.stopService(context);
        channel.setMethodCallHandler(null);
        activity = null;
        context = null;
    }

    @Override
    public void onAttachedToActivity(@NonNull ActivityPluginBinding binding) {
        activity = binding.getActivity();
    }

    @Override
    public void onDetachedFromActivityForConfigChanges() {
        activity = null;
    }

    @Override
    public void onReattachedToActivityForConfigChanges(@NonNull ActivityPluginBinding binding) {
        activity = binding.getActivity();
    }

    @Override
    public void onDetachedFromActivity() {
        activity = null;
    }

}
