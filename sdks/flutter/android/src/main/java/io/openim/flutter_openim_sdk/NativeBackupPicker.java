package io.openim.flutter_openim_sdk;

import android.app.Activity;
import android.content.Context;
import android.content.Intent;
import android.net.Uri;
import android.os.Handler;
import android.os.Looper;
import io.flutter.plugin.common.MethodChannel;
import io.flutter.plugin.common.PluginRegistry;
import java.io.*;
import java.util.*;
import java.util.concurrent.Executor;
import java.util.function.BooleanSupplier;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;

/** Only complete encrypted backup containers may cross SAF. No selected
 * document is copied to a public/plaintext application cache. */
final class NativeBackupPicker implements PluginRegistry.ActivityResultListener {
 private final Context context;private final Executor executor;private final EncryptedScratchFiles.Authority authority;private final BooleanSupplier canSave;
 private final Handler main=new Handler(Looper.getMainLooper());private Activity activity;private int nextCode=0x6e00;
 private Pending pending;private File exported;private android.os.ParcelFileDescriptor input;private String inputID;private long inputGeneration;private long exportGeneration;
 private static final class Pending{final int code;final long generation;final String account;final boolean save;final File file;final MethodChannel.Result result;Pending(int code,long generation,String account,boolean save,File file,MethodChannel.Result result){this.code=code;this.generation=generation;this.account=account;this.save=save;this.file=file;this.result=result;}}
 NativeBackupPicker(Context context,Executor executor,EncryptedScratchFiles.Authority authority,BooleanSupplier canSave){this.context=context;this.executor=executor;this.authority=authority;this.canSave=canSave;}
 synchronized void attach(Activity value){activity=value;}
 private File ownedExport(String path,String account)throws IOException{
  if(path==null||account==null||!account.matches("emp_[0-9a-f]{32}"))throw new IOException("Invalid encrypted backup ownership");
  try{
   StringBuilder hash=new StringBuilder();for(byte value:MessageDigest.getInstance("SHA-256").digest(account.getBytes(StandardCharsets.UTF_8)))hash.append(String.format(Locale.ROOT,"%02x",value));
   File parent=new File(context.getNoBackupFilesDir().getCanonicalFile(),"puty-local/accounts/"+hash+"/backups").getAbsoluteFile(),file=new File(path).getAbsoluteFile();
   if(!parent.equals(parent.getCanonicalFile())||!file.equals(file.getCanonicalFile())||!parent.equals(file.getParentFile())||!file.getName().matches("PUTY_IM-[0-9]{8}-[0-9]{6}-[0-9a-f]{24}\\.backup"))throw new IOException("Invalid encrypted backup ownership");
   android.system.StructStat info=android.system.Os.lstat(file.getPath());if(!android.system.OsConstants.S_ISREG(info.st_mode)||info.st_nlink!=1)throw new IOException("Invalid encrypted backup ownership");return file;
  }catch(java.security.NoSuchAlgorithmException|android.system.ErrnoException invalid){throw new IOException("Invalid encrypted backup ownership",invalid);}
 }
 synchronized void retainExport(String path,long generation,String account)throws IOException{File file=ownedExport(path,account);if(generation!=authority.generation()||!account.equals(authority.authorizedAccount())||!canSave.getAsBoolean())throw new IOException("Backup save permission changed");if(exported!=null&&!exported.equals(file))delete(exported);exported=file;exportGeneration=generation;}
 synchronized void discardUnretainedExport(String path,String account){try{File file=ownedExport(path,account);if(!file.equals(exported))delete(file);}catch(IOException ignored){}}
 synchronized void discardExport(String path){if(exported!=null&&exported.getAbsolutePath().equals(path)){delete(exported);exported=null;}}
 private static void delete(File file){if(file!=null&&file.isFile())file.delete();}
 private boolean current(Pending p){return p.generation==authority.generation()&&p.account!=null&&p.account.equals(authority.authorizedAccount())&&(!p.save||canSave.getAsBoolean());}
 void launch(boolean save,String path,MethodChannel.Result result){main.post(()->{try{final Pending p;synchronized(this){if(activity==null||pending!=null||nextCode>0x6eff||authority.authorizedAccount()==null||(save&&!canSave.getAsBoolean()))throw new IOException("Backup picker unavailable");File file=null;if(save){file=new File(path).getCanonicalFile();if(exported==null||!file.equals(exported)||exportGeneration!=authority.generation())throw new IOException("Unknown encrypted backup export");}p=new Pending(nextCode++,authority.generation(),authority.authorizedAccount(),save,file,result);if(!current(p))throw new IOException("Backup identity changed");pending=p;Intent intent=new Intent(save?Intent.ACTION_CREATE_DOCUMENT:Intent.ACTION_OPEN_DOCUMENT);intent.addCategory(Intent.CATEGORY_OPENABLE);intent.setType(save?"application/octet-stream":"*/*");if(save)intent.putExtra(Intent.EXTRA_TITLE,file.getName());activity.startActivityForResult(intent,p.code);}}catch(Exception denied){result.error("LOCAL_BACKUP_UNAVAILABLE","BackupPickerUnavailable",null);}});}
 @Override public boolean onActivityResult(int requestCode,int resultCode,Intent data){final Pending p;synchronized(this){if(pending==null||pending.code!=requestCode)return false;p=pending;pending=null;}
  executor.execute(()->{Object value=null;String error=null;try{
   if(!current(p))throw new IOException("Backup identity changed");Uri uri=data==null?null:data.getData();
   if(resultCode!=Activity.RESULT_OK||uri==null){value=p.save?false:null;}
   else{if(!"content".equals(uri.getScheme()))throw new IOException("Unsupported document provider");
    if(p.save){try(InputStream from=new FileInputStream(p.file);OutputStream to=context.getContentResolver().openOutputStream(uri,"w")){if(to==null)throw new IOException("Backup destination unavailable");copy(from,to,p);to.flush();}value=true;}
    else{
     android.os.ParcelFileDescriptor descriptor=context.getContentResolver().openFileDescriptor(uri,"r");if(descriptor==null)throw new IOException("Backup document unavailable");
     synchronized(this){releaseInput(inputID);input=descriptor;inputID="puty-backup:"+UUID.randomUUID();inputGeneration=p.generation;value=inputID;}

    }
   }
   if(!current(p))throw new IOException("Backup identity changed");
  }catch(Exception denied){error="BackupOperationRejected";}finally{if(p.save)synchronized(this){if(exported!=null&&exported.equals(p.file)){delete(exported);exported=null;}}}
  final Object response=value;final String failure=error;main.post(()->{if(!current(p)||failure!=null)p.result.error("LOCAL_BACKUP_UNAVAILABLE","BackupOperationRejected",null);else p.result.success(response);});});return true;
 }
 private void copy(InputStream from,OutputStream to,Pending p)throws IOException{byte[] buffer=new byte[65536];long total=0;try{for(;;){if(!current(p))throw new IOException("Backup identity changed");int n=from.read(buffer);if(n==-1)break;if(!current(p))throw new IOException("Backup identity changed");total+=n;if(total>64L*1024*1024*1024)throw new IOException("Encrypted backup capacity exceeded");to.write(buffer,0,n);if(!current(p))throw new IOException("Backup identity changed");}}finally{Arrays.fill(buffer,(byte)0);}}
 synchronized int inputDescriptor(String identity)throws IOException{if(input==null||identity==null||!identity.equals(inputID)||inputGeneration!=authority.generation()||authority.authorizedAccount()==null)throw new IOException("Backup descriptor unavailable");return input.getFd();}
 synchronized void releaseInput(String identity){if(input!=null&&Objects.equals(identity,inputID)){try{input.close();}catch(IOException ignored){}input=null;inputID=null;}}
 synchronized void close(){Pending p=pending;pending=null;if(p!=null)main.post(()->p.result.error("LOCAL_BACKUP_UNAVAILABLE","BackupIdentityChanged",null));delete(exported);exported=null;releaseInput(inputID);}
}
