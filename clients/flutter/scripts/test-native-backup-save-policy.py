"""Compile/run complete Broker and BackupPicker with JVM OS/SAF/JNI doubles.

No product source instrumentation, network, AVD or real account is used.
Restore/JNI payloads are synthetic; this is a permission boundary regression,
not normal Main, native backup codec, Android picker UI or device acceptance.
"""
from pathlib import Path
import argparse, hashlib, importlib.util, json, os, subprocess, tempfile, zipfile

ROOT=Path(__file__).resolve().parents[3]
spec=importlib.util.spec_from_file_location('broker_jvm_fixture',Path(__file__).with_name('test-native-vault-renew-snapshot.py'))
base=importlib.util.module_from_spec(spec);spec.loader.exec_module(base)


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--broker-source',type=Path);parser.add_argument('--picker-source',type=Path)
    args=parser.parse_args();source=ROOT/'sdks/flutter/android/src/main/java'/base.PACKAGE
    originals={name:(getattr(args,flag) or source/(name+'.java')).read_bytes() for name,flag in [('NativeVaultBroker','broker_source'),('NativeBackupPicker','picker_source')]}
    with tempfile.TemporaryDirectory(prefix='backup-save-jvm-',dir='D:/PUTY-local-build/mobile-temp') as directory:
        work=Path(directory);prod=work/'production';prod.mkdir();doubles=work/'doubles';doubles.mkdir();generated=work/'source';files=[]
        java=[]
        for name,raw in originals.items():
            p=generated/base.PACKAGE/(name+'.java');p.parent.mkdir(parents=True,exist_ok=True);p.write_bytes(raw);java.append(p)
        stub=dict(base.STUBS)
        for name in [base.PACKAGE+'/NativeBackupPicker.java',base.PACKAGE+'/RenewSnapshotHarness.java',base.PACKAGE+'/RenewSnapshotHooks.java']:stub.pop(name,None)
        stub.update(STUBS)
        for name,body in stub.items():
            p=work/'test-source'/name;p.parent.mkdir(parents=True,exist_ok=True);p.write_text(body,encoding='utf-8');files.append(p)
        jar=work/'go-bindings.jar'
        with zipfile.ZipFile(ROOT/'sdks/flutter/android/libs/puty-core-sdk-3.8.3-patch15-enterprise.aar') as archive:jar.write_bytes(archive.read('classes.jar'))
        embedding=list((ROOT/'.cache/gradle/caches/modules-2/files-2.1/io.flutter/flutter_embedding_release').rglob('*.jar'));assert len(embedding)==1
        android=base.SDK/'platforms/android-35/android.jar'
        if not android.is_file():android=base.SDK/'platforms/android-34/android.jar'
        cp=os.pathsep.join(map(str,[android,jar,*embedding]))
        helpers=['NativeVaultRenewal','NativeVaultSessionState','EncryptedSelectedFiles','EncryptedScratchFiles','MobileLegacyQuarantine']
        result=base.run([base.JAVA/'javac.exe','-encoding','UTF-8','-classpath',cp,'-d',prod,*java,*[source/(name+'.java') for name in helpers]])
        if result.returncode:return result.returncode
        result=base.run([base.JAVA/'javac.exe','-encoding','UTF-8','-classpath',os.pathsep.join([str(prod),cp]),'-d',doubles,*files])
        if result.returncode:return result.returncode
        runtime=work/'runtime';runtime.mkdir()
        result=base.run([base.JAVA/'java.exe','-Djava.io.tmpdir='+str(runtime),'-classpath',os.pathsep.join([str(doubles),str(prod),cp]),'io.openim.flutter_openim_sdk.BackupSaveHarness'])
        print(json.dumps({'wholeProductionClassesCompiledAndExecuted':True,'productSourceInstrumentation':False,
                         'sources':{k:hashlib.sha256(v).hexdigest() for k,v in originals.items()},'AndroidSAFAndJNITestDoubles':True,
                         'normalMainOrDeviceAcceptance':False,'productionRequests':0,'exitCode':result.returncode}))
        if result.returncode==0:print(result.stdout,end='')
        return result.returncode


STUBS={
'android/os/Handler.java':'''package android.os; import java.util.*;public final class Handler {public static boolean deferred;private static final Queue<Runnable> queue=new ArrayDeque<>();public Handler(Looper l){}public boolean post(Runnable r){if(deferred)queue.add(r);else r.run();return true;}public static void drain(){deferred=false;while(!queue.isEmpty())queue.remove().run();}}''',
'android/app/Activity.java':'''package android.app;public class Activity extends android.content.Context {public int launches,lastCode;public android.content.Intent lastIntent;public void startActivityForResult(android.content.Intent intent,int code){launches++;lastIntent=intent;lastCode=code;}}''',
'android/content/Intent.java':'''package android.content;public class Intent {public String action;private android.net.Uri data;public Intent(){}public Intent(String v){action=v;}public Intent addCategory(String v){return this;}public Intent setType(String v){return this;}public Intent putExtra(String k,String v){return this;}public Intent setData(android.net.Uri v){data=v;return this;}public android.net.Uri getData(){return data;}}''',
'android/net/Uri.java':'''package android.net;public final class Uri {private final String value;private Uri(String v){value=v;}public static Uri parse(String v){return new Uri(v);}public String getScheme(){return value.substring(0,value.indexOf(':'));}}''',
'android/os/ParcelFileDescriptor.java':'''package android.os;public final class ParcelFileDescriptor implements java.io.Closeable {public int getFd(){return 17;}public void close()throws java.io.IOException{}}''',
'android/content/ContentResolver.java':'''package android.content;import java.io.*;public class ContentResolver {public OutputStream output;public int outputOpens,inputOpens;public OutputStream openOutputStream(android.net.Uri u,String mode){outputOpens++;return output;}public android.os.ParcelFileDescriptor openFileDescriptor(android.net.Uri u,String mode){inputOpens++;return new android.os.ParcelFileDescriptor();}}''',
'android/content/Context.java':'''package android.content;import java.io.*;public class Context {public static File root;public static ContentResolver resolver;public Context getApplicationContext(){return this;}public File getNoBackupFilesDir(){return root;}public File getCacheDir(){return root;}public File getExternalFilesDir(String type){return root;}public File getExternalCacheDir(){return root;}public android.content.pm.ApplicationInfo getApplicationInfo(){return new android.content.pm.ApplicationInfo();}public ContentResolver getContentResolver(){return resolver;}}''',
'android/system/StructStat.java':'''package android.system;public final class StructStat {public int st_mode;public long st_nlink,st_dev,st_ino,st_size,st_mtime;}''',
'android/system/OsConstants.java':'''package android.system;public final class OsConstants {public static boolean S_ISREG(int mode){return (mode&0170000)==0100000;}}''',
'android/system/ErrnoException.java':'''package android.system;public final class ErrnoException extends Exception {public ErrnoException(){super("SYNTHETIC_STAT");}}''',
'android/system/Os.java':'''package android.system;import java.nio.file.*;public final class Os {public static StructStat lstat(String path)throws ErrnoException{try{Path p=Paths.get(path);StructStat s=new StructStat();s.st_mode=Files.isSymbolicLink(p)?0120000:Files.isRegularFile(p)?0100000:0040000;s.st_nlink=((Number)Files.getAttribute(p,"unix:nlink",LinkOption.NOFOLLOW_LINKS)).longValue();s.st_size=Files.size(p);return s;}catch(UnsupportedOperationException e){try{StructStat s=new StructStat();Path p=Paths.get(path);s.st_mode=Files.isSymbolicLink(p)?0120000:Files.isRegularFile(p)?0100000:0040000;s.st_nlink=1;s.st_size=Files.size(p);return s;}catch(Exception x){throw new ErrnoException();}}catch(Exception e){throw new ErrnoException();}}}''',
'open_im_sdk/NativeRestore.java':'''package open_im_sdk;public final class NativeRestore {public void abort(){}public byte[] master(){return new byte[32];}public boolean prepareCommit(byte[] value){return true;}public boolean commit(byte[] value){return true;}}''',
'open_im_sdk/Open_im_sdk.java':'''package open_im_sdk;import java.nio.file.*;public final class Open_im_sdk {public static int locks,exports,prepares;public static String exportPath;public static Runnable duringExport;public static byte[] bytes=new byte[150000];public static void lockNativeLocalStore(String a){locks++;}public static String exportNativeBackup(String a,String p){exports++;try{Files.write(Paths.get(exportPath),bytes);}catch(Exception e){throw new RuntimeException(e);}if(duringExport!=null)duringExport.run();return exportPath;}public static NativeRestore prepareNativeRestoreDescriptor(String a,long fd,String p){prepares++;return new NativeRestore();}public static NativeRestore prepareNativeRestore(String a,String path,String p){prepares++;return new NativeRestore();}public static byte[] readNativeAttachment(String a,String task,long limit){return new byte[]{17,18};}}''',
'io/openim/flutter_openim_sdk/BackupSaveHarness.java':r'''package io.openim.flutter_openim_sdk;
import java.io.*;import java.lang.reflect.*;import java.nio.file.*;import java.security.*;import java.util.*;import java.util.concurrent.*;import java.util.concurrent.atomic.*;import java.util.function.*;import io.flutter.plugin.common.MethodChannel;import open_im_sdk.Open_im_sdk;
public final class BackupSaveHarness {
 static final String ACCOUNT="emp_00000000000000000000000000000017";
 static int passed;
 static void check(boolean v,String code){if(!v)throw new AssertionError(code);}
 static Field field(String name)throws Exception{Field f=NativeVaultBroker.class.getDeclaredField(name);f.setAccessible(true);return f;}
 static void set(NativeVaultBroker b,String n,Object v)throws Exception{field(n).set(b,v);}
 static final class Direct extends AbstractExecutorService{public void execute(Runnable r){r.run();}public void shutdown(){}public java.util.List<Runnable> shutdownNow(){return Collections.emptyList();}public boolean isShutdown(){return false;}public boolean isTerminated(){return false;}public boolean awaitTermination(long n,TimeUnit u){return true;}}
 static final class Result implements MethodChannel.Result{Object value;int successes,errors;public void success(Object v){value=v;successes++;}public void error(String c,String m,Object d){errors++;}public void notImplemented(){errors++;}}
 static final class Fixture {
  NativeVaultBroker b;NativeBackupPicker p;android.app.Activity activity=new android.app.Activity();android.content.ContentResolver resolver=new android.content.ContentResolver();File file;
  Fixture()throws Exception{
   android.os.Handler.deferred=false;android.os.Handler.drain();android.content.Context.root=Files.createTempDirectory(Paths.get(System.getProperty("java.io.tmpdir")),"owned-").toFile();android.content.Context.resolver=resolver;
   Field u=sun.misc.Unsafe.class.getDeclaredField("theUnsafe");u.setAccessible(true);b=(NativeVaultBroker)((sun.misc.Unsafe)u.get(null)).allocateInstance(NativeVaultBroker.class);
   set(b,"context",new android.content.Context());set(b,"stateLock",new Object());set(b,"generation",new AtomicLong(1));set(b,"authorizationSerial",new AtomicLong(1));set(b,"unlocking",new AtomicBoolean());set(b,"refreshing",new AtomicBoolean());set(b,"restores",new HashMap<>());set(b,"executor",new Direct());set(b,"authorizationExecutor",new Direct());set(b,"activeAccount",ACCOUNT);set(b,"leaseDeadline",android.os.SystemClock.elapsedRealtime()+8000);set(b,"saveAllowed",true);set(b,"previewAllowed",true);
   EncryptedScratchFiles.Authority authority=new EncryptedScratchFiles.Authority(){public String authorizedAccount(){try{return android.os.SystemClock.elapsedRealtime()<field("leaseDeadline").getLong(b)?(String)field("activeAccount").get(b):null;}catch(Exception e){throw new RuntimeException(e);}}public long generation(){try{return ((AtomicLong)field("generation").get(b)).get();}catch(Exception e){throw new RuntimeException(e);}}};
   Constructor<?> constructor=NativeBackupPicker.class.getDeclaredConstructors()[0];constructor.setAccessible(true);
   if(constructor.getParameterCount()==4)p=(NativeBackupPicker)constructor.newInstance(new android.content.Context(),new Direct(),authority,(BooleanSupplier)()->{try{return authority.authorizedAccount()!=null&&field("saveAllowed").getBoolean(b);}catch(Exception e){throw new RuntimeException(e);}});else p=(NativeBackupPicker)constructor.newInstance(new android.content.Context(),new Direct(),authority);
   set(b,"backupPicker",p);p.attach(activity);StringBuilder digest=new StringBuilder();for(byte v:MessageDigest.getInstance("SHA-256").digest(ACCOUNT.getBytes("UTF-8")))digest.append(String.format("%02x",v));File directory=new File(android.content.Context.root,"puty-local/accounts/"+digest+"/backups");check(directory.mkdirs(),"FIXTURE_DIRECTORY");file=new File(directory,"PUTY_IM-20261104-120000-000000000000000000000017.backup");Open_im_sdk.exportPath=file.getCanonicalPath();Open_im_sdk.exports=0;Open_im_sdk.prepares=0;Open_im_sdk.duringExport=null;
  }
  void allow(boolean value){try{set(b,"saveAllowed",value);}catch(Exception e){throw new RuntimeException(e);}}
  Result call(String method,Map<String,Object> args){Result r=new Result();b.handle(method,args,r);return r;}
  void retain()throws Exception{Files.write(file.toPath(),Open_im_sdk.bytes);for(Method m:NativeBackupPicker.class.getDeclaredMethods())if(m.getName().equals("retainExport")){m.setAccessible(true);if(m.getParameterCount()==3)m.invoke(p,file.getCanonicalPath(),1L,ACCOUNT);else m.invoke(p,file.getCanonicalPath(),1L);return;}throw new AssertionError("RETAIN_MISSING");}
  Result launchSave()throws Exception{Result r=call("backup/save",Collections.singletonMap("path",file.getCanonicalPath()));return r;}
  void selected(){p.onActivityResult(activity.lastCode,-1,new android.content.Intent().setData(android.net.Uri.parse("content://synthetic/document")));}
 }
 static void test(String name,Checked action)throws Exception{try{action.run();passed++;System.out.println("PASS "+name);}catch(AssertionError e){System.out.println("FAIL "+name+" "+e.getMessage());}}
 interface Checked{void run()throws Exception;}
 public static void main(String[] args)throws Exception{
  test("initial_export_deny_never_invokes_go",()->{Fixture f=new Fixture();f.allow(false);Result r=f.call("backup/export",Collections.singletonMap("passphrase","SYNTHETIC_PASSPHRASE"));check(r.errors==1&&Open_im_sdk.exports==0,"FORBIDDEN_EXPORT_CREATED");check(ACCOUNT.equals(field("activeAccount").get(f.b)),"SAVE_DENY_LOCKED_LOGIN");});
  test("initial_save_deny_never_opens_picker",()->{Fixture f=new Fixture();f.retain();f.allow(false);Result r=f.launchSave();check(r.errors==1&&f.activity.launches==0,"FORBIDDEN_PICKER_OPENED");});
  test("allowed_export_returns_owned_cipher_only",()->{Fixture f=new Fixture();Result r=f.call("backup/export",Collections.singletonMap("passphrase","SYNTHETIC_PASSPHRASE"));check(r.successes==1&&Open_im_sdk.exports==1&&f.file.isFile(),"ALLOWED_EXPORT_REJECTED");});
  test("allowed_save_copies_complete_cipher_and_releases_owned_export",()->{Fixture f=new Fixture();f.retain();ByteArrayOutputStream sink=new ByteArrayOutputStream();f.resolver.output=sink;Result r=f.launchSave();f.selected();check(r.successes==1&&Boolean.TRUE.equals(r.value)&&sink.size()==Open_im_sdk.bytes.length&&!f.file.exists(),"ALLOWED_SAVE_REJECTED");});
  test("export_revocation_before_retain_deletes_only_owned_output",()->{Fixture f=new Fixture();Open_im_sdk.duringExport=()->f.allow(false);Result r=f.call("backup/export",Collections.singletonMap("passphrase","SYNTHETIC_PASSPHRASE"));check(r.errors==1&&!f.file.exists(),"REVOKED_EXPORT_RETAINED");});
  test("foreign_unretained_export_is_never_deleted",()->{Fixture f=new Fixture();File foreign=new File(android.content.Context.root,"foreign.backup");Open_im_sdk.exportPath=foreign.getCanonicalPath();Open_im_sdk.duringExport=()->f.allow(false);Result r=f.call("backup/export",Collections.singletonMap("passphrase","SYNTHETIC_PASSPHRASE"));check(r.errors==1&&foreign.isFile(),"FOREIGN_EXPORT_DELETED_OR_DELIVERED");});
  test("export_delivery_rechecks_save_permission",()->{Fixture f=new Fixture();android.os.Handler.deferred=true;Result r=f.call("backup/export",Collections.singletonMap("passphrase","SYNTHETIC_PASSPHRASE"));f.allow(false);android.os.Handler.drain();check(r.errors==1&&r.successes==0&&!f.file.exists(),"REVOKED_EXPORT_DELIVERED");});
  test("save_revoked_while_picker_open_does_not_open_destination",()->{Fixture f=new Fixture();f.retain();Result r=f.launchSave();f.allow(false);f.selected();check(r.errors==1&&f.resolver.outputOpens==0,"DENIED_DESTINATION_OPENED");});
  test("save_revocation_stops_cipher_copy_per_chunk",()->{Fixture f=new Fixture();f.retain();ByteArrayOutputStream sink=new ByteArrayOutputStream();f.resolver.output=new FilterOutputStream(sink){public void write(byte[] b,int o,int n)throws IOException{out.write(b,o,n);f.allow(false);}};Result r=f.launchSave();f.selected();check(r.errors==1&&sink.size()>0&&sink.size()<=65536,"REVOKED_COPY_CONTINUED");});
  test("save_revocation_at_flush_rejects_completion",()->{Fixture f=new Fixture();f.retain();f.resolver.output=new ByteArrayOutputStream(){public void flush(){f.allow(false);}};Result r=f.launchSave();f.selected();check(r.errors==1&&r.successes==0,"REVOKED_SAVE_SUCCESS");});
  test("save_delivery_rechecks_current_actor_and_permission",()->{Fixture f=new Fixture();f.retain();f.resolver.output=new ByteArrayOutputStream();Result r=f.launchSave();android.os.Handler.deferred=true;f.selected();f.allow(false);android.os.Handler.drain();check(r.errors==1&&r.successes==0,"REVOKED_SAVE_RESULT_DELIVERED");});
  test("save_denied_restore_pick_prepare_and_preview_remain_available",()->{Fixture f=new Fixture();f.allow(false);Result picked=f.call("backup/pick",null);check(f.activity.launches==1,"RESTORE_PICK_DENIED");f.selected();check(picked.successes==1&&picked.value instanceof String,"RESTORE_DESCRIPTOR_DENIED");Map<String,Object> a=new HashMap<>();a.put("path",picked.value);a.put("passphrase","SYNTHETIC_PASSPHRASE");Result prepared=f.call("backup/prepare",a);check(prepared.successes==1&&Open_im_sdk.prepares==1,"RESTORE_PREPARE_DENIED");Result read=f.call("attachment/read",Collections.singletonMap("taskID","synthetic-task"));check(read.successes==1&&read.value instanceof byte[],"INTERNAL_PREVIEW_DENIED");});
  check(passed==12,"BACKUP_SAVE_POLICY_FAILED_"+passed+"_OF_12");System.out.println("COMPLETE 12/12 whole Broker/BackupPicker save-policy cases");
 }
}''',
}

if __name__=='__main__':raise SystemExit(main())
