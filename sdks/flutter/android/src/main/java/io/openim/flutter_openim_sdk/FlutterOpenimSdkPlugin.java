package io.openim.flutter_openim_sdk;

import android.app.Activity;
import android.content.Context;

import androidx.annotation.NonNull;

import java.lang.reflect.Field;
import java.lang.reflect.Method;

import io.flutter.Log;
import io.flutter.embedding.engine.plugins.FlutterPlugin;
import io.flutter.embedding.engine.plugins.activity.ActivityAware;
import io.flutter.embedding.engine.plugins.activity.ActivityPluginBinding;
import io.flutter.plugin.common.MethodCall;
import io.flutter.plugin.common.MethodChannel;
import io.flutter.plugin.common.MethodChannel.MethodCallHandler;
import io.flutter.plugin.common.MethodChannel.Result;
import io.openim.flutter_openim_sdk.connectivity.ConnectivityListener;
import io.openim.flutter_openim_sdk.connectivity.VisibilityListener;
import io.openim.flutter_openim_sdk.manager.ConversationManager;
import io.openim.flutter_openim_sdk.manager.FriendshipManager;
import io.openim.flutter_openim_sdk.manager.GroupManager;
import io.openim.flutter_openim_sdk.manager.IMManager;
import io.openim.flutter_openim_sdk.manager.MessageManager;
import io.openim.flutter_openim_sdk.manager.UserManager;


/**
 * FlutterOpenimSdkPlugin
 */
public class FlutterOpenimSdkPlugin implements FlutterPlugin, MethodCallHandler, ActivityAware {
    /// The MethodChannel that will the communication between Flutter and native Android
    ///
    /// This local reference serves to register the plugin with the Flutter Engine and unregister it
    /// when the Flutter Engine is detached from the Activity
    private static final String CHANNEL_NAME = "flutter_openim_sdk";
    public static MethodChannel channel;
    private static IMManager imManager;
    private static UserManager userManager;
    private static FriendshipManager friendshipManager;
    private static MessageManager messageManager;
    private static ConversationManager conversationManager;
    private static GroupManager groupManager;
    private static Activity activity;
    private static Context context;
    private ConnectivityListener connectivityListener;
    private VisibilityListener visibilityListener;
    private NativeVaultBroker nativeVault;
    public static volatile boolean isInitialized;

    public FlutterOpenimSdkPlugin() {
        FlutterOpenimSdkPlugin.imManager = new IMManager();
        FlutterOpenimSdkPlugin.userManager = new UserManager();
        FlutterOpenimSdkPlugin.friendshipManager = new FriendshipManager();
        FlutterOpenimSdkPlugin.messageManager = new MessageManager();
        FlutterOpenimSdkPlugin.conversationManager = new ConversationManager();
        FlutterOpenimSdkPlugin.groupManager = new GroupManager();
    }


    @Override
    public void onAttachedToEngine(@NonNull FlutterPluginBinding flutterPluginBinding) {
        channel = new MethodChannel(flutterPluginBinding.getBinaryMessenger(), CHANNEL_NAME,io.flutter.plugin.common.StandardMethodCodec.INSTANCE,flutterPluginBinding.getBinaryMessenger().makeBackgroundTaskQueue());
        context = flutterPluginBinding.getApplicationContext();
        nativeVault = new NativeVaultBroker(context);
        MethodChannel vaultChannel=new MethodChannel(flutterPluginBinding.getBinaryMessenger(), "puty/local_store");
        nativeVault.setChannel(vaultChannel);
        vaultChannel.setMethodCallHandler((call,result) -> nativeVault.handle(call.method,call.arguments,result));
        channel.setMethodCallHandler(this);
        connectivityListener = new ConnectivityListener(context);
        visibilityListener = new VisibilityListener();
        connectivityListener.register();
    }

    @Override
    public void onMethodCall(@NonNull MethodCall call, @NonNull Result result) {
        parse(call, result);
    }

    @Override
    public void onDetachedFromEngine(@NonNull FlutterPluginBinding binding) {
        if(nativeVault!=null){nativeVault.close();nativeVault=null;}
        FlutterOpenimSdkPlugin.channel.setMethodCallHandler(null);
        connectivityListener.unregisterReceiver();
    }


    @Override
    public void onAttachedToActivity(@NonNull ActivityPluginBinding binding) {
        visibilityListener.register(activity = binding.getActivity());
        nativeVault.backupPicker.attach(activity);binding.addActivityResultListener(nativeVault.backupPicker);
    }

    @Override
    public void onDetachedFromActivityForConfigChanges() {
        nativeVault.backupPicker.attach(null);
        visibilityListener.unregisterReceiver(activity);
        activity = null;
    }

    @Override
    public void onReattachedToActivityForConfigChanges(@NonNull ActivityPluginBinding binding) {
        visibilityListener.register(activity = binding.getActivity());
        nativeVault.backupPicker.attach(activity);binding.addActivityResultListener(nativeVault.backupPicker);
    }

    @Override
    public void onDetachedFromActivity() {
        nativeVault.backupPicker.attach(null);
        visibilityListener.unregisterReceiver(activity);
        activity = null;
    }

    void parse(@NonNull MethodCall call, @NonNull Result result) {
        try {
            String managerName = call.argument("ManagerName");
            Field field = FlutterOpenimSdkPlugin.class.getDeclaredField(managerName);
            Method method = field.get(new Object()).getClass().getDeclaredMethod(call.method, MethodCall.class, Result.class);
            Log.i("F-OpenIMSDK(flutter call native)", "{ class:" + managerName + ",  method:" + method.getName() + " }");
            method.invoke(field.get(new Object()), call, result);
        } catch (Exception e) {
            result.error("NATIVE_OPERATION_REJECTED", e.getClass().getSimpleName(),null);
        }
    }
}
