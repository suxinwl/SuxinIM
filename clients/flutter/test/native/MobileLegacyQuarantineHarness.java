package io.openim.flutter_openim_sdk;

import java.io.*;
import java.nio.charset.StandardCharsets;
import java.nio.file.*;
import java.nio.file.attribute.*;
import java.security.*;
import java.util.*;
import javax.crypto.*;
import javax.crypto.spec.*;

/** Exercises the actual production core/codec with only Android FS/Keystore
 * boundaries substituted. All inputs are synthetic and under one test root. */
public final class MobileLegacyQuarantineHarness {
  static int passed;
  static Path home;
  interface Case {void run()throws Exception;}
  static void test(String name,Case value)throws Exception{value.run();passed++;System.out.println("PASS "+name);}
  static void require(boolean value)throws Exception{if(!value)throw new Exception("Synthetic assertion failed");}
  static void reject(Case action)throws Exception{boolean failed=false;try{action.run();}catch(Exception expected){failed=true;}require(failed);}
  static String sha(byte[] data)throws Exception{StringBuilder out=new StringBuilder();for(byte b:MessageDigest.getInstance("SHA-256").digest(data))out.append(String.format(Locale.ROOT,"%02x",b));return out.toString();}
  static byte[] bytes(String value){return value.getBytes(StandardCharsets.UTF_8);}
  static byte[] all(InputStream input)throws Exception{try(InputStream in=input;ByteArrayOutputStream out=new ByteArrayOutputStream()){byte[] b=new byte[32768];for(int n;(n=in.read(b))!=-1;)out.write(b,0,n);return out.toByteArray();}}
  static final class Keys implements MobileLegacyQuarantine.KeyProvider {
    final byte[] key=new byte[32];Keys(){Arrays.fill(key,(byte)77);}
    public byte[] wrap(byte[] value)throws Exception{Cipher c=Cipher.getInstance("AES/GCM/NoPadding");c.init(Cipher.ENCRYPT_MODE,new SecretKeySpec(key,"AES"));byte[] b=c.doFinal(value);ByteArrayOutputStream out=new ByteArrayOutputStream();out.write(c.getIV());out.write(b);return out.toByteArray();}
    public byte[] unwrap(byte[] value)throws Exception{Cipher c=Cipher.getInstance("AES/GCM/NoPadding");c.init(Cipher.DECRYPT_MODE,new SecretKeySpec(key,"AES"),new GCMParameterSpec(128,Arrays.copyOf(value,12)));return c.doFinal(value,12,value.length-12);}
  }
  static class TestFs implements MobileLegacyQuarantine.Fs {
    boolean diskFull,failSync,failDelete,dataFaultOnly;int writes,partialFaults,dataSyncCalls,failOnDataSync=-1;
    boolean dataDirectory(File path){return path.getParentFile()!=null&&path.getParentFile().getName().equals("objects");}
    public MobileLegacyQuarantine.Entry stat(File f)throws Exception{
      BasicFileAttributes a=Files.readAttributes(f.toPath(),BasicFileAttributes.class,LinkOption.NOFOLLOW_LINKS);
      // fileKey is stable across reopen and changes when an exact path is replaced.
      long id=a.fileKey()==null?f.getAbsolutePath().hashCode():a.fileKey().hashCode();
      return new MobileLegacyQuarantine.Entry(1,id,a.size(),a.lastModifiedTime().toMillis(),a.isRegularFile(),a.isDirectory(),1);
    }
    public InputStream read(File f,MobileLegacyQuarantine.Entry e)throws Exception{if(!e.regular||!e.same(stat(f)))throw new IOException("Synthetic read guard");return Files.newInputStream(f.toPath(),LinkOption.NOFOLLOW_LINKS);}
    public FileOutputStream create(File f)throws Exception{
      Files.createFile(f.toPath());writes++;
      return new FileOutputStream(f){int written;
        @Override public void write(byte[] b,int at,int count)throws IOException{
          if(diskFull&&(!dataFaultOnly||dataDirectory(f.getParentFile()))){int remaining=Math.max(0,100-written);if(count>remaining){if(remaining>0){super.write(b,at,remaining);written+=remaining;}partialFaults++;throw new IOException("Synthetic ENOSPC");}}
          super.write(b,at,count);written+=count;
        }
        @Override public void write(byte[] b)throws IOException{write(b,0,b.length);}
      };
    }
    public void mkdir(File f)throws Exception{Files.createDirectory(f.toPath());}
    public void moveNew(File from,File to)throws Exception{Files.move(from.toPath(),to.toPath());}
    public void replace(File from,File to)throws Exception{Files.move(from.toPath(),to.toPath(),StandardCopyOption.REPLACE_EXISTING,StandardCopyOption.ATOMIC_MOVE);}
    public void sync(File f)throws Exception{if(dataDirectory(f)&&++dataSyncCalls==failOnDataSync)throw new IOException("Synthetic metadata directory EIO");if(failSync&&(!dataFaultOnly||dataDirectory(f)))throw new IOException("Synthetic EIO");}
    public void syncFile(File f,MobileLegacyQuarantine.Entry expected)throws Exception{if(!expected.same(stat(f))||!expected.regular)throw new IOException("Synthetic file sync guard");try(RandomAccessFile file=new RandomAccessFile(f,"rw")){file.getFD().sync();}}
    public void remove(File f,MobileLegacyQuarantine.Entry e)throws Exception{if(failDelete)throw new IOException("Synthetic EACCES");if(!e.same(stat(f))||!e.regular)throw new IOException("Synthetic delete guard");Files.delete(f.toPath());}
  }
  static final class Fixture implements AutoCloseable {
    final Path base,source,quarantine;final Keys keys=new Keys();final TestFs fs=new TestFs();MobileLegacyQuarantine q;
    Fixture()throws Exception{base=Files.createTempDirectory(home,"case-").toRealPath();source=Files.createDirectory(base.resolve("app_flutter"));quarantine=base.resolve("no_backup/puty-local/legacy-quarantine");open(phase->{});}
    void open(MobileLegacyQuarantine.Hook hook)throws Exception{if(q!=null)q.close();q=new MobileLegacyQuarantine(quarantine.toFile(),Collections.singletonList(new MobileLegacyQuarantine.Root(source.toFile())),fs,keys,hook);}
    Path file(String name,byte[] data)throws Exception{Path path=source.resolve(name);Files.createDirectories(path.getParent());Files.write(path,data);return path;}
    Path object(Path source,byte[] data,String extension)throws Exception{return quarantine.resolve("objects").resolve(sha(bytes(source.toString()))).resolve(sha(data)+extension);}
    void absentTemps()throws Exception{try(java.util.stream.Stream<Path> list=Files.walk(quarantine)){require(list.noneMatch(p->p.getFileName().toString().startsWith(".sealed-")));}}
    public void close()throws Exception{if(q!=null)q.close();if(!base.toRealPath().startsWith(home))throw new Exception("Synthetic cleanup scope");try(java.util.stream.Stream<Path> list=Files.walk(base)){for(Path p:(Iterable<Path>)list.sorted(Comparator.reverseOrder())::iterator)Files.deleteIfExists(p);}}
  }
  public static void main(String[] args)throws Exception{
    home=Paths.get(args[0]).toRealPath();if(!home.getFileName().toString().startsWith("mobile-legacy-tests-"))throw new Exception("Synthetic root required");
    if(args.length==2&&args[1].equals("verify-go")){
      byte[] key=new byte[32];Arrays.fill(key,(byte)42);byte[] body=interopBody();String source="/synthetic/app_flutter/unknown.bin",digest=sha(body),id=sha(bytes(source))+"-"+digest;
      require(Arrays.equals(body,all(new MobileLegacyQuarantine.ObjectInput(Files.newInputStream(home.resolve("interop-go.data")),key,id,"legacy-file"))));
      String meta=new String(all(new MobileLegacyQuarantine.ObjectInput(Files.newInputStream(home.resolve("interop-go.meta")),key,id,"legacy-metadata")),StandardCharsets.UTF_8);
      require(meta.contains("\"source\":\""+source+"\"")&&meta.contains("\"sha256\":\""+digest+"\""));System.out.println("PASS real_Go_to_Java_PUTYLS1_data_and_metadata");return;
    }
    test("streaming_empty_boundary_and_multichunk",()->{
      for(int size:new int[]{0,1,1048575,1048576,1048577,2097171}){byte[] data=new byte[size];new Random(11).nextBytes(data);byte[] master=new byte[32];Arrays.fill(master,(byte)42);ByteArrayOutputStream output=new ByteArrayOutputStream();MobileLegacyQuarantine.encrypt(output,new ByteArrayInputStream(data),master,"synthetic-object","legacy-file");byte[] encrypted=output.toByteArray();require(Arrays.equals(Arrays.copyOf(encrypted,8),bytes("PUTYLS1\n")));require(Arrays.equals(data,all(new MobileLegacyQuarantine.ObjectInput(new ByteArrayInputStream(encrypted),master,"synthetic-object","legacy-file"))));}
    });
    test("codec_tamper_truncate_trailing_wrong_identity",()->{
      byte[] key=new byte[32];ByteArrayOutputStream output=new ByteArrayOutputStream();MobileLegacyQuarantine.encrypt(output,new ByteArrayInputStream(bytes("synthetic-private-marker")),key,"identity","legacy-file");byte[] object=output.toByteArray();
      for(int i=0;i<object.length;i++){byte[] changed=object.clone();changed[i]^=1;reject(()->all(new MobileLegacyQuarantine.ObjectInput(new ByteArrayInputStream(changed),key,"identity","legacy-file")));}
      for(int i=0;i<object.length;i++){final int length=i;reject(()->all(new MobileLegacyQuarantine.ObjectInput(new ByteArrayInputStream(Arrays.copyOf(object,length)),key,"identity","legacy-file")));}
      reject(()->all(new MobileLegacyQuarantine.ObjectInput(new ByteArrayInputStream(Arrays.copyOf(object,object.length+1)),key,"identity","legacy-file")));
      reject(()->all(new MobileLegacyQuarantine.ObjectInput(new ByteArrayInputStream(object),key,"other","legacy-file")));
      reject(()->all(new MobileLegacyQuarantine.ObjectInput(new ByteArrayInputStream(object),key,"identity","legacy-metadata")));
      byte[] other=key.clone();other[0]=1;reject(()->all(new MobileLegacyQuarantine.ObjectInput(new ByteArrayInputStream(object),other,"identity","legacy-file")));
    });
    test("sealed_verified_exact_delete_and_restricted_recovery",()->{try(Fixture f=new Fixture()){
      byte[] data=bytes("synthetic unknown other-account cache");Path p=f.file("other-account/cache.bin",data);MobileLegacyQuarantine.Result result=f.q.sealRemaining();require(result.sealedFiles==1&&!Files.exists(p));require(Files.exists(f.object(p,data,".data"))&&Files.exists(f.object(p,data,".meta")));
      require(Arrays.equals(data,all(f.q.resolveStream(p.toString(),sha(data),source->source.equals(p.toString())))));
      reject(()->f.q.resolveStream(p.toString(),sha(data),source->false));reject(()->f.q.resolveStream(p.toString()+"/../escape",sha(data),source->true));
      try(java.util.stream.Stream<Path> files=Files.walk(f.quarantine)){for(Path path:(Iterable<Path>)files.filter(Files::isRegularFile)::iterator)require(!new String(Files.readAllBytes(path),StandardCharsets.ISO_8859_1).contains("synthetic unknown"));}
      f.open(phase->{});require(f.q.sealRemaining().sealedFiles==0);f.absentTemps();
    }});
    test("old_same_source_versions_never_overwritten",()->{try(Fixture f=new Fixture()){
      byte[] old=bytes("first"),next=bytes("second");Path p=f.file("hive.box",old);f.q.sealRemaining();byte[] previous=Files.readAllBytes(f.object(p,old,".data"));f.file("hive.box",next);f.q.sealRemaining();require(Arrays.equals(previous,Files.readAllBytes(f.object(p,old,".data"))));require(Arrays.equals(old,all(f.q.resolveStream(p.toString(),sha(old),s->true))));require(Arrays.equals(next,all(f.q.resolveStream(p.toString(),sha(next),s->true))));
    }});
    test("sqlite_and_wal_family_retained_managed_directory_skipped",()->{try(Fixture f=new Fixture()){
      for(String n:new String[]{"OpenIM_v3_synthetic.db","OpenIM_v3_synthetic.db-wal","OpenIM_v3_synthetic.db-shm","OpenIM_v3_synthetic.db-journal","puty-local/current/encrypted.data"})f.file(n,bytes("preserve"));f.file("unknown.log",bytes("seal"));MobileLegacyQuarantine.Result result=f.q.sealRemaining();require(result.sealedFiles==1&&result.skippedDatabases==4);require(Files.exists(f.source.resolve("OpenIM_v3_synthetic.db-wal")));require(Files.exists(f.source.resolve("puty-local/current/encrypted.data")));
    }});
    test("enospc_keeps_source_and_old_objects_no_temp_leak",()->{try(Fixture f=new Fixture()){
      byte[] old=bytes("first");Path p=f.file("cache.txt",old);f.q.sealRemaining();byte[] previous=Files.readAllBytes(f.object(p,old,".data"));f.file("cache.txt",bytes("next"));f.fs.diskFull=true;f.fs.dataFaultOnly=true;reject(()->f.q.sealRemaining());require(f.fs.partialFaults==1&&Files.exists(p));require(!Files.exists(f.object(p,bytes("next"),".data")));require(Arrays.equals(previous,Files.readAllBytes(f.object(p,old,".data"))));f.absentTemps();f.fs.diskFull=false;require(f.q.sealRemaining().sealedFiles==1);
    }});
    test("durability_failure_never_unlinks_source",()->{try(Fixture f=new Fixture()){
      byte[] body=bytes("sync-fault");Path p=f.file("cache.txt",body);f.fs.failSync=true;f.fs.dataFaultOnly=true;reject(()->f.q.sealRemaining());require(Files.exists(p)&&Files.exists(f.object(p,body,".data"))&&!Files.exists(f.object(p,body,".meta")));f.fs.failSync=false;require(f.q.sealRemaining().sealedFiles==1);f.absentTemps();
    }});
    test("cipher_corruption_before_delete_rejected",()->{try(Fixture f=new Fixture()){
      byte[] data=bytes("retain source");Path p=f.file("cache.txt",data);f.open(phase->{if(phase.equals("data-durable")){Path sealed=f.object(p,data,".data");byte[] b=Files.readAllBytes(sealed);b[b.length-1]^=1;Files.write(sealed,b);}});reject(()->f.q.sealRemaining());require(Files.exists(p));
    }});
    test("resume_reestablishes_complete_receipt_durability_before_unlink",()->{try(Fixture f=new Fixture()){
      byte[] data=bytes("metadata rename before directory EIO");Path p=f.file("cache.txt",data);f.fs.failOnDataSync=2;reject(()->f.q.sealRemaining());require(Files.exists(p)&&Files.exists(f.object(p,data,".data"))&&Files.exists(f.object(p,data,".meta")));
      f.fs.failOnDataSync=-1;f.fs.failSync=true;f.fs.dataFaultOnly=true;reject(()->f.q.sealRemaining());require(Files.exists(p));f.fs.failSync=false;require(f.q.sealRemaining().sealedFiles==1&&!Files.exists(p));
    }});
    test("source_change_after_verified_receipt_never_deleted",()->{try(Fixture f=new Fixture()){
      Path p=f.file("cache.txt",bytes("old"));f.open(phase->{if(phase.equals("before-delete"))Files.write(p,bytes("replacement"));});reject(()->f.q.sealRemaining());require(Arrays.equals(bytes("replacement"),Files.readAllBytes(p)));f.open(phase->{});require(f.q.sealRemaining().sealedFiles==1);
    }});
    test("crash_after_data_before_meta_resumes_same_object",()->{try(Fixture f=new Fixture()){
      byte[] data=bytes("restartable");Path p=f.file("cache.txt",data);f.open(phase->{if(phase.equals("data-durable"))throw new IOException("Synthetic process interruption");});reject(()->f.q.sealRemaining());byte[] sealed=Files.readAllBytes(f.object(p,data,".data"));require(Files.exists(p)&&!Files.exists(f.object(p,data,".meta")));f.open(phase->{});f.q.sealRemaining();require(!Files.exists(p)&&Arrays.equals(sealed,Files.readAllBytes(f.object(p,data,".data"))));
    }});
    test("crash_after_some_unlinks_resumes_remaining_only",()->{try(Fixture f=new Fixture()){
      Path a=f.file("a.txt",bytes("a")),b=f.file("b.txt",bytes("b"));f.open(phase->{if(phase.equals("source-deleted"))throw new IOException("Synthetic interruption");});reject(()->f.q.sealRemaining());require(!Files.exists(a)&&Files.exists(b));f.open(phase->{});require(f.q.sealRemaining().sealedFiles==1&&!Files.exists(b));
    }});
    test("unlink_denied_source_preserved_then_retry",()->{try(Fixture f=new Fixture()){
      byte[] data=bytes("delete retry");Path p=f.file("cache.txt",data);f.fs.failDelete=true;reject(()->f.q.sealRemaining());require(Files.exists(p)&&Files.exists(f.object(p,data,".meta")));f.fs.failDelete=false;require(f.q.sealRemaining().sealedFiles==1&&!Files.exists(p));
    }});
    test("missing_or_corrupt_master_never_rekeys_sealed_archive",()->{try(Fixture f=new Fixture()){
      f.file("cache.txt",bytes("sealed"));f.q.sealRemaining();Files.delete(f.quarantine.resolve("master.wrap"));reject(()->f.open(phase->{}));require(!Files.exists(f.quarantine.resolve("master.wrap")));
    }});
    test("new_file_during_migration_retained_and_next_run_sealed",()->{try(Fixture f=new Fixture()){
      f.file("a.txt",bytes("a"));f.open(phase->{if(phase.equals("source-deleted"))f.file("new.txt",bytes("new"));});reject(()->f.q.sealRemaining());require(Files.exists(f.source.resolve("new.txt")));f.open(phase->{});require(f.q.sealRemaining().sealedFiles==1);
    }});
    test("nonregular_or_link_source_zero_delete",()->{try(Fixture f=new Fixture()){
      Path outside=f.base.resolve("outside.txt");Files.write(outside,bytes("outside"));Path link=f.source.resolve("linked.txt");boolean made=false;try{Files.createSymbolicLink(link,outside);made=true;}catch(FileSystemException unavailable){/* Windows without symlink privilege: exercise exact nonregular stat below. */}
      if(made){reject(()->f.q.sealRemaining());require(Files.exists(outside));Files.delete(link);}else{Path p=f.file("nonregular",bytes("keep"));TestFs nonregular=new TestFs(){public MobileLegacyQuarantine.Entry stat(File file)throws Exception{MobileLegacyQuarantine.Entry e=super.stat(file);return file.equals(p.toFile())?new MobileLegacyQuarantine.Entry(e.device,e.inode,e.size,e.modified,false,false,1):e;}};f.q.close();MobileLegacyQuarantine q=new MobileLegacyQuarantine(f.quarantine.toFile(),Collections.singletonList(new MobileLegacyQuarantine.Root(f.source.toFile())),nonregular,f.keys,phase->{});try{reject(q::sealRemaining);require(Files.exists(p));}finally{q.close();}}
    }});
    test("master_copy_is_independent_and_closed_instance_rejected",()->{try(Fixture f=new Fixture()){
      byte[] first=f.q.masterCopy(),second=f.q.masterCopy();require(Arrays.equals(first,second));Arrays.fill(first,(byte)0);require(Arrays.equals(second,f.q.masterCopy()));f.q.close();reject(()->f.q.masterCopy());reject(()->f.q.sealRemaining());
    }});
    test("recovery_stream_rechecks_attribution_and_close_each_read",()->{try(Fixture f=new Fixture()){
      byte[] data=bytes("native-only attributed recovery");Path p=f.file("cache.txt",data);f.q.sealRemaining();java.util.concurrent.atomic.AtomicBoolean allowed=new java.util.concurrent.atomic.AtomicBoolean(true);
      try(InputStream input=f.q.resolveStream(p.toString(),sha(data),s->allowed.get())){require(input.read()==data[0]);allowed.set(false);reject(input::read);}
      InputStream input=f.q.resolveStream(p.toString(),sha(data),s->true);f.q.close();try{reject(input::read);}finally{input.close();}
    }});
    byte[] interopKey=new byte[32];Arrays.fill(interopKey,(byte)42);byte[] interop=interopBody();String source="/synthetic/app_flutter/unknown.bin",digest=sha(interop),id=sha(bytes(source))+"-"+digest;
    try(OutputStream out=Files.newOutputStream(home.resolve("interop-java.data"))){MobileLegacyQuarantine.encrypt(out,new ByteArrayInputStream(interop),interopKey,id,"legacy-file");}
    String meta="{\"source\":\""+source+"\",\"kind\":\"file\",\"account\":\"\",\"size\":"+interop.length+",\"sha256\":\""+digest+"\"}";
    try(OutputStream out=Files.newOutputStream(home.resolve("interop-java.meta"))){MobileLegacyQuarantine.encrypt(out,new ByteArrayInputStream(bytes(meta)),interopKey,id,"legacy-metadata");}
    System.out.println("MobileLegacyQuarantine "+passed+"/"+passed+" passed (synthetic local only)");
  }
  static byte[] interopBody(){byte[] body=new byte[1048593];for(int i=0;i<body.length;i++)body[i]=(byte)(i%251);return body;}
}
