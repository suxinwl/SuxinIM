package com.zero.app_installer;

import android.app.Activity;
import android.content.ActivityNotFoundException;
import android.content.ClipData;
import android.content.Context;
import android.content.Intent;
import android.net.Uri;
import android.os.Build;
import android.text.TextUtils;
import androidx.annotation.NonNull;
import androidx.core.content.FileProvider;
import java.io.File;
import java.io.IOException;
import java.util.Map;
import io.flutter.embedding.engine.plugins.FlutterPlugin;
import io.flutter.embedding.engine.plugins.activity.ActivityAware;
import io.flutter.embedding.engine.plugins.activity.ActivityPluginBinding;
import io.flutter.plugin.common.MethodCall;
import io.flutter.plugin.common.MethodChannel;
import io.flutter.plugin.common.MethodChannel.Result;
import io.flutter.plugin.common.PluginRegistry;

/** APK installation handoff. The system installer retains user consent. */
public class AppInstallerPlugin implements FlutterPlugin, ActivityAware, MethodChannel.MethodCallHandler, PluginRegistry.ActivityResultListener {
    private Context applicationContext;
    private Activity activity;
    private ActivityPluginBinding activityBinding;
    private MethodChannel channel;

    @Override public void onAttachedToEngine(FlutterPluginBinding binding) {
        applicationContext=binding.getApplicationContext();
        channel=new MethodChannel(binding.getBinaryMessenger(),"app_installer");
        channel.setMethodCallHandler(this);
    }
    @Override public void onDetachedFromEngine(@NonNull FlutterPluginBinding binding) {
        detachActivity();applicationContext=null;
        if(channel!=null)channel.setMethodCallHandler(null);
        channel=null;
    }
    @Override public void onAttachedToActivity(ActivityPluginBinding binding) {
        detachActivity();activityBinding=binding;activity=binding.getActivity();
        binding.addActivityResultListener(this);
    }
    @Override public void onDetachedFromActivityForConfigChanges(){detachActivity();}
    @Override public void onReattachedToActivityForConfigChanges(@NonNull ActivityPluginBinding binding){onAttachedToActivity(binding);}
    @Override public void onDetachedFromActivity(){detachActivity();}
    private void detachActivity(){
        if(activityBinding!=null)activityBinding.removeActivityResultListener(this);
        activityBinding=null;activity=null;
    }
    private boolean activityReady(){
        return applicationContext!=null&&activity!=null&&!activity.isFinishing()
            &&(Build.VERSION.SDK_INT<17||!activity.isDestroyed());
    }

    @Override public void onMethodCall(MethodCall call,@NonNull Result target){
        OnceResult result=new OnceResult(target);
        try{handle(call,result);}
        catch(SecurityException failure){result.error("INSTALLER_URI_PERMISSION_DENIED","Cannot grant temporary APK read access",null);}
        catch(ActivityNotFoundException failure){result.error("INSTALLER_NOT_AVAILABLE","System package installer is unavailable",null);}
        catch(IllegalArgumentException failure){result.error("INSTALLER_FILE_NOT_SHARED","APK is outside the configured installation cache",null);}
        catch(IOException failure){result.error("INSTALLER_FILE_UNREADABLE","Downloaded APK cannot be read",null);}
        catch(Exception|LinkageError failure){result.error("INSTALLER_UNAVAILABLE","System installation handoff failed",null);}
    }
    private void handle(MethodCall call,Result result)throws IOException{
        if("installApk".equals(call.method)){
            if(!(call.arguments instanceof Map)){result.error("INSTALLER_ARGUMENT_INVALID","An APK file is required",null);return;}
            Object path=call.argument("apkPath");
            if(!(path instanceof String)||TextUtils.isEmpty((String)path)){
                result.error("INSTALLER_ARGUMENT_INVALID","An APK file is required",null);return;
            }
            install(new File((String)path).getCanonicalFile(),result);return;
        }
        if("goStore".equals(call.method)){
            if(!activityReady()){result.error("INSTALLER_ACTIVITY_UNAVAILABLE","Open the application before installing",null);return;}
            Object value=call.arguments instanceof Map?call.argument("androidAppId"):null;
            if(value!=null&&!(value instanceof String)){result.error("INSTALLER_ARGUMENT_INVALID","An application identifier is required",null);return;}
            String name=value==null?activity.getPackageName():(String)value;
            Intent intent=new Intent(Intent.ACTION_VIEW,Uri.parse("market://details?id="+Uri.encode(name)));
            intent.addFlags(Intent.FLAG_ACTIVITY_NO_HISTORY|Intent.FLAG_ACTIVITY_NEW_DOCUMENT|Intent.FLAG_ACTIVITY_MULTIPLE_TASK);
            activity.startActivity(intent);result.success(true);return;
        }
        result.notImplemented();
    }
    private void install(File file,Result result){
        if(!activityReady()){result.error("INSTALLER_ACTIVITY_UNAVAILABLE","Open the application before installing",null);return;}
        if(!file.isFile()||!file.canRead()||file.length()<=0){result.error("INSTALLER_FILE_UNREADABLE","Downloaded APK cannot be read",null);return;}
        Intent intent=new Intent(Intent.ACTION_VIEW).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
        if(Build.VERSION.SDK_INT>=Build.VERSION_CODES.N){
            Uri uri=FileProvider.getUriForFile(applicationContext,applicationContext.getPackageName()+".fileProvider",file);
            intent.setDataAndType(uri,"application/vnd.android.package-archive");
            intent.addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION);
            // Preserve the same narrowly scoped grant if an OEM installer
            // forwards the content URI. No write or persistent grant is used.
            intent.setClipData(ClipData.newRawUri("APK update",uri));
        }else intent.setDataAndType(Uri.fromFile(file),"application/vnd.android.package-archive");
        activity.startActivity(intent);
        result.success(true); // Handoff accepted; not installation success.
    }
    @Override public boolean onActivityResult(int requestCode,int resultCode,Intent intent){
        // This plugin has no pending settings activity. Never replay an old
        // MethodChannel reply after rotation or an unrelated activity result.
        return false;
    }
    private static final class OnceResult implements Result{
        private final Result target;private boolean completed;
        OnceResult(Result target){this.target=target;}
        @Override public synchronized void success(Object value){if(completed)return;completed=true;target.success(value);}
        @Override public synchronized void error(String code,String message,Object details){if(completed)return;completed=true;target.error(code,message,details);}
        @Override public synchronized void notImplemented(){if(completed)return;completed=true;target.notImplemented();}
    }
}
